# Offline converter probes

`offline-converters.ts` contains pure functions over caller-supplied, already
parsed JSON objects. It performs no file reads, network requests, model calls,
path resolution, storage writes or execution. Run tests with:

```sh
node --test offline-converters.test.ts
```

This is **not** the viewer import API, persistence model, or a promise of complete
framework support. The return types are source-native snapshots and narrow
projections, not a common interchange schema. No durable identities are invented.

## Supported inputs

- `decodePromptfoo`: full structured export with nonempty `evalId`, object
  `config`, `results.version === 3`, and `results.results` row array. Validates
  these envelope fields and selected row discriminators, not the entire vendor
  schema. Retains all supplied JSON fields including assertion trees, weights,
  named scores, grader tokens, config, evidence, source IDs and extensions.
  Only explicit `failureReason: 2` projects to execution-error; `1` projects to
  assessment-failed. Other/missing discriminators remain unspecified: neither
  `error` prose nor zero score is sufficient to classify an execution failure.
  No numeric scale, aggregation formula, currency or telemetry IDs are inferred.
- `decodeKit`: explicit object `{ manifest, pilot, ... }`, with manifest models
  array and `pilot.results.results` rows, matching the existing kit wrapper.
  Summary version can be absent (multi-batch form) or 3. Optional supplied
  `modelSummary`, `graderSummary`, `pricing`, review-history/evidence, deferred-slot
  or raw batch fields are retained unchanged. Optional artifacts are not
  discovered or reconstructed. `retryOf`, slot indices, session sets, original
  grades and evidence remain source data; no cohort merge, recovery substitution,
  deduplication or global identity is inferred.
- `projectKitReport(decoded, validatedReviews)`: explicitly **lossy allowlisted
  display projection**, delegating to the unchanged report `aggregate` function.
  It uses optional `modelSummary`, `graderSummary`, `pricing` supplied above.
  Reviews are the existing `ReviewedAttempt[]` projection from a trusted,
  previously validated review reader, not raw human-review records. This pure
  function cannot verify signatures, files, history hashes or signoff itself.
  Retain raw review history separately in the input snapshot when needed.
  Original and signed scores stay separate; report means retain kit plan rules.
  Display order is not planned-slot identity. Unknown usage/cost stays unknown.
- `decodeInspectProbe`: version-2 JSON object with `eval` object and explicit
  `samples` array; sample IDs are strings or safe integers. Supplied named scores
  must be objects with a `value` member. Preserves finite JSON score values,
  including categorical strings, booleans, arrays, maps and null, plus explanations,
  history and other source fields. Missing scores remain missing, not empty
  assessments or zeros. It does not derive pass/fail, reducers, or OTel references.

All decoders return detached snapshots. Unknown JSON fields are retained, **not
safe for reader-facing rendering or sharing**: they may contain private content.
Only the explicit kit report projection has the existing report privacy policy.
There is no serialization, attachment resolution or completeness assertion.

## Unsupported inputs and limits

No file/container autodetection: JSON text, row-only JSONL, CSV, YAML, SQLite,
JUnit, full XML, `.eval` ZIP archives, live events and other schema versions are
unsupported. Inspect headers without samples, condensed event resolution and
NaN/Inf logs need a separate reader. References stay opaque: attachment bytes,
referenced paths/URLs and telemetry availability are not verified or fetched.

The boundary accepts ordinary finite JSON data only. Undefined, bigint, cycles,
non-JSON objects, nonfinite numbers and unsafe integer numbers are rejected,
never silently replaced. Numbers retain the supplied binary64 value; these
functions cannot recover precision already lost by a caller's JSON parser.
Large exact integers/decimal amounts should remain source strings if the source
uses strings; this adapter does not convert numeric fields to invented strings.
Missing properties and explicit null/zero remain distinct. The snapshot is not
an archive of original text/byte formatting, duplicate JSON keys or numeric lexemes.

## Fixture provenance

Tests construct tiny synthetic records in memory, **not real model outputs**.
They follow the shapes established by these pinned sources without copying a
large fixture or accessing private saved results:

- Kit `native-evaluate.ts:78-99`, `run-overnight.ts:30-48`, existing
  `presentation/aggregate.test.ts`, and `presentation/telemetry-links.ts`.
  The kit pins Promptfoo 0.124.1; this is not assumed identical to current upstream.
- `promptfoo/promptfoo@ea8b3d073a63769f7be243ac3c3460f72881a930`:
  `test/__fixtures__/sample-export.json`, `src/types/index.ts`,
  `src/types/results.ts`, `src/util/output.ts`. The public fixture uses v3
  `results.results`; assertion error prose and nested grading are retained.
- `UKGovernmentBEIS/inspect_ai@a699376315c95b6410f5aec01af37c8dec452aea`:
  `tests/log/test_eval_log/log_formats.json` (`match.value: "I"`, answer `"No"`),
  `src/inspect_ai/log/_log.py`, `src/inspect_ai/scorer/_metric.py`;
  structured/history variants are synthetic compatibility probes inspired by
  `tests/scorer/test_score_editing.py` and `test_multiscorer.py`, not copied logs.

The tests validate retention and these narrow projections, not full upstream
schema conformance or reproduction of the private completed cohort.
