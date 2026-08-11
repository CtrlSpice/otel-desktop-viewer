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
// Files written before versioning existed carry no stamp at all and are
// detected separately.
const Version = 2

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
