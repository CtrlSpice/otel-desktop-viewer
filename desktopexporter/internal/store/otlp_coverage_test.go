package store

import (
	"database/sql"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// TestOTLPFieldCoverage walks every field OTLP gives us and asserts the store
// has somewhere to put it.
//
// Two fields were being dropped when this was written, and both were found by
// doing this comparison by hand: Span.Flags and SpanLink.Flags (the W3C
// sampled bit and whether the parent context was remote), then Metric.Metadata.
// Nothing failed while they were missing -- ingest simply never read them, and
// a span read back out of the store was quietly not the span that went in.
//
// The point is that OTLP keeps growing. When the next field is added, this
// fails and someone decides what to do with it, instead of the field being
// silently discarded until a person happens to diff two lists again.
//
// Failing is not the same as "must store it". Some fields legitimately live
// elsewhere or are deliberately not kept -- that is what livesElsewhere
// records, and adding an entry there is a decision, written down.
func TestOTLPFieldCoverage(t *testing.T) {
	s, _, teardown := setupStore(t)
	defer teardown()

	columns := map[string]map[string]bool{}
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.Query(`select table_name, column_name from duckdb_columns()`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var table, col string
			if err := rows.Scan(&table, &col); err != nil {
				return err
			}
			if columns[table] == nil {
				columns[table] = map[string]bool{}
			}
			columns[table][col] = true
		}
		return rows.Err()
	}))

	// Fields that are real, and are not columns on the owner's table. Each
	// entry says where the value actually goes, so an unexplained gap cannot
	// hide among the explained ones.
	livesElsewhere := map[string]string{
		// Child collections get their own tables.
		"Span.Events":                             "events table",
		"Span.Links":                              "links table",
		"NumberDataPoint.Exemplars":               "exemplars table",
		"HistogramDataPoint.Exemplars":            "exemplars table",
		"ExponentialHistogramDataPoint.Exemplars": "exemplars table",
		"HistogramDataPoint.ExplicitBounds":       "histogram_bounds table, content-hashed",
		"SummaryDataPoint.QuantileValues":         "datapoints, as the summary quantile columns",

		// Attribute maps go into the shared dictionary as uuid[] references.
		"Span.Attributes":                          "attributes dictionary via attribute_ids",
		"SpanEvent.Attributes":                     "attributes dictionary via attribute_ids",
		"SpanLink.Attributes":                      "attributes dictionary via attribute_ids",
		"LogRecord.Attributes":                     "attributes dictionary via attribute_ids",
		"NumberDataPoint.Attributes":               "attributes dictionary via attribute_ids",
		"HistogramDataPoint.Attributes":            "attributes dictionary via attribute_ids",
		"ExponentialHistogramDataPoint.Attributes": "attributes dictionary via attribute_ids",
		"SummaryDataPoint.Attributes":              "attributes dictionary via attribute_ids",
		"Exemplar.FilteredAttributes":              "attributes dictionary via attribute_ids",
		"Metric.Metadata":                          "metric_ingests.metadata_ids, scope 'metadata'",

		// Status is flattened onto the span rather than nested.
		"Span.Status": "spans.status_code / status_message",

		// The oneof: which arm is set is recorded as metric_type, and the arm's
		// datapoints are rows in the datapoints table.
		"Metric.Gauge":                "metric_streams.metric_type + datapoints",
		"Metric.Sum":                  "metric_streams.metric_type + datapoints",
		"Metric.Histogram":            "metric_streams.metric_type + datapoints",
		"Metric.ExponentialHistogram": "metric_streams.metric_type + datapoints",
		"Metric.Summary":              "metric_streams.metric_type + datapoints",
		"Metric.Description":          "metric_ingests.description -- per batch, not identity",

		// Bucket structure is stored as its own columns rather than as the
		// nested Buckets messages pdata exposes.
		"ExponentialHistogramDataPoint.Positive": "datapoints positive bucket columns",
		"ExponentialHistogramDataPoint.Negative": "datapoints negative bucket columns",

		// Body is a value, not a map, and has its own columns.
		"LogRecord.Body": "logs body columns",
	}

	cases := []struct {
		name  string
		val   any
		table string
	}{
		{"Span", ptrace.NewSpan(), "spans"},
		{"SpanEvent", ptrace.NewSpanEvent(), "events"},
		{"SpanLink", ptrace.NewSpanLink(), "links"},
		{"LogRecord", plog.NewLogRecord(), "logs"},
		{"NumberDataPoint", pmetric.NewNumberDataPoint(), "datapoints"},
		{"HistogramDataPoint", pmetric.NewHistogramDataPoint(), "datapoints"},
		{"ExponentialHistogramDataPoint", pmetric.NewExponentialHistogramDataPoint(), "datapoints"},
		{"SummaryDataPoint", pmetric.NewSummaryDataPoint(), "datapoints"},
		{"Exemplar", pmetric.NewExemplar(), "exemplars"},
		{"Metric", pmetric.NewMetric(), "metric_streams"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cols := columns[tc.table]
			require.NotEmpty(t, cols, "no such table: %s", tc.table)

			for _, field := range getters(tc.val) {
				key := tc.name + "." + field
				if where, ok := livesElsewhere[key]; ok {
					require.NotEmpty(t, where)
					continue
				}
				require.True(t, hasColumn(cols, field),
					"%s is in OTLP but has nowhere to go.\n"+
						"Either add a column to %s, or add %q to livesElsewhere "+
						"saying where the value is kept.", key, tc.table, key)
			}
		})
	}
}

// getters returns the zero-argument, single-result methods of a pdata value:
// its fields, as pdata exposes them.
func getters(v any) []string {
	t := reflect.TypeOf(v)
	var out []string
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		// Setters, mutators and pdata plumbing are not fields.
		if strings.HasPrefix(m.Name, "Set") || strings.HasPrefix(m.Name, "Remove") {
			continue
		}
		switch m.Name {
		case "MoveTo", "CopyTo", "Type", "ValueType", "Equal":
			continue
		}
		// Getters take only the receiver and return exactly one value.
		if m.Type.NumIn() != 1 || m.Type.NumOut() != 1 {
			continue
		}
		out = append(out, m.Name)
	}
	return out
}

var (
	acronyms      = strings.NewReplacer("ID", "Id", "URL", "Url")
	camelBoundary = regexp.MustCompile(`(.)([A-Z])`)
)

// hasColumn matches a pdata field name against snake_case columns, allowing
// the shape differences the schema uses deliberately: timestamps stored as
// _time, counts suffixed, attribute maps stored as _ids arrays.
func hasColumn(cols map[string]bool, field string) bool {
	snake := strings.ToLower(camelBoundary.
		ReplaceAllString(acronyms.Replace(field), "${1}_${2}"))

	for _, candidate := range []string{
		snake,
		strings.TrimSuffix(snake, "_unix_nano"),
		strings.ReplaceAll(snake, "timestamp", "time"),
		snake + "_ids",
		snake + "_id",
		snake + "_count",
	} {
		if cols[candidate] {
			return true
		}
	}
	// Last resort: a column that contains the field name, which covers
	// start_time_unix_nano vs start_time and similar.
	for col := range cols {
		if strings.Contains(col, snake) || strings.Contains(snake, col) {
			return true
		}
	}
	return false
}
