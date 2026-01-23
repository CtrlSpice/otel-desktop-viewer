# axolotel Architecture Decisions

For the actionable checklist, see [TODO.md](TODO.md).

---

## Core Architectural Decision: JSON Rows from DuckDB

**This is the foundation - implement this first in Phase 2.**

### The Approach

Have DuckDB output each query row as a JSON object, eliminating all intermediate Go structs:

```sql
SELECT json_object(
    'traceID', TraceID,
    'rootSpan', CASE 
        WHEN service_name IS NOT NULL THEN json_object(
            'serviceName', service_name,
            'name', root_name,
            'startTime', CAST(start_time AS VARCHAR),
            'endTime', CAST(end_time AS VARCHAR)
        )
        ELSE NULL
    END,
    'spanCount', span_count,
    'errorCount', error_count,
    'exceptionCount', exception_count
) AS json_row
FROM spans
```

```go
var jsonRows []json.RawMessage
for rows.Next() {
    var jsonStr string
    rows.Scan(&jsonStr)
    jsonRows = append(jsonRows, json.RawMessage(jsonStr))
}
return jsonRows, nil  // jsonrpc2 marshals as JSON array
```

### Why This Changes Everything

**Before:**
- OTLP → Go structs → DuckDB (storage)
- DuckDB → Go structs → JSON (responses)
- Two sets of structs, scanning, marshaling

**After:**
- OTLP → DuckDB (direct, no structs)
- DuckDB → JSON rows (direct, no structs)
- Zero intermediate structs anywhere

### Impact on Architecture

1. **Phase 1 (Schema)**: No change - still need STRUCT(v,t) instead of UNION
2. **Phase 2 (Server)**: This IS the approach - all queries output JSON rows
3. **Phase 3-10 (Frontend)**: No change - frontend still receives JSON, just built differently
4. **Type Safety**: Response structure lives in SQL, not Go structs (trade-off for simplicity)
5. **Debugging**: Inspect JSON strings instead of structs (acceptable trade-off)

### Implementation Order

1. **First**: Update one query (e.g., `getTraceSummaries`) to output JSON rows
2. **Then**: Update all other queries to follow the same pattern
3. **Finally**: Remove all response struct definitions

This becomes the pattern for all JSON-RPC methods.

---

## Phase 1: Database Schema Rework

Related: [TODO.md - Phase 1](TODO.md#phase-1-database-schema-rework)

### Problem

The current schema uses DuckDB UNION types for attributes:

```sql
CREATE TYPE attribute AS UNION(
    string VARCHAR,
    int64 BIGINT,
    float64 DOUBLE,
    boolean BOOLEAN,
    string_list VARCHAR[],
    int64_list BIGINT[],
    float64_list DOUBLE[],
    boolean_list BOOLEAN[]
);
```

Querying UNIONs requires `union_tag()` and `union_extract()`:

```sql
-- Current (painful)
WHERE union_tag(Attributes['http.method']) = 'string' 
  AND union_extract(Attributes['http.method'], 'string') = 'GET'

-- What we want (with normalized attributes)
WHERE EXISTS(SELECT 1 FROM attributes 
  WHERE EntityID = SpanID AND EntityType = 'span' 
  AND AttributeScope = 'span' AND Key = 'http.method' AND Value = 'GET')
```

### Decision

Replace UNION with a simple struct with an enum for the type:

```sql
CREATE TYPE attr_type AS ENUM(
    'string',
    'int64',
    'float64',
    'bool',
    'string[]',
    'int64[]',
    'float64[]',
    'boolean[]'
);

CREATE TYPE attr_value AS STRUCT(v VARCHAR, t attr_type);
-- v = value as string
-- t = type enum (one of the above values)
```

Keep events, links, and exemplars as arrays of structs. Attributes are normalized into a separate table (see below), so these structs don't need attribute fields:

```sql
CREATE TYPE event AS STRUCT(
    Name VARCHAR,
    Timestamp BIGINT,
    DroppedAttributesCount UINTEGER
    -- Attributes stored in normalized attributes table
);

CREATE TYPE link AS STRUCT(
    TraceID VARCHAR,
    SpanID VARCHAR,
    TraceState VARCHAR,
    DroppedAttributesCount UINTEGER
    -- Attributes stored in normalized attributes table
);

CREATE TYPE exemplar AS STRUCT(
    Timestamp BIGINT,
    Value DOUBLE,
    TraceID VARCHAR,
    SpanID VARCHAR
    -- FilteredAttributes stored in normalized attributes table
);
```

### Notes

- DuckDB handles nested STRUCTs and arrays efficiently - the pain point is specifically the UNION type
- Numbers stored as strings preserve full precision; cast when needed in queries
- Frontend can use `.t` to render values appropriately
- This should fix AppenderWrapper memory issues (currently flushing every 10 rows due to reflection overhead)

### Additional Changes

Add indexes (currently none exist):

```sql
CREATE INDEX idx_spans_trace_id ON spans(TraceID);
CREATE INDEX idx_spans_start_time ON spans(StartTime DESC);
CREATE INDEX idx_logs_timestamp ON logs(Timestamp DESC);
```

Pre-compute span depth:

```sql
ALTER TABLE spans ADD COLUMN Depth INTEGER DEFAULT 0;
```

Simplify log body:

```sql
Body VARCHAR,      -- store as string (was UNION)
BodyType VARCHAR,  -- 'string', 'json', 'bytes', etc.
```

### Simplify Metrics Schema

**Problem:** Metrics use a UNION type for data points, and we need efficient aggregation for visualization:

```sql
CREATE TYPE dataPoints AS UNION(
    Gauge gauge[],
    Sum sum[],
    Histogram histogram[],
    ExponentialHistogram exponentialHistogram[]
)
```

This has two issues:
1. UNION type complexity (same as attributes)
2. **Can't efficiently aggregate** - need to query data points by time range, group by buckets, filter by attributes for charts

**Decision:** Normalize data points into a separate table for efficient querying:

```sql
-- Metrics table (metadata only)
CREATE TABLE metrics (
    MetricID VARCHAR PRIMARY KEY,
    Name VARCHAR,
    Description VARCHAR,
    Unit VARCHAR,
    MetricType VARCHAR,  -- 'Gauge', 'Sum', 'Histogram', 'ExponentialHistogram'
    ResourceDroppedAttributesCount UINTEGER,
    ScopeName VARCHAR,
    ScopeVersion VARCHAR,
    ScopeDroppedAttributesCount UINTEGER,
    Received BIGINT
    -- ResourceAttributes and ScopeAttributes stored in normalized attributes table
);

-- Data points table (one row per data point)
CREATE TABLE metric_data_points (
    MetricID VARCHAR,
    Timestamp BIGINT,
    StartTime BIGINT,
    Flags UINTEGER,
    -- Attributes stored in normalized attributes table
    
    -- Common fields (one will be populated based on MetricType)
    Value DOUBLE,              -- for Gauge/Sum
    ValueType VARCHAR,         -- 'int' or 'double'
    
    -- Sum-specific
    IsMonotonic BOOLEAN,
    AggregationTemporality VARCHAR,
    
    -- Histogram-specific
    Count UBIGINT,
    Sum DOUBLE,
    Min DOUBLE,
    Max DOUBLE,
    BucketCounts UBIGINT[],
    ExplicitBounds DOUBLE[],
    
    -- ExponentialHistogram-specific
    Scale INTEGER,
    ZeroCount UBIGINT,
    PositiveBucketOffset INTEGER,
    PositiveBucketCounts UBIGINT[],
    NegativeBucketOffset INTEGER,
    NegativeBucketCounts UBIGINT[],
    
    -- Exemplars (stored as JSON array for simplicity)
    Exemplars JSON,
    
    FOREIGN KEY (MetricID) REFERENCES metrics(MetricID)
);

CREATE INDEX idx_metric_data_points_metric_time ON metric_data_points(MetricID, Timestamp DESC);
CREATE INDEX idx_metric_data_points_time ON metric_data_points(Timestamp DESC);
```

**Why this works:**
- **Efficient aggregation**: `SELECT AVG(Value), time_bucket(Timestamp) FROM metric_data_points WHERE MetricID = ? GROUP BY time_bucket`
- **Time range queries**: `WHERE Timestamp BETWEEN ? AND ?`
- **Attribute filtering**: Join with normalized `attributes` table (no MAP access needed)
- **No UNION type** - eliminates reflection complexity
- **Normalized structure** - DuckDB can optimize queries on structured columns

**Query examples for visualization:**

```sql
-- Get time series for a gauge metric
SELECT Timestamp, Value
FROM metric_data_points
WHERE MetricID = ? AND Timestamp BETWEEN ? AND ?
ORDER BY Timestamp;

-- Aggregate by time buckets (1 minute)
SELECT 
    date_trunc('minute', to_timestamp(Timestamp / 1000000000)) as bucket,
    AVG(Value) as avg_value,
    MIN(Value) as min_value,
    MAX(Value) as max_value
FROM metric_data_points
WHERE MetricID = ? AND Timestamp BETWEEN ? AND ?
GROUP BY bucket
ORDER BY bucket;

-- Filter by attributes (using normalized attributes table)
SELECT dp.Timestamp, dp.Value
FROM metric_data_points dp
JOIN attributes a ON dp.MetricID = a.EntityID 
  AND a.EntityType = 'metric'
  AND a.AttributeScope = 'span'
WHERE dp.MetricID = ?
  AND EXISTS(SELECT 1 FROM attributes WHERE EntityID = dp.MetricID AND Key = 'service' AND Value = 'api')
  AND EXISTS(SELECT 1 FROM attributes WHERE EntityID = dp.MetricID AND Key = 'env' AND Value = 'prod')
ORDER BY dp.Timestamp;
```

**Ingestion:** When ingesting metrics, insert into both tables:
1. Insert/update `metrics` table with metadata
2. Insert all data points into `metric_data_points` table

**JSON rows:** When building JSON responses, join and aggregate:
```sql
SELECT json_object(
    'metricID', m.MetricID,
    'name', m.Name,
    'type', m.MetricType,
    'dataPoints', json_array_agg(
        json_object('timestamp', dp.Timestamp, 'value', dp.Value)
    )
)
FROM metrics m
JOIN metric_data_points dp ON m.MetricID = dp.MetricID
WHERE m.MetricID = ?
GROUP BY m.MetricID, m.Name, m.MetricType
```

### How JSON Rows Affect Schema

**Storage schema doesn't change** - we still store data the same way. But JSON output affects query design:

**Computed values in queries:**
- Values like `service_name`, `root_name`, `span_count`, `error_count` are computed on-the-fly using window functions
- No new columns needed - these are computed when building JSON
- Example: `COUNT(*) OVER (PARTITION BY TraceID) as span_count`

**Nested structures:**
- Events, Links, Exemplars are stored as arrays of structs: `event[]`, `link[]`, `exemplar[]`
- DuckDB can convert these to JSON arrays directly: `json(Events)` or `json_array_agg(...)`
- No schema change needed - DuckDB handles the conversion

**Attributes are normalized into a separate table:**

Normalize attributes for efficient querying and discovery:

```sql
CREATE TABLE attributes (
    EntityType VARCHAR,      -- 'span', 'log', 'metric', 'event', 'link', 'exemplar'
    EntityID VARCHAR,        -- SpanID, LogID, MetricID, Event index, Link index, etc.
    AttributeScope VARCHAR,  -- 'resource', 'span', 'scope', 'event', 'link', 'exemplar'
    Key VARCHAR,
    Value VARCHAR,           -- stored as string (same as attr_value.v)
    Type attr_type,          -- enum type (same as attr_value.t)
    
    PRIMARY KEY (EntityType, EntityID, AttributeScope, Key)
);

CREATE INDEX idx_attributes_key_value ON attributes(Key, Value);
CREATE INDEX idx_attributes_entity ON attributes(EntityType, EntityID);
CREATE INDEX idx_attributes_scope_key ON attributes(AttributeScope, Key);
CREATE INDEX idx_attributes_entity_scope_key ON attributes(EntityType, EntityID, AttributeScope, Key);
-- Composite index for common query patterns: entity lookups, key-based filters
```

**Why normalize attributes:**
- **Efficient attribute discovery**: `SELECT DISTINCT Key, Type FROM attributes WHERE EntityType = 'span'` (no UNNEST needed)
- **Simple searching**: `SELECT EntityID FROM attributes WHERE Key = 'service' AND Value = 'api'` (no UNNEST needed)
- **No complex UNNEST operations** - current attribute discovery uses expensive `UNNEST(map_entries(Attributes))` on all spans
- **Event/link attributes**: No double UNNEST - just query the attributes table
- **Global search**: Simple join instead of `UNNEST(map_entries(s.Attributes))`
- **Consistent structure** across all entity types
- **Query builder friendly**: With a query builder, joins are much simpler to compose than complex UNNEST expressions. Instead of building `EXISTS(SELECT 1 FROM UNNEST(map_entries(s.Attributes)) WHERE ...)`, we can simply add `JOIN attributes ON ... WHERE attributes.Key = ? AND attributes.Value = ?`

**Query examples:**

```sql
-- Attribute discovery (simple, no UNNEST)
SELECT DISTINCT Key, Type, AttributeScope
FROM attributes
WHERE EntityType = 'span'
ORDER BY Key, AttributeScope;

-- Search by attribute (simple join, easy for query builder)
SELECT DISTINCT s.*
FROM spans s
JOIN attributes a ON s.SpanID = a.EntityID 
WHERE a.EntityType = 'span'
  AND a.AttributeScope = 'span'
  AND a.Key = 'service'
  AND a.Value = 'api';

-- Search event attributes (no double UNNEST, simple join)
SELECT DISTINCT s.*
FROM spans s
JOIN attributes a ON s.SpanID = a.EntityID
WHERE a.EntityType = 'span'
  AND a.AttributeScope = 'event'
  AND a.Key = 'event.name'
  AND a.Value = 'error';

-- Multiple attribute filters (easy to compose in query builder)
SELECT DISTINCT s.*
FROM spans s
JOIN attributes a1 ON s.SpanID = a1.EntityID AND a1.Key = 'service' AND a1.Value = 'api'
JOIN attributes a2 ON s.SpanID = a2.EntityID AND a2.Key = 'env' AND a2.Value = 'prod'
WHERE a1.EntityType = 'span' AND a1.AttributeScope = 'span'
  AND a2.EntityType = 'span' AND a2.AttributeScope = 'span';
```

**Query builder benefits:**
- **Before (MAPs)**: Must build complex `EXISTS(SELECT 1 FROM UNNEST(map_entries(s.Attributes)) WHERE ...)` expressions
- **After (normalized)**: Simply add `JOIN attributes ON ... WHERE attributes.Key = ? AND attributes.Value = ?`
- **Composability**: Multiple attribute filters = multiple simple joins (much easier than nested EXISTS/UNNEST)
- **Event/link attributes**: No special handling needed - just join with `AttributeScope = 'event'` or `'link'`

**Does indexing give us back MAP flexibility?**

Yes - with proper indexes, normalized attributes are actually MORE flexible than MAPs:

**What we "lost" (syntactic convenience):**
- MAP: `WHERE Attributes['service'].v = 'api'` (direct access)
- Normalized: `JOIN attributes ON ... WHERE attributes.Key = 'service' AND attributes.Value = 'api'` (requires join)

**What we gained (query flexibility):**
- **Indexed key lookups**: `idx_attributes_key_value` makes key/value queries as fast as MAP access
- **Discovery**: `SELECT DISTINCT Key FROM attributes` (impossible with MAPs without UNNEST)
- **Cross-entity queries**: "Find all entities with service='api'" (simple with normalized, requires scanning all MAPs otherwise)
- **Complex filters**: Multiple attributes, ranges, aggregations (much easier with normalized)
- **Event/link attributes**: No special handling needed (MAPs require double UNNEST)

**Performance comparison:**
- **MAP direct access**: O(1) hash lookup in MAP (but requires UNNEST for discovery/search)
- **Normalized with index**: O(log n) index lookup (but enables efficient discovery/search)
- **MAP discovery/search**: O(n) full table scan + UNNEST all MAPs (very expensive)
- **Normalized discovery/search**: O(log n) index lookup (fast)

**The "flexibility" we lost is just syntactic sugar** - we gain much more query flexibility with normalized + indexes.

**Trade-offs:**
- **Requires joins** to get attributes with entities (but joins are standard SQL, and we build JSON with them anyway)
- **More storage** - one row per attribute vs. one MAP per entity (but enables efficient querying)
- **More complex ingestion** - insert into both entity table and attributes table (but simpler query code)
- **Much simpler and faster queries** for discovery, search, and complex filters

**Why we need both entity tables AND attributes table:**

**Entity tables (spans, logs, metrics) store:**
- **Core structured data**: TraceID, SpanID, Name, StartTime, EndTime, StatusCode, etc.
- **Fixed schema fields**: These are always present and have specific types
- **Primary identifiers**: EntityID, relationships (ParentSpanID), timestamps
- **Arrays of structs**: Events[], Links[] (not normalized - stored as arrays)
- **Counts**: DroppedAttributesCount, etc.

**Attributes table stores:**
- **Variable key-value pairs**: User-defined metadata (service.name, http.method, etc.)
- **No fixed schema**: Keys vary per entity
- **Supplementary data**: Not part of core entity structure

**Why we can't eliminate entity tables:**
- **Core queries**: `WHERE TraceID = ?` or `WHERE StartTime BETWEEN ? AND ?` need the entity table
- **Efficient indexing**: Core fields (TraceID, StartTime) are indexed on entity table
- **Entity identity**: SpanID, TraceID define the entity - attributes are just metadata
- **Structured data**: Name, Kind, StatusCode are always present with known types

**Why we normalize attributes:**
- **Variable keys**: Can't have a column for every possible attribute key
- **Discovery**: Need to find all attribute keys across entities
- **Search**: Need to query by attribute key/value efficiently
- **Cross-entity queries**: "Find all spans with service='api'" requires scanning attributes

**Ingestion:** When ingesting entities, insert into both tables:
1. Insert entity into main table (spans, logs, metrics, etc.) - stores core structured data
2. Insert all attributes into `attributes` table - stores variable key-value metadata

**JSON rows:** When building JSON responses, join attributes back:
```sql
SELECT json_object(
    'spanID', s.SpanID,
    'attributes', json_object_agg(a.Key, json_object('v', a.Value, 't', a.Type))
)
FROM spans s
LEFT JOIN attributes a ON s.SpanID = a.EntityID AND a.EntityType = 'span' AND a.AttributeScope = 'span'
WHERE s.SpanID = ?
GROUP BY s.SpanID
```

**Why normalize both metrics and attributes:**
- **Metric data points**: Need time-series aggregation across many rows
- **Attributes**: Need efficient discovery and search across all entities
- Both benefit from normalization for their primary use cases

**Optional optimization: trace_summaries table**
- For frequently-queried trace summaries, consider a materialized table:
```sql
CREATE TABLE trace_summaries AS
SELECT 
    TraceID,
    service_name,
    root_name,
    start_time,
    end_time,
    span_count,
    error_count,
    exception_count
FROM (aggregation query)
```
- Trade-off: faster queries vs. maintenance overhead (update on ingest)
- Start without it, add later if needed

**Key point:** JSON rows don't require schema changes - they're a query pattern, not a storage pattern.

---

## Phase 2: Server Rework

Related: [TODO.md - Phase 2](TODO.md#phase-2-server-rework)

**This phase implements the core architectural decision: JSON rows from DuckDB (see top of document).**

### Problem

Current flow has two unnecessary translation layers:

1. **Storage**: OTLP pdata → intermediate Go structs → DuckDB
2. **Retrieval**: DuckDB → Go structs → JSON

The intermediate structs (`SpanData`, `LogData`, `MetricData`) duplicate pdata and add memory overhead.

### Decision

**Two-part solution:**

1. **Ingestion**: Translate directly from OTLP pdata to DuckDB (no intermediate structs)
2. **Queries**: Have DuckDB output each row as a JSON object (no response structs)

### Implementation

**Direct OTLP → DuckDB translation:**

```go
func (s *Store) IngestTraces(ctx context.Context, traces ptrace.Traces) error {
    appender, _ := NewAppenderWrapper(s.conn, "", "", "spans")
    defer appender.Close()

    for resourceSpan := range traces.ResourceSpans().All() {
        resource := resourceSpan.Resource()
        resourceAttrs := convertAttributes(resource.Attributes())
        
        for scopeSpan := range resourceSpan.ScopeSpans().All() {
            scope := scopeSpan.Scope()
            for span := range scopeSpan.Spans().All() {
                appender.AppendRow(
                    span.TraceID().String(),
                    convertAttributes(span.Attributes()),
                    convertEvents(span.Events()),
                    // ... direct from pdata to appender
                )
            }
        }
    }
    return appender.Flush()
}
```

**Attribute conversion helper:**

```go
type AttrValue struct {
    V string `json:"v"`
    T string `json:"t"`
}

func convertAttributes(attrs pcommon.Map) map[string]AttrValue {
    result := make(map[string]AttrValue, attrs.Len())
    attrs.Range(func(k string, v pcommon.Value) bool {
        result[k] = AttrValue{
            V: valueToString(v),
            T: valueTypeName(v.Type()),
        }
        return true
    })
    return result
}
```

**JSON rows from queries:**

```sql
SELECT json_object(
    'traceID', TraceID,
    'rootSpan', CASE 
        WHEN service_name IS NOT NULL THEN json_object(
            'serviceName', service_name,
            'name', root_name,
            'startTime', CAST(start_time AS VARCHAR),
            'endTime', CAST(end_time AS VARCHAR)
        )
        ELSE NULL
    END,
    'spanCount', span_count,
    'errorCount', error_count,
    'exceptionCount', exception_count
) AS json_row
FROM spans
```

```go
var jsonRows []json.RawMessage
for rows.Next() {
    var jsonStr string
    rows.Scan(&jsonStr)
    jsonRows = append(jsonRows, json.RawMessage(jsonStr))
}
return jsonRows, nil  // jsonrpc2 marshals as JSON array
```

### How This Changes Architecture

**Before:**
- OTLP → Go structs → DuckDB (storage)
- DuckDB → Go structs → JSON (responses)
- Two sets of structs, scanning, marshaling

**After:**
- OTLP → DuckDB (direct, no structs)
- DuckDB → JSON rows (direct, no structs)
- Zero intermediate structs anywhere

**Impact on other phases:**
- **Phase 1**: No change - still need STRUCT(v,t) instead of UNION
- **Phase 3-10**: No change - frontend still receives JSON, just built differently
- **Type safety**: Response structure lives in SQL, not Go structs (trade-off for simplicity)
- **Debugging**: Inspect JSON strings instead of structs (acceptable trade-off)

### Notes

- Eliminates ~30% memory overhead from intermediate allocations
- Single translation point makes code easier to maintain
- With UNION types gone, can flush once per batch instead of every 10 rows
- Timestamp conversion in SQL: `CAST(timestamp AS VARCHAR)`
- Response structure defined in SQL (single source of truth)
- Add WebSocket notification hooks here - notify connected clients when data is ingested

---

## Phase 5: WebSocket Architecture

Related: [TODO.md - Phase 5](TODO.md#phase-5-websocket-frontend)

### Problem

Frontend currently polls JSON-RPC for updates. This is wasteful, laggy, and not suitable for live tail.

### Decision

Hybrid approach - keep JSON-RPC for queries, add WebSocket for push:

- JSON-RPC: fetch trace list, search with filters, get trace details
- WebSocket: new trace notifications, live tail, stats updates

Message protocol:

```typescript
// Server → Client
{ type: "trace.new", data: TraceSummary }
{ type: "log.new", data: LogData }
{ type: "stats", data: { traces, logs, metrics } }

// Client → Server
{ type: "subscribe", channels: ["traces", "logs"] }
{ type: "tail.start", filter?: QueryTree }
{ type: "tail.pause" }
```

### Notes

- Use `nhooyr.io/websocket` (modern Go, better than gorilla)
- Store notifies WebSocket handler when data is ingested
- Client subscribes to channels and can start/pause live tail

---

## Phase 10: Deployment & Configuration

Related: [TODO.md - Phase 10](TODO.md#phase-10-deployment--rename)

### Configuration: Declarative YAML Support

**Current state:** Configuration is done via command-line flags, which builds YAML config strings dynamically.

**Decision:** Support declarative YAML configuration files (collector already supports this via `--config` flag).

**Why:**
- OpenTelemetry Collector already supports YAML config files
- More maintainable than command-line flags for complex configurations
- Standard collector pattern - users expect `--config` flag
- Can still support command-line flags as overrides

**Implementation:**
- Add `--config` flag to accept YAML config file path
- Support both file-based config AND command-line flags (flags override config)
- Example YAML config:
```yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: localhost:4318
        cors:
          allowed_origins: ["https://*", "http://*"]
      grpc:
        endpoint: localhost:4317

exporters:
  desktop:
    endpoint: localhost:8000
    db: /path/to/db.duckdb
    retry:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 300s

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [desktop]
    metrics:
      receivers: [otlp]
      exporters: [desktop]
    logs:
      receivers: [otlp]
      exporters: [desktop]
```

### Retry Logic

**Current state:** No retry logic configured - if ingestion fails, data is lost.

**Decision:** Use collector's `exporterhelper` retry functionality.

**Why:**
- Collector provides built-in retry via `exporterhelper.WithRetry()`
- Handles transient failures (database locks, temporary errors)
- Configurable backoff strategy
- Standard collector pattern

**Implementation:**
- Add retry configuration to `Config` struct:
```go
type Config struct {
    Endpoint string `mapstructure:"endpoint"`
    Db       string `mapstructure:"db"`
    Retry    configretry.Config `mapstructure:"retry"`
}
```

- Use `exporterhelper.WithRetry()` in factory:
```go
return exporterhelper.NewTraces(ctx, set, cfg,
    e.Unwrap().pushTraces,
    exporterhelper.WithRetry(cfg.Retry),
    exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
    exporterhelper.WithStart(e.Start),
    exporterhelper.WithShutdown(e.Shutdown),
)
```

**Retry configuration options:**
- `enabled`: Enable/disable retry
- `initial_interval`: Initial backoff interval (default: 5s)
- `max_interval`: Maximum backoff interval (default: 30s)
- `max_elapsed_time`: Maximum time to retry (default: 300s)
- `multiplier`: Backoff multiplier (default: 2.0)

**When retry helps:**
- Database lock contention
- Temporary DuckDB errors
- Network issues (if we add remote database support later)

**When retry doesn't help:**
- Schema errors (permanent, shouldn't retry)
- Invalid data (permanent, shouldn't retry)

### Deployment Notes

axolotel works behind any reverse proxy. nginx requires explicit WebSocket config:

```nginx
location /ws {
    proxy_pass http://localhost:8000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400;
}
```

Caddy and Traefik handle WebSocket automatically.

Rename: otel-desktop-viewer → axolotel (update go.mod, imports, Docker image, CI)

---

## Summary

| Decision | Choice | Why |
|----------|--------|-----|
| Attribute storage | Normalized `attributes` table | Efficient discovery/search, no UNNEST needed |
| Events/Links/Exemplars | Keep as arrays of structs | DuckDB handles nesting well |
| Metrics data points | Normalized `metric_data_points` table | Efficient aggregation for visualization |
| OTLP translation | Direct to DuckDB | Eliminate intermediate structs |
| Query responses | JSON rows from DuckDB | Zero Go structs, SQL defines response shape |
| Real-time updates | WebSocket + JSON-RPC | Push for notifications, RPC for queries |
| Configuration | YAML config files + CLI flags | Declarative config, standard collector pattern |
| Retry logic | `exporterhelper.WithRetry()` | Built-in collector retry for transient failures |
| Charting | uPlot | Fastest, smallest |
| WebSocket lib | nhooyr.io/websocket | Modern Go |

