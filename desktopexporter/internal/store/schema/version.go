package schema

// Version is the schema this build writes and expects.
//
// Bump it for each persisted schema change, update current DDL, and add the
// corresponding immutable SQL migration. See migrations/README.md.
const Version = 27

// VersionTableQuery creates the version table.
//
// The startup inspector reads existing metadata before application DDL. The
// transaction executor creates and stamps this table only for a fresh database,
// after its DDL succeeds.
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
			and count(*) filter (where column_name in ('resource', 'resource_id', 'resource_dropped_attributes_count')) = 1
			and count(*) filter (where column_name in ('scope', 'scope_id', 'scope_dropped_attributes_count')) = 1)
		or (table_name = 'logs'
			and count(*) filter (where column_name in ('trace_id', 'observed_timestamp')) = 2
			and count(*) filter (where column_name in ('resource', 'resource_id', 'resource_dropped_attributes_count')) = 1
			and count(*) filter (where column_name in ('scope', 'scope_id', 'scope_dropped_attributes_count')) = 1)
	)`
)
