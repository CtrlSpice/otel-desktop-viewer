// Package queries holds every piece of SQL the store runs, as files.
//
// DDL files define ordered types, tables, indexes, and macros. Signal
// directories contain one file per read query. Ingest remains in the signal
// packages because it walks pdata and drives appenders.
//
// Templates are parsed at package initialization with missing keys rejected.
// Registry checks cover embedded files in both directions, and golden tests pin
// rendered query text.
package queries

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"
)

//go:embed ddl/tables/*.sql ddl/indexes/*.sql ddl/macros/*.sql
//go:embed ddl/types/_order ddl/tables/_order ddl/indexes/_order ddl/macros/_order
//go:embed spans/*.sql metrics/*.sql logs/*.sql
var files embed.FS

// Statement is one DDL object: the SQL, plus the file it came from.
type Statement struct {
	Name string
	SQL  string
}

// Types, Tables, Indexes and Macros return the DDL in creation order.
func Types() []Statement   { return ddl("types", typeFiles) }
func Tables() []Statement  { return ddl("tables", tableFiles) }
func Indexes() []Statement { return ddl("indexes", indexFiles) }
func Macros() []Statement  { return ddl("macros", macroFiles) }

func ddl(kind string, names []string) []Statement {
	out := make([]Statement, 0, len(names))
	for _, n := range names {
		out = append(out, Statement{
			Name: kind + "/" + n,
			SQL:  mustRead(path.Join("ddl", kind, n)),
		})
	}
	return out
}

// Name identifies one read-path query. Typed rather than a bare string so a
// caller cannot pass an arbitrary path, and so the set is enumerable for the
// registration check below.
type Name string

const (
	// GetTraceView fetches one whole trace: the recursive tree walk, its
	// payload, and the resource/scope maps the wire format references.
	GetTraceView Name = "spans/get_trace_view.sql"
	// GetTraceOverview returns the compact span rows and exact timing summary used by
	// the trace command. It deliberately omits full span detail.
	GetTraceOverview Name = "spans/get_trace_overview.sql"
	// GetSpanSummaries returns bounded summary rows for one exact span ID.
	GetSpanSummaries Name = "spans/get_span_summaries.sql"
	// GetSpan returns one full span selected by its composite identity.
	GetSpan Name = "spans/get_span.sql"

	// SalvageSpans adds a cycle-aware walk for spans GetTraceView cannot reach.
	SalvageSpans Name = "spans/salvage_spans.sql"

	// SearchTraceSummaries lists trace summaries for the trace list view.
	SearchTraceSummaries Name = "spans/search_trace_summaries.sql"
	// GetTraceOTLP reconstructs every stored span for one trace as standard
	// OTLP JSON, without applying UI search or tree-reachability rules.
	GetTraceOTLP Name = "spans/get_trace_otlp.sql"

	// GetMetric returns one exact Metric identity and its series catalogue.
	GetMetric Name = "metrics/get_metric.sql"
	// GetMetricSeries returns one exact series' retained received datapoints,
	// grouped by their owning received Metric reports.
	GetMetricSeries Name = "metrics/get_metric_series.sql"
	// GetMetricView returns the chart/UI projection for one Metric.
	GetMetricView Name = "metrics/get_metric_view.sql"
	// GetMetricOTLP reconstructs one stored Metric as standard OTLP JSON,
	// without applying UI aggregation or time-window rules.
	GetMetricOTLP Name = "metrics/get_metric_otlp.sql"
	// GetMetricAttributeDefinitions lists the attribute keys metrics carry.
	GetMetricAttributeDefinitions Name = "metrics/get_metric_attribute_definitions.sql"

	// GetLog returns one log record with its attributes resolved.
	GetLog Name = "logs/get_log.sql"
	// GetLogOTLP reconstructs one stored log record as standard OTLP JSON.
	GetLogOTLP Name = "logs/get_log_otlp.sql"
	// GetTraceLogSummaries returns lightweight summaries for every log in one trace.
	GetTraceLogSummaries Name = "logs/get_trace_log_summaries.sql"
	// GetSpanLogs returns full logs associated with one composite span identity.
	GetSpanLogs Name = "logs/get_span_logs.sql"
	// GetLogAttributeDefinitions lists the attribute keys logs carry.
	GetLogAttributeDefinitions Name = "logs/get_log_attribute_definitions.sql"

	// SearchMetricSummaries lists Metrics for the metrics list view.
	SearchMetricSummaries Name = "metrics/search_summaries.sql"
	// SearchLogSummaries lists log summaries for the logs list view.
	SearchLogSummaries Name = "logs/search_log_summaries.sql"
)

// queryNames is every read-path query. Kept beside the constants so adding one
// without registering it is a visible omission rather than a silent one.
var queryNames = []Name{
	GetTraceView, SalvageSpans, SearchTraceSummaries, GetTraceOverview, GetSpanSummaries, GetSpan, GetTraceOTLP,
	GetMetric, GetMetricSeries, GetMetricView, GetMetricOTLP, GetMetricAttributeDefinitions,
	GetLog, GetLogOTLP, GetTraceLogSummaries, GetSpanLogs, GetLogAttributeDefinitions,
	SearchMetricSummaries, SearchLogSummaries,
}

// Names returns every registered read-path query, so callers that need to
// walk the set -- the parse test, for one -- do not have to restate it.
func Names() []Name { return append([]Name(nil), queryNames...) }

var templates = parseAll()

func parseAll() map[Name]*template.Template {
	parsed := make(map[Name]*template.Template, len(queryNames))
	for _, n := range queryNames {
		parsed[n] = template.Must(
			template.New(string(n)).Option("missingkey=error").Parse(mustRead(string(n))),
		)
	}
	checkRegistered(parsed)
	return parsed
}

// checkRegistered walks the embedded tree and fails init on anything the
// registry does not account for -- a .sql file nobody references would never
// run, and would rot unnoticed.
func checkRegistered(parsed map[Name]*template.Template) {
	known := make(map[string]bool, len(parsed)+64)
	for n := range parsed {
		known[string(n)] = true
	}
	for kind, names := range map[string][]string{
		"types": typeFiles, "tables": tableFiles,
		"indexes": indexFiles, "macros": macroFiles,
	} {
		for _, n := range names {
			known[path.Join("ddl", kind, n)] = true
		}
	}
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".sql") {
			return nil
		}
		if !known[p] {
			panic("queries: embedded file is not registered: " + p)
		}
		return nil
	})
	if err != nil {
		panic("queries: walking embedded files: " + err.Error())
	}
}

func mustRead(p string) string {
	b, err := files.ReadFile(p)
	if err != nil {
		// Unreachable for registered names: go:embed fails to compile if the
		// directory is missing, and the registration check covers the rest.
		panic("queries: missing embedded file " + p + ": " + err.Error())
	}
	return string(b)
}

// Render fills a query's template fields.
//
// data is normally a struct whose fields are the conditional fragments the
// query composes -- named, so that adding or reordering one cannot silently
// change which fragment lands where.
func Render(name Name, data any) (string, error) {
	tmpl, ok := templates[name]
	if !ok {
		// Unreachable through the Name constants; reachable if a caller
		// converts a string.
		return "", fmt.Errorf("queries: unknown query %q", name)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("queries: rendering %s: %w", name, err)
	}
	return sb.String(), nil
}
