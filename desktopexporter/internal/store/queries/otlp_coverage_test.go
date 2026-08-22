package queries_test

import (
	"database/sql"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// TestSchemaCoversOTLP walks every field OTLP gives us and fails if the schema
// has nowhere to put it.
//
// This exists because two fields were being silently discarded and nothing
// said so: Span.Flags and SpanLink.Flags were dropped at ingest while logs and
// datapoints kept theirs, and Metric.Metadata was never read at all. Each was
// found by hand, by diffing pdata against the tables one type at a time. That
// is not a thing to do on a schedule, and OTLP keeps adding fields.
//
// A dropped field is invisible in the worst way. It does not fail a build, a
// query, or a round trip -- the value simply never arrives, so the store holds
// something subtly less than what was sent and every export made from it is
// wrong in a way that looks fine.
//
// Two choices worth stating. Columns come from duckdb_columns() against a
// schema this test actually creates, not from parsing the .sql files, so a
// column that fails to materialise cannot pass. And fields are matched by
// name, which is loose -- a column named for a field it does not really hold
// would satisfy this. It catches absence, which is the failure that happened
// twice, not misuse.
func TestSchemaCoversOTLP(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()

	for _, stmt := range queries.Types() {
		db.Exec(stmt.SQL) // "already exists" is fine
	}
	for _, stmt := range queries.Tables() {
		_, err := db.Exec(stmt.SQL)
		require.NoErrorf(t, err, "%s", stmt.Name)
	}

	columns := func(table string) map[string]bool {
		rows, err := db.Query(
			`select column_name from duckdb_columns() where table_name = ?`, table)
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var c string
			require.NoError(t, rows.Scan(&c))
			out[c] = true
		}
		require.NotEmpty(t, out, "table %q has no columns; did it fail to create?", table)
		return out
	}

	// Fields OTLP puts on one message that this schema deliberately keeps
	// somewhere else. Each entry is a claim that the value is stored, just not
	// in this table -- so removing one should make the test fail, not pass.
	elsewhere := map[string]string{
		"Attributes":         "attributes dictionary, via attribute_ids",
		"FilteredAttributes": "attributes dictionary, via attribute_ids",
		"Metadata":           "attributes dictionary, via metadata_ids",
		"Exemplars":          "exemplars table",
		"ExplicitBounds":     "histogram_bounds table, hashed and shared",
		"QuantileValues":     "datapoints, as the summary quantile columns",
		"Positive":           "datapoints, as the exponential bucket columns",
		"Negative":           "datapoints, as the exponential bucket columns",
		"Description":        "metric_ingests: per batch, not stream identity",
		// The oneof carrying a metric's datapoints. Not a field in its own
		// right; which arm is set is metric_streams.metric_type.
		"Gauge":                "datapoints, selected by metric_type",
		"Sum":                  "datapoints, selected by metric_type",
		"Histogram":            "datapoints, selected by metric_type",
		"ExponentialHistogram": "datapoints, selected by metric_type",
		"Summary":              "datapoints, selected by metric_type",
	}

	// Accessors that describe pdata's own plumbing rather than OTLP content.
	notAField := map[string]bool{
		"MoveTo": true, "CopyTo": true, "Type": true, "ValueType": true,
		"AsRaw": true, "AsString": true, "Len": true, "At": true,
	}

	// CamelCase -> snake_case. Written out rather than with a regexp because
	// Go's RE2 has no lookahead, and acronyms are folded first so SpanID
	// becomes span_id rather than span_i_d.
	snake := func(s string) string {
		s = strings.NewReplacer("ID", "Id", "URL", "Url").Replace(s)
		var b strings.Builder
		for i, r := range s {
			if i > 0 && r >= 'A' && r <= 'Z' {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		}
		return strings.ToLower(b.String())
	}

	cases := []struct {
		what  string
		val   any
		table string
	}{
		{"ptrace.Span", ptrace.NewSpan(), "spans"},
		{"ptrace.SpanEvent", ptrace.NewSpanEvent(), "events"},
		{"ptrace.SpanLink", ptrace.NewSpanLink(), "links"},
		{"plog.LogRecord", plog.NewLogRecord(), "logs"},
		{"pmetric.NumberDataPoint", pmetric.NewNumberDataPoint(), "datapoints"},
		{"pmetric.HistogramDataPoint", pmetric.NewHistogramDataPoint(), "datapoints"},
		{"pmetric.ExponentialHistogramDataPoint", pmetric.NewExponentialHistogramDataPoint(), "datapoints"},
		{"pmetric.SummaryDataPoint", pmetric.NewSummaryDataPoint(), "datapoints"},
		{"pmetric.Exemplar", pmetric.NewExemplar(), "exemplars"},
		{"pmetric.Metric", pmetric.NewMetric(), "metric_streams"},
	}

	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			cols := columns(tc.table)
			typ := reflect.TypeOf(tc.val)

			var missing []string
			for i := 0; i < typ.NumMethod(); i++ {
				m := typ.Method(i)
				// Getters take no argument and return one value.
				if m.Type.NumIn() != 1 || m.Type.NumOut() != 1 {
					continue
				}
				if strings.HasPrefix(m.Name, "Set") || notAField[m.Name] {
					continue
				}
				if _, ok := elsewhere[m.Name]; ok {
					continue
				}

				n := snake(m.Name)
				candidates := []string{
					n,
					strings.TrimSuffix(n, "_unix_nano"),
					strings.ReplaceAll(n, "timestamp", "time"),
					n + "_ids", n + "_id", n + "_count",
				}
				found := false
				for _, c := range candidates {
					if cols[c] {
						found = true
						break
					}
				}
				// Last resort: a column whose name contains the field, for the
				// handful spelled differently (status -> status_code).
				if !found {
					for c := range cols {
						if strings.Contains(c, n) || strings.Contains(n, c) {
							found = true
							break
						}
					}
				}
				if !found {
					missing = append(missing, m.Name)
				}
			}

			sort.Strings(missing)
			require.Emptyf(t, missing,
				"%s has fields with nowhere to go in %q: %s\n\n"+
					"Either add a column, or -- if the value is stored in another "+
					"table -- add it to the `elsewhere` map above saying where.",
				tc.what, tc.table, strings.Join(missing, ", "))
		})
	}
}
