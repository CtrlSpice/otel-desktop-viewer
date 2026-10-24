# Evaluation data ingestion

Evals requires `make build EVALS=1`; public builds omit its UI and HTTP handlers.
Within that build, `evals.enabled` remains off by default. The global file drop/picker uses the
existing serial queue, cancellation and failure UI. No receiver or port is added.
Input files are never modified, moved, or deleted.

## Framing and supported producers

`json-file.ts` extracts the existing `@streamparser/json` streaming syntax pass
from `otlp-file.ts`. Both use `paths: []`, `keepStack: false`, and newline-separated
top-level objects. UTF-8 decoding is fatal. Detection uses content, not extensions.
Detection finishes before dispatch; OTLP then uses its existing byte-range scan
and 20 MiB Blob batches, receiver configuration, cancellation and partial errors.
This currently means two streaming passes for OTLP, not one combined scan.

The eval endpoint accepts:

- A full Promptfoo export: nonempty string `evalId`, object `config`, object
  `results` with `version: 3` and a `results` array of result objects.
- JSONL containing **complete independent Promptfoo envelopes**. Each
  envelope retains its own context, row ordering and dataset identity. Newlines
  separate envelopes; pretty-printed single JSON is also supported.

This is supported envelope framing, **not a claim that Promptfoo's native
row-only JSONL export contains full experiment context**. The checked-in adapter evidence
does not establish a canonical experiment-metadata association for bare-row JSONL.
Bare rows are rejected. Supporting those requires producer evidence or an explicit
manifest/row association contract; no experiment, repetition, score maximum, timing,
grader identity, or relationship is inferred. Non-Promptfoo envelopes,
OTLP/eval mixtures, duplicate decoded JSON keys, malformed UTF-8, and unpaired
Unicode surrogate escapes are rejected.

Rows are received source objects, not a claim that every field has a display
adapter. Envelope recognition is authoritative on the backend. Unknown row and
metadata fields are retained. Existing display adapters can reject unsupported
field values without changing the stored data.

## HTTP API and storage

`POST /api/evals/ingest?name=<filename>` accepts the raw Blob body on the existing
viewer origin. No JSON-RPC escaping or whole-file parsing is used. A disabled
feature returns 403 with an explicit message. Successful replies contain
`{ids: string[], created: boolean, complete: true}`. The filename is provenance,
not a schema discriminator; it is at most 256 UTF-8 bytes.

The backend stages bounded, compact JSON records in a viewer-owned plaintext
temporary file. It validates the entire input before submitting telemetry. Staging is removed when
the request returns. A process crash may leave an OS temporary file; it is not a
retained artifact or part of persistence. Only the DuckDB file is needed on reopen.

The ingestion path is:

```
file upload → evaldecode → OTLP HTTP receiver → redaction → batch
→ desktop exporter → retained logs → Evals caches/indexes
```

`desktopexporter/internal/evaldecode` owns producer parsing and historical pdata
conversion. The server owns staging, forwarding, cancellation, and persistence
confirmation. Native OTLP bypasses the producer decoder. No receiver or port is
added, and upload data is never written directly to persistent Evals storage.
The internal HTTP hop uses a server-owned reusable client with proxying disabled,
including for non-loopback bind addresses. Environment proxy settings cannot
redirect pre-redaction observations; HTTP redirects are refused.

The following tables are derived from retained, processed log evidence:

- `eval_imports`: import identity and completion marker.
- `eval_datasets`: one envelope's metadata/context, result count, source filename,
  format, local identity and deletion marker. Context retains the source envelope
  with its result array replaced by `[]`.
- `eval_records`: ordered result objects, keyed by import/envelope/ordinal, with
  byte sizes and native JSON payloads.
- `eval_experiments` / `eval_experiment_rows`: rebuildable Experiment summaries and source-row mapping.
- `eval_experiment_search`: rebuildable text extracted from every JSON string value in
  each result row. Object keys and non-string scalars are not copied into search.
- `eval_evidence`: physical retained-log references and their transfer identity.
- `eval_parts`: logical source coordinates and processed-content fingerprints.
- `eval_transfers`: rebuild state and an incomplete-transfer reason.

Each imported envelope produces a context log and ordered result logs. A final
manifest log declares the envelope count and import name. These are historical
observations under service `otel-desktop-viewer` and instrumentation scope
`otel-desktop-viewer.evaldecode`, version `1`. Event name is
`viewer.eval.import.v1`. Their observed timestamp records conversion time;
their event timestamp, trace ID and span ID are unset. Source timing and explicit
correlation remain in the source body. Conversion creates no execution/grader
spans, causal parents, aggregate metrics, or error status from failed judgments.

The versioned body contains `version`, `producer`, `importID`, `envelope`,
`ordinal`, `count`, `source`, `numberPaths`, and `numberPathsHash`. Coordinates
and counts are canonical decimal strings. Context uses ordinal `-1`; results
use nonnegative ordinals; the manifest uses envelope `-1` and ordinal `-2`.
`producer` currently identifies `promptfoo-v3`.

Source numbers travel as exact string lexemes at their original structured
locations. `numberPaths` contains only type/path metadata: a sparse tree with
`true` at numeric leaves, positional arrays, and object edges encoded as `k`
plus unpadded base64url UTF-8 keys. This avoids duplicating secret values and
repeating long shared paths. Its SHA-256 detects processor changes to that
metadata; it is an integrity check, not producer authentication.

Reconstruction validates the encoding, numeric paths, bounds and numeric text.
It restores numbers only from retained leaves. A masked `****` value or subtree
stays masked; metadata never restores its contents. Unknown versions, damaged
metadata and incompatible processed structures prevent completion. Embedded JSON
strings remain strings, including when redaction makes their contents invalid JSON.

Exact numeric lexemes, Unicode meaning, unknown fields, source IDs,
missing/null/zero/false distinctions, array order and producer context survive,
except for deliberate Collector processing such as redaction.
Whitespace, object formatting and escape spelling are not a download guarantee.
Local dataset IDs identify storage, never replace received experiment/sample IDs.
The local import identity hashes the compact staging representation; it is not
a claim of semantic equivalence across reordered fields or different escapes.

The server forwards one bounded observation per OTLP protobuf request. Receiver
HTTP 200 is not persistence confirmation. Completion requires retained context
and every distinct expected result ordinal, a valid manifest, and a ready or empty
experiment index. Missing, invalid, conflicting or unsupported evidence stays
hidden from completed Evals. Identical physical duplicates count once; conflicting
content at one logical identity is not silently resolved. Ordinary logs retain
their existing append-only behavior, so retries can leave duplicate physical logs.
Before publication, the write-locked check also examines relevant retained logs
that discovery has not registered yet. Late arrivals trigger rediscovery before
completion, including benign duplicates and conflicts.

Reconciliation reads one retained log at a time, outside cache-write windows.
Cache cleanup and experiment projection use bounded batches. Upload, validation,
staging reads and forwarding never hold the database write lock. Explicit Evals
reads also reconcile late evidence and rebuild indexes; arrival does not navigate
or automatically refresh the UI. Large rebuilds can take a long request and remain
request-cancellable; there is no background indexing scheduler.

Cancellation stops further submission and waiting. It cannot retract telemetry
already accepted by the Collector. A fully submitted transfer can complete on a
later explicit read. Waiting for missing evidence has a one-minute timer; a large
reconciliation itself can take longer. An unconfirmed request can be retried with
the same file. Retention and log deletion invalidate affected completion and
remove source/search caches in the same transaction as evidence deletion.
Cancellation or cleanup failure rolls back both changes. Reconciliation can rebuild only from evidence still
retained. Existing dataset deletion markers survive cache rebuilds; explicit
reimport of a marked dataset uses a fresh deterministic generation.

Identical completed staging content is idempotent and retains first-import names.
Simultaneous identical imports receive 409 while one is active. If the response
is lost after publication, retry returns the completed IDs with `created:false`.

Limits are **per value/context**, not a total-file limit: each result object and
each opaque metadata value is at most 4 MiB; assembled envelope metadata is at
most 4 MiB. Nested opaque values are limited to 64 levels. A file contains at most
1,000 envelopes. Browser tokenizer memory scales with the largest token, and OTLP
range plans scale with record count; this is not an O(1)-memory claim. Backend
staging uses disk proportional to input, bounded value/batch buffers, and DuckDB's
own buffers. The upload uses one streamed Blob HTTP request, not resumable chunks.
Its body deadline is cancellation-bound rather than the ordinary 30-second server
deadline. No telemetry responsiveness benchmark has been performed.

Re-escaping processed JSON strings can expand their serialized representation;
reconstruction permits up to 24 MiB per part without increasing the 4 MiB input
limit. Page readers normally stop at 4 MiB of row bytes but permit one larger
reconstructed row. Context is likewise subject to the reconstruction bound.

The retained encoding supports local rebuilding and is tested through existing
single-log OTLP serialization. There is no Evals archive export API/UI or
end-to-end Evals export/reimport feature.

## Producer adapters and UI integration

The Evals workspace reads native ingested datasets only. The global drop/file
picker is the supported import path. Promptfoo v3 is the first producer adapter;
it is not the canonical definition of every evaluation system. Additional
open-source producers require producer-native readers over the same retained
source/index boundary.

The normalized presentation distinguishes:

- **Experiment**: the top-level evaluation group.
- **Test case**: one logical sample, question or test item.
- **Result**: one recorded outcome for a test case. Its native result ID
  identifies it; a repetition number is not required.
- **Attempt**: an explicit retry within one Result, only when the producer records it.

Promptfoo expands `testIdx` across repetitions. The adapter groups complete native
test definitions and resolved variables within each provider/prompt group; equal
question text alone is insufficient. Each result retains its native `id`.
Result rows have no displayed ordinal. Available start times determine their default
order, with stable source order for ties or missing timing. A singleton without
recorded repetition metadata displays its evidence directly on its test case.

Execution and grading are roles, not required wrapper rows. A distinct execution
span can appear when explicitly referenced; assertion results remain assessments
on the result, with grader rows only when telemetry is recorded. Native scores,
pass/fail, reasons and `componentResults` are displayed without assuming a numeric
maximum. Missing tool evidence is unknown, not zero calls.

Attempt nodes require explicit retry evidence. Native bare-row producer JSONL
remains an explicit contract/evidence gap.

## DB-wide experiment summaries and selected-experiment source pages

The new experiment APIs supersede the dataset-list approach for the parent sidebar:

```
GET /api/evals/experiments?q=experiment+name&sort=started&direction=desc&limit=50&offset=0
GET /api/evals/experiments?outcome=success%3APass&outcome=error%3AFail
GET /api/evals/experiments/{experimentID}/rows?after=-1&limit=100
```

Typed methods and DTOs live in `telemetry-service.ts` and the new
`src/services/eval-experiment-types.ts`: `searchEvalExperiments` and `getEvalExperimentPage`.

`eval_experiments` holds precomputed whole-experiment counts. `eval_experiment_rows` maps global source
ordinals to an experiment without copying newly ingested result bodies. `eval_experiment_search`
stores materialized string values per source row. `eval_experiment_indexes` records
completion/unsupported/empty status and processed source context. Retained logs remain
authoritative; all four are rebuildable indexes. New imports build the index before
their publication marker, with projection outside the write lock and at most 64
projected rows per write transaction. A malformed display-semantic row marks that
dataset's index unsupported; the import remains incomplete and its processed
evidence remains in retained logs. The upload reports this as HTTP 422. Unsupported indexes never
expose partial experiment counts.

Collector-backed Evals uses schema version 24. Older databases are rejected;
there is no migration or parallel legacy table set. Use a new database and
reimport retained source exports. Experiment IDs remain stable for the same
imported data. The UI accepts old `run`/`rep` selection links and replaces their
keys with `experiment`/`result`; new API responses use `experiments`,
`adapterExperimentID`, and `resultCount`.

Grouping matches the adapter tuple:

```
[datasetID, "run", evaluationID, providerID, providerName, model,
 promptID, promptIndex, missingProviderOrPrompt ? globalOrdinal : null]
```

`id` is a stable local SHA-256 identifier over that tuple with a version prefix;
the opaque `"run"` namespace is retained so existing experiment IDs remain stable.
`adapterExperimentID` is the corresponding adapter tuple string. Provider/prompt variants
remain distinct. Missing identities isolate source rows instead of inferring
cross-row membership. Name and prompt-label display use the first source row in
the group. Source identifiers and global source ordinals remain available in the
source records. Names above 32,768 UTF-8 bytes and grouping identities above 8,192
bytes produce explicit unsupported-index reasons instead of unbounded summaries.

The summary returns `resultCount`, `outcomes`, `assessmentName`, name,
evaluation, source, format, dataset locator, and recorded start timestamp where
the adapter recognizes one. `plannedCount:null`, `state:"Unknown"`,
`pauseReason:null`, and `stateReason` explicitly avoid inferring a completed experiment
or planned-slot coverage from saved rows. The parent must not turn unknown planned
counts into zero or treat received Result counts as declared planned counts.

Outcome counts preserve received Promptfoo pass/fail labels without inventing a
numeric maximum. Failure reason 2 and native assertion `metadata.graderError`
map to the existing Internal fault category;
otherwise reason 1 / explicit grading failure wins over success. Other numeric
scores do not establish a scale. Decimal equality is
checked exactly without float rounding or evaluating unbounded exponents.
Counts cover every stored row in the experiment, never just the current detail page.
This intentionally fixes the old card helper's loss of Promptfoo pass/fail-only
labels into its Unscored category; parent should consume these precomputed parts.

Search splits plain whitespace-separated text into case-insensitive terms. Each
term can occur in Experiment name/evaluation/source or any materialized JSON string value
from that Experiment's result rows. This includes questions, answers, errors, grader
explanations, tool output, IDs and unknown string fields without assigning those
fields new semantics. Object keys, numbers and booleans are not search text. It is
not a telemetry query parser. Outcome choices are ORed. `total` is the matching indexed experiment count across
the DB, and `hasMore` describes the requested page. `facets` counts each experiment once
per category after text search but before outcome filtering. Started sorting uses
recorded timestamps, unknown dates last. Name sorting is case-insensitive with
natural ASCII digit sequences; stable IDs break ties. Server order is authoritative
and is not a locale-specific browser collation promise.

### Native dataset indexing

Explicit list/detail requests first reconcile newly retained transfer evidence.
This can rebuild every envelope in an affected transfer. List requests then repair
at most one additional unindexed dataset cache. The reply includes
`indexing.remaining`, `unsupported`, `empty`, and up to 25
dataset issue records (`hasMoreIssues` distinguishes a truncated issue list).
While `remaining > 0`, totals/facets cover indexed sources and are not a claim that
backfill is complete. Parent should continue requesting lists while showing that
state; unsupported and empty sources remain explicit, not phantom zero-result
experiments. Cancellation leaves a hidden pending index; the next attempt clears partial
derived rows in bounded batches before rebuilding. A cancellable server gate
serializes evidence rebuilds and index repairs. No startup scan or service restart is
required. Very large previously unindexed *new-style* datasets can take a long
request despite bounded buffers; no time-bounded background job scheduler exists.

### Selected-experiment integration

`getEvalExperimentPage` always returns the selected experiment's **full summary**, independent
of current sidebar filters, plus:

```
{
  summary,
  contextJSON: "{...producer envelope, result array replaced by []...}",
  rows: [{ ordinal: 7, json: "{...exact source result...}" }],
  nextAfter: 7,
  hasMore: true
}
```

Use `nextAfter` for the next request. It is a global source ordinal, not a
experiment-relative offset. Pages return only the selected experiment's members, at most 100
rows, normally stopping at 4 MiB of source row bytes, plus context. One larger
reconstructed row and context can each reach the 24 MiB reconstruction bound.
The typed JSON strings avoid
precision loss in ordinary `response.json()` calls. This is bounded response
serialization, not the old whole-file escaped upload path. Context can contain
dataset-wide producer stats; use `summary` for selected-experiment counts.

`projectPromptfooDataset` accepts optional `rowOrdinals`. Parent can assemble just
the selected experiment with `assembleEvalExperimentSource(pages)` from `eval-experiment-source.ts`.
This helper rejects mixed, incomplete, duplicate-ordinal or oversized page sets
and replaces the source result-array syntax range without parsing/re-encoding
received values. It returns the exact `EvalSourceDataset` input for the adapter.
Check `summary.resultCount` before fetching when using the bounded adapter.
Alternatively, parent can assemble
the selected experiment's rows into its source envelope, retain their exact strings,
and pass those global ordinals with `id: summary.datasetID` and
`format: summary.format`. Copy pointers address the assembled local payload while
experiment/result/case identities use global positions. Aggregate selected-experiment pages
before using the current adapter's duplicate-slot logic; independent projections
of pages cannot establish duplicate-slot relationships across pages. The existing
8 MiB / 5,000-row / safe-display-number adapter limits still apply. The API does
not require downloading other experiments to select one.

The workspace consumes DB-wide summaries, pages the sidebar, maps unknown lifecycle
state honestly, and lazily fetches only the selected Experiment's rows.
