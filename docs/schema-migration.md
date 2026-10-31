# Schema compatibility

The store uses schema version 26. It does not migrate databases.

The viewer uses DuckDB 2.0.0. Attributes, log bodies, and Evals payloads use
native VARIANT storage. Schema-24 databases require a fresh database and reloading
the source data.
The application schema version is independent of the DuckDB engine version.

Evals source tables remain rebuildable caches of retained, Collector-processed
log evidence. The compatibility check applies to the entire database, including
ordinary telemetry.

Store startup validates `schema_meta` before running application DDL. A new,
empty database receives the current version. Any database with malformed
metadata, stored telemetry without version metadata, or a different version is
rejected without modification.

To continue after a version mismatch, delete the database or choose another
`--db` path.

Tests cover these cases:

- a new empty database is stamped with version 25;
- a matching database opens normally;
- malformed, empty, duplicate, older, and newer version metadata is rejected;
- an unversioned telemetry database is rejected;
- rejected files remain byte-for-byte unchanged.
