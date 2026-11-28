package metrics_test

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestSearchMetricSummariesNullableTimeRangesExecute(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("nullable.summary")
	gauge := metric.SetEmptyGauge()
	for _, timestamp := range []int64{100, 200, 300} {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(timestamp))
		dp.SetIntValue(timestamp)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))

	start, end := uint64(200), uint64(200)
	for _, tc := range []struct {
		name          string
		timeRange     store.TimeRange
		wantCount     float64
		wantLastSeen  string
		wantLastValue float64
	}{
		{"unbounded", store.TimeRange{}, 3, "300", 300},
		{"end only", store.TimeRange{End: &end}, 2, "200", 200},
		{"start only", store.TimeRange{Start: &start}, 2, "300", 300},
		{"bounded", store.TimeRange{Start: &start, End: &end}, 1, "200", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.SearchSummaries(ctx, db, tc.timeRange, nil)
			})
			require.NoError(t, err)
			var got []map[string]any
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Len(t, got, 1)
			require.Equal(t, "nullable.summary", got[0]["name"])
			require.Equal(t, tc.wantCount, got[0]["dataPointCount"])
			require.Equal(t, tc.wantLastSeen, got[0]["lastSeen"])
			require.Equal(t, tc.wantLastValue, got[0]["lastValue"])
		})
	}
}

func TestSearchMetricSummariesLimit(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	}))

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummariesWithLimit(ctx, db, store.BoundedTimeRange(0, maxNano), nil, 2)
	})
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.Len(t, summaries, 2)
	require.Equal(t, "exponential_histogram_metric", summaries[0]["name"])
	require.Equal(t, "histogram_metric", summaries[1]["name"])

	_, err = storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummariesWithLimit(ctx, db, store.BoundedTimeRange(0, maxNano), nil, 0)
	})
	require.ErrorIs(t, err, metrics.ErrInvalidMetricLimit)
}

func TestSearchMetricSummariesSortBeforeLimit(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "sort-test")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("sort-test")
	base := time.Now().UnixNano()
	for _, fixture := range []struct {
		name      string
		timestamp int64
		points    int
	}{
		{name: "dense", timestamp: base, points: 3},
		{name: "sparse", timestamp: base + int64(time.Second), points: 1},
	} {
		metric := sm.Metrics().AppendEmpty()
		metric.SetName(fixture.name)
		gauge := metric.SetEmptyGauge()
		for i := 0; i < fixture.points; i++ {
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetTimestamp(pcommon.Timestamp(fixture.timestamp + int64(i)))
			dp.SetDoubleValue(float64(i))
			dp.Attributes().PutInt("series", int64(i))
		}
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))

	limit := int64(1)
	for _, field := range []string{"dataPointCount", "seriesCount"} {
		t.Run(field, func(t *testing.T) {
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.SearchSummariesWithOptions(ctx, db, store.BoundedTimeRange(0, maxNano), nil, search.ResultOptions{
					Limit: &limit,
					Sort:  &search.Sort{Field: field, Direction: "desc"},
				})

			})
			require.NoError(t, err)
			var summaries []map[string]any
			require.NoError(t, json.Unmarshal(raw, &summaries))
			require.Len(t, summaries, 1)
			require.Equal(t, "dense", summaries[0]["name"], "aggregate sort must run before LIMIT")
		})
	}
}

// TestMetricSuite runs tests on ingested metrics using SearchMetrics (DB-generated JSON).
func TestMetricSuite(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	err := s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	})
	assert.NoError(t, err, "ingest test metrics")

	t.Run("MetricRetrieval", func(t *testing.T) {
		metrics := searchMetricsAll(t, s, ctx)
		assert.Len(t, metrics, 5, "should have five metrics")
		names := make([]string, len(metrics))
		for i, m := range metrics {
			if n, ok := m["name"].(string); ok {
				names[i] = n
			}
		}
		assert.Contains(t, names, "gauge_metric")
		assert.Contains(t, names, "gauge_int_metric")
		assert.Contains(t, names, "sum_metric")
		assert.Contains(t, names, "histogram_metric")
		assert.Contains(t, names, "exponential_histogram_metric")
	})

	t.Run("GaugeMetric", func(t *testing.T) {
		gauge := getMetricFullByName(t, s, ctx, "gauge_metric")
		requireMetric(t, gauge, "gauge_metric")
		assert.Equal(t, "Current memory usage", gauge["description"])
		assert.Equal(t, "bytes", gauge["unit"])
		datapoints := metricDatapoints(gauge)
		assert.NotEmpty(t, datapoints)
		dp0, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp0)
		// DB returns doubleValue; value type may vary
		if v, ok := dp0["doubleValue"].(float64); ok {
			assert.Equal(t, 1024.5, v)
		}
	})

	t.Run("GaugeIntMetric", func(t *testing.T) {
		// Covers numberDataPointValue Int branch: return nil, dp.IntValue(), typeStr
		m := getMetricFullByName(t, s, ctx, "gauge_int_metric")
		requireMetric(t, m, "gauge_int_metric")
		datapoints := metricDatapoints(m)
		assert.Len(t, datapoints, 1)
		dp, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp)
		assert.Equal(t, "Int", dp["valueType"], "valueType for integer datapoint")
		assert.Equal(t, "42", dp["intValue"])
	})

	t.Run("SumMetric", func(t *testing.T) {
		sum := getMetricFullByName(t, s, ctx, "sum_metric")
		requireMetric(t, sum, "sum_metric")
		assert.Equal(t, "Total requests processed", sum["description"])
		datapoints := metricDatapoints(sum)
		assert.NotEmpty(t, datapoints)
	})

	t.Run("HistogramMetric", func(t *testing.T) {
		hist := getMetricFullByName(t, s, ctx, "histogram_metric")
		requireMetric(t, hist, "histogram_metric")
		datapoints := metricDatapoints(hist)
		assert.NotEmpty(t, datapoints)
		dp, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp)
		assert.Equal(t, "100", dp["count"])
		assert.Equal(t, 25.5, dp["sum"])
	})

	t.Run("ExponentialHistogramMetric", func(t *testing.T) {
		exp := getMetricFullByName(t, s, ctx, "exponential_histogram_metric")
		requireMetric(t, exp, "exponential_histogram_metric")
		datapoints := metricDatapoints(exp)
		assert.NotEmpty(t, datapoints)
		dp, _ := datapoints[0].(map[string]any)
		assert.NotNil(t, dp)
		assert.Equal(t, "50", dp["count"])
		assert.Equal(t, float64(2), dp["scale"])
	})

	t.Run("MetricResourceAndScope", func(t *testing.T) {
		for _, name := range []string{
			"gauge_metric", "gauge_int_metric", "sum_metric",
			"histogram_metric", "exponential_histogram_metric",
		} {
			m := getMetricFullByName(t, s, ctx, name)
			resource, _ := m["resource"].(map[string]any)
			assert.NotNil(t, resource, "metric %s resource", name)
			scope, _ := m["scope"].(map[string]any)
			assert.NotNil(t, scope, "metric %s scope", name)
			assert.Equal(t, "test-scope", scope["name"], "metric %s scope name", name)
			assert.Equal(t, "v1.0.0", scope["version"], "metric %s scope version", name)
		}
	})

	t.Run("Exemplars", func(t *testing.T) {
		assert.Greater(t, storetest.CountRows(t, s, ctx, "select count(*) from exemplars"), 0, "exemplars should be ingested")
		gauge := getMetricFullByName(t, s, ctx, "gauge_metric")
		requireMetric(t, gauge, "gauge_metric")
		datapoints := metricDatapoints(gauge)
		assert.NotEmpty(t, datapoints)
		dp0, _ := datapoints[0].(map[string]any)
		exemplars, _ := dp0["exemplars"].([]any)
		assert.Len(t, exemplars, 1, "gauge datapoint should have one exemplar")
		ex, _ := exemplars[0].(map[string]any)
		assert.Equal(t, "Double", ex["valueType"], "exemplar value type")
		assert.Equal(t, 1000.0, ex["doubleValue"], "exemplar double value")
		assert.Nil(t, ex["intValue"], "a double exemplar has no integer arm")
		assert.NotEmpty(t, ex["traceID"], "exemplar traceID")
		assert.NotEmpty(t, ex["spanID"], "exemplar spanID")
	})

	t.Run("QueryByServiceName", func(t *testing.T) {
		// Exercise ParseQueryTree(query) and BuildMetricSQL with a resource attribute condition.
		base := time.Now().UnixNano()
		startTime := base - int64(2*time.Hour)
		endTime := base + int64(2*time.Hour)
		query := map[string]any{
			"id":   "q1",
			"type": "condition",
			"query": map[string]any{
				"field": map[string]any{
					"name":           "service.name",
					"searchScope":    "attribute",
					"attributeScope": "resource",
				},
				"fieldOperator": "=",
				"value":         "test-service",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5, "should find all metrics for service name test-service")
		for i, m := range metrics {
			assert.Equal(t, "test-service", m["serviceName"], "metric %d serviceName", i)
		}
	})

	// Field expression tests (mapMetricFieldExpression cases)
	base := time.Now().UnixNano()
	startTime := base - int64(2*time.Hour)
	endTime := base + int64(2*time.Hour)

	t.Run("Field_name", func(t *testing.T) {
		query := map[string]any{
			"id":   "f1",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "gauge_metric",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 1)
		assert.Equal(t, "gauge_metric", metrics[0]["name"])
	})

	t.Run("Field_description", func(t *testing.T) {
		query := map[string]any{
			"id":   "f2",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "description", "searchScope": "field"},
				"fieldOperator": "CONTAINS",
				"value":         "memory",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 1)
		assert.Equal(t, "gauge_metric", metrics[0]["name"])
	})

	t.Run("Field_unit", func(t *testing.T) {
		query := map[string]any{
			"id":   "f3",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "unit", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "bytes",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 2) // gauge_metric, exponential_histogram_metric
		names := make([]string, len(metrics))
		for i, m := range metrics {
			names[i] = m["name"].(string)
		}
		assert.Contains(t, names, "gauge_metric")
		assert.Contains(t, names, "exponential_histogram_metric")
	})

	t.Run("Field_scope.name", func(t *testing.T) {
		query := map[string]any{
			"id":   "f4",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "scope.name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "test-scope",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
	})

	t.Run("Field_scope.version", func(t *testing.T) {
		query := map[string]any{
			"id":   "f5",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "scope.version", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "v1.0.0",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5)
	})

	t.Run("Field_droppedAttributesCount", func(t *testing.T) {
		for name, value := range map[string]string{
			"resource.droppedAttributesCount": "7",
			"scope.droppedAttributesCount":    "9",
		} {
			query := map[string]any{
				"id":   "f6",
				"type": "condition",
				"query": map[string]any{
					"field":         map[string]any{"name": name, "searchScope": "field"},
					"fieldOperator": "=",
					"value":         value,
				},
			}
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
			})
			assert.NoError(t, err, name)
			var results []map[string]any
			assert.NoError(t, json.Unmarshal(raw, &results), name)
			assert.Len(t, results, 5, name)
		}
	})

	t.Run("Rejects_legacy_field_aliases", func(t *testing.T) {
		for _, name := range []string{
			"scopeName",
			"scopeVersion",
			"resourceDroppedAttributesCount",
			"scopeDroppedAttributesCount",
		} {
			query := map[string]any{
				"id":   "legacy-field",
				"type": "condition",
				"query": map[string]any{
					"field":         map[string]any{"name": name, "searchScope": "field"},
					"fieldOperator": "=",
					"value":         "value",
				},
			}
			_, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
			})
			assert.ErrorIs(t, err, metrics.ErrInvalidMetricQuery, name)
		}
	})

	// Global search (mapMetricGlobalExpressions: explicit fields + attributes)
	t.Run("GlobalSearch_Description", func(t *testing.T) {
		query := map[string]any{
			"id":   "g1",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "memory",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 1, "description contains 'memory' (gauge description)")
		assert.Equal(t, "gauge_metric", metrics[0]["name"])
	})

	t.Run("GlobalSearch_Attribute", func(t *testing.T) {
		query := map[string]any{
			"id":   "g2",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "test-service",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Len(t, metrics, 5, "resource attribute service.name = test-service")
	})

	t.Run("GlobalSearch_NoResults", func(t *testing.T) {
		query := map[string]any{
			"id":   "g3",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"searchScope": "global"},
				"fieldOperator": "CONTAINS",
				"value":         "nonexistent-metric-xyz",
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(startTime, endTime), query)
		})
		assert.NoError(t, err)
		var metrics []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &metrics))
		assert.Empty(t, metrics)
	})
}

// TestSearchSummaries_CardFields verifies the slim summary projection used
// by drawer cards: Metric reference, series count, scalar last value, last seen.
func TestSearchSummaries_CardFields(t *testing.T) {
	t.Parallel()
	t.Run("Gauge", func(t *testing.T) {
		s, ctx := storetest.New(t)

		ts := time.Unix(1700000000, 0)
		md := pmetric.NewMetrics()
		rm := md.ResourceMetrics().AppendEmpty()
		rm.Resource().Attributes().PutStr("service.name", "gauge-svc")
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("test-scope")
		m := sm.Metrics().AppendEmpty()
		m.SetName("gauge_card_test")
		m.SetDescription("Memory used by the process")
		g := m.SetEmptyGauge()
		dp1 := g.DataPoints().AppendEmpty()
		dp1.SetTimestamp(pcommon.Timestamp(ts.UnixNano()))
		dp1.SetDoubleValue(1.0)
		dp2 := g.DataPoints().AppendEmpty()
		dp2.SetTimestamp(pcommon.Timestamp(ts.Add(time.Second).UnixNano()))
		dp2.SetDoubleValue(2.0)
		dp2.Attributes().PutStr("host", "a")

		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))

		summary := findSummary(t, searchSummariesAll(t, s, ctx), "gauge_card_test")
		assert.NotEmpty(t, summary["metricRef"])
		assert.NotContains(t, summary, "description")
		assert.EqualValues(t, 2, summary["seriesCount"])
		assert.EqualValues(t, 2, summary["dataPointCount"])
		assert.InDelta(t, 2.0, summary["lastValue"], 1e-9)
		assert.NotEmpty(t, summary["lastSeen"])
	})

	t.Run("HistogramOmitsLastValue", func(t *testing.T) {
		s, ctx := storetest.New(t)

		bounds := []float64{1.0, 2.0}
		ts := time.Unix(1700000000, 0)
		md := makeHistogramFixtureT("hist_card_test", pmetric.AggregationTemporalityDelta, []histTestDP{
			{timestamp: ts, attrs: map[string]string{"host": "a"},
				bounds: bounds, counts: []uint64{1, 2, 3},
				count: 6, sum: 7.0, min: 0.5, max: 2.5},
		})
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, md)
		}))

		summary := findSummary(t, searchSummariesAll(t, s, ctx), "hist_card_test")
		assert.NotEmpty(t, summary["metricRef"])
		assert.EqualValues(t, 1, summary["seriesCount"])
		assert.EqualValues(t, 1, summary["dataPointCount"])
		assert.Nil(t, summary["lastValue"])
	})
}

// Datapoint and exemplar labels are searchable through their owned maps.
func TestMetricSearch_DatapointAndExemplarLabels(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	}))

	search := func(t *testing.T, scope, name, op, value string) []map[string]any {
		t.Helper()
		query := map[string]any{
			"type": "condition",
			"query": map[string]any{
				"field": map[string]any{
					"name":           name,
					"searchScope":    "attribute",
					"attributeScope": scope,
				},
				"fieldOperator": op,
				"value":         value,
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, time.Now().UnixNano()+int64(time.Hour)), query)
		})
		require.NoError(t, err)
		var out []map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	t.Run("datapoint label matches its own metric only", func(t *testing.T) {
		// memory.type=heap is on the gauge metric's datapoint; the other four
		// fixture metrics must not match, or the predicate is not actually
		// filtering.
		got := search(t, "datapoint", "memory.type", "=", "heap")
		require.Len(t, got, 1)
		assert.Equal(t, "gauge_metric", got[0]["name"])
	})

	t.Run("datapoint label with a non-matching value finds nothing", func(t *testing.T) {
		assert.Empty(t, search(t, "datapoint", "memory.type", "=", "stack"))
	})

	// Mutation-driven: without these, making the key predicate always-true
	// (`a.key = ? or true`) left every other subtest passing, because each
	// fixture label value happens to be unique. These pair a real key with
	// another key's value, so a predicate that ignores the key over-matches.
	t.Run("the key is part of the match, not just the value", func(t *testing.T) {
		// "int" is a real datapoint label value -- but under key "type", not
		// "memory.type". Ignoring the key would return the int metric.
		assert.Empty(t, search(t, "datapoint", "memory.type", "=", "int"),
			"memory.type is heap; int belongs to a different key")

		// A key that exists nowhere, paired with a value that does.
		assert.Empty(t, search(t, "datapoint", "no.such.key", "=", "heap"),
			"a non-existent key must match nothing regardless of the value")

		// Same for exemplars.
		assert.Empty(t, search(t, "exemplar", "no.such.key", "=", "gauge"))
	})

	// Owner scope is explicit, so a datapoint search must not reach
	// an exemplar label and vice versa -- even though both live in the same
	// attributes table.
	t.Run("scopes do not leak into each other", func(t *testing.T) {
		assert.Empty(t, search(t, "datapoint", "exemplar.source", "=", "gauge"),
			"exemplar.source is not a datapoint label")
		assert.Empty(t, search(t, "exemplar", "memory.type", "=", "heap"),
			"memory.type is not an exemplar label")
	})

	t.Run("exemplar label", func(t *testing.T) {
		got := search(t, "exemplar", "exemplar.source", "=", "histogram")
		require.Len(t, got, 1)
		assert.Equal(t, "histogram_metric", got[0]["name"])
	})

	t.Run("exemplar labels distinguish metrics", func(t *testing.T) {
		// Every fixture metric carries exemplar.source, with a different value
		// each, so this pins that the value is compared and not merely the key.
		for _, tc := range []struct{ value, metric string }{
			{"gauge", "gauge_metric"},
			{"sum", "sum_metric"},
			{"exponential_histogram", "exponential_histogram_metric"},
		} {
			got := search(t, "exemplar", "exemplar.source", "=", tc.value)
			require.Len(t, got, 1, "value %q", tc.value)
			assert.Equal(t, tc.metric, got[0]["name"])
		}
	})

	t.Run("LIKE on a datapoint label", func(t *testing.T) {
		got := search(t, "datapoint", "memory.type", "CONTAINS", "hea")
		require.Len(t, got, 1)
		assert.Equal(t, "gauge_metric", got[0]["name"])
	})

	t.Run("global free-text reaches datapoint and exemplar labels", func(t *testing.T) {
		global := func(term string) []map[string]any {
			query := map[string]any{
				"type": "condition",
				"query": map[string]any{
					"field":         map[string]any{"searchScope": "global"},
					"fieldOperator": "CONTAINS",
					"value":         term,
				},
			}
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, time.Now().UnixNano()+int64(time.Hour)), query)
			})
			require.NoError(t, err)
			var out []map[string]any
			require.NoError(t, json.Unmarshal(raw, &out))
			return out
		}
		// "heap" appears only as a datapoint label value.
		got := global("heap")
		require.Len(t, got, 1, "global search must reach datapoint labels")
		assert.Equal(t, "gauge_metric", got[0]["name"])

		// "exponential_histogram" appears as an exemplar label value.
		assert.NotEmpty(t, global("exponential_histogram"),
			"global search must reach exemplar labels")
	})
}

// TestSearchSummariesByMetricMetadata covers metadata equality, general value
// matching, and attribute discovery.
func TestSearchSummariesByMetricMetadata(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	base := time.Now().Add(-time.Minute)

	md := makeSumFixtureT("meta.metric", pmetric.AggregationTemporalityCumulative, []sumTestDP{{
		timestamp: base, value: 1, attrs: map[string]string{"r": "/x"},
	}})
	md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).
		Metadata().PutStr("workload.type", "batch")
	plain := makeSumFixtureT("meta.plain", pmetric.AggregationTemporalityCumulative, []sumTestDP{{
		timestamp: base, value: 1, attrs: map[string]string{"r": "/y"},
	}})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		if err := metrics.Ingest(ctx, conn, md); err != nil {
			return err
		}
		return metrics.Ingest(ctx, conn, plain)
	}))
	end := time.Now().UnixNano() + int64(time.Hour)

	run := func(operator, value string) []map[string]any {
		query := &search.QueryNode{
			ID:   "q1",
			Type: "condition",
			Query: &search.Query{
				Field: &search.FieldDefinition{
					Name: "workload.type", SearchScope: "attribute",
					AttributeScope: "metadata", Type: "string",
				},
				FieldOperator: operator,
				Value:         value,
			},
		}
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, end), query)
		})
		require.NoError(t, err)
		var out []map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	// Equality takes the IDProbe fast path; CONTAINS takes attr_value.
	eq := run("=", "batch")
	require.Len(t, eq, 1)
	assert.Equal(t, "meta.metric", eq[0]["name"])
	like := run("CONTAINS", "atc")
	require.Len(t, like, 1)
	assert.Equal(t, "meta.metric", like[0]["name"])
	assert.Empty(t, run("=", "interactive"))

	// Discovery lists the key under its scope, so the dropdown can offer it.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAttributeDefinitions(ctx, db)
	})
	require.NoError(t, err)
	var attrs []map[string]any
	require.NoError(t, json.Unmarshal(raw, &attrs))
	found := false
	for _, a := range attrs {
		if a["name"] == "workload.type" && a["attributeScope"] == "metadata" {
			found = true
		}
	}
	assert.True(t, found, "discovery must surface metadata keys: %v", attrs)
}

func TestMetricTypeMembershipExecution(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata())
	}))

	count := func(operator, value string) int {
		t.Helper()
		query := &search.QueryNode{Type: "condition", Query: &search.Query{
			Field:         &search.FieldDefinition{Name: "type", SearchScope: "field"},
			FieldOperator: operator,
			Value:         value,
		}}
		var raw json.RawMessage
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			var err error
			raw, err = metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, time.Now().Add(time.Hour).UnixNano()), query)
			return err
		}))
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		return len(summaries)
	}

	assert.Equal(t, 2, count("IN", `["Gauge"]`))
	assert.Equal(t, 3, count("NOT IN", `["Gauge"]`))
}

func TestNestedComparisonPayloadLoadsOnlyOnDemand(t *testing.T) {
	var viewBytes []int
	for _, size := range []int{1024, 65536} {
		s, ctx := storetest.New(t)
		data := pmetric.NewMetrics()
		m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		m.SetName("nested-only")
		points := m.SetEmptyGauge().DataPoints()
		const key = "context\"雪"
		for i := range 4 {
			p := points.AppendEmpty()
			p.SetTimestamp(100)
			p.SetIntValue(1)
			value := p.Attributes().PutEmptyMap(key)
			value.PutStr("prefix", strings.Repeat("x", size))
			value.PutInt("tail", int64(i)+9007199254740992)
			p.Attributes().PutStr("unrelated", "do not ship this attribute")
		}
		require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
		id := findMetricRef(t, s, ctx, "nested-only")
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(0, maxNano), 0, nil, nil, 0, 0, 0, nil, "", []string{}, 0)
		})
		require.NoError(t, err)
		require.NotContains(t, string(raw), strings.Repeat("x", 256))
		var view struct {
			Timeseries []struct {
				SeriesRef  string `json:"seriesRef"`
				Attributes []any  `json:"attributes"`
				Previews   []struct {
					Key  string `json:"key"`
					Kind string `json:"kind"`
				} `json:"labelPreviews"`
			} `json:"timeseries"`
		}
		require.NoError(t, json.Unmarshal(raw, &view))
		require.Len(t, view.Timeseries, 4)
		refs := make([]string, 0, 4)
		for _, row := range view.Timeseries {
			require.Empty(t, row.Attributes)
			require.Len(t, row.Previews, 1)
			require.Equal(t, key, row.Previews[0].Key)
			require.Equal(t, "map", row.Previews[0].Kind)
			refs = append(refs, row.SeriesRef)
		}
		_, err = storetest.Read(s, func(db *sql.DB) (int, error) {
			result, err := metrics.ViewArrow(ctx, db, id, store.BoundedTimeRange(0, maxNano), 0, nil, nil, 0, 0, 0, nil, "", []string{}, 0, false, "")
			if err != nil {
				return 0, err
			}
			defer result.Close()
			var wire bytes.Buffer
			if err := result.Write(ctx, &wire); err != nil {
				return 0, err
			}
			viewBytes = append(viewBytes, wire.Len())
			return wire.Len(), nil
		})
		require.NoError(t, err)
		order, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesOrder(ctx, db, id, store.BoundedTimeRange(100, 100), refs, key, false)
		})
		require.NoError(t, err)
		require.Less(t, len(order), 200, "sorting transfers IDs only regardless of nested value size")
		var orderedIDs []string
		require.NoError(t, json.Unmarshal(order, &orderedIDs))
		require.ElementsMatch(t, refs, orderedIDs)
		for index, ref := range orderedIDs {
			exact, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeries(ctx, db, id, ref, store.BoundedTimeRange(100, 100), false)
			})
			require.NoError(t, err)
			require.Contains(t, string(exact), strings.Repeat("x", size))
			require.Contains(t, string(exact), strconv.FormatInt(9007199254740992+int64(index), 10))
		}
		t.Logf("nested bytes/value=%d initial JSON=%d Arrow=%d sort-order JSON=%d", size, len(raw), viewBytes[len(viewBytes)-1], len(order))
	}
	// Increasing each comparison value by 63KiB must not increase the initial
	// response proportionally. IDs/previews have the same bounded shape.
	require.LessOrEqual(t, viewBytes[1], viewBytes[0]+256)
}

func TestSeriesLabelProjectionPreservesTypedDistinctionsAndReceivedContext(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "labels")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("labels.typed")
	points := m.SetEmptyGauge().DataPoints()
	fixtures := []func(pcommon.Map){
		func(a pcommon.Map) {},
		func(a pcommon.Map) { a.PutEmpty("varying") },
		func(a pcommon.Map) { a.PutStr("varying", "") },
		func(a pcommon.Map) { a.PutInt("varying", 9007199254740992) },
		func(a pcommon.Map) { a.PutInt("varying", 9007199254740993) },
		func(a pcommon.Map) { a.PutDouble("varying", 0) },
		func(a pcommon.Map) { a.PutDouble("varying", math.Copysign(0, -1)) },
		func(a pcommon.Map) { a.PutDouble("varying", math.Inf(1)) },
		func(a pcommon.Map) { a.PutDouble("varying", math.Inf(-1)) },
		func(a pcommon.Map) { a.PutDouble("varying", math.NaN()) },
		func(a pcommon.Map) { a.PutStr("varying", "NaN") },
		func(a pcommon.Map) { a.PutEmptyMap("varying").PutInt("n", 1) },
		func(a pcommon.Map) { a.PutEmptyMap("varying").PutDouble("n", 1) },
		func(a pcommon.Map) { a.PutEmptySlice("varying").AppendEmpty().SetDouble(math.Copysign(0, -1)) },
		func(a pcommon.Map) {
			a.PutEmptyMap("varying").PutEmptySlice("nested").AppendEmpty().SetStr(strings.Repeat("received ", 3000))
		},
	}
	for _, fixture := range fixtures {
		p := points.AppendEmpty()
		p.SetTimestamp(100)
		p.SetIntValue(1)
		p.Attributes().PutEmptyMap("common").PutEmptySlice("nested").AppendEmpty().SetStr("retained")
		fixture(p.Attributes())
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	metricID := findMetricRef(t, s, ctx, "labels.typed")
	for _, shipping := range [][]string{nil, {}} {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(0, maxNano), 0, nil, nil, 0, 0, 0, nil, "", shipping, 0)
		})
		require.NoError(t, err)
		var view struct {
			Timeseries []struct {
				SeriesRef  string           `json:"seriesRef"`
				Attributes []map[string]any `json:"attributes"`
				Complete   bool             `json:"attributesComplete"`
				Previews   []struct {
					Key  string `json:"key"`
					Text string `json:"text"`
				} `json:"labelPreviews"`
			} `json:"timeseries"`
		}
		require.NoError(t, json.Unmarshal(raw, &view))
		require.Len(t, view.Timeseries, len(fixtures))
		ids := make([]string, len(view.Timeseries))
		for i, series := range view.Timeseries {
			ids[i] = series.SeriesRef
		}
		signatures := make(map[string]bool)
		previews := make(map[string]bool)
		for _, series := range view.Timeseries {
			require.Empty(t, series.Attributes, "view carries previews, not exact comparison payloads")
			require.False(t, series.Complete)
			require.Len(t, series.Previews, 1)
			require.Equal(t, "varying", series.Previews[0].Key)
			require.Less(t, len(series.Previews[0].Text), 512)
			require.False(t, previews[series.Previews[0].Text], "bounded preview collisions must be disambiguated")
			previews[series.Previews[0].Text] = true
			received, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeries(ctx, db, metricID, series.SeriesRef, store.BoundedTimeRange(0, maxNano))
			})
			require.NoError(t, err)
			require.Contains(t, string(received), `"common"`)
			require.Contains(t, string(received), `"retained"`)
			var exact struct {
				Attributes []map[string]any `json:"attributes"`
			}
			require.NoError(t, json.Unmarshal(received, &exact))
			selected := []map[string]any{}
			for _, attr := range exact.Attributes {
				if attr["key"] == "varying" {
					selected = append(selected, attr)
				}
			}
			encoded, err := json.Marshal(selected)
			require.NoError(t, err)
			require.False(t, signatures[string(encoded)], "typed label candidates collapsed")
			signatures[string(encoded)] = true
			contextOnly, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeries(ctx, db, metricID, series.SeriesRef, store.BoundedTimeRange(0, maxNano), false)
			})
			require.NoError(t, err)
			require.Contains(t, string(contextOnly), `"datapoints":[]`)
			require.Contains(t, string(contextOnly), `"retained"`)
		}
		for _, refs := range [][]string{{}, {"00000000-0000-0000-0000-000000000000"}} {
			empty, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeriesOrder(ctx, db, metricID, store.BoundedTimeRange(0, maxNano), refs, "varying", false)
			})
			require.NoError(t, err)
			require.JSONEq(t, "[]", string(empty))
		}
		foreign, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesOrder(ctx, db, "00000000-0000-0000-0000-000000000000", store.BoundedTimeRange(0, maxNano), ids, "varying", false)
		})
		require.NoError(t, err)
		require.JSONEq(t, "[]", string(foreign))
		missing, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesOrder(ctx, db, metricID, store.BoundedTimeRange(0, maxNano), []string{ids[0], ids[0]}, "'; select 1; --", false)
		})
		require.NoError(t, err)
		require.JSONEq(t, `["`+ids[0]+`"]`, string(missing))
	}
}

func TestMetricSeriesOrderClampsBeforeSorting(t *testing.T) {
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("order")
	points := m.SetEmptyGauge().DataPoints()
	for i, name := range []string{"item10", "item2", "item1", "item0"} {
		p := points.AppendEmpty()
		p.SetTimestamp(pcommon.Timestamp(100 + i*100))
		p.SetIntValue(1)
		p.Attributes().PutStr("key", name)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	id := findMetricRef(t, s, ctx, "order")
	refs := map[string]string{}
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select id::varchar, attributes.key::varchar from metric_series where metric_id=$1::uuid`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref, name string
			require.NoError(t, rows.Scan(&ref, &name))
			refs[name] = ref
		}
		return rows.Err()
	}))
	for _, descending := range []bool{false, true} {
		result, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesOrder(ctx, db, id, store.BoundedTimeRange(100, 200), []string{refs["item10"], refs["item2"], refs["item1"], refs["item0"]}, "key", descending)
		})
		require.NoError(t, err)
		var order []string
		require.NoError(t, json.Unmarshal(result, &order))
		expected := []string{refs["item2"], refs["item10"]}
		if descending {
			expected[0], expected[1] = expected[1], expected[0]
		}
		require.Equal(t, expected, order)
	}
	start, end := uint64(200), uint64(100)
	for _, test := range []struct {
		name           string
		window         store.TimeRange
		refs, expected []string
	}{
		{"open end", store.TimeRange{Start: &start}, []string{refs["item10"], refs["item2"]}, []string{refs["item2"]}},
		{"open start", store.TimeRange{End: &end}, []string{refs["item10"], refs["item2"]}, []string{refs["item10"]}},
		{"cohort only", store.TimeRange{}, []string{refs["item2"], refs["item2"]}, []string{refs["item2"]}},
		{"reversed window", store.TimeRange{Start: &start, End: &end}, []string{refs["item10"], refs["item2"]}, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricSeriesOrder(ctx, db, id, test.window, test.refs, "key", false)
			})
			require.NoError(t, err)
			var order []string
			require.NoError(t, json.Unmarshal(raw, &order))
			require.Equal(t, test.expected, order)
		})
	}
}

func TestMetricSeriesOrderExactValuesAndNumbersFirst(t *testing.T) {
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("order.typed")
	points := m.SetEmptyGauge().DataPoints()
	const key = "'雪.[0]"
	fixtures := []struct {
		name  string
		value any
	}{
		{"negative-infinity", math.Inf(-1)}, {"min-int", int64(math.MinInt64)},
		{"negative-ten", int64(-10)}, {"negative-two", float64(-2)},
		{"minus-zero", math.Copysign(0, -1)}, {"double-zero", float64(0)}, {"int-zero", int64(0)},
		{"double-one", float64(1)}, {"int-one", int64(1)},
		{"rounded", float64(9007199254740992)}, {"exact", int64(9007199254740993)},
		{"max-int", int64(math.MaxInt64)}, {"above-max-int", float64(math.MaxInt64)},
		{"positive-infinity", math.Inf(1)}, {"nan", math.NaN()},
		{"numeric-string", "-5"}, {"empty-string", ""}, {"empty", nil},
		{"false", false}, {"true", true}, {"string-false", "false"}, {"string-nan", "NaN"},
		{"item2", "item2"}, {"item10", "item10"}, {"unicode2", "雪2"}, {"unicode10", "雪10"},
		{"lower", "e2"}, {"upper", "E2"}, {"accent", "é2"},
		{"nested2", map[string]any{"prefix": strings.Repeat("same", 1000), "tail": int64(2)}},
		{"nested10", map[string]any{"prefix": strings.Repeat("same", 1000), "tail": int64(10)}},
		{"tie-a", "same"}, {"tie-b", "same"},
		{"string-absent", "[absent]"},
	}
	for _, fixture := range fixtures {
		p := points.AppendEmpty()
		p.SetTimestamp(pcommon.Timestamp(math.MaxUint64))
		p.SetIntValue(1)
		p.Attributes().PutStr("fixture", fixture.name)
		require.NoError(t, p.Attributes().PutEmpty(key).FromRaw(fixture.value))
	}
	missing := points.AppendEmpty()
	missing.SetTimestamp(pcommon.Timestamp(math.MaxUint64))
	missing.SetIntValue(1)
	missing.Attributes().PutStr("fixture", "missing")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	var id string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `select id::varchar from metrics where name='order.typed'`).Scan(&id)
	}))
	byName := map[string]string{}
	byID := map[string]string{}
	var refs []string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select id::varchar, attributes.fixture::varchar from metric_series where metric_id=$1::uuid`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref, name string
			require.NoError(t, rows.Scan(&ref, &name))
			byName[name], byID[ref] = ref, name
			refs = append(refs, ref)
		}
		return rows.Err()
	}))
	numerics := []string{"negative-infinity", "min-int", "negative-ten", "negative-two", "minus-zero", "double-zero", "int-zero", "double-one", "int-one", "rounded", "exact", "max-int", "above-max-int", "positive-infinity", "nan"}
	for _, descending := range []bool{false, true} {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesOrder(ctx, db, id, store.BoundedTimeRange(uint64(math.MaxUint64), uint64(math.MaxUint64)), refs, key, descending)
		})
		require.NoError(t, err)
		var ids []string
		require.NoError(t, json.Unmarshal(raw, &ids))
		require.ElementsMatch(t, refs, ids)
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			result, err := metrics.SeriesOrderArrow(ctx, db, id, store.BoundedTimeRange(uint64(math.MaxUint64), uint64(math.MaxUint64)), refs, key, descending)
			if err != nil {
				return err
			}
			defer result.Close()
			require.Len(t, result.Schema().Fields(), 1)
			var wire bytes.Buffer
			require.NoError(t, result.Write(ctx, &wire))
			reader, err := ipc.NewReader(&wire)
			if err != nil {
				return err
			}
			defer reader.Release()
			var arrowIDs []string
			for reader.Next() {
				column := reader.RecordBatch().Column(0).(*array.String)
				for i := 0; i < column.Len(); i++ {
					arrowIDs = append(arrowIDs, column.Value(i))
				}
			}
			require.NoError(t, reader.Err())
			require.Equal(t, ids, arrowIDs)
			return nil
		}))
		names := make([]string, len(ids))
		for i, id := range ids {
			names[i] = byID[id]
		}
		expectedNumbers := slices.Clone(numerics)
		if descending {
			slices.Reverse(expectedNumbers)
		}
		require.Equal(t, expectedNumbers, names[:len(numerics)])
		for _, pair := range [][2]string{{"item2", "item10"}, {"unicode2", "unicode10"}, {"nested2", "nested10"}, {"lower", "upper"}, {"upper", "accent"}, {"false", "string-false"}, {"missing", "string-absent"}} {
			left, right := slices.Index(names, pair[0]), slices.Index(names, pair[1])
			if descending {
				left, right = right, left
			}
			require.Less(t, left, right, "%v in %v", pair, names)
		}
		ties := []string{byName["tie-a"], byName["tie-b"]}
		slices.Sort(ties)
		require.Less(t, slices.Index(ids, ties[0]), slices.Index(ids, ties[1]), "identity ties stay ascending")
	}
}
