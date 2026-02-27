# axolotel TODO

## Phase 1: Database Schema Rework

**Architectural Decisions Made:**
- [x] Full normalization: Events, links, exemplars, attributes, and data points normalized into separate tables
- [x] Single table for metric data points: All metric types in one table with NULLs (optimized for columnar storage)
- [x] Attributes table: Separate ID columns (`SpanID`, `EventID`, `LinkID`, `LogID`, `MetricID`, `DataPointID`, `ExemplarID`) with foreign keys
- [x] Depth calculation: Query-time calculation (not stored)
- See [ARCHITECTURE.md - Database Architectural Decisions](ARCHITECTURE.md#database-architectural-decisions) for details

**Schema Implementation:**
- [x] Create `attr_type` ENUM type: `ENUM('string', 'int64', 'float64', 'bool', 'string[]', 'int64[]', 'float64[]', 'boolean[]')`
- [x] Remove old UNION types: `attribute`, `dataPoints` (no longer needed)
- [x] Update spans table schema:
  - [x] Remove `Attributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `Events event[]` column (normalized to `events` table)
  - [x] Remove `Links link[]` column (normalized to `links` table)
  - [x] Update `TraceID`, `SpanID`, `ParentSpanID` to BLOB type (16 bytes, 8 bytes, 8 bytes respectively)
  - [x] ~~Add `Depth` column~~ (not needed - calculate on query time)
- [x] Update logs table schema:
  - [x] Remove `Attributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
  - [x] Simplify `Body` to `VARCHAR` + `BodyType` (was UNION)
  - [x] Update primary key to `ID UUID` (self-generating, was `LogID` removed)
  - [x] Update `TraceID`, `SpanID` to BLOB type (16 bytes, 8 bytes respectively)
- [x] Update metrics table:
  - [x] Remove `DataPoints` column (moved to normalized `datapoints` table)
  - [x] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `MetricType` column (moved to `datapoints` table only)
  - [x] Update primary key to `ID UUID` (self-generating, was `MetricID VARCHAR`)
- [x] Create normalized `events` table (`ID UUID`, `SpanID BLOB`, `Name`, `Timestamp`, `DroppedAttributesCount`)
- [x] Create normalized `links` table (`ID UUID`, `SpanID BLOB`, `TraceID BLOB`, `LinkedSpanID BLOB`, `TraceState`, `DroppedAttributesCount`)
- [x] Create normalized `exemplars` table (`ID UUID`, `DataPointID UUID`, `Timestamp`, `Value`, `TraceID BLOB`, `SpanID BLOB`)
- [x] Create normalized `datapoints` table (renamed from `metric_data_points`, single table for all metric types with `MetricType` column, `ID UUID`, `MetricID UUID`)
- [x] Create normalized `attributes` table:
  - [x] Separate ID columns: `SpanID BLOB`, `EventID UUID`, `LinkID UUID`, `LogID UUID`, `MetricID UUID`, `DataPointID UUID`, `ExemplarID UUID`
  - [x] `Key`, `Value`, `Type` columns
  - [x] Foreign keys on all ID columns with CASCADE deletes
  - [x] CHECK constraint to ensure exactly one direct owner ID is populated (discriminated union pattern)
  - [x] UNIQUE constraint on all ID columns + `Key`
  - [x] Covering indexes on each ID column (`ID`, `Key`, `Value`, `Type`)
  - [x] Hierarchical indexes for parent-child queries
  - [x] Index on (`Key`, `Value`, `Type`)
- [x] Add indexes for spans (`TraceID`, `StartTime`, `ParentSpanID`)
- [x] Add indexes for events (`SpanID`, `Timestamp`)
- [x] Add indexes for links (`SpanID`, `TraceID`, `LinkedSpanID`)
- [x] Add indexes for logs (`Timestamp`, `TraceID`, `SeverityNumber`)
- [x] Add indexes for metrics (`Name`, `Received`)
- [x] Remove `MetricType` from `metrics` table (kept in `datapoints` only)
- [x] Add indexes for `datapoints` (`MetricType`, `MetricID`, `Timestamp`) and (`MetricID`, `Timestamp`)
- [x] Add CHECK constraints for `MetricType` validation and discriminated union enforcement (enforces which fields must be populated based on `MetricType`)
- [x] Add indexes for exemplars (`DataPointID`) and (`TraceID`, `SpanID`)
- [x] Remove `Index` column from `datapoints` and `exemplars` tables (not needed)
- [x] Use UUID type for self-generating IDs: `events.ID`, `links.ID`, `logs.ID`, `metrics.ID`, `datapoints.ID`, `exemplars.ID` (all use `UUID PRIMARY KEY DEFAULT gen_random_uuid()`)
- [x] Use BLOB type for TraceID (16 bytes) and SpanID (8 bytes) - native binary format from OpenTelemetry
- [x] Add CHECK constraints to enforce fixed-size binary: TraceID = 16 bytes, SpanID = 8 bytes
- [x] Update insert statements in `traces.go`, `logs.go`, `metrics.go` to match new schema (BLOB for IDs, remove nested data, UUID self-generating)
- [x] Update ARCHITECTURE.md to reflect current schema design

**Remaining Tasks:**
- [ ] Update all queries to use new schema (remove references to old columns)
- [ ] Test flush interval with new simple types
- [ ] Simplify AppenderWrapper (remove UNION reflection code)
- [ ] Optional: Create trace_summaries view

## Phase 2: Server Rework

**Foundation: JSON Rows from DuckDB (implement this first)**

The core architectural decision: have DuckDB output each query row as a JSON object, eliminating all intermediate Go structs. This is the foundation for everything else in Phase 2.

- [ ] **FIRST**: Update one query (e.g., `getTraceSummaries`) to output JSON rows using `json_object()`
- [ ] **FIRST**: Update handler to scan JSON strings into `[]json.RawMessage` instead of structs
- [ ] Create `store/ingest.go` with direct OTLP → DuckDB translation
- [ ] Create `convertAttributes()` helper for new format (returns rows for `attributes` table with appropriate ID columns populated)
- [ ] Create `convertEvents()` and `convertLinks()` helpers (no attributes in structs)
- [ ] Implement `IngestTraces()`:
  - [ ] Direct pdata to appender (insert into `spans` table)
  - [ ] Insert events into `events` table
  - [ ] Insert links into `links` table
  - [ ] Insert all attributes into `attributes` table (span attributes: `SpanID` only; event attributes: `EventID` + `SpanID`; link attributes: `LinkID` + `SpanID`)
- [ ] Implement `IngestLogs()`:
  - [ ] Direct pdata to appender (insert into `logs` table)
  - [ ] Insert all attributes into `attributes` table (log attributes: `LogID` only)
- [ ] Implement `IngestMetrics()`:
  - [ ] Insert into `metrics` table (metadata only)
  - [ ] Insert all data points into `datapoints` table
  - [ ] Insert exemplars into `exemplars` table
  - [ ] Insert all attributes into `attributes` table (metric attributes: `MetricID` only; data point attributes: `DataPointID` + `MetricID`; exemplar attributes: `ExemplarID` + `DataPointID` + `MetricID`)
- [ ] Update all remaining queries to output rows as JSON objects using `json_object()`
- [ ] Update all handlers to scan JSON strings into `[]json.RawMessage`
- [ ] Update queries to join `attributes` table when building JSON responses
- [ ] Remove all response struct definitions (no longer needed)
- [ ] Remove/repurpose intermediate structs (SpanData, LogData, MetricData)
- [ ] Update query builder to use joins instead of UNNEST for attributes (join on appropriate ID column based on entity type)
- [ ] Update attribute discovery query to use normalized attributes table (filter by ID column IS NOT NULL instead of SignalType/Scope)
- [ ] Add notification callbacks (OnSpansAdded, OnLogsAdded, OnMetricsAdded)
- [ ] Create `server/websocket_handler.go`
- [ ] Add `/ws` endpoint to server

## Phase 3: Waterfall View

- [ ] Create `WaterfallView.svelte`
- [ ] Create `WaterfallRow.svelte`
- [ ] Create `DurationBar.svelte`
- [ ] Create `HeaderRow.svelte`
- [ ] Add span selection state
- [ ] Wire selected span to DetailView
- [ ] Add click handlers for span selection

## Phase 4: Keyboard Navigation

- [ ] Arrow Up/Down, j/k: Navigate spans
- [ ] Arrow Left/Right, h/l: Collapse/expand children
- [ ] r: Refresh data
- [ ] ?: Show keyboard help modal
- [ ] Port `use-key-press.ts` logic to Svelte

## Phase 5: WebSocket Frontend

- [ ] Create `websocket-service.svelte.ts`
- [ ] Implement connection management (connect, disconnect, reconnect)
- [ ] Implement subscription management
- [ ] Implement live tail controls (start, stop, pause, resume)
- [ ] Create `ConnectionStatus.svelte`
- [ ] Create `LiveTailToggle.svelte`
- [ ] Integrate WebSocket into TracesPage
- [ ] Integrate WebSocket into HomePage (live stats)

## Phase 6: Logs View Enhancement

- [ ] Wire up PageHeader with DateTimeFilter and SearchInput
- [ ] Add pagination
- [ ] Add severity color-coded badges
- [ ] Add "jump to trace" link when TraceID present
- [ ] Add JSON syntax highlighting for log body
- [ ] Add live tail mode toggle
- [ ] Implement auto-scroll with pause on user scroll
- [ ] Add `searchLogs` JSON-RPC method

## Phase 7: Metrics View Enhancement

- [ ] Add uPlot dependency
- [ ] Create `MetricCard.svelte`
- [ ] Create `MetricDetailView.svelte`
- [ ] Implement Gauge visualization (value + sparkline)
- [ ] Implement Sum/Counter visualization (value + rate chart)
- [ ] Implement Histogram visualization (bucket bars + percentiles)
- [ ] Add list/grid view toggle
- [ ] Wire up WebSocket for live metric updates
- [ ] Add `searchMetrics` JSON-RPC method
- [ ] Add exemplar drill-down (link to traces)
  - [ ] Implement time-series queries using normalized `datapoints` table

## Phase 8: Landing Page

- [ ] Add live counts via WebSocket
- [ ] Add recent activity feed
- [ ] Add connection status indicator
- [ ] Add "receiving data" animation
- [ ] Add clear all data button
- [ ] Add generate sample data button
- [ ] Add empty state with setup instructions
- [ ] Add `getStats` JSON-RPC method

## Phase 9: Polish

- [ ] Create `KeyboardHelpModal.svelte`
- [ ] Improve error handling and loading states
- [ ] Add empty state illustrations
- [ ] Cross-browser testing
- [ ] Accessibility audit (keyboard nav, ARIA labels)
- [ ] Performance test with large traces (1000+ spans)

## Phase 10: Deployment, Configuration & Rename

- [ ] Add `--config` flag to support YAML config files
- [ ] Support both YAML config file AND command-line flags (flags override config)
- [ ] Add retry configuration to `Config` struct (`configretry.Config`)
- [ ] Add `exporterhelper.WithRetry()` to all exporter factories (traces, metrics, logs)
- [ ] Update `Config.Validate()` to validate retry settings
- [ ] Document YAML config format in README
- [ ] Add example YAML config file
- [ ] Rename GitHub repo to `axolotel`
- [ ] Update `go.mod` module path
- [ ] Update all internal Go imports
- [ ] Update `.goreleaser.yaml` (image name)
- [ ] Update `Dockerfile`
- [ ] Update CI workflows
- [ ] Fix CI (update references, test workflows)
- [ ] Update README
- [ ] Write nginx reverse proxy example
- [ ] Write Traefik reverse proxy example
- [ ] Write Caddy reverse proxy example
- [ ] Write Docker Compose example
