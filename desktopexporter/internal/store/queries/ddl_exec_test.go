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

// Every DDL statement must actually execute. Compiling proves only that the
// strings are valid Go.
func TestAllDDLExecutes(t *testing.T) {
	db := freshDB(t)

	for _, stmt := range queries.Types() {
		db.Exec(stmt.SQL) // "already exists" is fine
	}
	// Report the failing statement by name rather than list position.
	for _, group := range [][]queries.Statement{
		queries.Tables(), queries.Indexes(), queries.Macros(),
	} {
		for _, stmt := range group {
			_, err := db.Exec(stmt.SQL)
			require.NoErrorf(t, err, "%s:\n%s", stmt.Name, stmt.SQL)
		}
	}
}

func TestNativeUnsignedColumnTypesAndNullability(t *testing.T) {
	db := freshDB(t)

	tests := []struct {
		table, column, dataType, nullable string
	}{
		{"spans", "trace_id", "UUID", "NO"},
		{"spans", "span_id", "UBIGINT", "NO"},
		{"spans", "parent_span_id", "UBIGINT", "YES"},
		{"spans", "start_time", "UBIGINT", "YES"},
		{"spans", "end_time", "UBIGINT", "YES"},
		// DuckDB reports projected view columns as nullable; owner identity
		// constraints are enforced on the owning tables.
		{"events", "id", "UUID", "YES"},
		{"events", "trace_id", "UUID", "YES"},
		{"events", "span_id", "UBIGINT", "YES"},
		{"events", "timestamp", "UBIGINT", "YES"},
		{"links", "id", "UUID", "YES"},
		{"links", "trace_id", "UUID", "YES"},
		{"links", "span_id", "UBIGINT", "YES"},
		{"links", "linked_trace_id", "UUID", "YES"},
		{"links", "linked_span_id", "UBIGINT", "YES"},
		{"logs", "id", "UUID", "NO"},
		{"logs", "trace_id", "UUID", "YES"},
		{"logs", "span_id", "UBIGINT", "YES"},
		{"logs", "timestamp", "UBIGINT", "YES"},
		{"logs", "observed_timestamp", "UBIGINT", "YES"},
		{"metric_datapoints", "timestamp", "UBIGINT", "YES"},
		{"metric_datapoints", "start_time", "UBIGINT", "YES"},
		{"exemplars", "id", "UUID", "YES"},
		{"exemplars", "metric_datapoint_id", "UUID", "YES"},
		{"exemplars", "trace_id", "UUID", "YES"},
		{"exemplars", "span_id", "UBIGINT", "YES"},
		{"exemplars", "timestamp", "UBIGINT", "YES"},
	}
	for _, tc := range tests {
		t.Run(tc.table+"/"+tc.column, func(t *testing.T) {
			var dataType, nullable string
			err := db.QueryRow(`
				select data_type, is_nullable
				from information_schema.columns
				where table_name = ? and column_name = ?`, tc.table, tc.column).
				Scan(&dataType, &nullable)
			require.NoError(t, err)
			require.Equal(t, tc.dataType, dataType)
			require.Equal(t, tc.nullable, nullable)
		})
	}
}

func TestExemplarValueColumnTypes(t *testing.T) {
	db := freshDB(t)

	for _, tc := range []struct{ column, dataType string }{
		{"double_value", "DOUBLE"},
		{"int_value", "BIGINT"},
	} {
		t.Run(tc.column, func(t *testing.T) {
			var dataType, nullable string
			err := db.QueryRow(`
				select data_type, is_nullable
				from information_schema.columns
				where table_name = 'exemplars' and column_name = ?`, tc.column).
				Scan(&dataType, &nullable)
			require.NoError(t, err)
			require.Equal(t, tc.dataType, dataType)
			require.Equal(t, "YES", nullable)
		})
	}
}

func TestSpanIDWireMacro(t *testing.T) {
	db := macroDB(t)

	for _, tc := range []struct {
		name, literal, want string
	}{
		{"leading zeros", "1", "0000000000000001"},
		{"high bit", "9223372036854775808", "8000000000000000"},
		{"max uint64", "18446744073709551615", "ffffffffffffffff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			require.NoError(t, db.QueryRow(
				"select span_id_wire("+tc.literal+"::ubigint)").Scan(&got))
			require.Equal(t, tc.want, got)
		})
	}

	var null sql.NullString
	require.NoError(t, db.QueryRow(`select span_id_wire(null::ubigint)`).Scan(&null))
	require.False(t, null.Valid)
}

func TestSignalsOwnNativePayloads(t *testing.T) {
	db := freshDB(t)
	var obsolete int
	require.NoError(t, db.QueryRow(`select count(*) from duckdb_tables()
		where table_name in ('attributes','resources','scopes')`).Scan(&obsolete))
	require.Zero(t, obsolete)
	for _, table := range []string{"spans", "logs", "metrics", "metric_datapoints"} {
		for _, column := range []string{"resource", "scope"} {
			var typ string
			require.NoError(t, db.QueryRow(`select data_type from duckdb_columns()
				where table_name=? and column_name=?`, table, column).Scan(&typ))
			require.Contains(t, typ, "attributes VARIANT", table+"."+column)
		}
	}
}

// DuckDB cannot index a LIST directly. Casting the complete list to VARCHAR is
// allowed because it retains every element while giving the index a scalar key.
func TestNoIndexOnArrayColumns(t *testing.T) {
	for _, stmt := range queries.Indexes() {
		require.NotContains(t, stmt.SQL, "(attribute_ids)",
			"DuckDB cannot index a LIST column directly; this would fail at store open")
		require.NotContains(t, stmt.SQL, "(resource_attribute_ids)",
			"DuckDB cannot index a LIST column directly; this would fail at store open")
		require.NotContains(t, stmt.SQL, "(scope_attribute_ids)",
			"DuckDB cannot index a LIST column directly; this would fail at store open")
	}
}

// Cross-signal references must stay unconstrained.
//
// logs and exemplars carry trace_id / span_id, and links carries the
// linked_trace_id / linked_span_id pair it points at. None of them may become a
// foreign key: signals
// arrive independently and out of order, so the span a log names may arrive
// later, be dropped by sampling, or never be sent. An FK would reject that
// telemetry on arrival -- most often for partial or failed traces, which is
// precisely what someone opens this tool to look at.
//
// No ingest test would catch it either: they all write complete traces, where
// the referenced span happens to exist. So the guard has to be on the DDL.
func TestCrossSignalReferencesAreNotForeignKeys(t *testing.T) {
	for _, tc := range []struct{ file, column string }{
		{"logs.sql", "trace_id"},
		{"logs.sql", "span_id"},
		{"exemplars.sql", "trace_id"},
		{"exemplars.sql", "span_id"},
		{"links.sql", "linked_trace_id"},
		{"links.sql", "linked_span_id"},
	} {
		t.Run(tc.file+"/"+tc.column, func(t *testing.T) {
			var ddl string
			for _, stmt := range queries.Tables() {
				if strings.HasSuffix(stmt.Name, tc.file) {
					ddl = stmt.SQL
				}
			}
			require.NotEmpty(t, ddl, "table file not found: %s", tc.file)
			require.NotContains(t, ddl, "foreign key ("+tc.column+")",
				"%s.%s must stay unconstrained: signals arrive out of order, and an FK "+
					"would reject logs and exemplars whose span has not arrived yet",
				tc.file, tc.column)
		})
	}
}

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
		"Exemplar.FilteredAttributes":            {"exemplars", "attributes"},
		"HistogramDataPoint.ExplicitBounds":      {"metric_datapoints", "explicit_bounds"},
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
