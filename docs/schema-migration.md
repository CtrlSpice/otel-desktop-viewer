# Schema upgrades

The current schema is version 27, the format shipped in 0.7.0 and the baseline
for automatic upgrades. There are no production migration steps yet.

At startup, before accepting telemetry, the store checks `schema_meta`:

- New databases are created directly at the current version.
- Current-version databases open normally.
- Older supported databases run every missing migration in order.
- Databases older than 27, newer than the executable supports, or with invalid
  or missing telemetry version metadata are rejected before application DDL.

All pending migrations, current DDL, and the version stamp commit in one DuckDB
transaction. If a step fails, the transaction rolls back and startup fails with
the database path and failing step. Correct the cause and retry. Fresh-database
creation is transactional too. The store never resets databases automatically.

For a database newer than the executable, upgrade the viewer. For a pre-0.7.0
database, keep the original executable to view it, or use a new `--db` path and
reload source data. Malformed metadata requires a known-good backup or a new
database; the viewer does not guess the old schema. Downgrades are not supported.

The viewer uses DuckDB 2.0.0. Attributes, log bodies, and Evals payloads use
native VARIANT storage. The application schema version is independent of the
DuckDB engine version. SQL migrations require the engine to be able to open the
file; they do not convert an unreadable DuckDB file format.

Evals source tables remain rebuildable caches of retained, Collector-processed
log evidence. The compatibility check applies to the entire database, including
ordinary telemetry. Both build modes use the same schema and migration chain.

## Adding a migration

Increment `schema.Version`, update current DDL, and add an immutable
`NNN_description.sql` file under
`desktopexporter/internal/store/schema/migrations/`. The first step is `028_`.
The runner checks that the chain is complete, ordered, and nonempty before
changing any database, including a fresh one. Published steps are never edited.

SQL steps must preserve existing data and rebuild affected derived data. They
must not contain transaction commands or write `schema_meta`; the runner owns
the transaction and stamp. Use self-contained SQL rather than depending on
current macro definitions to transform old data.

Include populated predecessor fixtures and tests for data preservation, matching
fresh-schema structure, reopening without replay, and rollback. Runner tests use
synthetic steps to cover multiple upgrades and failures during migration, current
DDL, and version stamping without introducing a production schema change.
