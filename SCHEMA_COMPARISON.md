# Database Schema Comparison: Old vs New

## Overview

This document compares the old database schema (current implementation) with the new normalized schema design for the database revamp.

## Key Changes

1. **Attributes normalized**: All attributes moved from MAP columns to a separate `attributes` table
2. **Metrics normalized**: Data points moved from UNION type to separate `metric_data_points` table
3. **Simplified types**: Removed UNION types for attributes, replaced with simple STRUCT + ENUM
4. **Log body simplified**: Changed from UNION to VARCHAR + BodyType

---

## Types

### Old Schema Types

```sql
-- UNION type for attributes (removed)
CREATE TYPE attribute AS UNION(
    string VARCHAR,
    int64 BIGINT,
    float64 DOUBLE,
    boolean BOOLEAN,
    string_list VARCHAR[],
    int64_list BIGINT[],
    float64_list DOUBLE[],
    boolean_list BOOLEAN[]
)

-- UNION type for body (to be simplified)
CREATE TYPE body AS UNION(
    string VARCHAR,
    int64 BIGINT,
    float64 DOUBLE,
    boolean BOOLEAN,
    bytes BLOB,
    json JSON
)

-- UNION type for data points (to be removed)
CREATE TYPE dataPoints AS UNION(
    Gauge gauge[],
    Sum sum[],
    Histogram histogram[],
    ExponentialHistogram exponentialHistogram[]
)

-- Event with attributes (attributes removed)
CREATE TYPE event AS STRUCT(
    Name VARCHAR,
    Timestamp BIGINT,
    Attributes MAP(VARCHAR, attribute),  -- REMOVED
    DroppedAttributesCount UINTEGER
)

-- Link with attributes (attributes removed)
CREATE TYPE link AS STRUCT(
    TraceID VARCHAR,
    SpanID VARCHAR,
    TraceState VARCHAR,
    Attributes MAP(VARCHAR, attribute),  -- REMOVED
    DroppedAttributesCount UINTEGER
)

-- Exemplar with attributes (attributes removed)
CREATE TYPE exemplar AS STRUCT(
    Timestamp BIGINT,
    Value DOUBLE,
    TraceID VARCHAR,
    SpanID VARCHAR,
    FilteredAttributes MAP(VARCHAR, attribute)  -- REMOVED
)

-- Data point types with attributes (attributes to be removed)
CREATE TYPE gauge AS STRUCT(
    Timestamp BIGINT,
    StartTime BIGINT,
    Attributes MAP(VARCHAR, attribute),  -- TO BE REMOVED
    Flags UINTEGER,
    ValueType VARCHAR,
    Value DOUBLE,
    Exemplars exemplar[]
)
-- Similar for sum, histogram, exponentialHistogram
```

### New Schema Types

```sql
-- New ENUM for attribute types
CREATE TYPE attr_type AS ENUM(
    'string', 'int64', 'float64', 'bool',
    'string[]', 'int64[]', 'float64[]', 'boolean[]'
)

-- New STRUCT for attribute values
CREATE TYPE attr_value AS STRUCT(
    v VARCHAR,  -- value as string
    t attr_type -- type enum
)

-- Simplified body (VARCHAR + BodyType, not shown in detail)
-- Body will be VARCHAR with separate BodyType column

-- Event without attributes
CREATE TYPE event AS STRUCT(
    Name VARCHAR,
    Timestamp BIGINT,
    DroppedAttributesCount UINTEGER
    -- Attributes stored in normalized attributes table
)

-- Link without attributes
CREATE TYPE link AS STRUCT(
    TraceID VARCHAR,
    SpanID VARCHAR,
    TraceState VARCHAR,
    DroppedAttributesCount UINTEGER
    -- Attributes stored in normalized attributes table
)

-- Exemplar without attributes
CREATE TYPE exemplar AS STRUCT(
    Timestamp BIGINT,
    Value DOUBLE,
    TraceID VARCHAR,
    SpanID VARCHAR
    -- FilteredAttributes stored in normalized attributes table
)

-- Data point types without attributes (to be moved to metric_data_points table)
-- Attributes will be in normalized attributes table
```

---

## Tables

### Spans Table

#### Old Schema
```sql
CREATE TABLE spans (
    TraceID VARCHAR,
    TraceState VARCHAR,
    SpanID VARCHAR,
    ParentSpanID VARCHAR,
    Name VARCHAR,
    Kind VARCHAR,
    StartTime BIGINT,
    EndTime BIGINT,
    Attributes MAP(VARCHAR, attribute),              -- REMOVED
    Events event[],
    Links link[],
    ResourceAttributes MAP(VARCHAR, attribute),       -- REMOVED
    ResourceDroppedAttributesCount UINTEGER,
    ScopeName VARCHAR,
    ScopeVersion VARCHAR,
    ScopeAttributes MAP(VARCHAR, attribute),         -- REMOVED
    ScopeDroppedAttributesCount UINTEGER,
    DroppedAttributesCount UINTEGER,
    DroppedEventsCount UINTEGER,
    DroppedLinksCount UINTEGER,
    StatusCode VARCHAR,
    StatusMessage VARCHAR
)
```

#### New Schema
```sql
CREATE TABLE spans (
    TraceID VARCHAR,
    TraceState VARCHAR,
    SpanID VARCHAR,
    ParentSpanID VARCHAR,
    Name VARCHAR,
    Kind VARCHAR,
    StartTime BIGINT,
    EndTime BIGINT,
    Events event[],  -- No attributes in event struct
    Links link[],    -- No attributes in link struct
    ResourceDroppedAttributesCount UINTEGER,
    ScopeName VARCHAR,
    ScopeVersion VARCHAR,
    ScopeDroppedAttributesCount UINTEGER,
    DroppedAttributesCount UINTEGER,
    DroppedEventsCount UINTEGER,
    DroppedLinksCount UINTEGER,
    StatusCode VARCHAR,
    StatusMessage VARCHAR
    -- All attributes (span, resource, scope, event, link) in normalized attributes table
)

-- Indexes
CREATE INDEX idx_spans_traceid ON spans(TraceID);
CREATE INDEX idx_spans_starttime ON spans(StartTime);
CREATE INDEX idx_spans_parentspanid ON spans(ParentSpanID);
```

---

### Logs Table

#### Old Schema
```sql
CREATE TABLE logs (
    LogID VARCHAR,
    Timestamp BIGINT,
    ObservedTimestamp BIGINT,
    TraceID VARCHAR,
    SpanID VARCHAR,
    SeverityText VARCHAR,
    SeverityNumber INTEGER,
    Body body,  -- UNION type
    ResourceAttributes MAP(VARCHAR, attribute),      -- REMOVED
    ResourceDroppedAttributesCount UINTEGER,
    ScopeName VARCHAR,
    ScopeVersion VARCHAR,
    ScopeAttributes MAP(VARCHAR, attribute),         -- REMOVED
    ScopeDroppedAttributesCount UINTEGER,
    Attributes MAP(VARCHAR, attribute),              -- REMOVED
    DroppedAttributesCount UINTEGER,
    Flags UINTEGER,
    EventName VARCHAR
)
```

#### New Schema
```sql
CREATE TABLE logs (
    LogID VARCHAR,
    Timestamp BIGINT,
    ObservedTimestamp BIGINT,
    TraceID VARCHAR,
    SpanID VARCHAR,
    SeverityText VARCHAR,
    SeverityNumber INTEGER,
    Body VARCHAR,        -- Simplified from UNION
    BodyType VARCHAR,    -- New: 'string', 'int64', 'float64', 'boolean', 'bytes', 'json'
    ResourceDroppedAttributesCount UINTEGER,
    ScopeName VARCHAR,
    ScopeVersion VARCHAR,
    ScopeDroppedAttributesCount UINTEGER,
    DroppedAttributesCount UINTEGER,
    Flags UINTEGER,
    EventName VARCHAR
    -- All attributes (log, resource, scope) in normalized attributes table
)

-- Indexes
CREATE INDEX idx_logs_timestamp ON logs(Timestamp);
CREATE INDEX idx_logs_traceid ON logs(TraceID);
CREATE INDEX idx_logs_severitynumber ON logs(SeverityNumber);
```

---

### Metrics Table

#### Old Schema
```sql
CREATE TABLE metrics (
    MetricID VARCHAR,
    Name VARCHAR,
    Description VARCHAR,
    Unit VARCHAR,
    DataPoints dataPoints,  -- UNION type with nested arrays
    ResourceAttributes MAP(VARCHAR, attribute),     -- REMOVED
    ResourceDroppedAttributesCount UINTEGER,
    ScopeName VARCHAR,
    ScopeVersion VARCHAR,
    ScopeAttributes MAP(VARCHAR, attribute),         -- REMOVED
    ScopeDroppedAttributesCount UINTEGER,
    Received BIGINT
)
```

#### New Schema
```sql
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
    -- DataPoints moved to metric_data_points table
    -- All attributes (metric resource, metric scope, data point, exemplar) in normalized attributes table
)

-- Indexes
CREATE INDEX idx_metrics_name ON metrics(Name);
CREATE INDEX idx_metrics_received ON metrics(Received);
CREATE INDEX idx_metrics_metrictype ON metrics(MetricType);
```

---

### Metric Data Points Table (NEW)

#### New Schema
```sql
CREATE TABLE metric_data_points (
    DataPointID VARCHAR PRIMARY KEY,  -- Generated ID
    MetricID VARCHAR,
    Timestamp BIGINT,
    StartTime BIGINT,
    Flags UINTEGER,
    
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
    
    -- Exemplars stored as JSON array (for now)
    Exemplars JSON,
    
    FOREIGN KEY (MetricID) REFERENCES metrics(MetricID)
)

-- Indexes
CREATE INDEX idx_metric_data_points_metric_time ON metric_data_points(MetricID, Timestamp DESC);
CREATE INDEX idx_metric_data_points_time ON metric_data_points(Timestamp DESC);
```

---

### Attributes Table (NEW)

#### New Schema
```sql
CREATE TABLE attributes (
    Signal VARCHAR NOT NULL,        -- 'trace', 'log', 'metric'
    EntityID VARCHAR NOT NULL,      -- SpanID, LogID, MetricID, DataPointID
    Scope VARCHAR,                   -- 'resource', 'scope', 'event', 'link', 'data_point', 'exemplar', NULL
    Index INTEGER,                   -- NULL for top-level, array index for nested entities
    Key VARCHAR NOT NULL,
    Value VARCHAR NOT NULL,          -- stored as string
    Type attr_type NOT NULL,        -- enum type
    
    PRIMARY KEY (Signal, EntityID, Scope, Index, Key)
)

-- Indexes
CREATE INDEX idx_attributes_entity ON attributes(Signal, EntityID);
CREATE INDEX idx_attributes_key_value ON attributes(Key, Value);
```

#### Attribute Examples

**Trace/Span Attributes:**
- Span attributes: `Signal='trace'`, `EntityID=SpanID`, `Scope=NULL`, `Index=NULL`
- Resource attributes: `Signal='trace'`, `EntityID=SpanID`, `Scope='resource'`, `Index=NULL`
- Scope attributes: `Signal='trace'`, `EntityID=SpanID`, `Scope='scope'`, `Index=NULL`
- Event attributes: `Signal='trace'`, `EntityID=SpanID`, `Scope='event'`, `Index=0` (first event)
- Link attributes: `Signal='trace'`, `EntityID=SpanID`, `Scope='link'`, `Index=0` (first link)

**Log Attributes:**
- Log attributes: `Signal='log'`, `EntityID=LogID`, `Scope=NULL`, `Index=NULL`
- Resource attributes: `Signal='log'`, `EntityID=LogID`, `Scope='resource'`, `Index=NULL`
- Scope attributes: `Signal='log'`, `EntityID=LogID`, `Scope='scope'`, `Index=NULL`

**Metric Attributes:**
- Metric resource: `Signal='metric'`, `EntityID=MetricID`, `Scope='resource'`, `Index=NULL`
- Metric scope: `Signal='metric'`, `EntityID=MetricID`, `Scope='scope'`, `Index=NULL`
- Data point attributes: `Signal='metric'`, `EntityID=MetricID`, `Scope='data_point'`, `Index=DataPointIndex`
- Exemplar attributes: `Signal='metric'`, `EntityID=DataPointID`, `Scope='exemplar'`, `Index=ExemplarIndex`

---

## Summary of Changes

### Removed
- ❌ `attribute` UNION type → replaced with `attr_type` ENUM + `attr_value` STRUCT
- ❌ `dataPoints` UNION type → replaced with normalized `metric_data_points` table
- ❌ `Attributes MAP(VARCHAR, attribute)` columns from all tables
- ❌ `ResourceAttributes MAP(VARCHAR, attribute)` columns from all tables
- ❌ `ScopeAttributes MAP(VARCHAR, attribute)` columns from all tables
- ❌ `FilteredAttributes` from exemplar type
- ❌ `Attributes` from event and link types

### Added
- ✅ `attr_type` ENUM type
- ✅ `attr_value` STRUCT type
- ✅ `attributes` normalized table
- ✅ `metric_data_points` normalized table
- ✅ `BodyType` column in logs table
- ✅ `MetricType` column in metrics table
- ✅ Multiple indexes for performance

### Benefits
1. **Simpler queries**: No more `union_tag()` and `union_extract()` calls
2. **Better searchability**: Indexed key/value lookups in attributes table
3. **Efficient discovery**: `SELECT DISTINCT Key FROM attributes` (no UNNEST needed)
4. **Normalized metrics**: Time-series queries on data points table
5. **Columnar optimization**: Low cardinality columns compress well
6. **Consistent structure**: All attributes follow same pattern

### Migration Notes
- Existing data will need migration script
- Queries will need to be updated to join attributes table
- AppenderWrapper can be simplified (no UNION reflection code)
