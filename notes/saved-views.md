# Saved views and sharing — design notes

Working notes, not a plan of record. Written while thinking through the shape
of the problem, and the argument reversed more than once along the way —
freeze became reference became a Parquet bundle, and pinning and entry caps
came and went with it. Open questions are marked as such rather than resolved
silently.

---

Three problems, one answer: the home page has nothing useful after first run,
view state is persisted where it cannot travel, and a URL cannot carry a view
without becoming unreadable.

## Saving means exporting a Parquet bundle

`EXPORT DATABASE '<dir>' (FORMAT PARQUET)` writes one Parquet file per table
plus a `schema.sql`. `IMPORT DATABASE` reconstructs it. Verified against the
real schema on 2026-08-22:

- Imported into a **bare** DuckDB with none of our schema pre-created — row
  counts matched exactly.
- Types survive as real DuckDB types, not Parquet's lowest common denominator:
  `attribute_ids` returns as `UUID[]` and still joins the dictionary;
  `attr_type` returns as an `ENUM` **with declaration order intact**, so its
  sort semantics do not silently become lexicographic.
- `schema.sql` (~23KB) carries all 47 macros, 22 indexes and both sequences.
  `attrs_json(attribute_ids)` ran correctly against the imported file with no
  tool present.

So a bundle is a self-contained, queryable database. Attach one to a bug
report and anyone with the DuckDB CLI can dig through it — no viewer required.

**This removes the hard part.** A bundle is a copy at a point in time,
independent of the live store. Retention can prune whatever it likes; the save
is already safe. No pinning of referenced rows, no cap on entries, no
starvation guard — all of which were only needed because a saved view pointed
at data that could vanish.

It also sidesteps schema versioning. `store/version.go` enforces an exact
`schema_meta` match with no migration, which is stricter than DuckDB's own
storage guarantee (backward compatible from 0.10, guaranteed from 1.0.0; we
run 1.5.5). A Parquet bundle carries its own schema and does not care what
wrote it. Migration comes when the schema settles; today the version check
mostly stops us breaking our own tests.

## Saved views

A saved view is a name plus a route and its query params — a bookmark, stored
in its own table. It exports with everything else, so views travel with the
data they describe.

This still needs the view to be *expressible* in a URL, which is #355 and
unchanged. But length stops mattering: the human clicks a name, not a query
string. (For scale: ten visible series is 404 characters, forty is 1,574,
because `URLSearchParams` escapes every comma to `%2C`.)

## What moves out of localStorage

Pre-1.0, so this is replaced wholesale with no migration.

| kind | examples | moves? |
|---|---|---|
| Browser preference | theme, panel widths, recent time ranges | **No** — a shared view should render in the recipient's theme |
| Per-metric sticky state | `metrics:view:<streamID>` — visible series, aggregation view | **Yes** — describes the telemetry, not the person |
| Saved view | new | **Yes** |

## Open questions

- **Metrics are untested.** The round trip was proven on traces only.
  `ubigint[]` bucket counts, `double[]` bounds and exemplars were all empty in
  the fixtures. Confirm before relying on this.
- A bundle is a directory of ~16 files. Sharing wants one artifact, so zip it
  or adopt a convention.
- `schema_meta.parquet` exports too, so an import carries the exporter's
  version stamp. Fine if import means "open as a separate store", awkward if
  it means "merge into mine".
- Does a save capture the time window? A relative window ("last 15 minutes")
  shows different data tomorrow.
- Do sticky per-metric views and named saves share a table? One is written
  silently on every legend toggle, the other is deliberate.
