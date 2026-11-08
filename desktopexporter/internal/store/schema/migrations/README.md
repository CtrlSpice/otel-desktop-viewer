# Schema migrations

Schema 27 (release 0.7.0) is the baseline. There are no migration steps yet.

For each persisted schema change, increment `schema.Version`, update the current
DDL, and add `NNN_description.sql` here, starting with `028_`. Each file upgrades
the preceding version. Keep the sequence contiguous and never edit a published
migration; add a corrective migration instead.

Migrations run before current DDL, in one transaction covering all pending steps,
DDL, and the version stamp. Use SQL supported inside a DuckDB transaction. Do not
include transaction commands or change `schema_meta`; the runner owns both.
Preserve existing telemetry and rebuild any affected derived data. SQL must be
self-contained: do not rely on the current macro definitions to transform older
data. Both build modes use this same chain.

Add populated predecessor fixtures and tests for data preservation, equivalence
to fresh DDL, reopening without replay, and rollback on failure. Fresh databases
use current DDL directly, so keep it consistent with the migration result.
