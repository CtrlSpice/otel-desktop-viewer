# Schema migration — design notes

Working notes, not a plan of record. The decision so far is deliberate
non-support: every version bump hard-fails on mismatch, and this file records
how a migration ladder would work when the schema settles enough to earn one.

---

Every released user is on v0.5.0 = schema 7. The valuable rung is therefore
7→8, which is the expensive kind (see below) — a ladder that only walks the
cheap rungs helps almost nobody. Retention bounds how much data is worth
preserving, and a Parquet export carries its own schema, which together keep
hard-fail tolerable for now.

## Where it hooks in

`checkSchemaVersion` (store/version.go) already runs on every open with the
stored version, the target, and a live `*sql.DB` — after the version read,
before tables/indexes/macros are (re)created. A ladder replaces the hard fail:
walk `stored+1 … target`, one step per version, and stamp `schema_meta` after
each rung so an interrupted migration resumes instead of restarting.

Copy the `.db` file first and swap on success. Cheap, and a botched rung
becomes a non-event.

## Two kinds of rung

**Column-shaped** — every bump v1→v7, and 8→9. DuckDB 1.5.5 handles these
directly (probed, not assumed):

- `alter table add column if not exists x TYPE default v` works and backfills.
- `rename column` and `drop column` work.
- `add column x TYPE not null` **fails** ("Adding columns with constraints not
  yet supported") — a NOT NULL column needs a DEFAULT-first path.

**Constraint-shaped** — 7→8, which rekeyed spans on (trace_id, span_id).
DuckDB cannot alter a primary key, so this kind is a rebuild: create the
new-shape table, `insert … select`, drop, rename — for spans, events, and
links. One saving grace: backfilling `events.trace_id` needs a join on span id
alone, which is exactly the ambiguous join the new key exists to prevent —
but v7's old primary key guaranteed span ids unique, so for v7 data
specifically that join is sound.

The 8→9 rung, for reference (ingest_rejections sample columns): rename
`sample` to `sample_span_id`, add `sample_trace_id`, drop `detail`, then null
the old samples — they are padded-uuid form missing their trace half, and the
sample columns move as a pair or not at all.

## Testing

Commit tiny fixture `.db` files per old version. The migration test opens
each, walks the ladder, and asserts the ordinary suite invariants hold on the
result. Fixtures are generated once from the old DDL and never regenerated —
that is the point of them.

## Open questions

- Whether DuckDB's `alter column set not null` exists as a second step after a
  DEFAULT backfill — not yet probed.
- Whether a rung runs in a transaction. DuckDB DDL is transactional in the
  main, but the rebuild kind moves whole tables; the file-copy net may be the
  honest guarantee rather than rollback.
- Whether `schema_meta` should grow a `migrated_from` audit column when the
  ladder lands.
