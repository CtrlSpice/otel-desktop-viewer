package schema

// Version is the schema this build writes and expects.
//
// Bump it whenever a change makes an existing database file unreadable by this
// build -- a dropped or renamed column, a changed constraint, a different
// meaning for existing data -- or when a compatible cleanup must apply to
// existing files rather than only new ones. Additive changes that
// `create table if not exists` and `create index if not exists` apply cleanly
// to an older file do not need a bump.
//
// There is no migration machinery. This exists so an incompatible file is
// reported clearly instead of failing later with something opaque: without it,
// `create table if not exists` silently leaves an old table in place and the
// mismatch first surfaces as a duckdb appender column-count error during ingest,
// or as an index creation failure against a column that does not exist.
// Version 22 requires last-occurrence selection per attribute collection and
// nested map before identity hashing. Earlier files can retain conflicting keys
// whose received order is no longer available.
const Version = 22

// VersionTableQuery creates the version table.
//
// Deliberately not part of TableCreationQueries: the version check has to run
// *before* those, so that opening an incompatible file reports a version
// mismatch rather than failing partway through creating tables and indexes
// against a schema that does not match them.
const VersionTableQuery = `create table if not exists schema_meta (version integer)`

// ReadVersionQuery returns the stamped version, or NULL for an empty metadata
// table. It is kept for callers that only need the recorded version.
const ReadVersionQuery = `select max(version) from schema_meta`

// ReadVersionMetadataQuery validates the version metadata shape before the
// store accepts it. A store writes exactly one stamp, so an empty or
// multiply-stamped table cannot establish compatibility.
const ReadVersionMetadataQuery = `select count(*), min(version), max(version) from schema_meta`

// StampVersionQuery records the current version. Only ever run against a
// database with no stamp and no data, so there is nothing to overwrite.
const StampVersionQuery = `insert into schema_meta (version) values (?)`

// The catalog probes avoid referring to a possibly absent table: DuckDB binds a
// whole statement before running it, so a subquery naming a missing table fails
// even when guarded by EXISTS.
const (
	SchemaMetaTableExistsQuery = `select count(*) from duckdb_tables() where schema_name = current_schema() and table_name = 'schema_meta'`
	SchemaMetaShapeQuery       = `select count(*), count(*) filter (where column_name = 'version' and data_type = 'INTEGER')
		from duckdb_columns() where schema_name = current_schema() and table_name = 'schema_meta'`
	TelemetryTableExistsQuery = `select count(*) from (
		select table_name from duckdb_columns()
		where schema_name = current_schema() and table_name in ('spans', 'logs')
		group by table_name
		having (table_name = 'spans'
			and count(*) filter (where column_name in ('trace_id', 'span_id')) = 2
			and count(*) filter (where column_name in ('resource_id', 'resource_dropped_attributes_count')) = 1
			and count(*) filter (where column_name in ('scope_id', 'scope_dropped_attributes_count')) = 1)
		or (table_name = 'logs'
			and count(*) filter (where column_name in ('trace_id', 'observed_timestamp')) = 2
			and count(*) filter (where column_name in ('resource_id', 'resource_dropped_attributes_count')) = 1
			and count(*) filter (where column_name in ('scope_id', 'scope_dropped_attributes_count')) = 1)
	)`
)
