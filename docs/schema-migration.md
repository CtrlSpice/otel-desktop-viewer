# Schema compatibility

The store uses schema version 22. It does not migrate databases.

Version 22 keeps the last received value per key within each owner's attribute
collection and each nested map before sorting and hashing. Earlier files can
retain conflicts whose received order is unavailable, so they cannot establish
the selected value under this rule.

Store startup validates `schema_meta` before running application DDL. A new,
empty database receives the current version. Any database with malformed
metadata, stored telemetry without version metadata, or a different version is
rejected without modification.

To continue after a version mismatch, delete the database or choose another
`--db` path.

Tests cover these cases:

- a new empty database is stamped with version 22;
- a matching database opens normally;
- malformed, empty, duplicate, older, and newer version metadata is rejected;
- an unversioned telemetry database is rejected;
- rejected files remain byte-for-byte unchanged.
