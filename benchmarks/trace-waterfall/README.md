# Trace Waterfall Ownership Benchmark

This directory contains the protocol, harness, and eventually the raw results for the trace-waterfall ownership experiment.

The experiment asks where a trace viewer should turn a flat set of spans into the ordered, depth-annotated rows consumed by the waterfall: DuckDB, Go, or the browser.
It is not designed to prove that one layer is universally faster.
It is designed to make the costs and tradeoffs observable under equivalent inputs and output requirements.

The protocol is preregistered in [`experiment.json`](./experiment.json).
Any change made after collecting non-pilot measurements must increment `protocolVersion` and be described alongside the results.

## Current Status

Phase 4 is complete.
The benchmark boundary, positive-control programs, deterministic fixtures, browser semantic oracle, and Arm A/C correctness paths exist.
No paired runner or result in this directory is suitable for a performance claim yet.

The first implementation comparison will be Arm A versus Arm C on the current application schema.
Arm B and alternate-schema work remain disabled until that comparison is correct and reviewable.

## Experimental Axes

### Ownership

| Arm | Query result | Tree owner | Wire result | Status |
| --- | --- | --- | --- | --- |
| A | Current recursive trace query | DuckDB | Current production JSON shape | First comparison |
| B | Flat span rows | Go | Current production JSON shape | Later diagnostic |
| C | Flat span rows | Browser | Neutral flat-row JSON | First comparison |

Arm labels describe ownership only.
Schema alternatives are a separate axis so a schema change is not accidentally credited to a tree-building layer.

### Schema

The first comparison uses the current normalized `otel-desktop-viewer` schema.
Warm selection runs both arms against the same populated store; headline trials give each arm a fresh store populated from the same fixture bytes.
A later comparison may use the schema exposed by `duckdb-otlp`, pinned to a source revision and checksummed extension artifact.
Schema results must report ingestion, selection, storage, and searchability separately.

## Questions

The experiment should answer these questions in order:

1. Do all enabled arms produce the same canonical waterfall structure?
2. For the current schema, what changes when tree construction moves from DuckDB to the browser?
3. Where do query execution, row scanning, JSON encoding, transfer, parsing, tree construction, and rendering spend time and memory?
4. From identical serialized OTLP bytes, how long does each arm take to reach a stable production waterfall?
5. Does an alternate schema materially improve the full result, and what search or operational capabilities does it trade away?

## Non-Goals

- Replacing production code as part of the benchmark.
- Adding benchmark RPC methods to the production dispatcher.
- Treating malformed topology recovery as headline performance data.
- Comparing development-server timings with production bundles.
- Calling one SQL statement one physical pass through the data.
- Publishing a winner when the paired difference is within the declared practical-equivalence margin.

## Fixture Contract

The headline fixture contains 159 spans with level widths `[1, 4, 8, 12, 16, 20, 22, 20, 16, 12, 10, 8, 5, 3, 2]` and a maximum depth of 14, where the root has depth zero.
It includes deterministic resources, scopes, attributes, events, links, status values, and payload sizes representative of a real trace-viewer workload.

Every cross-arm fixture must give siblings and roots within each ordering class unique start timestamps.
The current application does not define a stable final tie-breaker for every equal-time relationship, so equal timestamps would turn an ordering ambiguity into benchmark noise.
Every cross-arm fixture must also give events unique timestamps.
The current schema does not persist a link ordinal, so link arrays are compared as canonical multisets rather than by incidental query order.

Correctness-only fixtures cover a single span, a wide trace, a deep trace, multiple roots, an orphan, and a cycle.
Malformed fixtures exercise recovery behavior but are not included in headline timing aggregates.

The serialized OTLP fixture bytes are immutable experiment inputs.
All arms must decode exactly the same bytes and must record their SHA-256 digest with every run.
The checked `*.otlp.pb` files are protobuf-encoded OTLP `ExportTraceServiceRequest` bodies embedded in the tagged benchmark command.
[`manifest.json`](../../desktopexporter/internal/cmd/waterfallbench/testdata/manifest.json) records each exact filename, digest, byte size, trace ID, span count, displayed count, maximum displayed depth, expected first span ID, and topology classification without a generated timestamp.
`experiment.json` pins the headline fixture name and digest while the manifest remains the source of its detailed metadata.

Regenerate fixture goldens only after a deliberate generator change, then review the source, manifest, and binary summary together:

```sh
cd desktopexporter
go test -tags=waterfallbench ./internal/cmd/waterfallbench -run TestFixtureGoldens -update-fixtures
go test -tags=waterfallbench ./internal/cmd/waterfallbench
```

The golden test builds every fixture twice, atomically replaces each protobuf body, writes the manifest last, compares generated bytes with the checked files, and validates the manifest and decoded OTLP topology.

## Correctness Gate

Performance samples are accepted only after the enabled arms agree on a canonical projection containing:

- trace ID, unplaced span count, and span preorder
- span ID, parent span ID, trace state, flags, kind, preorder index, and depth
- matched, salvaged, and cycle-point state
- start and end timestamps
- service and operation names
- status code and message, plus all span dropped counts
- timestamp-ordered event sequence, including event attributes and dropped counts
- canonical link multiset, including link metadata, attributes, and dropped counts
- canonical resource, scope, and span attributes, including resource and scope dropped counts

The model format is `odv.trace-waterfall.semantic.v1`; RFC 8785 JSON canonicalization and SHA-256 produce `odv.trace-waterfall.semantic.v1+jcs` hashes.
The harness will hash that projection after the measured interval and record the hash with each sample.
Any mismatch invalidates the timing run rather than becoming a performance result.
The projection is built from semantic fields available after production wire rehydration; private storage UUIDs are not part of the oracle.

The browser oracle must also confirm that `WaterfallView` receives the expected number of nodes, that its viewport has non-zero geometry, and that the first visible row belongs to the expected root.

### Arm A Correctness Path

The tagged benchmark command preloads every checked fixture into one fresh in-memory current-schema store, then starts the production HTTP server and JSON-RPC dispatcher with telemetry disabled.
This shared store exists only for correctness coverage and must not be used for fresh-state or warm-selection timing samples.

Run it from the repository root:

```sh
go run -tags=waterfallbench ./desktopexporter/internal/cmd/waterfallbench serve --listen=127.0.0.1:8001
```

The benchmark frontend proxies `/rpc` to that server and calls the production `telemetryAPI.searchSpans` method.
That path preserves the current recursive DuckDB query, DuckDB-owned JSON projection, JSON-RPC envelope, browser JSON parsing, and production wire rehydration before mounting the real `WaterfallView`.
The browser correctness suite exercises all seven fixtures, including production orphan ordering and cycle salvage, and computes the semantic hash only after the registered rendering-stability checks pass.
Arm A hashes are repeatability evidence and future cross-arm comparison inputs, not normative expected values; cross-arm acceptance begins only when Arm C can be compared in the same run.

### Arm C Correctness Path

The same tagged command starts a second benchmark-only listener on `127.0.0.1:8002`.
`POST /benchmark-api/trace-waterfall/flat-rows` queries the populated current-schema store through benchmark-owned non-recursive SQL and returns the closed `odv.trace-waterfall.flat.v1` contract.
The endpoint and SQL live under the tagged benchmark command; they are not registered with the production JSON-RPC dispatcher or production query catalog.

The top-level flat response contains exactly `format`, `traceID`, `resources`, `scopes`, and `rows`.
Resource and scope maps use transport-only decimal keys, and each row refers to them through `resourceRef` and `scopeRef`.
Each row carries the complete semantic span payload with absolute span and event timestamps encoded as unsigned decimal strings.
The row array is non-semantic and contains no preorder, depth, matched, salvage, cycle-point, child, sort-path, storage UUID, fixture expectation, hash, or timing field.

The versioned wire contract is:

```ts
type FlatTraceV1 = {
  format: "odv.trace-waterfall.flat.v1"
  traceID: string
  resources: Record<string, ResourceV1>
  scopes: Record<string, ScopeV1>
  rows: FlatSpanV1[]
}

type FlatSpanV1 = {
  spanID: string
  parentSpanID: string | null
  traceState: string
  flags: number
  name: string
  kind: string
  startTime: string
  endTime: string
  attributes: AttributeV1[]
  events: EventV1[]
  links: LinkV1[]
  resourceRef: string
  scopeRef: string
  droppedAttributesCount: number
  droppedEventsCount: number
  droppedLinksCount: number
  statusCode: string
  statusMessage: string
}

type AttributeV1 = { key: string; type: string; value: string }
type ResourceV1 = {
  attributes: AttributeV1[]
  droppedAttributesCount: number
}
type ScopeV1 = {
  name: string
  version: string
  attributes: AttributeV1[]
  droppedAttributesCount: number
}
type EventV1 = {
  name: string
  timestamp: string
  attributes: AttributeV1[]
  droppedAttributesCount: number
}
type LinkV1 = {
  traceID: string
  spanID: string
  traceState: string
  flags: number
  attributes: AttributeV1[]
  droppedAttributesCount: number
}
```

Every shown field is required, empty collections are `[]`, and an absent parent is `null`.
Unknown fields are rejected recursively so a tree field cannot move back across the browser-ownership boundary unnoticed.

The browser validates that closed wire shape, promotes timestamps directly to `bigint`, resolves resources and scopes, and builds the display tree without depending on incoming row order.
It emits genuine roots before promoted orphans, orders each class and sibling set by start time, walks healthy trees in depth-first preorder, and salvages stranded cycles from the earliest `(startTime, spanID)` entry.
Promoted orphans retain their reported missing parent ID, while salvaged rows receive the same `salvaged` and `cyclePoint` semantics as production Arm A.

Arm A and Arm C use one lifecycle coordinator and the same production `WaterfallView`, font gate, Svelte flush, viewport/root checks, and two animation frames.
The browser correctness suite runs both arms sequentially for every checked fixture and requires same-run equality of semantic hashes, topology, roots, counts, and maximum depth.
Neither arm's hash is stored as a normative golden.
The shared all-fixture store remains correctness-only and must not be used for Phase 5 samples.
The Phase 4 equivalence claim is limited to the registered fixtures: Arm C rejects equal-time healthy roots or siblings because production defines no final tie-breaker, and the checked cycle is intentionally unbranched because production does not totally order branched salvage rows at one depth.
Checked fixtures also keep every relative start offset and duration within the JSON safe-integer range used by Arm A; Arm C's absolute decimal strings do not broaden that Arm A contract.

## Measurement Layers

### Headline: OTLP Bytes to Stable Waterfall

The headline clock starts immediately before decoding the serialized OTLP payload into OpenTelemetry pdata.
It stops when the production `WaterfallView` has received the canonical nodes, fonts are ready, Svelte has flushed, the virtual list has non-zero geometry, and two animation frames have completed.

Each measured A/C pair contains two isolated arm trials, run in the pair order selected by the deterministic shuffle.
Each arm trial starts with its own fresh in-memory store and fresh browser context, then independently decodes and ingests the same fixture bytes.
Ingestion and renderer behavior are identical between ownership arms.

### Causal: Warm Trace Selection

The causal comparison starts with one populated current-schema store and a warm production build.
Arm A and Arm C requests alternate against that same immutable data.
This isolates selection and shaping ownership without repeatedly charging both arms for identical ingestion.

### Diagnostics

Diagnostic clocks may measure OTLP decode, ingest, DuckDB execution, row scanning, Go shaping, JSON encoding, response bytes, browser JSON parsing, browser tree construction, Svelte update, and layout stabilization.
Diagnostic clocks explain the headline result; they must not replace it.

DuckDB query profiles, Go allocation counts, browser performance entries, and heap measurements must be stored as diagnostics rather than silently combined with wall-clock latency.

### Searchability

Searchability is reported as a capability and cost matrix, not as a single score.
The matrix should cover direct field predicates, resource/scope/span attributes, event attributes, link attributes, regex support, type preservation, index use, query complexity, and visibility of newly ingested spans.

## Run Design

Pilot runs verify the harness and estimate variance; they are never included in reported samples.

Warm selection uses five fresh process replicates.
Each process performs ten warm-up requests per arm followed by fifty measured A/C pairs.
Pair order is balanced and deterministically shuffled with the seed in `experiment.json`.

The headline measurement uses thirty fresh-state pairs with no reused browser context or database.
An A/C pair must run on the same machine without another benchmark pair between its members.

The runner uses one Playwright worker, no retries, the lockfile-pinned Chromium build, a production Vite bundle, and localhost transport.
The run records whether the machine is on AC power, but it does not pretend to control scheduler placement or CPU frequency on platforms that do not expose those controls.

Report each arm's median and p95, the median paired `C / A` latency ratio, a bootstrap 95% confidence interval for that ratio, and the raw sample distribution.
Compute the ratio as the exponentiated median of paired log ratios so the result is symmetric on the multiplicative scale.
For warm selection, use a two-level cluster bootstrap that resamples process replicates and then pairs within each selected process.
For fresh-state headline trials, resample whole A/C pairs.
Use the percentile interval from 10,000 deterministic bootstrap draws derived from the order seed.
Call the latency result practically equivalent only when the entire confidence interval lies within `[0.95, 1.05]`; otherwise report it as material or inconclusive.
Call it materially different only when the entire interval is below `0.95` or above `1.05`; every other interval is inconclusive.
Resource and capability differences remain separate engineering considerations.

## Result Records

Raw measurements will be newline-delimited JSON.
Each record must include:

- protocol version, run ID, UTC timestamp, and deterministic order seed
- repository commit and clean-worktree confirmation
- operating system, architecture, CPU, memory, and power source
- Go, Node, npm, DuckDB, extension, and Chromium versions
- fixture name, byte digest, span count, maximum depth, and payload size
- schema, arm, phase, process replicate, pair, iteration, and warm-up flag
- correctness hash and oracle result
- wall-clock timings, diagnostic timings, allocation measurements, and response bytes
- failure, timeout, and exclusion reason when the sample is not accepted

Processed tables and charts must be derivable exclusively from committed raw records and analysis scripts.
The runner must reject non-pilot measurements from a dirty worktree because a boolean dirty flag cannot reproduce the implementation that was measured.

## Shipping Boundary

Benchmark code is intentionally outside every production entrypoint:

- Go harness files live under `desktopexporter/internal/cmd/waterfallbench` and require `//go:build waterfallbench`.
- The benchmark is a separate command; the root application never imports it, even if a global build enables the tag.
- Frontend harness files live under `desktopexporter/internal/frontend/benchmark`.
- The benchmark Vite build writes only to ignored `dist-benchmark` output.
- Production Tailwind source discovery explicitly excludes the benchmark directory.
- Benchmark wrapper geometry uses explicit plain CSS, not benchmark-only Tailwind utilities; production component utilities still come from `src`.
- Production builds copy and embed only `frontend/dist` through `internal/server/static`.
- Benchmark SQL must not be added to `desktopexporter/internal/store/queries`.
- Benchmark endpoints must be owned by the tagged command, not the production JSON-RPC dispatcher.

Run the boundary proof from the repository root:

```sh
./benchmarks/trace-waterfall/scripts/verify-production-exclusion.sh
```

The proof builds fresh temporary production and benchmark artifacts, checks the negative and positive controls, and leaves the committed production bundle untouched.

## Phase Gates

1. Protocol, isolated skeletons, and production-exclusion proof.
2. Deterministic OTLP fixture generator and canonical correctness oracle.
3. Arm A through the current production query and wire path.
4. Arm C through a benchmark-only flat query and browser tree builder.
5. Paired runner, raw records, analysis, and repeatability check.
6. Optional Go-owned Arm B.
7. Alternate-schema ingestion, equivalent arms, and searchability matrix.
8. Article claims derived from committed results.

Each phase stops for review before the next phase changes the experiment surface.
