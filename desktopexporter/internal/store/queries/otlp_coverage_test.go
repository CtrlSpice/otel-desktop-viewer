package queries_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// TestSchemaCoversOTLP verifies that every received OTLP field has a storage
// location.
//
// Columns come from an executed schema rather than parsed SQL. Name matching
// detects absent storage but cannot prove that a matching column is used correctly.
func TestSchemaCoversOTLP(t *testing.T) {
	db := macroDB(t)

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
	// Each entry names a table and column that must exist.
	type storedAt struct{ table, column string }
	elsewhere := map[string]storedAt{
		"NumberDataPoint.Exemplars":               {"exemplars", "metric_datapoint_id"},
		"HistogramDataPoint.Exemplars":            {"exemplars", "metric_datapoint_id"},
		"ExponentialHistogramDataPoint.Exemplars": {"exemplars", "metric_datapoint_id"},
		// Field is plural and prefixed; the column is the ordinary one.
		"Exemplar.FilteredAttributes":            {"exemplars", "attribute_ids"},
		"HistogramDataPoint.ExplicitBounds":      {"metric_datapoints", "bounds_id"},
		"ExponentialHistogramDataPoint.Positive": {"metric_datapoints", "positive_bucket_counts"},
		"ExponentialHistogramDataPoint.Negative": {"metric_datapoints", "negative_bucket_counts"},
		// The oneof carrying a metric's datapoints. Not a field in its own
		// right; which arm is set is metrics.metric_type.
		"Metric.Gauge":                {"metrics", "metric_type"},
		"Metric.Sum":                  {"metrics", "metric_type"},
		"Metric.Histogram":            {"metrics", "metric_type"},
		"Metric.ExponentialHistogram": {"metrics", "metric_type"},
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
	cases := []struct {
		what   string // bare pdata type name, used to key the maps above
		val    any
		tables []string
	}{
		{"Span", ptrace.NewSpan(), []string{"spans"}},
		{"SpanEvent", ptrace.NewSpanEvent(), []string{"events"}},
		{"SpanLink", ptrace.NewSpanLink(), []string{"links"}},
		{"LogRecord", plog.NewLogRecord(), []string{"logs"}},
		{"NumberDataPoint", pmetric.NewNumberDataPoint(), []string{"metric_datapoints"}},
		{"HistogramDataPoint", pmetric.NewHistogramDataPoint(), []string{"metric_datapoints"}},
		{"ExponentialHistogramDataPoint", pmetric.NewExponentialHistogramDataPoint(), []string{"metric_datapoints"}},
		{"Exemplar", pmetric.NewExemplar(), []string{"exemplars"}},
		{"Metric", pmetric.NewMetric(), []string{"metrics"}},
	}

	// Fields OTLP defines that this store does not keep, on purpose.
	//
	// Distinct from `elsewhere`, and deliberately so: that map says "stored,
	// look over there" and is verified against a real column. This one says
	// "not stored, and that is the decision" -- so the test does not fail over
	// it, and nobody has to rediscover why it is absent. If one ever becomes
	// supported, delete the line and the test starts guarding it.
	notSupported := map[string]string{
		"Metric.Summary": "Summary is not supported. Its quantiles are precomputed and " +
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
				key := tc.what + "." + m.Name
				if _, ok := elsewhere[key]; ok {
					continue
				}
				if _, ok := notSupported[key]; ok {
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
