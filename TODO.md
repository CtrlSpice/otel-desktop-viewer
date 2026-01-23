# axolotel TODO

## Phase 1: Database Schema Rework

- [ ] Create `attr_type` ENUM type: `ENUM('string', 'int64', 'float64', 'bool', 'string[]', 'int64[]', 'float64[]', 'boolean[]')`
- [ ] Create new `attr_value` type: `STRUCT(v VARCHAR, t attr_type)`
- [ ] Update `event`, `link`, `exemplar` types to remove Attributes fields (attributes normalized)
- [ ] Update spans table schema:
  - [ ] Remove `Attributes MAP(VARCHAR, attribute)` column
  - [ ] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [ ] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
  - [ ] Add `Depth` column (pre-compute span depth)
- [ ] Update logs table schema:
  - [ ] Remove `Attributes MAP(VARCHAR, attribute)` column
  - [ ] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [ ] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
  - [ ] Simplify Body to VARCHAR + BodyType (was UNION)
- [ ] Normalize metrics: create `metric_data_points` table (one row per data point)
- [ ] Update metrics table:
  - [ ] Add `MetricType VARCHAR` column
  - [ ] Remove `DataPoints` column (moved to normalized table)
  - [ ] Remove `ResourceAttributes MAP(VARCHAR, attribute)` column
  - [ ] Remove `ScopeAttributes MAP(VARCHAR, attribute)` column
- [ ] Create normalized `attributes` table:
  - [ ] EntityType, EntityID, AttributeScope, Key, Value, Type columns
  - [ ] Primary key on (EntityType, EntityID, AttributeScope, Key)
  - [ ] Index on (Key, Value)
  - [ ] Index on (EntityType, EntityID)
  - [ ] Index on (AttributeScope, Key)
  - [ ] Composite index on (EntityType, EntityID, AttributeScope, Key)
- [ ] Remove `dataPoints` UNION type (no longer needed)
- [ ] Remove `attribute` UNION type (replaced with `attr_value` STRUCT)
- [ ] Add indexes for spans (TraceID, StartTime, ParentSpanID)
- [ ] Add indexes for logs (Timestamp, TraceID, SeverityNumber)
- [ ] Add indexes for metrics (Name, Received, MetricType)
- [ ] Add indexes for metric_data_points (MetricID, Timestamp)
- [ ] Optional: Create trace_summaries table
- [ ] Write migration for existing data
- [ ] Test flush interval with new simple types
- [ ] Simplify AppenderWrapper (remove UNION reflection code)

## Phase 2: Server Rework

**Foundation: JSON Rows from DuckDB (implement this first)**

The core architectural decision: have DuckDB output each query row as a JSON object, eliminating all intermediate Go structs. This is the foundation for everything else in Phase 2.

- [ ] **FIRST**: Update one query (e.g., `getTraceSummaries`) to output JSON rows using `json_object()`
- [ ] **FIRST**: Update handler to scan JSON strings into `[]json.RawMessage` instead of structs
- [ ] Create `store/ingest.go` with direct OTLP → DuckDB translation
- [ ] Create `convertAttributes()` helper for new format (returns map for attributes table)
- [ ] Create `convertEvents()` and `convertLinks()` helpers (no attributes in structs)
- [ ] Implement `IngestTraces()`:
  - [ ] Direct pdata to appender (insert into spans table)
  - [ ] Insert all attributes into `attributes` table (resource, scope, span, event, link attributes)
- [ ] Implement `IngestLogs()`:
  - [ ] Direct pdata to appender (insert into logs table)
  - [ ] Insert all attributes into `attributes` table
- [ ] Implement `IngestMetrics()`:
  - [ ] Insert into metrics table (metadata only)
  - [ ] Insert all data points into `metric_data_points` table
  - [ ] Insert all attributes into `attributes` table (resource, scope, metric data point attributes)
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
