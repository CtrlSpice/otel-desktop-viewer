# axolotel TODO

## Phase 1: Database Schema Rework

**Architectural Decisions Made:**
- [x] Full normalization: Events, links, exemplars, attributes, and data points normalized into separate tables
- [x] Single table for metric data points: All metric types in one table with NULLs (optimized for columnar storage)
- [x] Attributes table: `SignalType` + `SignalID` + `Scope` + `OwnerID` structure
- [x] Depth calculation: Query-time calculation (not stored)
- See [ARCHITECTURE.md - Database Architectural Decisions](ARCHITECTURE.md#database-architectural-decisions) for details

**Schema Implementation:**
- [x] Create `attr_type` ENUM type: `ENUM('string', 'int64', 'float64', 'bool', 'string[]', 'int64[]', 'float64[]', 'boolean[]')`
- [x] Create new `attr_value` type: `STRUCT(v VARCHAR, t attr_type)`
- [x] Create `signal_type` ENUM: `ENUM('traces', 'logs', 'metrics')`
- [x] Create `attribute_scope` ENUM: `ENUM('span', 'resource', 'scope', 'event', 'link', 'log', 'metric', 'data_point', 'exemplar')`
- [x] Remove old UNION types: `attribute`, `dataPoints` (no longer needed)
- [x] Update spans table schema:
  - [x] Remove `Attributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `Events event[]` column (normalized to `events` table)
  - [x] Remove `Links link[]` column (normalized to `links` table)
  - [x] ~~Add `Depth` column~~ (not needed - calculate on query time)
- [x] Update logs table schema:
  - [x] Remove `Attributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
  - [x] Simplify `Body` to `VARCHAR` + `BodyType` (was UNION)
- [x] Update metrics table:
  - [x] Add `MetricType VARCHAR` column
  - [x] Remove `DataPoints` column (moved to normalized table)
  - [x] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [x] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
- [x] Create normalized `events` table (`ID`, `SpanID`, `Name`, `Timestamp`, `DroppedAttributesCount`)
- [x] Create normalized `links` table (`ID`, `SpanID`, `TraceID`, `LinkedSpanID`, `TraceState`, `DroppedAttributesCount`)
- [x] Create normalized `exemplars` table (`ID`, `DataPointID`, `Index`, `Timestamp`, `Value`, `TraceID`, `SpanID`)
- [x] Create normalized `metric_data_points` table (single table for all metric types with `MetricType` column)
- [x] Create normalized `attributes` table:
  - [x] `SignalType`, `SignalID`, `Scope`, `OwnerID`, `Key`, `Value`, `Type` columns
  - [x] Primary key on (`SignalType`, `SignalID`, `Scope`, `OwnerID`, `Key`)
  - [x] Index on (`SignalType`, `SignalID`)
  - [x] Index on (`OwnerID`)
  - [x] Index on (`Key`, `Value`)
- [x] Add indexes for spans (`TraceID`, `StartTime`, `ParentSpanID`)
- [x] Add indexes for events (`SpanID`, `Timestamp`)
- [x] Add indexes for links (`SpanID`, `TraceID`, `LinkedSpanID`)
- [x] Add indexes for logs (`Timestamp`, `TraceID`, `SeverityNumber`)
- [x] Add indexes for metrics (`Name`, `Received`, `MetricType`)
- [x] Add indexes for `metric_data_points` (`MetricType`, `MetricID`, `Timestamp`) and (`MetricID`, `Timestamp`)
- [x] Add indexes for exemplars (`DataPointID`, `Index`) and (`TraceID`, `SpanID`)

**Remaining Tasks:**
- [ ] Update all queries to use new schema (remove references to old columns)
- [ ] Write migration for existing data
- [ ] Test flush interval with new simple types
- [ ] Simplify AppenderWrapper (remove UNION reflection code)
- [ ] Optional: Create trace_summaries table

## Phase 2: Server Rework

**Foundation: JSON Rows from DuckDB (implement this first)**

The core architectural decision: have DuckDB output each query row as a JSON object, eliminating all intermediate Go structs. This is the foundation for everything else in Phase 2.

- [ ] **FIRST**: Update one query (e.g., `getTraceSummaries`) to output JSON rows using `json_object()`
- [ ] **FIRST**: Update handler to scan JSON strings into `[]json.RawMessage` instead of structs
- [ ] Create `store/ingest.go` with direct OTLP → DuckDB translation
- [ ] Create `convertAttributes()` helper for new format (returns map for `attributes` table)
- [ ] Create `convertEvents()` and `convertLinks()` helpers (no attributes in structs)
- [ ] Implement `IngestTraces()`:
  - [ ] Direct pdata to appender (insert into `spans` table)
  - [ ] Insert events into `events` table
  - [ ] Insert links into `links` table
  - [ ] Insert all attributes into `attributes` table (resource, scope, span, event, link attributes)
- [ ] Implement `IngestLogs()`:
  - [ ] Direct pdata to appender (insert into `logs` table)
  - [ ] Insert all attributes into `attributes` table
- [ ] Implement `IngestMetrics()`:
  - [ ] Insert into `metrics` table (metadata only)
  - [ ] Insert all data points into `metric_data_points` table
  - [ ] Insert exemplars into `exemplars` table
  - [ ] Insert all attributes into `attributes` table (resource, scope, metric, data point, exemplar attributes)
- [ ] Update all remaining queries to output rows as JSON objects using `json_object()`
- [ ] Update all handlers to scan JSON strings into `[]json.RawMessage`
- [ ] Update queries to join `attributes` table when building JSON responses
- [ ] Remove all response struct definitions (no longer needed)
- [ ] Remove/repurpose intermediate structs (SpanData, LogData, MetricData)
- [ ] Update query builder to use joins instead of UNNEST for attributes
- [ ] Update attribute discovery query to use normalized attributes table (no UNNEST)
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
- [ ] Implement time-series queries using normalized `metric_data_points` table

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
