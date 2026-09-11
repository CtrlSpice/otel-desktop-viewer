# Snapshot, Save, and Share

This document is the plan of record for reproducible views, named saves, and
portable telemetry snapshots. It separates three things that have different
lifecycles:

- A **view snapshot** is immutable, versioned application state.
- A **saved view** is a mutable name that points at a view snapshot.
- A **database snapshot** is a point-in-time copy of all retained telemetry and
  the view records that describe it.

The implementation is split into three product phases. Each phase may ship as
several independently reviewable pull requests.

## Goals

- Reproduce the meaningful state of traces, logs, and metrics without putting
  high-cardinality state in the URL.
- Keep existing compact deep links useful.
- Let named views travel with the database they describe.
- Export one artifact that preserves every correlation the viewer can follow.
- Open shared artifacts without replacing or merging into the live ingest
  store.
- Keep browser preferences local to their user and browser.

## Reproducibility boundary

A complete result needs three inputs:

```text
database snapshot + view snapshot + compatible viewer
```

A view snapshot describes how to inspect telemetry. It does not contain the
telemetry itself. IDs for traces, logs, metric streams, series, and datapoints
resolve only within the database that minted or preserved them.

Trace correlation can expand from one span to its trace, linked logs,
resources, and metrics. The first share format therefore exports the whole
retained database. Computing a smaller correlation closure is deferred until a
real use case can define what that closure must include.

## ViewSpecV1

`ViewSpecV1` is a plain, versioned value owned by the application state layer.
It is independent of URL, DuckDB, and component state. URL revisions, named
saves, and snapshot manifests all refer to the same canonical value.

It captures:

- The signal and selected trace, log, or metric.
- The effective time selection.
- Submitted search text and list sort.
- Trace span and event focus.
- Metric aggregation, histogram tab and scope, selected series and datapoint,
  visible series, active quantile, analytical toggles, and the timezone used to
  align aggregation buckets.

It excludes:

- Theme and the recipient's display-timezone preference.
- Drawer, panel, and column dimensions.
- Recent time ranges.
- Expanded rows, pagination, disclosures, hover state, and transient chart
  cursors.
- Parsed search trees, chart colours, request caches, and fetched telemetry.

An explicit empty collection is meaningful. For example, `visibleSeries: []`
means that the user deliberately hid every series. Omission is not used to
smuggle in data-dependent defaults once a complete view snapshot has been
created.

Visible series are always an exact ID array. There is no semantic `all` marker:
capture expands an unfiltered chart to the series currently visible so later
ingest cannot silently change the revision.

Relative presets are resolved to an absolute range when captured. An explicit
All selection remains semantic All. It becomes a fixed result in an exported
database snapshot and continues to include newly ingested data in the live
database.

Metric bucket alignment is query behavior. A metric view records either its
Internet Assigned Numbers Authority (IANA) timezone name or its fixed offset so
aggregate boundaries reproduce across machines. The recipient may still
display timestamps in their preferred local timezone.

`utcOffsetMinutes` is the number of minutes added to UTC to obtain local wall
time. For example, UTC-05:00 is `-300`. This is the opposite sign from
JavaScript's `Date.getTimezoneOffset()`.

## Canonical form and identity

Every accepted value is normalized before storage:

- Object fields have one schema-defined order.
- Set-like arrays contain canonical lowercase identifiers, are deduplicated,
  and are sorted by ASCII code unit.
- Unknown fields are discarded.
- Numbers, enums, field-specific identifiers, text, and cross-field invariants
  are validated.

Canonical bytes are the UTF-8 encoding of the normalized object's compact JSON
with fields in schema order. Strings use JavaScript `JSON.stringify` escaping.
Text must contain valid Unicode scalar values; U+2028 and U+2029 are excluded
so the Go and JavaScript encoders have one byte representation. Go encoding
must disable HTML escaping and pass the shared golden vectors before it can
write revision rows.

Absolute timestamps are integer Unix milliseconds bounded so conversion to the
query protocol's signed 64-bit nanoseconds cannot overflow.

The revision ID is the base64url SHA-256 digest of those bytes, prefixed with
its view-spec version. Repeatedly capturing the same state produces the same ID
and one stored row. Revisions are immutable. Canonical payloads are limited to
512 KiB. Submitted search text is limited to 64 KiB of UTF-8.

Content hashes provide identity and integrity within the database. They do not
authenticate a snapshot received from another person.

The store derives the ID from validated canonical JSON. It does not accept an
independently supplied ID or version. Reads and imports parse the payload,
recompute its canonical bytes and ID, and reject any mismatch before hydration.

## URL policy

Existing paths and compact query parameters remain supported as ad hoc deep
links:

- `start` and `end`
- `span` and `event`
- `agg`, `htab`, and `hscope`
- `series` and `dp`

A complete view uses the selected item path plus one revision parameter:

```text
/metrics/<stream-id>?view=<revision-id>
```

When `view` is present, the revision is authoritative and generated links omit
the other view-state parameters. The path remains readable, supports the SPA
fallback, and gives the application a destination while the revision loads.

Visible-series lists, quantile state, submitted search text, and sort state do
not receive raw query parameters. Settled user actions create a revision and
update the URL using the action's existing push or replace history semantics.
Search editor keystrokes do not create revisions; submitting the search does.

Applying a revision is atomic. Components do not write one history entry per
restored field.

## Storage model

Phase 1 introduces immutable revisions and telemetry-specific defaults:

```text
view_revisions
  id
  spec_version
  spec_json
  created_at

metric_view_defaults
  metric_stream_id
  defaults_json
  updated_at
```

`metric_view_defaults` replaces `metrics:view:<streamID>` browser storage. The
payload has its own `MetricViewDefaultsV1` schema containing only aggregation,
series visibility, and the all-series aggregate toggle. It cannot restore time,
search, selection, or another metric's state. The store validates that the
metric exists when writing the record and removes defaults before deleting a
metric stream. The record follows the metric's database identity and is
included in database exports. Browser preferences remain in localStorage.

Phase 2 adds mutable human-facing aliases:

```text
saved_views
  id
  name
  revision_id
  created_at
  updated_at
```

Updating a saved view changes its revision pointer. Existing revision URLs keep
resolving to their original immutable state. Renaming a save does not alter its
identity.

## Hydration and stale data

Hydration has two phases. It first loads and verifies the revision, validates
the destination against the active database, compiles the search, and resolves
every data-dependent refinement into one candidate state. It then commits that
state and its canonical route once. Structural, hash, destination, and search
errors leave the current view untouched and produce a visible error.

Valid revisions can outlive individual telemetry rows. Refinements degrade in
one direction:

- A missing destination item rejects the revision. The application does not
  silently select a different trace, log, or metric.
- A missing trace event falls back to its span; a missing span falls back to
  the trace.
- A missing metric datapoint falls back to its selected series; a missing
  series clears that selection.
- Missing visible series are removed from the exact set without reseeding
  defaults. An empty result remains empty.
- An aggregation unavailable for the resolved metric falls back to `raw`.

The UI reports degraded refinements after the atomic application. If the URL
path disagrees with a valid revision, the revision destination wins and the
path is canonicalized.

## Revision lifecycle

Settled actions are coalesced before creating a revision, so a burst of legend
or sort changes stores its final state. Search revisions are created on submit,
never for editor keystrokes. Scalar series selections retain the UI's 22-series
cap. Histogram selections are bounded by item count and the 512 KiB canonical
payload limit.

Revision rows are retained because a copied URL may refer to any of them.
Content addressing deduplicates equal states; there is no automatic revision
garbage collection in the first format. Storage growth is measured during
Phase 1 before that durability contract is reconsidered.

## Phase 1: View Snapshot Foundation

Phase 1 delivers reproducible state without Save or Share controls.

1. Define, validate, normalize, serialize, and hash `ViewSpecV1` with focused
   pure tests.
2. Add `view_revisions` and `metric_view_defaults` with store and JSON-RPC
   operations.
3. Capture and hydrate revisions atomically in the frontend.
4. Add the `view` URL parameter while preserving existing deep links.
5. Move metric visible-series, aggregation, and aggregate-toggle defaults out
   of localStorage.
6. Capture visible series, active quantiles, search, sort, and the other
   reproducible state listed above.
7. Close #355 through the short revision URL rather than an expanded series
   query string.

Phase 1 is complete when refresh and back/forward restore the same semantic
view, including an explicit empty series set, and malformed or stale state
fails predictably without corrupting current state.

Search text, time, and sort form one declarative list request. Changing sort
reruns a submitted limited search so the saved sort is also the sort that chose
result membership.

## Phase 2: Save

Save is a deliberate user action over an existing immutable revision.

1. Add saved-view create, list, rename, update, and delete operations.
2. Add a global Views menu and a Saved views section on Home.
3. Let users save the current revision, open a saved view, or move a saved name
   to the current revision.
4. Show that saves in the default in-memory database last for the current
   process, while saves in a configured database survive restart.

Save remains separate from export. Users can maintain several inexpensive
views and choose one later as a shared snapshot's entry point.

## Phase 3: Share

Share packages the full retained database and selects one view revision as its
entry point.

The artifact is one application-specific ZIP containing:

```text
manifest.json
schema.sql
load.sql
*.parquet
```

The manifest records:

- Artifact format and snapshot IDs.
- Creation time.
- Application, database schema, and DuckDB versions.
- Default view revision.
- An allowlist of files with sizes and SHA-256 digests.

Export runs under the store write lock so it cannot capture a partially
prepared application ingest. Archiving and download streaming happen after the
DuckDB export has completed and the lock has been released.

Binary transfer uses dedicated same-origin HTTP endpoints. JSON-RPC remains the
control plane for small metadata operations. The Share UI states clearly that
the artifact contains all retained telemetry.

Shared artifacts are untrusted input. The viewer never executes their
`schema.sql` or `load.sql`. Opening validates the archive and manifest, creates
a fresh database from application-owned DDL, and loads only allowlisted Parquet
tables. The first format requires an exact supported schema version.

An opened snapshot uses a separate read-only query store. Live ingest continues
against the configured store. A persistent snapshot-mode banner identifies the
active artifact and provides a Return to live data action. Failed imports leave
the current query store untouched.

## Compatibility

Three versions remain separate:

- View-spec version controls state decoding and canonical identity.
- Snapshot-format version controls archive and manifest decoding.
- Database schema version controls which application queries can read the
  reconstructed store.

A Parquet export preserving DuckDB types does not make an arbitrary future
application schema readable by the current viewer. Cross-version import needs
explicit adapters or migrations and is deferred.

Changes to search grammar, sort meaning, aggregation semantics, or any other
behavior that changes how an existing payload is interpreted require a
view-spec version change or an explicit compatibility adapter.

## Verification gates

Each behavior slice includes focused tests at its nearest boundary. Before a
slice is ready for review, it also passes the repository's broad `make test`
gate and regenerates committed frontend assets when frontend source changes.

Snapshot round-trip coverage must include traces, logs, gauges, sums,
histograms, exponential histograms, exemplars, array edge cases, enums,
rejections, saved views, and the default revision. Export and import tests also
cover cancellation, concurrent ingest, cleanup, corrupt archives, path
traversal, duplicate entries, size limits, version mismatch, and preserving the
active store after failure.
