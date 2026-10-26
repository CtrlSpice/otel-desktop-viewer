# Schema compatibility

The store uses schema version 24. It does not migrate databases.

The viewer uses DuckDB 2.0.0. This engine update retains application schema 24,
so schema-24 databases created by viewer 0.6.0 open without a schema conversion.
The application schema version is independent of the DuckDB engine version.

Version 24 makes Evals source tables rebuildable caches of retained,
Collector-processed log evidence. Earlier versions contain independently stored
producer JSON and cannot establish this processing boundary. The compatibility
check applies to the entire database, including ordinary telemetry.

Store startup validates `schema_meta` before running application DDL. A new,
empty database receives the current version. Any database with malformed
metadata, stored telemetry without version metadata, or a different version is
rejected without modification.

To continue after a version mismatch, delete the database or choose another
`--db` path.

Tests cover these cases:

- a new empty database is stamped with version 24;
- a matching database opens normally;
- malformed, empty, duplicate, older, and newer version metadata is rejected;
- an unversioned telemetry database is rejected;
- rejected files remain byte-for-byte unchanged.
