# Schema migration proposal

Status: proposal, not implemented. The current store uses schema version 17
and deliberately rejects a database with a different or malformed version.
Users must delete the incompatible file or select another `--db` path.

This document records a possible migration design for future review. It does
not promise compatibility with any previous schema.

## Integration point

`inspectSchemaVersion` in `desktopexporter/internal/store/version.go` reads the
catalog and validates `schema_meta` before any application DDL runs. A future
migration ladder would run only after that inspection and before current DDL is
applied.

The ladder would execute one explicit step for each version from the stored
version to the current version. A version stamp alone does not make a partial
step resumable. Unless a step and its stamp are proven atomic or the step is
explicitly idempotent, interruption invalidates the migration copy.

For an on-disk database, migration should operate on a copy and replace the
original only after the complete ladder and validation succeed. The original
file must remain usable after a failed or cancelled migration. An invalidated
copy is discarded and a later attempt starts again from the original.

## Migration shapes

Some changes can use direct DuckDB `ALTER TABLE` operations. Adding a required
column may need a nullable or defaulted backfill before applying its
constraint.

Changes to keys or table relationships may require rebuilding a table: create
the new shape, copy and transform rows, validate the result, then replace the
old table. Each such step needs version-specific evidence that its source data
contains enough information for a lossless conversion. A migration must fail
rather than invent values or silently discard telemetry.

## Verification

Keep a small immutable database fixture for each supported source version.
Tests should open each fixture, run the complete ladder, and then apply the
same store invariants and representative queries used for a newly created
database. Fixtures must be generated from the historical schema they
represent, not regenerated from current DDL.

Also test interruption, cancellation, insufficient disk space, malformed
metadata, unsupported source versions, and preservation of the original file
after failure.

## Open questions

- Which historical schema versions are worth supporting once migration work is
  prioritized.
- Whether each DuckDB DDL sequence can be safely transactional or must rely on
  copy-and-replace for rollback.
- Whether migration provenance belongs in `schema_meta`.
- How migrations interact with future database snapshot import formats.
