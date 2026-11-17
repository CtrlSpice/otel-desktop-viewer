# Database upgrades

The current schema is version 27, shipped in 0.7.0. It is also the oldest schema
supported by automatic upgrades; there are no production migration steps yet.

## Open a database

Before accepting telemetry, the store checks `schema_meta`:

- New databases are created at the current version.
- Current-version databases open normally.
- Supported older databases run each missing migration in order.
- Pre-27, newer, malformed or unversioned telemetry databases are rejected before
  application schema changes.

All migrations, current schema definitions and the version stamp commit in one
DuckDB transaction. Failure rolls back the transaction and stops startup with
the database path and failing step. Fresh-database creation is transactional too.
The viewer never resets a database automatically.

For a newer database, upgrade the viewer. For a pre-0.7.0 database, use its
original viewer or choose a new `--db` path and reload source data. Malformed
metadata requires a known-good backup or a new database. Downgrades are unsupported.

The application schema version is separate from the DuckDB engine version,
currently 2.0.0. SQL migrations require DuckDB to open the file; they cannot
convert an unreadable engine file format. All viewer builds share the schema
and migration chain.

## Add a migration

1. Increment `schema.Version` and update the current SQL definitions in
   [`queries/ddl`](../desktopexporter/internal/store/queries/ddl/), including
   their `_order` manifests.
2. Add `NNN_description.sql` under
   [`schema/migrations`](../desktopexporter/internal/store/schema/migrations/).
   The first step is `028_`. Published steps are immutable.
3. Test populated upgrades, data preservation, equivalence with a fresh schema,
   reopening without replay, and rollback.

Each step must preserve existing data and rebuild affected derived data.
Use self-contained SQL that does not depend on current macro definitions.
The runner owns the transaction and version stamp: migration SQL must not
contain transaction commands or write `schema_meta`.

The runner checks for a complete, ordered, nonempty chain before changing any
database. Its tests use synthetic steps to cover multiple upgrades and failures
during migration, current schema creation and version stamping.
