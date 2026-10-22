# Evaluation data ingestion

`evals.enabled` remains off by default. The global file drop/picker uses the
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
row-only JSONL export contains full run context**. The checked-in adapter evidence
does not establish a canonical run-metadata association for bare-row JSONL.
Bare rows are rejected. Supporting those requires producer evidence or an explicit
manifest/row association contract; no run, repetition, score maximum, timing,
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

The backend stages bounded, compact JSON records in a viewer-owned temporary file.
It validates the entire input before writing to DuckDB. Staging is removed when
the request returns. A process crash may leave an OS temporary file; it is not a
retained artifact or part of persistence. Only the DuckDB file is needed on reopen.

Additive tables:

- `eval_imports`: import identity and completion marker.
- `eval_datasets`: one envelope's metadata/context, result count, source filename,
  format, local identity and deletion marker. Context retains the source envelope
  with its result array replaced by `[]`.
- `eval_records`: ordered result objects, keyed by import/envelope/ordinal, with
  byte sizes and native JSON payloads.
- `eval_runs` / `eval_run_rows`: rebuildable Run summaries and source-row mapping.
- `eval_run_search`: rebuildable text extracted from every JSON string value in
  each result row. Object keys and non-string scalars are not copied into search.

Raw JSON is validated/compacted without float64 decoding and cast directly to
DuckDB JSON. Exact numeric lexemes, Unicode meaning, unknown fields, source IDs,
missing/null/zero/false distinctions, array order and producer context survive.
Whitespace, object formatting and escape spelling are not a download guarantee.
Local dataset IDs identify storage, never replace received run/sample IDs.
The local import identity hashes the compact staging representation; it is not
a claim of semantic equivalence across reordered fields or different escapes.

The server writes at most 64 staging parts per transaction, stopping after the
batch reaches 4 MiB (one final part can take it below 8 MiB). It releases
`Store.WithDBWrite` between transactions. Upload, validation, staging reads and
network output never run under the write lock. Publication is one final marker
update; search/read APIs exclude all pending rows. Cancellation before publication
leaves no visible partial dataset. Cleanup removes rows in groups of 64 with a
30-second cleanup context. If cleanup is interrupted, pending data stays hidden;
retrying the same input removes the interrupted copy before restarting.

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

## Producer adapters and UI integration

The Evals workspace reads native ingested datasets only. The global drop/file
picker is the supported import path. Promptfoo v3 is the first producer adapter;
it is not the canonical definition of every evaluation system. Additional
open-source producers require producer-native readers over the same retained
source/index boundary.

The normalized presentation distinguishes:

- **Experiment**: the top-level evaluation group (existing APIs use `run`).
- **Test case**: one logical sample, question or test item.
- **Result**: one recorded outcome for a test case. Its native result ID
  identifies it; a repetition number is not required.
- **Attempt**: an explicit retry within one Rep, only when the producer records it.

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

## DB-wide run summaries and selected-run source pages

The new run APIs supersede the dataset-list approach for the parent sidebar:

```
GET /api/evals/runs?q=run+name&sort=started&direction=desc&limit=50&offset=0
GET /api/evals/runs?outcome=success%3APass&outcome=error%3AFail
GET /api/evals/runs/{runID}/rows?after=-1&limit=100
```

Typed methods and DTOs live in `telemetry-service.ts` and the new
`src/services/eval-run-types.ts`: `searchEvalRuns` and `getEvalRunPage`.

`eval_runs` holds precomputed whole-run counts. `eval_run_rows` maps global source
ordinals to a run without copying newly ingested result bodies. `eval_run_search`
stores materialized string values per source row. `eval_run_indexes` records
completion/unsupported/empty status and source context. The source remains
authoritative; all four are rebuildable indexes. New imports build the index before
their publication marker, with projection outside the write lock and at most 64
projected rows per write transaction. A malformed display-semantic row marks that
dataset's index unsupported; source ingestion itself can still succeed and the
received data remains retained in source tables. Unsupported indexes never
expose partial run counts.

Grouping matches the adapter tuple:

```
[datasetID, "run", evaluationID, providerID, providerName, model,
 promptID, promptIndex, missingProviderOrPrompt ? globalOrdinal : null]
```

`id` is a stable local SHA-256 identifier over that tuple with a version prefix;
`adapterRunID` is the corresponding adapter tuple string. Provider/prompt variants
remain distinct. Missing identities isolate source rows instead of inferring
cross-row membership. Name and prompt-label display use the first source row in
the group. Source identifiers and global source ordinals remain available in the
source records. Names above 32,768 UTF-8 bytes and grouping identities above 8,192
bytes produce explicit unsupported-index reasons instead of unbounded summaries.

The summary returns `repCount`, `outcomes`, `assessmentName`, name,
evaluation, source, format, dataset locator, and recorded start timestamp where
the adapter recognizes one. `plannedCount:null`, `state:"Unknown"`,
`pauseReason:null`, and `stateReason` explicitly avoid inferring a completed run
or planned-slot coverage from saved rows. The parent must not turn unknown planned
counts into zero or treat received Rep counts as declared planned counts.

Outcome counts preserve received Promptfoo pass/fail labels without inventing a
numeric maximum. Failure reason 2 and native assertion `metadata.graderError`
map to the existing Internal fault category;
otherwise reason 1 / explicit grading failure wins over success. Other numeric
scores do not establish a scale. Decimal equality is
checked exactly without float rounding or evaluating unbounded exponents.
Counts cover every stored row in the run, never just the current detail page.
This intentionally fixes the old card helper's loss of Promptfoo pass/fail-only
labels into its Unscored category; parent should consume these precomputed parts.

Search splits plain whitespace-separated text into case-insensitive terms. Each
term can occur in Run name/evaluation/source or any materialized JSON string value
from that Run's result rows. This includes questions, answers, errors, grader
explanations, tool output, IDs and unknown string fields without assigning those
fields new semantics. Object keys, numbers and booleans are not search text. It is
not a telemetry query parser. Outcome choices are ORed. `total` is the matching indexed run count across
the DB, and `hasMore` describes the requested page. `facets` counts each run once
per category after text search but before outcome filtering. Started sorting uses
recorded timestamps, unknown dates last. Name sorting is case-insensitive with
natural ASCII digit sequences; stable IDs break ties. Server order is authoritative
and is not a locale-specific browser collation promise.

### Native dataset indexing

Each list request indexes at most one unindexed native dataset. The reply includes
`indexing.remaining`, `unsupported`, `empty`, and up to 25
dataset issue records (`hasMoreIssues` distinguishes a truncated issue list).
While `remaining > 0`, totals/facets cover indexed sources and are not a claim that
backfill is complete. Parent should continue requesting lists while showing that
state; unsupported and empty sources remain explicit, not phantom zero-result
runs. Cancellation leaves a hidden pending index; the next attempt clears partial
derived rows in bounded batches before rebuilding. A per-dataset server guard
prevents concurrent duplicate backfills. No startup scan or service restart is
required. Very large previously unindexed *new-style* datasets can take a long
request despite bounded buffers; no time-bounded background job scheduler exists.

### Selected-run integration

`getEvalRunPage` always returns the selected run's **full summary**, independent
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
run-relative offset. Pages return only the selected run's members, at most 100
rows and 4 MiB of source row bytes plus 4 MiB context. The typed JSON strings avoid
precision loss in ordinary `response.json()` calls. This is bounded response
serialization, not the old whole-file escaped upload path. Context can contain
dataset-wide producer stats; use `summary` for selected-run counts.

`projectPromptfooDataset` accepts optional `rowOrdinals`. Parent can assemble just
the selected run with `assembleEvalRunSource(pages)` from `eval-run-source.ts`.
This helper rejects mixed, incomplete, duplicate-ordinal or oversized page sets
and replaces the source result-array syntax range without parsing/re-encoding
received values. It returns the exact `EvalSourceDataset` input for the adapter.
Check `summary.repCount` before fetching when using the bounded adapter.
Alternatively, parent can assemble
the selected run's rows into its source envelope, retain their exact strings,
and pass those global ordinals with `id: summary.datasetID` and
`format: summary.format`. Copy pointers address the assembled local payload while
run/rep/case identities use global positions. Aggregate selected-run pages
before using the current adapter's duplicate-slot logic; independent projections
of pages cannot establish duplicate-slot relationships across pages. The existing
8 MiB / 5,000-row / safe-display-number adapter limits still apply. Numeric-safe,
arbitrarily large result-table projection remains parent/future work; the API does
not require downloading other runs to select one.

The workspace consumes DB-wide summaries, pages the sidebar, maps unknown lifecycle
state honestly, and lazily fetches only the selected Run's rows.
