package schema

// Version is the schema this build writes and expects.
//
// Bump it whenever a change makes an existing database file unreadable by this
// build -- a dropped or renamed column, a changed constraint, a different
// meaning for existing data. Additive changes that `create table if not exists`
// and `create index if not exists` apply cleanly to an older file do not need a
// bump.
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
// Version 4 changes what a resource id means, not the resources table's
// columns. It used to be sha256(attribute_ids, dropped_attributes_count) --
// the resource's whole attribute set -- which meant anything that enriched a
// resource mid-stream (a processor resolving k8s metadata, an SDK adding
// telemetry.sdk.* partway through) minted a new row for the same running
// process; metrics.SeriesID then worked around the resulting instability by
// keying series on InstanceKey instead of the resource id. It is now
// sha256(service.namespace, service.name, service.instance.id) -- the OTel
// triplet the spec actually commits to as identity -- so InstanceKey is gone
// and SeriesID takes the resource id again. Existing resource rows, and every
// metric_series row hashed from them, were keyed by the old function: this
// build would compute different ids for the same resources, so a version 3
// file's rows are unreadable under the new identity, not merely stale.
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
const Version = 8

// VersionTableQuery creates the version table.
//
// Deliberately not part of TableCreationQueries: the version check has to run
// *before* those, so that opening an incompatible file reports a version
// mismatch rather than failing partway through creating tables and indexes
// against a schema that does not match them.
const VersionTableQuery = `create table if not exists schema_meta (version integer)`

// ReadVersionQuery returns the stamped version, or NULL for a file that has
// none -- either brand new, or written before versioning existed. The caller
// distinguishes those two cases.
const ReadVersionQuery = `select max(version) from schema_meta`

// StampVersionQuery records the current version. Only ever run against a
// database with no stamp and no data, so there is nothing to overwrite.
const StampVersionQuery = `insert into schema_meta (version) values (?)`

// A stamp-less file is either brand new or predates versioning. Telling those
// apart is a two-step probe: does the spans table exist, and if so does it hold
// anything.
//
// It has to be two queries rather than one guarded by EXISTS, because DuckDB
// binds the whole statement before executing it -- a subquery naming `spans`
// fails to bind on a database where that table does not exist yet, which is
// every brand-new file.
//
// The probe asks duckdb_tables() rather than checking for a specific column, so
// it keeps working across future schema changes: the question is "does this file
// predate versioning", not "does it match some particular past shape".
const (
	SpansTableExistsQuery = `select count(*) from duckdb_tables() where table_name = 'spans'`
	SpanCountQuery        = `select count(*) from spans`
)
