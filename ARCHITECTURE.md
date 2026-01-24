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

Normalize events, links, and exemplars into separate tables for better queryability. Attributes are normalized into a separate table (see below).

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

-- Data points table (single table for all metric types)
-- Columnar storage compresses NULLs efficiently, and MetricType filters restrict the set
CREATE TABLE metric_data_points (
    ID VARCHAR PRIMARY KEY,
    MetricID VARCHAR NOT NULL,
    MetricType VARCHAR NOT NULL,  -- 'Gauge', 'Sum', 'Histogram', 'ExponentialHistogram'
    Index INTEGER NOT NULL,
    Timestamp BIGINT,
    StartTime BIGINT,
    Flags UINTEGER,
    -- Gauge/Sum fields (NULL for Histogram types)
    Value DOUBLE,
    ValueType VARCHAR,
    -- Sum-specific (NULL for Gauge/Histogram types)
    IsMonotonic BOOLEAN,
    AggregationTemporality VARCHAR,
    -- Histogram/ExponentialHistogram fields (NULL for Gauge/Sum)
    Count UBIGINT,
    Sum DOUBLE,
    Min DOUBLE,
    Max DOUBLE,
    BucketCounts UBIGINT[],
    ExplicitBounds DOUBLE[],
    -- ExponentialHistogram-specific (NULL for other types)
    Scale INTEGER,
    ZeroCount UBIGINT,
    PositiveBucketOffset INTEGER,
    PositiveBucketCounts UBIGINT[],
    NegativeBucketOffset INTEGER,
    NegativeBucketCounts UBIGINT[],
    FOREIGN KEY (MetricID) REFERENCES metrics(MetricID) ON DELETE CASCADE
);

CREATE INDEX idx_metric_data_points_type_metric_time ON metric_data_points(MetricType, MetricID, Timestamp DESC);
CREATE INDEX idx_metric_data_points_metric_time ON metric_data_points(MetricID, Timestamp DESC);
CREATE INDEX idx_metric_data_points_time ON metric_data_points(Timestamp DESC);

CREATE INDEX idx_gauge_data_points_metric_time ON gauge_data_points(MetricID, Timestamp DESC);
CREATE INDEX idx_sum_data_points_metric_time ON sum_data_points(MetricID, Timestamp DESC);
CREATE INDEX idx_histogram_data_points_metric_time ON histogram_data_points(MetricID, Timestamp DESC);
CREATE INDEX idx_exponential_histogram_data_points_metric_time ON exponential_histogram_data_points(MetricID, Timestamp DESC);
```

**Why this works:**
- **Columnar compression**: NULLs compress extremely well with run-length encoding
- **Low cardinality filter**: MetricType has only 4-5 values, compresses and filters efficiently
- **Filter-first pattern**: DuckDB filters by MetricType first, then only scans relevant columns
- **Efficient aggregation**: After filtering by MetricType, NULLs are already excluded
- **Attribute filtering**: Join with normalized `attributes` table using ID as OwnerID
- **No UNION type** - eliminates reflection complexity
- **Simpler schema**: One table instead of four tables + view

**Query examples for visualization:**

```sql
-- Get time series for a gauge metric (DuckDB filters by MetricType first)
SELECT Timestamp, Value
FROM metric_data_points
WHERE MetricType = 'Gauge' AND MetricID = ? AND Timestamp BETWEEN ? AND ?
ORDER BY Timestamp;

-- Aggregate histogram data (after MetricType filter, only relevant columns scanned)
SELECT 
    date_trunc('minute', to_timestamp(Timestamp / 1000000000)) as bucket,
    AVG(Sum) as avg_sum,
    AVG(Count) as avg_count
FROM metric_data_points
WHERE MetricType = 'Histogram' AND MetricID = ? AND Timestamp BETWEEN ? AND ?
GROUP BY bucket
ORDER BY bucket;

-- Filter by attributes (using normalized attributes table with ID as OwnerID)
SELECT dp.Timestamp, dp.Value
FROM metric_data_points dp
JOIN attributes a ON dp.ID = a.OwnerID 
  AND a.SignalType = 'metrics'
  AND a.Scope = 'data_point'
  WHERE dp.MetricType = 'Gauge'
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

### Database Architectural Decisions

This section documents the key architectural decisions made during the schema redesign, including tradeoffs and rationale.

#### Decision 1: Full Normalization

**What we normalized:**
- **Attributes**: Moved from MAP columns to separate `attributes` table
- **Events**: Moved from `event[]` arrays to separate `events` table
- **Links**: Moved from `link[]` arrays to separate `links` table
- **Exemplars**: Moved from nested arrays to separate `exemplars` table
- **Metric Data Points**: Moved from UNION type to separate `metric_data_points` table

**Why normalize:**
1. **Queryability**: Can query events/links/data points independently
   - "Find all error events across all spans" → direct query on `events` table
   - "Get time series for metric" → direct query on `metric_data_points` table
2. **Indexing**: Better indexing for analytical workloads
   - Index on event timestamps, link trace IDs, data point timestamps
   - Enables efficient time-series queries and aggregations
3. **Consistency**: Everything normalized, not just attributes
   - Consistent pattern across all nested entities
   - Easier to reason about and maintain
4. **Analytical workloads**: Built for OpenTelemetry viewer use case
   - Need to filter/search events, links, data points independently
   - Time-series aggregations for metrics visualization

**Tradeoffs:**
- **Insertion complexity**: More inserts per entity (span + events + links + attributes)
  - **Mitigation**: Batch inserts in transactions, collectors already batch data
- **Join overhead**: Reconstructing entities requires joins
  - **Mitigation**: DuckDB optimizes joins efficiently, and we build JSON with joins anyway
- **More tables**: More schema to manage
  - **Mitigation**: Cascading deletes simplify cleanup, consistent patterns

**Verdict**: Normalization is worth it for our use case (viewer + analytical queries).

#### Decision 2: Single Table for Metric Data Points

**What we considered:**
- **Option A**: Separate tables (`gauge_data_points`, `sum_data_points`, etc.) + view
- **Option B**: Single table (`metric_data_points`) with NULLs for type-specific fields

**Why single table:**
1. **Columnar storage optimization**: 
   - NULLs compress extremely well with run-length encoding
   - Low cardinality `MetricType` column (4-5 values) compresses and filters efficiently
2. **Filter-first pattern**: 
   - DuckDB filters by `MetricType` first (very fast, low cardinality)
   - Then only scans relevant columns for that type
   - No need to handle NULLs - they're already excluded by the filter
3. **Simpler schema**: 
   - One table instead of four tables + view
   - No view maintenance or UNION complexity
4. **Better for aggregations**:
   - After filtering by `MetricType`, aggregations work on non-NULL values
   - Index on `(MetricType, MetricID, Timestamp)` is very efficient

**Why not separate tables:**
- **More complexity**: Four tables + view to maintain
- **View overhead**: UNION ALL in view adds query complexity
- **No real benefit**: Columnar storage handles NULLs so well that separate tables don't provide significant advantage

**Tradeoffs:**
- **Type safety**: Can't enforce which fields should be populated at database level
  - **Mitigation**: Application-level validation based on `MetricType`
- **Some NULLs**: Each row has many NULL columns
  - **Mitigation**: Columnar compression makes this negligible

**Verdict**: Single table is better for columnar storage. DuckDB's columnar engine handles sparse data efficiently.

#### Decision 3: Attributes Table Design

**Final structure:**
```sql
CREATE TABLE attributes (
    -- ID columns (only relevant ones populated per row based on attribute scope)
    -- For span/resource/scope attributes: SpanID only
    -- For event attributes: EventID (direct), SpanID (parent)
    -- For link attributes: LinkID (direct), SpanID (parent)
    -- For log/resource/scope attributes: LogID only
    -- For metric/resource/scope attributes: MetricID only
    -- For data_point attributes: DataPointID (direct), MetricID (parent)
    -- For exemplar attributes: ExemplarID (direct), DataPointID (parent), MetricID (grandparent)
    SpanID VARCHAR,
    EventID VARCHAR,
    LinkID VARCHAR,
    LogID VARCHAR,
    MetricID VARCHAR,
    DataPointID VARCHAR,
    ExemplarID VARCHAR,
    -- Attribute data
    Key VARCHAR NOT NULL,
    Value VARCHAR NOT NULL,
    Type attr_type NOT NULL,
    -- Foreign keys (all with CASCADE deletes)
    FOREIGN KEY (SpanID) REFERENCES spans(SpanID) ON DELETE CASCADE,
    FOREIGN KEY (EventID) REFERENCES events(EventID) ON DELETE CASCADE,
    FOREIGN KEY (LinkID) REFERENCES links(LinkID) ON DELETE CASCADE,
    FOREIGN KEY (LogID) REFERENCES logs(LogID) ON DELETE CASCADE,
    FOREIGN KEY (MetricID) REFERENCES metrics(MetricID) ON DELETE CASCADE,
    FOREIGN KEY (DataPointID) REFERENCES datapoints(ID) ON DELETE CASCADE,
    FOREIGN KEY (ExemplarID) REFERENCES exemplars(ID) ON DELETE CASCADE,
    -- Unique constraint: combination of all ID columns + Key ensures uniqueness
    UNIQUE (SpanID, EventID, LinkID, LogID, MetricID, DataPointID, ExemplarID, Key)
);

-- CHECK constraint ensures exactly one direct owner ID is populated and parent IDs are correct
ALTER TABLE attributes ADD CONSTRAINT chk_attributes_one_owner CHECK (
    (SpanID IS NOT NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (EventID IS NOT NULL AND SpanID IS NOT NULL AND LinkID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (LinkID IS NOT NULL AND SpanID IS NOT NULL AND EventID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (LogID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (DataPointID IS NOT NULL AND MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND ExemplarID IS NULL) OR
    (ExemplarID IS NOT NULL AND DataPointID IS NOT NULL AND MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL)
);

-- Covering indexes for efficient queries
CREATE INDEX idx_attributes_span ON attributes(SpanID, Key, Value, Type);
CREATE INDEX idx_attributes_event ON attributes(EventID, Key, Value, Type);
CREATE INDEX idx_attributes_link ON attributes(LinkID, Key, Value, Type);
CREATE INDEX idx_attributes_log ON attributes(LogID, Key, Value, Type);
CREATE INDEX idx_attributes_metric ON attributes(MetricID, Key, Value, Type);
CREATE INDEX idx_attributes_datapoint ON attributes(DataPointID, Key, Value, Type);
CREATE INDEX idx_attributes_exemplar ON attributes(ExemplarID, Key, Value, Type);
CREATE INDEX idx_attributes_span_hierarchy ON attributes(SpanID, EventID, LinkID);
CREATE INDEX idx_attributes_metric_hierarchy ON attributes(MetricID, DataPointID, ExemplarID);
CREATE INDEX idx_attributes_key_value ON attributes(Key, Value, Type);
```

**Why this design:**
1. **Separate ID columns**: Each entity type has its own column
   - Leverages columnar storage: most columns are NULL per row, compresses extremely well
   - Type can be inferred from which ID columns are populated (no SignalType/Scope needed)
2. **Foreign key integrity**: All ID columns have foreign keys with CASCADE deletes
   - Database-enforced referential integrity
   - Automatic cleanup when parent entities are deleted
3. **CHECK constraints**: Enforce correct ID combinations
   - Exactly one direct owner ID must be populated
   - Parent IDs must be populated when required (e.g., EventID requires SpanID)
4. **Covering indexes**: Include Key, Value, Type for index-only queries
   - Avoids table lookups for common queries
   - Hierarchical indexes for parent-child queries

**Why not other approaches:**
- **SignalType + SignalID + Scope + OwnerID** (previous design):
  - Simpler structure but no foreign key support
  - Required application-level validation
- **Path strings** (e.g., `"span.resource"`, `"metric.data_point[2].exemplar[0]"`):
  - More flexible but harder to index and query
  - String parsing needed for queries
- **Single OwnerID with conditional FKs**:
  - SQL doesn't support conditional foreign keys
  - Would require application-level validation

**Tradeoffs:**
- **More columns**: 7 ID columns instead of 2-3
  - **Mitigation**: Columnar storage compresses NULLs extremely well (run-length encoding)
  - Most rows have only 1-2 columns populated
- **Wider indexes**: Composite indexes include all ID columns
  - **Mitigation**: NULLs are excluded from uniqueness checks
  - Indexes are optimized for columnar storage
- **CHECK constraint complexity**: Long constraint expression
  - **Mitigation**: Validated at insert time, ensures data integrity

**Verdict**: This design provides foreign key integrity, leverages columnar storage efficiently, and enables direct queries on specific entity types without needing SignalType/Scope columns.

#### Decision 4: Depth Calculation

**What we considered:**
- **Option A**: Pre-compute and store `Depth` column in spans table
- **Option B**: Calculate depth on query time using recursive CTE

**Why query-time calculation:**
1. **Dynamic depth**: Orphan spans can find parents in later batches
   - Stored depth would become stale
   - Would need to recalculate anyway
2. **Database efficiency**: DuckDB's recursive CTE is optimized
   - More efficient than Go traversals
   - Database is better at tree operations
3. **Simpler inserts**: No need to calculate depth during ingestion
   - Avoids per-insert lookups and complexity
4. **Always needed**: Depth is only needed when querying traces
   - No point storing it if we calculate it on query anyway

**Tradeoffs:**
- **Query overhead**: Recursive CTE on every trace query
  - **Mitigation**: Only calculated when fetching full trace (not summaries)
  - DuckDB optimizes recursive CTEs well

**Verdict**: Query-time calculation is better for our use case.

#### Summary of Tradeoffs

| Decision | Chose | Tradeoff |
|----------|-------|----------|
| Normalization | Full normalization | More inserts, but better queryability |
| Metric data points | Single table | Some NULLs, but columnar compression handles it |
| Attributes design | Separate ID columns with FKs | More columns, but foreign key integrity and columnar optimization |
| Depth | Query-time calculation | CTE overhead, but handles dynamic relationships |

**Overall philosophy**: Optimize for queryability and analytical workloads, accept insertion complexity (which is manageable with batching).

### How JSON Rows Affect Schema

**Storage schema doesn't change** - we still store data the same way. But JSON output affects query design:

**Computed values in queries:**
- Values like `service_name`, `root_name`, `span_count`, `error_count` are computed on-the-fly using window functions
- No new columns needed - these are computed when building JSON
- Example: `COUNT(*) OVER (PARTITION BY TraceID) as span_count`

**Nested structures normalized:**
- Events, Links, Exemplars are normalized into separate tables for better queryability
- Can query events/links/data points independently
- Better indexing for analytical workloads

**Normalized tables:**

```sql
-- Events table
CREATE TABLE events (
    EventID VARCHAR PRIMARY KEY,
    SpanID VARCHAR NOT NULL,
    Name VARCHAR,
    Timestamp BIGINT,
    DroppedAttributesCount UINTEGER,
    FOREIGN KEY (SpanID) REFERENCES spans(SpanID) ON DELETE CASCADE
);

-- Links table
CREATE TABLE links (
    LinkID VARCHAR PRIMARY KEY,
    SpanID VARCHAR NOT NULL,
    TraceID VARCHAR,
    LinkedSpanID VARCHAR,
    TraceState VARCHAR,
    DroppedAttributesCount UINTEGER,
    FOREIGN KEY (SpanID) REFERENCES spans(SpanID) ON DELETE CASCADE
);

-- Exemplars table
CREATE TABLE exemplars (
    ID VARCHAR PRIMARY KEY DEFAULT gen_random_uuid()::VARCHAR,
    DataPointID VARCHAR NOT NULL,
    Timestamp BIGINT,
    Value DOUBLE,
    TraceID VARCHAR,
    SpanID VARCHAR,
    FOREIGN KEY (DataPointID) REFERENCES datapoints(ID) ON DELETE CASCADE
);
```

**Attributes are normalized into a separate table with separate ID columns:**

Normalize attributes for efficient querying and discovery, using separate ID columns to enable foreign key integrity:

```sql
CREATE TABLE attributes (
    -- ID columns (only relevant ones populated per row)
    SpanID VARCHAR,
    EventID VARCHAR,
    LinkID VARCHAR,
    LogID VARCHAR,
    MetricID VARCHAR,
    DataPointID VARCHAR,
    ExemplarID VARCHAR,
    -- Attribute data
    Key VARCHAR NOT NULL,
    Value VARCHAR NOT NULL,
    Type attr_type NOT NULL,
    -- Foreign keys (all with CASCADE deletes)
    FOREIGN KEY (SpanID) REFERENCES spans(SpanID) ON DELETE CASCADE,
    FOREIGN KEY (EventID) REFERENCES events(EventID) ON DELETE CASCADE,
    FOREIGN KEY (LinkID) REFERENCES links(LinkID) ON DELETE CASCADE,
    FOREIGN KEY (LogID) REFERENCES logs(LogID) ON DELETE CASCADE,
    FOREIGN KEY (MetricID) REFERENCES metrics(MetricID) ON DELETE CASCADE,
    FOREIGN KEY (DataPointID) REFERENCES datapoints(ID) ON DELETE CASCADE,
    FOREIGN KEY (ExemplarID) REFERENCES exemplars(ID) ON DELETE CASCADE,
    -- Unique constraint ensures one attribute per entity+key combination
    UNIQUE (SpanID, EventID, LinkID, LogID, MetricID, DataPointID, ExemplarID, Key)
);

-- CHECK constraint ensures exactly one direct owner ID is populated
ALTER TABLE attributes ADD CONSTRAINT chk_attributes_one_owner CHECK (
    (SpanID IS NOT NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (EventID IS NOT NULL AND SpanID IS NOT NULL AND LinkID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (LinkID IS NOT NULL AND SpanID IS NOT NULL AND EventID IS NULL AND LogID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (LogID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND MetricID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND DataPointID IS NULL AND ExemplarID IS NULL) OR
    (DataPointID IS NOT NULL AND MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL AND ExemplarID IS NULL) OR
    (ExemplarID IS NOT NULL AND DataPointID IS NOT NULL AND MetricID IS NOT NULL AND SpanID IS NULL AND EventID IS NULL AND LinkID IS NULL AND LogID IS NULL)
);

-- Covering indexes for efficient queries
CREATE INDEX idx_attributes_span ON attributes(SpanID, Key, Value, Type);
CREATE INDEX idx_attributes_event ON attributes(EventID, Key, Value, Type);
CREATE INDEX idx_attributes_link ON attributes(LinkID, Key, Value, Type);
CREATE INDEX idx_attributes_log ON attributes(LogID, Key, Value, Type);
CREATE INDEX idx_attributes_metric ON attributes(MetricID, Key, Value, Type);
CREATE INDEX idx_attributes_datapoint ON attributes(DataPointID, Key, Value, Type);
CREATE INDEX idx_attributes_exemplar ON attributes(ExemplarID, Key, Value, Type);
CREATE INDEX idx_attributes_span_hierarchy ON attributes(SpanID, EventID, LinkID);
CREATE INDEX idx_attributes_metric_hierarchy ON attributes(MetricID, DataPointID, ExemplarID);
CREATE INDEX idx_attributes_key_value ON attributes(Key, Value, Type);
```

**Why normalize attributes:**
- **Efficient attribute discovery**: `SELECT DISTINCT Key, Type FROM attributes WHERE SpanID IS NOT NULL` (no UNNEST needed)
- **Simple searching**: `SELECT SpanID FROM attributes WHERE Key = 'service' AND Value = 'api'` (no UNNEST needed)
- **No complex UNNEST operations** - current attribute discovery uses expensive `UNNEST(map_entries(Attributes))` on all spans
- **Event/link attributes**: Direct query on attributes table with `EventID IS NOT NULL` or `LinkID IS NOT NULL`
- **Global search**: Simple join instead of `UNNEST(map_entries(s.Attributes))`
- **Consistent structure** across all entity types
- **Query builder friendly**: With a query builder, joins are much simpler to compose than complex UNNEST expressions. Instead of building `EXISTS(SELECT 1 FROM UNNEST(map_entries(s.Attributes)) WHERE ...)`, we can simply add `JOIN attributes ON ... WHERE attributes.Key = ? AND attributes.Value = ?`
- **Foreign key integrity**: Database-enforced referential integrity with CASCADE deletes

**Query examples:**

```sql
-- Attribute discovery for spans (simple, no UNNEST)
SELECT DISTINCT Key, Type
FROM attributes
WHERE SpanID IS NOT NULL AND EventID IS NULL AND LinkID IS NULL
ORDER BY Key;

-- Search spans by attribute (simple join, easy for query builder)
SELECT DISTINCT s.*
FROM spans s
JOIN attributes a ON s.SpanID = a.SpanID
WHERE a.SpanID IS NOT NULL 
  AND a.EventID IS NULL 
  AND a.LinkID IS NULL
  AND a.Key = 'service'
  AND a.Value = 'api';

-- Get all attributes for a span (including events and links)
SELECT a.*
FROM attributes a
WHERE a.SpanID = ?;

-- Get attributes for a specific event
SELECT a.*
FROM attributes a
WHERE a.EventID = ?;

-- Search spans by event attributes (direct query on events table)
SELECT DISTINCT s.*
FROM spans s
JOIN events e ON s.SpanID = e.SpanID
JOIN attributes a ON e.EventID = a.EventID
WHERE a.Key = 'event.name'
  AND a.Value = 'error';

-- Multiple attribute filters (easy to compose in query builder)
SELECT DISTINCT s.*
FROM spans s
JOIN attributes a1 ON s.SpanID = a1.OwnerID AND a1.Key = 'service' AND a1.Value = 'api'
JOIN attributes a2 ON s.SpanID = a2.OwnerID AND a2.Key = 'env' AND a2.Value = 'prod'
WHERE a1.SignalType = 'traces' AND a1.Scope = 'span'
  AND a2.SignalType = 'traces' AND a2.Scope = 'span';
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
- **No nested arrays**: Events, Links normalized into separate tables
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

**Ingestion:** When ingesting entities, insert into multiple tables:
1. Insert entity into main table (spans, logs, metrics, etc.) - stores core structured data
2. Insert events/links/exemplars into normalized tables
3. Insert all attributes into `attributes` table - stores variable key-value metadata

**JSON rows:** When building JSON responses, join normalized tables and attributes back:
```sql
SELECT json_object(
    'spanID', s.SpanID,
    'events', json_array_agg(json_object('eventID', e.EventID, 'name', e.Name, ...)),
    'links', json_array_agg(json_object('linkID', l.LinkID, ...)),
    'attributes', json_object_agg(a.Key, json_object('v', a.Value, 't', a.Type))
)
FROM spans s
LEFT JOIN events e ON s.SpanID = e.SpanID
LEFT JOIN links l ON s.SpanID = l.SpanID
LEFT JOIN attributes a ON s.SpanID = a.OwnerID AND a.SignalType = 'traces' AND a.Scope = 'span'
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

