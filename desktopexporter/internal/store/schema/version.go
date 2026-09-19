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
//
// Version 1 was the owner-keyed attributes schema -- the last shape before the
// dictionary. It shipped stamped, so databases written by that build exist in
// the wild and must be recognised rather than silently reused.
//
// Version 2 is the attribute dictionary: attributes deduped into a table of
// distinct (key, value, type, scope) rows, owners referencing them by uuid[],
// resources and scopes as shared tables, and metric_series. Nothing about a
// version 1 file can be read by this build -- the columns it indexes do not
// exist -- so the bump is what stops that being discovered as an appender
// column-count error midway through an ingest.
//
// Version 3 adds resource_schema_url and scope_schema_url to spans, logs and
// metric_ingests. Additive in the loose sense, but `create table if not exists`
// does not alter a table that already exists: a version 2 file keeps its
// narrower spans table and the first ingest fails with "invalid column count:
// expected 16, got 18". That is precisely the opaque failure this constant
// exists to turn into a clear message, so a new column on an existing table
// bumps it even though a new table or a new index does not.
//
// Version 4 changed what a resource id meant, not the resources table's
// columns. It replaced sha256(attribute_ids, dropped_attributes_count) with
// sha256(service.namespace, service.name, service.instance.id), removed
// InstanceKey, and keyed SeriesID on that resource id. Existing resource and
// metric_series rows were keyed by the old functions, so a version 3 file was
// unreadable under version 4 rather than merely stale. Version 15 supersedes
// that service-triplet identity while retaining this history.
//
// Files written before versioning existed carry no stamp at all and are
// detected separately.
//
// Version 5 moved explicit_bounds off datapoints into the histogram_bounds
// dictionary, referenced by bounds_id. A version 4 file has the vector where
// this build expects a reference, so its histogram datapoints are unreadable
// under this schema, not merely stale.
// Version 6 adds flags to spans and links: the W3C trace flags, and on a span
// the bit saying whether the parent context was remote. Logs and metric
// datapoints had stored theirs from the start, so this is the same field
// arriving late rather than a new idea. Like version 3, it is a new column on
// an existing table, which `create table if not exists` will not add to a
// version 5 file -- that file keeps its narrower spans table and the first
// ingest fails on the appender's column count.
//
// Version 7 adds metadata_ids to metric_ingests, for OTLP's Metric.metadata --
// an attribute map describing the instrument rather than identifying a series.
// It sits beside description because both vary per batch and neither is part
// of stream identity. Same mechanism as versions 3 and 6: a new column on an
// existing table, so a version 6 file fails the metric appender's column count
// on its first metric batch.
//
// Version 8 rekeys spans on (trace_id, span_id): a span id is only required
// to be unique within its trace, and the old span_id-only primary key
// rejected conformant senders whose ids repeat across traces. events and
// links gained the owning trace_id and reference the pair, and links renamed
// its trace_id to linked_trace_id. New columns and changed constraints on
// existing tables, so a version 7 file is unreadable under this schema.
//
// Version 9 replaces ingest_rejections' sample and detail columns with a
// bounded samples array -- the most recent refused spans, both halves of each
// identity, newest first. A version 8 file keeps the old columns, so the
// rejection insert and the stats read would both fail against it.
//
// Version 10 stores every 8-byte OTLP span ID as DuckDB UBIGINT rather than a
// zero-padded UUID. This changes seven existing columns across spans, events,
// links, logs, and exemplars, so a version 9 file is incompatible.
//
// Version 11 replaces exemplars.value with separate double_value and int_value
// columns. The old DOUBLE column had already rounded integer exemplars during
// ingest, so a version 10 file cannot provide the typed, exact values this
// build's queries expect.
//
// Version 12 stops creating four explicit multicolumn ART indexes on events,
// links, and datapoints. DuckDB 1.5.5 cannot use multicolumn indexes for index
// scans; measurements found only write, delete, and storage costs. Merely
// removing their creation queries would leave the indexes in existing version
// 11 files, so the bump applies the completed cleanup under the no-migration
// policy.
// Version 13 stores recursive tagged JSON values in the scope-free attribute
// dictionary and log bodies. Existing value/type/scope rows are incompatible.
// Version 14 stores every received OTLP timestamp as DuckDB UBIGINT. The prior
// BIGINT columns cannot represent the upper half of OTLP's uint64 range.
// Version 15 restores resource ids as hashes of the complete received
// attribute payload and dropped count. Version 14 rows use service-triplet
// resource ids, so their record associations cannot be reinterpreted safely.
// Version 16 gives the existing nullable histogram sum/min/max columns their
// OTLP meaning: NULL is absent and zero is present zero. Version 15 ingestion
// wrote zero for both states, so its rows cannot be reinterpreted safely.
const Version = 16

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
		where schema_name = current_schema() and table_name in ('spans', 'logs', 'metric_ingests')
		group by table_name
		having (table_name = 'spans'
			and count(*) filter (where column_name in ('trace_id', 'span_id')) = 2
			and count(*) filter (where column_name in ('resource_id', 'resource_dropped_attributes_count')) = 1
			and count(*) filter (where column_name in ('scope_id', 'scope_dropped_attributes_count')) = 1)
		or (table_name = 'logs'
			and count(*) filter (where column_name in ('trace_id', 'observed_timestamp')) = 2
			and count(*) filter (where column_name in ('resource_id', 'resource_dropped_attributes_count')) = 1
			and count(*) filter (where column_name in ('scope_id', 'scope_dropped_attributes_count')) = 1)
		or (table_name = 'metric_ingests'
			and count(*) filter (where column_name in ('id', 'stream_id')) = 2)
	)`
)
