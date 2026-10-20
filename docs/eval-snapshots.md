# Evals JSON snapshots (experimental)

The archive stores original result files in the viewer's existing DuckDB database.
Enable `evals.enabled` through the existing feature-flag configuration. The UI
capability endpoint remains `GET /api/features`. This archive does not run an
evaluator, load plugins, fetch URLs, open attachment paths, or attach results to
telemetry. Source references and unknown fields remain inert JSON content.

## JSON-RPC contract

Send JSON-RPC 2.0 requests to `POST /rpc`. Named and positional parameters have
the same validation. Unknown named parameters fail. `/openrpc.json` and
`rpc.discover` describe these methods even when the feature is disabled; the
four methods return `-32601` while disabled, before store access.

| Method | Positional parameters, in order | Result |
| --- | --- | --- |
| `importEvalSnapshot` | `source`, `format`, `name`, `originalJSON` | `{snapshot: Metadata, created: boolean}` |
| `searchEvalSnapshots` | `limit?`, `offset?` | `{snapshots: Metadata[], hasMore: boolean}` |
| `getEvalSnapshot` | `id` | Metadata fields plus `originalJSON: string` |
| `deleteEvalSnapshot` | `id` | `{id: string, deleted: true}` |

`Metadata` contains:

```ts
{
  id: string;          // 64 lowercase hex characters: SHA-256 of original bytes
  digest: string;      // same SHA-256, without a prefix
  source: string;      // caller-supplied source label
  format: 'promptfoo-v3' | 'viewer-kit' | 'unknown-json';
  name: string;        // caller-supplied display name
  importedAt: string;  // server import time, UTC RFC 3339
  byteSize: number;    // UTF-8 byte count, not JavaScript string length
}
```

All four import parameters are required strings. `source` is 1–1024 UTF-8
bytes; `name` is 1–256 UTF-8 bytes. Neither permits NUL. The format is explicit
and receives no format-specific semantic validation: even an unfamiliar JSON
shape can be archived with one of these labels. There is no format detection
or adaptation in the generic store.

`originalJSON` is the original file text, **not** a parsed object or the output
of `JSON.stringify(JSON.parse(file))`. It must be valid UTF-8 JSON accepted by
Go's JSON validator and DuckDB's JSON type. Invalid UTF-8, a BOM, multiple
top-level documents, and lone Unicode surrogate escapes are rejected. Objects,
arrays and JSON scalars are accepted. Unknown fields and duplicate object keys
are retained.

Import parameters are checked for paired Unicode surrogate escapes in their
raw RPC strings, before named-parameter normalization or string decoding.
Unpaired escapes in those parameter strings return `-32602`; they are never repaired
to U+FFFD and archived. Literal/escaped U+FFFD and valid surrogate pairs remain
accepted. Escaped backslashes retain their normal meaning, so literal text
such as `\\ud800` inside an archived JSON string is preserved.

Each payload is at most **16 MiB (16,777,216 bytes)**. With Evals enabled, the
complete RPC request body is at most **20 MiB**, including metadata and JSON
string escaping. Both bounds apply: unusually escape-heavy files can reach
the envelope bound before the payload bound. With Evals disabled, the existing
1 MiB RPC body limit applies. No new external dependencies are required.

An exact repeat returns `created: false` with the original metadata. A different
source, name or format on that repeat does not update the archived artifact.
Any byte change, including whitespace, produces a separate content ID. After
explicit deletion, reimport creates an artifact with that same content ID and
new import metadata.

Search defaults to `limit: 50`, `offset: 0`. Limits are 1–100 and offsets are
0–1,000,000; out-of-range values fail rather than being silently clamped.
Whole-number JSON tokens or decimal strings are accepted. Omitted/null values
use defaults. Results are ordered by `importedAt` descending, then `id`
ascending. One look-ahead row supplies `hasMore`. An exhausted page returns
`snapshots: []`. Pages contain no payloads and no unbounded total/count query;
separate requests see current archive state.

Errors follow the existing taxonomy:

- `-32602`: invalid metadata, format, JSON, payload size, ID or page bounds.
- `-32011`: a valid ID was not found (including repeated deletion).
- `-32010`: request canceled or deadline exceeded.
- `-32603`: unexpected store failure.
- `-32600`: RPC request body exceeds the transport bound.
- `-32700`: malformed RPC JSON or invalid UTF-8 in the request body.

Example named import:

```json
{"jsonrpc":"2.0","id":1,"method":"importEvalSnapshot","params":{"source":"local-file","format":"unknown-json","name":"results.json","originalJSON":" {\"score\":1.00,\"future\":true}\n"}}
```

## Exact JSON download

`GET /api/evals/snapshots/{id}/download` returns the original UTF-8 bytes with
`Content-Type: application/json`, an attachment filename based on the content
ID, and `Cache-Control: no-store`. It supports HEAD and byte ranges through
the existing `http.ServeContent` pattern. It accepts no query parameters.

Disabled Evals returns HTTP 404. With Evals enabled, malformed IDs return 400,
missing IDs return 404, and unexpected store failures return 500. Download
and RPC get each materialize at most one bounded payload.

## Storage, compatibility and retention

`eval_snapshots` has a SHA-256 `VARCHAR` primary key, source/format/name labels,
`TIMESTAMP imported_at`, `BIGINT byte_size`, and native **`JSON payload`**. The
digest is the primary key, so a second digest column is unnecessary. There is
an additional index on import time. The store uses its existing `WithDBWrite`
boundary for import/deletion and `WithDBRead` for reads, with request contexts
passed to DuckDB operations.

The payload is inserted with `CAST(? AS JSON)` and read with
`CAST(payload AS VARCHAR)`. No `json()` normalization, object extraction or
JSON reconstruction occurs in the archive path. File-backed tests prove this
round trip preserves original bytes across close/reopen: whitespace, duplicate
keys, unknown fields, large integers, negative zero, decimal/exponent spelling,
Unicode and escapes. Native JSON therefore needs no separate original-text
column for the supported input. If a future DuckDB version changes this
behavior, the fidelity tests must continue to pass; retain original text
separately instead of normalizing source files.

This is additive `CREATE TABLE/INDEX IF NOT EXISTS` DDL under **schema version
22**, consistent with the existing additive compatibility policy. It changes
no existing columns or telemetry meaning and requires no version bump. A test
reopens a real current-version file without the new table, creates the archive,
and reopens it again with payloads intact. Existing mismatch/pre-versioning
checks still run before application DDL. The existing general read-only SQL
query API can inspect the table; the feature flag gates Evals methods and
routes, not general database access.

**Snapshots require explicit deletion.** Telemetry clear, dictionary sweeping
and retention pruning never delete them. This is a per-file/per-request bound,
not a total archive quota: accumulated snapshots can grow the database. They
count toward the existing whole-database size measurement. Consequently,
snapshots can trigger telemetry pruning, and the database may remain above
its retention cap after telemetry is pruned. Delete unwanted snapshots
explicitly; DuckDB reuses freed storage and does not guarantee immediate file
shrinkage. In-memory mode lasts for the process; durable snapshots require
the existing `--db` file option.

## Backend verification

```sh
go test ./desktopexporter/internal/server ./desktopexporter/internal/store/...
```

Tests cover native JSON fidelity on real file reopen, additive schema creation,
immutable duplicate metadata, concurrent dedupe, changed content, page bounds,
UTF-8/JSON/payload bounds, explicit deletion, retention/clear survival, named
and positional RPC parity, feature gates, cancellation, not-found errors,
request-body limits, exact downloads, HEAD, byte ranges and OpenRPC discovery.

## Viewer workflow

With Evals enabled, open **Snapshot library**, choose a JSON file and its explicit
source format, and import it. Importing leaves the current synthetic preview in
place. Open an archived file to inspect its projected runs. **Synthetic preview**
returns to the examples without deleting the archive.

The library reads metadata in pages of 25. File decoding rejects invalid UTF-8;
the browser sends the untouched decoded file text rather than parsing and
reserializing it. The UI checks both file and escaped-envelope byte limits.
Cancellation stops awaiting the request; because a server mutation may already
have completed, the UI asks the user to refresh the archive to establish its state.

`promptfoo-v3` supports the full version-3 result envelope. `viewer-kit` supports
the explicit `{manifest,pilot}` wrapper. A declared format that does not match
the envelope is not projected. `unknown-json` remains inspectable without an
adapter. Unsupported versions and bounded-projection failures do not destroy
the original file.

**Original JSON and projection context** exposes the retained source in bounded
32,768-character sections, mapping limitations and a selected-result JSON Pointer
when available. Download always uses the complete original file, independent of
the preview page. Routes carry the snapshot digest and snapshot-qualified run/row
identities. They resolve only against this database; a copied URL does not include
the file or associated telemetry.

Field Copy resolves complete recorded values from source pointers only when
requested, even when a display preview reaches its per-field or aggregate limit.
String values copy as decoded text; structured values retain their original JSON
subtree, including numeric lexemes. Derived labels retain their documented display
formatting. Missing tool output is not represented as a recorded empty string.

Projection limitations are deliberate: arbitrary Promptfoo scores have no assumed
maximum, source costs are not relabelled CAD, and absent execution/repetition
metadata cannot be recovered. Source-native values remain available in the original
JSON. See [the integration roadmap](eval-integration-roadmap.md) for the richer
typed-score, content and identity work still needed.

## Code walkthrough

1. `frontend/src/services/telemetry-service.ts`: typed RPC calls, abort signals,
   UTF-8/envelope bounds and complete-file download.
2. `frontend/src/components/evals/EvalSnapshotLibrary.svelte`: explicit import,
   paged archive, source inspection and deletion controls.
3. `frontend/src/pages/EvalsWorkspace.svelte`: snapshot route resolution and
   stale-request protection; synthetic examples remain the default source.
4. `frontend/src/components/evals/snapshot-adapters.ts`: pure bounded source
   interpretation, provenance pointers and declared projection limitations.
5. `frontend/src/components/evals/EvalTelemetrySummaries.svelte`: compact received
   span name/service/status/duration summaries using the existing referenced-trace
   fetch; native links open the exact span in Traces. Missing references stay
   navigable without inventing timing or parentage.
6. `server/eval_snapshots.go`: import Unicode validation and bounded RPC/download
   handlers; existing feature gating and error taxonomy stay at the server boundary.
7. `store/evals/snapshots.go` and `store/queries/ddl/tables/eval_snapshots.sql`:
   content identity, immutable metadata, native JSON storage and exact text reads.

Paths in this walkthrough are relative to `desktopexporter/internal/`. No evaluator
is invoked by this workflow. Saved UI-view definitions and portable database bundles
are separate future contracts, not additional meanings of an eval artifact.
