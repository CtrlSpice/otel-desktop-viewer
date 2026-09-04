package queries

import (
	"strings"
)

// DDL creation order, read from the `_order` manifest in each directory.
//
// The order is load-bearing and the manifests are the only place it is written
// down. Filenames say what an object is; `_order` says when it is built.
//
// # What constrains it
//
// Declared foreign keys, and only those:
//
//	spans, logs        -> resources, scopes
//	events, links      -> spans
//	metric_series      -> metric_streams, resources
//	metric_ingests     -> metric_streams, resources, scopes
//	datapoints         -> metric_streams, metric_series, metric_ingests
//	exemplars          -> datapoints
//
// So attributes comes first (it references nothing), then resources and
// scopes, then each signal above its own children.
//
// Macros are last, and among themselves they are ordered too: DuckDB binds a
// macro body when the macro is created, so `exp_buckets` must follow
// `exp_pos_buckets`. Creating them alphabetically does not work.
//
// # What does not constrain it, and must not start to
//
// The store carries cross-signal references that are deliberately *not*
// foreign keys:
//
//	logs.trace_id, logs.span_id
//	exemplars.trace_id, exemplars.span_id
//	links.linked_trace_id, links.linked_span_id
//
// These point at spans, but nothing enforces that the span exists. That is
// required, not an oversight. Signals arrive independently and out of order: a
// log is written the moment it is received, and the span it belongs to may
// arrive in a later batch, be dropped by sampling, or never be sent at all. A
// foreign key here would reject perfectly good telemetry on arrival, and it
// would do so most often under exactly the conditions a debugging tool exists
// for -- a partial or failed trace.
//
// So logs and exemplars impose no ordering against spans, and linked targets do
// not add another ordering constraint to links. Adding an FK to any column
// listed above would break ingest for out-of-order arrival, which no test that
// ingests a complete trace would catch.
var (
	typeFiles  = manifest("types")
	tableFiles = manifest("tables")
	indexFiles = manifest("indexes")
	macroFiles = manifest("macros")
)

// manifest reads one directory's `_order`, skipping blanks and # comments.
func manifest(kind string) []string {
	b, err := files.ReadFile("ddl/" + kind + "/_order")
	if err != nil {
		// Unreachable: go:embed fails at compile time if the file is absent.
		panic("queries: missing order manifest for " + kind + ": " + err.Error())
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		panic("queries: order manifest for " + kind + " is empty")
	}
	return out
}
