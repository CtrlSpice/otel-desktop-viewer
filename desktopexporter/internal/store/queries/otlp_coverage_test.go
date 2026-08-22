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

	// Fields whose value lives in a shape the name cannot find: a different
	// table, or columns spelled nothing like the field.
	//
	// Each entry names the table and column that must exist. That is the whole
	// point -- an exception that merely skipped the field would turn this test
	// off for it, which is exactly the bug it is meant to catch. Verified: a
	// first draft listed Metadata as a comment string, and deleting
	// metric_ingests.metadata_ids still passed.
	type storedAt struct{ table, column string }
	elsewhere := map[string]storedAt{
		"Exemplars": {"exemplars", "datapoint_id"},
		// Field is plural and prefixed; the column is the ordinary one.
		"FilteredAttributes": {"exemplars", "attribute_ids"},
		"ExplicitBounds":     {"datapoints", "bounds_id"},
		"Positive":           {"datapoints", "positive_bucket_counts"},
		"Negative":           {"datapoints", "negative_bucket_counts"},
		// The oneof carrying a metric's datapoints. Not a field in its own
		// right; which arm is set is metric_streams.metric_type.
		"Gauge":                {"metric_streams", "metric_type"},
		"Sum":                  {"metric_streams", "metric_type"},
		"Histogram":            {"metric_streams", "metric_type"},
		"ExponentialHistogram": {"metric_streams", "metric_type"},
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

	// pmetric.SummaryDataPoint is deliberately absent, and that absence is the
	// finding rather than an oversight: eachDatapoint handles Gauge, Sum,
	// Histogram and ExponentialHistogram, and nothing else. A Summary metric
	// is dropped whole at ingest -- not one field of it, all of it -- so there
	// is no table to check it against. Listing it here with an excuse would
	// have hidden that. Filed separately.
	//
	// A pdata message can map to more than one table: a Metric's identity is
	// metric_streams while its per-batch fields (description, metadata) are on
	// metric_ingests, and both are "stored".
	cases := []struct {
		what   string
		val    any
		tables []string
	}{
		{"ptrace.Span", ptrace.NewSpan(), []string{"spans"}},
		{"ptrace.SpanEvent", ptrace.NewSpanEvent(), []string{"events"}},
		{"ptrace.SpanLink", ptrace.NewSpanLink(), []string{"links"}},
		{"plog.LogRecord", plog.NewLogRecord(), []string{"logs"}},
		{"pmetric.NumberDataPoint", pmetric.NewNumberDataPoint(), []string{"datapoints"}},
		{"pmetric.HistogramDataPoint", pmetric.NewHistogramDataPoint(), []string{"datapoints"}},
		{"pmetric.ExponentialHistogramDataPoint", pmetric.NewExponentialHistogramDataPoint(), []string{"datapoints"}},
		{"pmetric.Exemplar", pmetric.NewExemplar(), []string{"exemplars"}},
		{"pmetric.Metric", pmetric.NewMetric(), []string{"metric_streams", "metric_ingests"}},
	}

	// Fields OTLP defines that this store does not keep, on purpose.
	//
	// Distinct from `elsewhere`, and deliberately so: that map says "stored,
	// look over there" and is verified against a real column. This one says
	// "not stored, and that is the decision" -- so the test does not fail over
	// it, and nobody has to rediscover why it is absent. If one ever becomes
	// supported, delete the line and the test starts guarding it.
	notSupported := map[string]string{
		"Summary": "Summary is not supported. Its quantiles are precomputed and " +
			"'cannot always be merged in a meaningful way' (metrics.proto), so it " +
			"does not fit the aggregation path histograms use. eachDatapoint " +
			"handles Gauge, Sum, Histogram and ExponentialHistogram only.",
	}

	// Every exception must point at a column that is really there.
	for field, at := range elsewhere {
		require.Truef(t, columns(at.table)[at.column],
			"%s is excused as living in %s.%s, but that column does not exist",
			field, at.table, at.column)
	}

	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			cols := map[string]bool{}
			for _, tbl := range tc.tables {
				for c := range columns(tbl) {
					cols[c] = true
				}
			}
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
				if _, ok := notSupported[m.Name]; ok {
					continue
				}

				n := snake(m.Name)
				candidates := []string{
					n,
					strings.TrimSuffix(n, "_unix_nano"),
					strings.ReplaceAll(n, "timestamp", "time"),
					n + "_ids", n + "_id", n + "_count",
					// Attributes -> attribute_ids: field plural, column singular.
					strings.TrimSuffix(n, "s") + "_ids",
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
				"%s has fields with nowhere to go in %v: %s\n\n"+
					"Either add a column, or -- if the value is stored in another "+
					"table -- add it to the `elsewhere` map above saying where.",
				tc.what, tc.tables, strings.Join(missing, ", "))
		})
	}
}
