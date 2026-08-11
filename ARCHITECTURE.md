# otel-desktop-viewer Architecture

otel-desktop-viewer is a custom [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) distribution with a single custom **`desktop` exporter**. The exporter receives OTLP traces, metrics, and logs, stores them in **DuckDB**, and serves a **Svelte 5** web UI over **HTTP + JSON-RPC**.

The design optimizes for local development: easy install, minimal moving parts, fast analytical queries over telemetry, and a UI for exploring all three signals.

## System overview

```mermaid
flowchart TB
  subgraph ingest [Ingestion]
    SDK[OTel SDK / Collector / test apps] -->|OTLP gRPC :4317 or HTTP :4318| OTLP[otlp receiver]
    OTLP --> Desktop[desktop exporter]
    Desktop --> Spans[spans.Ingest]
    Desktop --> Metrics[metrics.Ingest]
    Desktop --> Logs[logs.Ingest]
    Spans --> DuckDB[(DuckDB)]
    Metrics --> DuckDB
    Logs --> DuckDB
  end

  subgraph serve [Serving]
    Browser[Browser] -->|GET /| Static[Embedded static assets]
    Browser -->|POST /rpc| RPC[JSON-RPC handler]
    RPC --> Query[Search / get SQL]
    Query --> DuckDB
    Query -->|json.RawMessage| RPC
    RPC -->|JSON| Browser
  end

  Desktop -->|starts at collector boot| HTTP[HTTP server :8000]
  HTTP --> Static
  HTTP --> RPC
```

**Default ports**

| Port | Purpose |
|------|---------|
| 4317 | OTLP gRPC |
| 4318 | OTLP HTTP |
| 8000 | Web UI + JSON-RPC (`POST /rpc`) |
| 3001 | Vite dev server (frontend only; proxies `/rpc` → 8000) |

## Repository layout

```
otel-desktop-viewer/
├── main.go                    # CLI entry; builds inline collector config from flags
├── main_others.go / main_windows.go
├── components.go              # OCB-generated component registry
├── desktopexporter/           # Custom exporter package
│   ├── factory.go             # Exporter factory + shared-component wiring
│   ├── exporter.go            # pushTraces / pushMetrics / pushLogs
│   └── internal/
│       ├── server/            # HTTP server, JSON-RPC, embedded static assets
│       ├── sharedcomponent/   # Thread-safe shared exporter instance per config
│       ├── store/             # DuckDB store, schema, ingest, search, query
│       └── frontend/          # Svelte 5 + Vite UI
├── scripts/                   # OTLP seed scripts for local dev
├── Makefile                   # Build, run, dev, test targets
└── ARCHITECTURE.md
```

The root module builds the collector binary. The frontend builds into `desktopexporter/internal/server/static/` for production embed.

## Collector binary

Built with the [OpenTelemetry Collector Builder (OCB)](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder). Generated files (`main.go`, `components.go`) should not be edited by hand except where already customized. The distribution currently builds against **collector v0.157.0 / v1.63.0** (`go.mod`); `components.go` also records module versions in `ReceiverModules` / `ProcessorModules` metadata strings (keep these in sync when bumping). Requires **Go 1.26**.

**Registered components** (`components.go`):

| Kind | Component | Used in default config? |
|------|-----------|---------------------------|
| Receiver | `otlp` (HTTP + gRPC) | Yes |
| Exporter | `desktop` | Yes |
| Processor | `batch` | Registered, **not wired** into default pipelines |

**Default pipelines** (built from CLI flags in `main.go`):

```
traces:  otlp → desktop
metrics: otlp → desktop
logs:    otlp → desktop
```

**CLI flags**

| Flag | Default | Purpose |
|------|---------|---------|
| `--http` | 4318 | OTLP HTTP listen port |
| `--grpc` | 4317 | OTLP gRPC listen port |
| `--browser-port` | 8000 | UI + JSON-RPC port |
| `--host` | localhost | Bind address for all endpoints |
| `--db` | *(empty)* | DuckDB file path; empty = in-memory |
| `--db-max-size` | *(empty)* | Store size cap (e.g. `512MB`, `2GB`); oldest telemetry pruned when exceeded. `0` disables pruning. Defaults to 512 MB in-memory, 2 GB on disk. |
| `--open-browser` | true | Open UI on startup |

Configuration is injected as inline YAML resolver URIs at startup. There is no `--config` file path exposed by the CLI today, though the underlying collector supports YAML providers.

## Desktop exporter

The `desktop` exporter is the heart of the application. For a given config it owns:

1. A **DuckDB store** (`internal/store`)
2. An **HTTP server** (`internal/server`) that serves the UI and JSON-RPC

Trace, metrics, and logs exporters are created separately by the collector factory, but they **share one `desktopExporter` instance** per config via a local `sharedcomponent` package (`internal/sharedcomponent/`, mutex-guarded; wired from `factory.go`). This ensures a single database and a single HTTP listener.

**Ingest path** (`exporter.go`):

```
OTLP pdata → exporterhelper → pushTraces|pushMetrics|pushLogs → store.WithConn → spans|metrics|logs.Ingest
```

Ingest writes directly from OpenTelemetry pdata into DuckDB appenders. There are no intermediate Go domain structs between OTLP and storage.

**Lifecycle**: `newDesktopExporter` opens the store and HTTP server using the factory startup context. `Start()` binds the listen address synchronously (bind failures such as port-in-use propagate to the collector), then serves HTTP on a background goroutine. Ingest paths check `ctx.Err()` before work and on every record (metrics pass 1 included); `CloseAppenders` on exit flushes buffered rows. When a retention cap is configured, a background loop enforces it every 30 seconds. `Shutdown()` cancels the retention loop and waits for it to finish, gracefully shuts down the HTTP server and waits for the serve goroutine, then closes the store.

**Retention**: `--db-max-size` sets a byte cap on stored telemetry. When usage exceeds the cap, the oldest traces, logs, and metrics are pruned. `getStats` reports current usage and the configured cap alongside signal counts.

## Storage (DuckDB)

**Engine**: DuckDB via `github.com/duckdb/duckdb-go/v2` (CGO required).

**Connection model**: `store.Store` holds two handles to one DuckDB database, ordered by a single `sync.RWMutex`.

- `conn` — a dedicated `driver.Conn` from `connector.Connect`, used only by ingest. DuckDB appenders are bound to the connection that created them, so ingest cannot run on the pool.
- `db` — a `*sql.DB` pool from `sql.OpenDB`, used by every query and by the delete/checkpoint paths. The pool is capped via `SetMaxOpenConns`, because each pooled connection is a real DuckDB connection with real memory cost.

Access is chosen by intent, not by handle:

| Method | Lock | Handle | Used by |
|--------|------|--------|---------|
| `WithConn` | write | `conn` | ingest (appenders) |
| `WithDBWrite` | write | `db` | clear, delete, retention prune + checkpoint |
| `WithDBRead` | read | `db` | all queries |

The write lock is shared across both handles, so appender writes, deletes, and retention checkpoints are mutually exclusive even though they run on different connections. Reads run concurrently with one another and never overlap a write.

What this does **not** guarantee: a query does not see rows that an in-flight ingest has appended but not yet flushed. DuckDB appenders buffer client-side and become visible on flush — automatically at the chunk threshold, or on `Flush`/`Close` at the end of each ingest call. Under load the UI can lag ingest by up to one batch. That is a visibility window, not a lost write.

`EnforceRetention` takes the write lock once per prune round rather than across a whole pass, so queries interleave between rounds instead of blocking for up to three checkpoints.

`Store` exposes no accessor for its `*sql.DB`. Handing out the pool would let a caller query after the lock released, which is the ordering these methods exist to enforce, so every caller — production and test alike — passes a closure to `WithDBRead` or `WithDBWrite`. CI enforces this: a `.DB()` call outside `_test.go` files fails the `go-checks` job.

### Schema

Schema is defined in `desktopexporter/internal/store/schema/schema.go` and applied on store creation.

**Core tables**

| Table | Role |
|-------|------|
| `attributes` | Dictionary of distinct `(key, value, type, scope)` rows, keyed by a content hash |
| `resources` | Deduped resources, shared across all three signals; `seq` is the wire key |
| `scopes` | Deduped instrumentation scopes; `seq` is the wire key |
| `spans` | Span records; `resource_id`, `scope_id`, `attribute_ids`, plus `service_name` denormalized from `service.name` |
| `events` | Span events (normalized) |
| `links` | Span links (normalized) |
| `logs` | Log records; same reference columns as `spans` |
| `metric_streams` | Canonical identity for a logical metric (name, unit, type, scope, service, …) |
| `metric_series` | One row per chart line: `(stream_id, resource_id, attribute_ids)` under a content-hashed id |
| `metric_ingests` | One row per OTLP batch arrival for a stream (description, `resource_id`, `scope_id`) |
| `datapoints` | All metric data points in one table; `metric_type` discriminates gauge/sum/histogram/exponential histogram; `series_id` names the line |
| `exemplars` | Metric exemplars (normalized) |

**Design themes**

- **All IDs are UUIDs in DuckDB.** OpenTelemetry 8-byte span IDs are zero-padded to 16 bytes on ingest. JSON-RPC responses and search comparisons use OTLP **wire form** (dash-less lowercase hex: 32 chars for trace IDs, 16 for span IDs).
- **Attributes are a content-addressed dictionary.** One row per distinct `(key, value, type, scope)` for the whole database, with `id = sha256(...)` truncated to 16 bytes and computed in Go at unwrap. Every owner holds an inline `uuid[]`, deduped and sorted by id. Because identity is the content, ingest knows every id before it writes and needs no read-back, and repeat writes are `on conflict (id) do nothing`.
- **Scope is part of dictionary identity**, not a free-form tag. That is what lets attribute discovery answer from `select distinct key, scope, type from attributes` alone, instead of unnesting every owner array. The cost is that the same triple used as both a resource and a span attribute is two rows.
- **Normalized nested data.** Events, links, and exemplars live in separate tables—not nested arrays or DuckDB UNION types.
- **Single `datapoints` table.** Type-specific columns use NULLs for irrelevant fields; `metric_type` + CHECK constraints enforce the discriminated union. Columnar compression makes sparse rows cheap.
- **`metric_streams` + `metric_ingests`.** Stream identity is deduplicated across batches; per-batch metadata varies without splitting logical metrics.
- **No referential integrity on array elements.** DuckDB cannot declare a foreign key into a `LIST`, so nothing at the engine level stops an `attribute_ids` entry pointing at a missing dictionary row. This is a knowing trade for the dedupe: it becomes ingest's responsibility, and store-level consistency tests assert no dangling references survive a `Clear` → ingest cycle. `resources` / `scopes` are reached by a real FK; only the arrays are unenforced.
- **Orphans are swept, not cascaded.** Since no FK covers the arrays, the `Clear` and delete-by-id paths leave dictionary rows behind rather than reference-counting them. `ingest.SweepOrphans` builds the live id set by unnesting every owner and deletes what nothing references. It runs from two places. The `clearTraces` / `clearLogs` / `clearMetrics` handlers sweep in the same write-locked closure as the truncate, so clearing a signal actually reclaims its share of the dictionary — this cannot be left to retention, which is size-driven and does not run at all when the cap is disabled, so the orphans would survive until restart. Retention sweeps too, once before its first size measurement and again at the end of each prune round, since the prunes are what create orphans. The invariant that buys: **no round deletes real telemetry to make room for rows nothing references** — which matters because orphans count toward the size the cap is compared against.

The per-id delete paths (`deleteSpansByTraceID`, `deleteSpanByID`, `deleteLogByID`, `deleteMetricStream`) deliberately do **not** sweep — deleting one trace would otherwise pay for a full unnest of every owner table — so their orphans wait for the next clear or retention round.
- **`service_name` stays denormalized** on `spans` and `logs` even though resources are now deduped. With ~24 resource rows the join is cheap, but this is the hottest filter in span search and a column scan still beats a join plus an array unnest.
- **Indexes are equality-only, by engine constraint.** DuckDB's ART indexes serve equality and `IN` on a single column — never ranges, joins, aggregation or sorting — and min-max zonemaps are maintained automatically for every column. So the time-column indexes were dropped: they cost every write and, measured alternating to avoid cache bias, made no difference to reads. A `LIST` column cannot be indexed or FK'd at all, which is why `metric_series` exists — it turns a chart's grouping key from an unindexable array into one indexable `uuid`.
- **Depth is computed at query time** via recursive CTEs when building trace waterfalls—not stored on ingest.
- **The schema is versioned.** `schema_meta` holds a single integer, checked against `schema.Version` before the table and index loops run. A mismatch, or a pre-versioning database with data in it, is refused with a message naming the db path — deliberately an error rather than a warning, so an incompatible database fails immediately instead of surfacing later as an opaque query error.

### Ingest

Ingest is **two-pass**, and per OTLP request costs three small inserts and no reads regardless of span count:

1. Walk the hierarchy hashing every attribute into `(key, value, type, scope)` ids, deduped in a Go map, and build each owner's sorted `uuid[]`.
2. Insert `attributes` **first**, then `resources` / `scopes`. Ordering is deliberate: no FK can enforce it, and a crash between the two leaves collectable orphans rather than a resource referencing rows that do not exist.
3. Open the appenders and walk again, appending owners with their arrays.

| Signal | Package | Notes |
|--------|---------|-------|
| Traces | `store/spans` | Flushes appenders every 500 spans |
| Metrics | `store/metrics` | Stream find-or-insert, series resolve, then datapoints/exemplars |
| Logs | `store/logs` | Flushes appenders every 500 records |

The dictionary inserts cannot use an appender: appenders have no conflict handling, and a constraint violation errors at flush and takes the whole chunk with it. The high-volume tables have no dedupe requirement and keep the appender.

`store/ingest/` holds the shared pieces: `dictionary.go` (hashing and id construction), `flushed.go` (a per-store cache of ids already written, so a repeat batch skips the insert entirely — invalidated in the one function that deletes dictionary rows), and `sweep.go`.

## Query layer and API

### JSON rows from DuckDB

Query functions build JSON in SQL using `json_object`, `to_json(list(...))`, etc., and scan each result row into `json.RawMessage`. The JSON-RPC layer forwards these bytes without Go response structs.

Ordered aggregation is `to_json(list(x order by k))` rather than `json_group_array`, which is a macro and therefore rejects `ORDER BY` inside it. Attribute arrays are ordered by key on the read path, which is also what makes the JSON deterministic — the previous output followed scan order with no `ORDER BY` anywhere, so it was never actually order-stable.

**Shared shapes live in SQL macros** (`MacroCreationQueries`), layered the way the histogram math already was: leaf value helpers (`attrs_json`, `attr_value`, `has_attr`, `trace_id_wire`, `span_id_wire`), then component objects (`resource_json`, `scope_json`, `attribute_def_json`). `attrs_json(ids)` alone replaced the same unnest-and-join fragment repeated across spans, logs and metrics.

**Why**: Response shape is defined once in SQL. No duplicate struct tags, no scan-then-marshal step. The frontend is the primary consumer.

**Trade-off**: Response structure is not statically typed in Go; it lives in SQL strings, and macros are invisible to Go tooling — a typo surfaces at runtime, which is what the macro unit tests exist for.

### Search

The frontend builds a **query tree** (`src/components/shared/Search/queryTree.ts`). `store/search/search_tree.go` walks the tree and generates SQL with:

- Positional parameter binding (ordered param list)
- `{COND}` placeholders for composable WHERE fragments
- `{RAW}` for array containment checks
- Signal-specific field mappers in `spans`, `logs`, and `metrics` packages

Global search casts scalar fields to strings and searches attribute key/value pairs through the dictionary.

**Attribute equality takes a fast path.** An attribute id is a pure function of `(key, value, type, scope)`, so an equality search can compute the id it wants before the query runs: `ingest.IDProbe` emits `list_contains(attribute_ids, '<id>'::uuid)` and the predicate never joins the dictionary at all (2.67 ms → 0.13 ms on the reference capture). It is narrow on purpose and returns `""` — falling back to the correct-but-slower value comparison — for anything it cannot answer byte-exactly: any operator but `=`, the `NULL` sentinel, and any type token the schema enum does not contain. The type comes from the field definition, which for attribute fields is the token ingest wrote, served back by discovery.

The `attr_id` / `attr_frame` SQL macros reimplement the same hash independently. They are deliberately kept **off** the correctness path — used only to audit that stored ids match their content — because one implementation writing and reading with a second one checking is what makes the check meaningful. Putting the macro in search predicates would turn a Go/SQL divergence into search silently returning nothing.

Attribute *discovery* is served from the dictionary (`store/attributes`), which also answers value-first lookup: given text a user can see in the UI, return the keys that hold it, across every signal in one scan of a small table.

### HTTP server

`internal/server/server.go`:

| Route | Handler |
|-------|---------|
| `POST /rpc` | JSON-RPC 2.0 (`golang.org/x/exp/jsonrpc2`); request bodies capped at 1 MB |
| `GET /*` | Embedded static files; extension-less unknown paths fall back to `index.html` for client-side routing |

CORS is enabled for local dev (Vite on port 3001).

**Static assets**

- Embedded via `//go:embed static` after `make build-ts` (which wipes `server/static/` before copying, so stale hashed assets do not accumulate)
- The root `Dockerfile` builds the frontend in a Node stage before `go build`, so `docker build` embeds the current UI without a local `make build-ts`
- Frontend iteration uses the Vite dev server (`make dev-ts` on port 3001), which proxies `/rpc` to the Go server

### JSON-RPC methods

| Method | Purpose |
|--------|---------|
| `searchTraces` | Trace summaries for list view |
| `searchSpans` | Full trace with spans, events, links, attributes |
| `getTraceSpanCount` | Span count for a trace |
| `getTraceAttributes` | Attribute key discovery, served from the dictionary (search autocomplete) |
| `searchAttributes` | Value-first discovery: given text, the fields that would find it |
| `getAttributesByTraceID` | Attribute key discovery for one trace |
| `searchLogs` / `getLog` | Log list and detail |
| `getLogAttributes` | Attribute discovery for logs |
| `searchMetricSummaries` | Metric stream list |
| `getMetric` | Metric detail and time series for one stream in a time window |
| `getMetricAttributes` | Attribute discovery for metrics |
| `getStats` | Signal counts plus store `sizeBytes` / `maxSizeBytes` (used for polling and retention UI) |
| `clearTraces` / `clearLogs` / `clearMetrics` | Delete all data for a signal |
| `deleteSpansByTraceID` | Delete one or more traces by ID (batch param) |
| `deleteSpanByID` / `deleteLogByID` | Delete one or more spans or logs by ID (batch param) |
| `deleteMetricStream` | Delete one metric stream and its cascade (single ID, not a batch) |

Domain errors map to JSON-RPC error codes in `internal/server/errors.go`. The API has one not-found convention: requesting a specific entity that does not exist returns an error (`-32001` trace, `-32002` log, `-32003` metric), never a `null` result. `getMetric` distinguishes an unknown stream (`-32003`) from a known stream with no datapoints in the requested window (valid `MetricData` with an empty `timeseries`). Invalid ID *params* return dedicated codes rather than surfacing as internal errors on read and delete paths. `deleteMetricStream` takes a single ID rather than a batch, unlike the span and log delete methods: metrics address a stream by one UUID everywhere else in the API (see `getMetric`), and the store's delete cascade is keyed on a single `stream_id`. Deleting a stream that does not exist is a no-op, not an error — the cascade is a series of unconditional `DELETE`s, and the UI relies on that when a list poll races a delete. IDs embedded in search query trees (`traceID`, `spanID`, `link.*`, etc.) compare in OTLP wire form: values are dash-stripped and lowercased, columns are converted to the same wire shape, and malformed input returns empty results instead of `-32603` cast errors. The frontend service layer (`telemetry-service.ts`) translates these codes into whatever shape its callers want (e.g. `getMetric` returns `null` on `-32003`).

| Code | Meaning |
|------|---------|
| `-32001` | Trace not found (`searchSpans`, `getTraceSpanCount`, …) |
| `-32002` | Log not found (`getLog`) |
| `-32003` | Metric stream not found (`getMetric`) |
| `-32004` | Invalid trace ID param |
| `-32005` | Invalid log ID param |
| `-32007` | Invalid search query tree |
| `-32008` | Invalid span ID param |
| `-32009` | Invalid metric stream ID param |

## Frontend

**Location**: `desktopexporter/internal/frontend/`

### Stack

| Layer | Choice |
|-------|--------|
| Framework | Svelte 5 (runes: `$state`, `$derived`, `$effect`) |
| Build | Vite 8 |
| Routing | First-party (History API, `src/route/`) |
| Styling | Tailwind CSS 4 + DaisyUI 5 |
| Components | bits-ui |
| Search UI | CodeMirror 6 + Lezer grammar (`src/components/shared/Search/codemirror/`) |
| Charts | layerchart |
| Tests | Vitest (unit, component, context) + svelte-check in CI |

### Routing

`App.svelte`:

| Route | Page |
|-------|------|
| `/` | Home — onboarding, OTLP setup snippets, stats |
| `/traces`, `/traces/{id}` | Trace list, waterfall, span detail |
| `/logs`, `/logs/{id}` | Log list and detail |
| `/metrics`, `/metrics/{id}` | Metric summaries, charts, detail panels |

Selection and sub-view state (span, metric datapoint/tab, time window) live in the URL via `src/route/`. The server serves `index.html` for extension-less client routes on hard load and refresh.

Every URL write takes an explicit `HistoryMode` (`'push' | 'replace'`, defined in `src/route/router.ts`): navigation — selecting an item, switching signals, picking a tab — pushes so the back button retraces steps; adjustments — aggregation, scope, time window — replace so history isn't flooded. The mode flows through all layers (router → query modules → contexts) without re-encoding.

### State management

No global store library. State uses Svelte **context modules** (`.svelte.ts`) and page-local `$state`:

| Context | Scope |
|---------|-------|
| `route-context.svelte.ts` | Reactive view of the current URL (path + query) |
| `time-context.svelte.ts` | App-wide time range and timezone |
| `metric-view-context.svelte.ts` | Per-metrics-page chart aggregation, heatmaps, legend |
| `panel-split-resize-context.svelte.ts` | Resizable panel preferences |
| `theme.svelte.ts` | DaisyUI theme via `data-theme` |

Each signal page owns list/selection state locally.

### UI layout pattern

Three-pane model via `PageLayout.svelte` and `SignalListDrawer.svelte`:

1. **Drawer** — navigation, search toolbar, virtualized list
2. **Main** — waterfall (traces), chart/table (metrics), or log stream
3. **Detail** — span/log/metric inspector (optional resizable split)

### API client

`services/telemetry-service.ts` posts JSON-RPC requests to `/rpc`. Wire payloads are typed in `types/wire-types.ts` (`Json*` interfaces); revivers convert them to domain types in `types/api-types.ts`. Search queries are sent as query trees; time ranges are converted to nanosecond strings for the backend.

**Metrics**: The backend returns raw datapoints; histogram quantiles, rates, heatmaps, and legend state are computed client-side in `metric-view-context.svelte.ts`.

### Real-time updates

The UI **polls** `getStats` on an interval to detect new data and show refresh affordances: every 3 seconds on trace, log, and metric pages; every 5 seconds on home. There is no WebSocket push channel.

## Data flows

### Write path (ingest)

```
App / SDK
  → OTLP (gRPC or HTTP)
  → otlp receiver
  → desktop exporter (exporterhelper)
  → spans|metrics|logs.Ingest
  → pass 1: hash attributes → insert dictionary, then resources/scopes
  → pass 2: DuckDB appenders (owners carrying uuid[] references)
```

### Read path (traces example)

```
TracesPage
  → telemetryAPI.searchTraces(startNs, endNs, queryTree)
  → POST /rpc searchTraces
  → spans.SearchTraces (SQL + search tree → []json.RawMessage)
  → Trace list rendered

User selects trace
  → searchSpans(traceID)
  → Full trace JSON with depth CTE, events, links, attributes
  → traceDataFromJSON rehydrates the compressed wire shape
  → Waterfall + detail panels
```

### Wire format

`searchSpans` does not repeat data that is constant across the response.

- **Resources and scopes are sent once**, as top-level maps keyed by the store-stable `resources.seq` / `scopes.seq`, with each span carrying short `r` and `s` references. On the reference trace that is 23 resources and 1 scope against 4,891 spans that previously carried a full copy each — over half the payload. The keys are sequence values rather than response-local indices precisely so a client can cache "resource 7" across fetches and across signals; sequences are never reused after retention deletes a row, so a cached entry can go missing but never go wrong.
- **Times are an offset plus a duration.** The root carries `traceStart` as absolute nanoseconds; each span carries `start` (offset from it) and `dur`. Not two offsets: an end-offset inherits the trace's full magnitude however brief the span, while a duration stays small. It is also what a waterfall bar is — a position and a width.
- **`traceID` is not repeated** in `spanData`; it is at the response root.

`traceStart` is `min(start_time)` across the response, not the root span's start: clock skew across hosts means a child can legitimately report an earlier start than its parent, and a trace may have no root at all.

**This imposes one constraint on the frontend**: it must keep replacing the trace wholesale per fetch. Anything that merged spans incrementally into an existing view would mix offsets computed against two different baselines.

The whole shape is absorbed at `traceDataFromJSON` in `telemetry-service.ts`, so `SpanData` and every view are unaware the transport changed. Resolved resources are shared by reference rather than copied, so the client does not rebuild the duplication the wire format removes.

### Dev workflow

```
Terminal 1: make dev-go     # Go server on :8000, seeds sample data
Terminal 2: make dev-ts     # Vite on :3001, proxies /rpc → :8000
Browser:    http://localhost:3001
```

Or run production-like: `make build && ./otel-desktop-viewer` (embedded assets, opens browser on :8000).

## Key design decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Storage | DuckDB | Columnar OLAP; fast filters and aggregations on local telemetry |
| Schema | Normalized tables | Query events, links, datapoints independently; avoid UNION/MAP pain |
| Metric identity | `metric_streams` + `metric_ingests` | Dedupe logical streams; preserve per-batch metadata |
| Metric series | `metric_series`, id hashed from `(stream_id, resource_id, attribute_ids)` | Splits replicas that would otherwise interleave into one line; gives a chart line a stable id a URL can name |
| Datapoints | Single table with NULLs | Simpler than per-type tables; columnar NULL compression |
| Attributes | Content-hashed dictionary + `uuid[]` on owners | Dedupes at the atom; ids known before write, so ingest needs no read-back |
| Attribute ids | sha256 truncated to 128 bits | Fits `uuid`; birthday bound is far below the machine's own error rate. Audited by an independent SQL macro rather than trusted |
| Ingest | pdata → DuckDB appenders | No intermediate Go structs |
| API responses | JSON rows from SQL | SQL is the single source of truth for response shape |
| Transport | JSON-RPC over HTTP | One endpoint; typed methods; no REST surface |
| Frontend updates | Polling `getStats` | Simple; sufficient for local dev viewer |
| Span depth | Query-time recursive CTE | Handles orphan spans finding parents in later batches |

## Not implemented (yet)

These appear in older notes or collector capabilities but are **not** part of the current architecture:

- WebSocket push / live tail
- `--config` YAML file exposed on the CLI (inline flag-built config only)
- `batch` processor in default pipelines
- `exporterhelper.WithRetry()` on the desktop exporter

## Related files

**Backend entry and wiring**: `main.go`, `components.go`, `desktopexporter/factory.go`, `desktopexporter/exporter.go`, `desktopexporter/internal/sharedcomponent/`

**Server and API**: `desktopexporter/internal/server/server.go`, `jsonrpc_handler.go`, `errors.go`

**Storage**: `desktopexporter/internal/store/store.go`, `schema/schema.go`, `spans/`, `metrics/`, `logs/`, `search/search_tree.go`

**Frontend**: `desktopexporter/internal/frontend/src/App.svelte`, `pages/`, `services/telemetry-service.ts`, `types/wire-types.ts`, `contexts/`

**Tooling**: `Makefile`, `Dockerfile`, `.goreleaser.yaml`
