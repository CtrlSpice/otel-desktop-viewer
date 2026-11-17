package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/jsonrpc2"
)

// TestSearchMetricSummariesInvalidParams ensures searchMetricSummaries with wrong param count returns ErrInvalidParams.
func TestSearchMetricSummariesInvalidParams(t *testing.T) {
	handler := setupHandler(t)

	req := createRequest("searchMetricSummaries", []string{"0"}) // only one param
	result, err := handler.Handle(context.Background(), req)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
}

func TestClearMetrics(t *testing.T) {
	handler := setupHandlerWithMetrics(t)
	ctx := context.Background()

	result, err := handler.Handle(ctx, createRequest("clearMetrics", nil))
	assert.NoError(t, err)
	assert.Equal(t, "Metrics cleared successfully", result)

	searchResult, err := handler.Handle(ctx, createRequest("searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	assert.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var summaries []map[string]any
	assert.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, 0)
}

func TestDeleteMetric(t *testing.T) {
	handler := setupHandlerWithMetrics(t)
	ctx := context.Background()

	maxNano := strconv.FormatInt(1<<63-1, 10)
	searchResult, err := handler.Handle(ctx, createRequest("searchMetricSummaries", []string{"0", maxNano}))
	require.NoError(t, err)
	raw, ok := searchResult.(json.RawMessage)
	require.True(t, ok)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(raw, &summaries))
	require.NotEmpty(t, summaries, "fixture must provide at least one Metric")
	metricRef, ok := summaries[0]["metricRef"].(string)
	require.True(t, ok)
	before := len(summaries)

	result, err := handler.Handle(ctx, createRequest("deleteMetric", []string{metricRef}))
	assert.NoError(t, err)
	assert.Equal(t, "Metric deleted successfully", result)

	// Only the deleted Metric disappears from search.
	searchResult, err = handler.Handle(ctx, createRequest("searchMetricSummaries", []string{"0", maxNano}))
	require.NoError(t, err)
	raw, ok = searchResult.(json.RawMessage)
	require.True(t, ok)
	require.NoError(t, json.Unmarshal(raw, &summaries))
	assert.Len(t, summaries, before-1, "exactly one Metric should be gone")
	for _, s := range summaries {
		assert.NotEqual(t, metricRef, s["metricRef"], "deleted Metric must not reappear")
	}
}

// TestDeleteMetricNotFound covers deleting a Metric that does not exist.
// The cascade is a series of unconditional DELETEs, so this is a no-op rather
// than an error -- the UI relies on that when a poll races a delete.
func TestDeleteMetricNotFound(t *testing.T) {
	handler := setupHandlerWithMetrics(t)

	result, err := handler.Handle(context.Background(),
		createRequest("deleteMetric", []string{"00000000-0000-0000-0000-0000000000ff"}))
	assert.NoError(t, err)
	assert.Equal(t, "Metric deleted successfully", result)
}

func TestGetMetricAttributeDefinitions(t *testing.T) {
	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

		result, err := handler.Handle(context.Background(), createRequest("getMetricAttributeDefinitions", []any{}))
		assert.NoError(t, err)
		assertAttributeDiscovery(t, result)
	})

	t.Run("Invalid Parameters", func(t *testing.T) {
		handler := setupHandler(t)

		result, err := handler.Handle(context.Background(), createRequest("getMetricAttributeDefinitions", []string{"123"}))
		assert.Nil(t, result)
		assert.Equal(t, jsonrpc2.ErrInvalidParams, err)
	})
}

func TestSearchMetricSummaries(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		handler := setupHandler(t)

		req := createRequest("searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		assert.NoError(t, json.Unmarshal(raw, &summaries))
		assert.Len(t, summaries, 0)
	})

	t.Run("With Data", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

		req := createRequest("searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1, "should return one metric summary")
		assert.Equal(t, "test.gauge", summaries[0]["name"])
		assert.NotContains(t, summaries[0], "description")
		assert.Equal(t, "test-svc", summaries[0]["serviceName"])
		assert.Equal(t, "Gauge", summaries[0]["metricType"])
		assert.Equal(t, "bytes", summaries[0]["unit"])
		assert.NotEmpty(t, summaries[0]["metricRef"])
		assert.NotNil(t, summaries[0]["seriesCount"])
		assert.NotNil(t, summaries[0]["lastValue"])
	})

	t.Run("With Query", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

		query := map[string]any{
			"id":   "q1",
			"type": "condition",
			"query": map[string]any{
				"field":         map[string]any{"name": "name", "searchScope": "field"},
				"fieldOperator": "=",
				"value":         "test.gauge",
			},
		}
		req := createRequest("searchMetricSummaries", []any{
			"0", strconv.FormatInt(1<<63-1, 10), query,
		})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1)
		assert.Equal(t, "test.gauge", summaries[0]["name"])
	})

	t.Run("Limit", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)
		maxTime := strconv.FormatInt(1<<63-1, 10)

		result, err := handler.Handle(context.Background(), createRequest("searchMetricSummaries", []any{"0", maxTime, nil, 1}))
		require.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		require.Len(t, summaries, 1)

		result, err = handler.Handle(context.Background(), createRequest("searchMetricSummaries", []any{"0", maxTime, nil, 0}))
		assert.Nil(t, result)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	})
}

func TestGetMetricView(t *testing.T) {
	t.Run("Found", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

		summaryReq := createRequest("searchMetricSummaries", []string{
			"0", strconv.FormatInt(1<<63-1, 10),
		})
		summaryResult, err := handler.Handle(context.Background(), summaryReq)
		require.NoError(t, err)
		summaryRaw, ok := summaryResult.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(summaryRaw, &summaries))
		require.Len(t, summaries, 1)
		metricRef, ok := summaries[0]["metricRef"].(string)
		require.True(t, ok)
		require.NotEmpty(t, metricRef)

		req := createRequest("getMetricView", []any{
			metricRef, "0", strconv.FormatInt(1<<63-1, 10),
		})
		result, err := handler.Handle(context.Background(), req)

		assert.NoError(t, err)
		raw, ok := result.(json.RawMessage)
		assert.True(t, ok, "Expected json.RawMessage, got %T", result)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		assert.Equal(t, "test.gauge", metric["name"])
		assert.Equal(t, "bytes", metric["unit"])
		// MetricViewData is grouped by timeseries (per attribute set)
		// rather than a flat datapoint list. Each timeseries owns the
		// attributes for its group plus the pure-OTLP datapoints.
		timeseries, _ := metric["timeseries"].([]any)
		require.Len(t, timeseries, 1, "should have one timeseries")
		ts, _ := timeseries[0].(map[string]any)
		require.NotNil(t, ts, "timeseries must be a JSON object")
		assert.Contains(t, ts, "seriesRef", "timeseries should expose its grouping key")
		assert.Contains(t, ts, "attributes", "timeseries should own its attribute set")
		dps, _ := ts["datapoints"].([]any)
		assert.Len(t, dps, 1, "should have one datapoint inside the timeseries")
	})

	t.Run("Not Found", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

		req := createRequest("getMetricView", []any{
			"00000000-0000-0000-0000-000000000000",
			"0", strconv.FormatInt(1<<63-1, 10),
		})
		result, err := handler.Handle(context.Background(), req)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrMetricNotFound, err,
			"not-found must use the shared error convention, not a null result")
	})

	// A known Metric queried over a window with no datapoints is NOT a
	// not-found: it returns valid MetricViewData with an empty timeseries list.
	// Only an unknown Metric reference gets ErrMetricNotFound.
	t.Run("Known Metric, Empty Window", func(t *testing.T) {
		handler := setupHandlerWithMetrics(t)

		summaryReq := createRequest("searchMetricSummaries", []string{
			"0", strconv.FormatInt(1<<63-1, 10),
		})
		summaryResult, err := handler.Handle(context.Background(), summaryReq)
		require.NoError(t, err)
		summaryRaw, ok := summaryResult.(json.RawMessage)
		require.True(t, ok)
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(summaryRaw, &summaries))
		require.Len(t, summaries, 1)
		metricRef, ok := summaries[0]["metricRef"].(string)
		require.True(t, ok)

		// Test data is timestamped time.Now(); the window [0, 1] ns is
		// guaranteed to miss it.
		req := createRequest("getMetricView", []any{metricRef, "0", "1"})
		result, err := handler.Handle(context.Background(), req)

		require.NoError(t, err,
			"an empty window on a known Metric must not be treated as not-found")
		raw, ok := result.(json.RawMessage)
		require.True(t, ok, "Expected json.RawMessage, got %T", result)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		assert.Equal(t, "test.gauge", metric["name"])
		assert.Equal(t, "bytes", metric["unit"])
		timeseries, ok := metric["timeseries"].([]any)
		require.True(t, ok, "timeseries must be a JSON array, got %T", metric["timeseries"])
		assert.Empty(t, timeseries)
	})
}

func TestGetMetricAndSeries(t *testing.T) {
	handler := setupHandlerWithMetrics(t)
	summaryResult, err := handler.Handle(context.Background(), createRequest(
		"searchMetricSummaries", []any{nil, nil}))
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summaryResult.(json.RawMessage), &summaries))
	require.Len(t, summaries, 1)
	metricRef := summaries[0]["metricRef"].(string)

	discoveryResult, err := handler.Handle(context.Background(), createRequest(
		"getMetric", map[string]any{"metricRef": metricRef}))
	require.NoError(t, err)
	var discovery map[string]any
	require.NoError(t, json.Unmarshal(discoveryResult.(json.RawMessage), &discovery))
	require.Equal(t, metricRef, discovery["metricRef"])
	require.Equal(t, "test.gauge", discovery["name"])
	require.Equal(t, "Gauge", discovery["metricType"])
	require.Equal(t, "A test gauge", discovery["description"])
	require.NotContains(t, discovery, "datapoints")
	series := discovery["series"].([]any)
	require.Len(t, series, 1)
	seriesSummary := series[0].(map[string]any)
	require.Equal(t, "1", seriesSummary["datapointCount"])
	require.NotNil(t, seriesSummary["firstDatapointTimestamp"])
	require.Equal(t, seriesSummary["firstDatapointTimestamp"], seriesSummary["lastDatapointTimestamp"])

	seriesResult, err := handler.Handle(context.Background(), createRequest(
		"getMetricSeries", map[string]any{
			"metricRef": metricRef, "seriesRef": seriesSummary["seriesRef"],
			"startTime": nil, "endTime": nil,
		}))
	require.NoError(t, err)
	var selected map[string]any
	require.NoError(t, json.Unmarshal(seriesResult.(json.RawMessage), &selected))
	require.Equal(t, metricRef, selected["metricRef"])
	require.Equal(t, seriesSummary["seriesRef"], selected["seriesRef"])
	require.Len(t, selected["datapoints"].([]any), 1)

	emptyResult, err := handler.Handle(context.Background(), createRequest(
		"getMetricSeries", []any{metricRef, seriesSummary["seriesRef"], "0", "1"}))
	require.NoError(t, err)
	var empty map[string]any
	require.NoError(t, json.Unmarshal(emptyResult.(json.RawMessage), &empty))
	require.Empty(t, empty["datapoints"])

	missingResult, err := handler.Handle(context.Background(), createRequest(
		"getMetric", []any{"00000000-0000-0000-0000-000000000000"}))
	require.Nil(t, missingResult)
	require.Equal(t, ErrMetricNotFound, err)
	missingSeriesResult, err := handler.Handle(context.Background(), createRequest(
		"getMetricSeries", []any{metricRef, "00000000-0000-0000-0000-000000000000", nil, nil}))
	require.Nil(t, missingSeriesResult)
	require.Equal(t, ErrMetricNotFound, err)
}

func TestMetricHandlersAcceptNullableBoundsPositionallyAndByName(t *testing.T) {
	handler := setupHandlerWithMetrics(t)

	summaryResult, err := handler.Handle(context.Background(), createRequest(
		"searchMetricSummaries", []any{nil, nil}))
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summaryResult.(json.RawMessage), &summaries))
	require.Len(t, summaries, 1)
	metricRef := summaries[0]["metricRef"].(string)

	for _, tc := range rpcNullableRangeCases() {
		t.Run("getMetric/"+tc.name, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest(
				"getMetricView", metricRangeParams(metricRef, tc)))
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(result.(json.RawMessage), &got))
			require.Equal(t, "test.gauge", got["name"])
			require.Equal(t, float64(1), got["datapointCount"])

			lastSeen := got["lastSeenNs"].(string)
			window := got["window"].(map[string]any)
			requested := window["requested"].(map[string]any)
			effective := window["effective"].(map[string]any)
			require.Equal(t, map[string]any{"startNs": tc.start, "endNs": tc.end}, requested)
			wantEffectiveStart := tc.start
			if wantEffectiveStart == nil {
				wantEffectiveStart = lastSeen
			}
			wantEffectiveEnd := tc.end
			if wantEffectiveEnd == nil {
				wantEffectiveEnd = lastSeen
			}
			require.Equal(t, map[string]any{
				"startNs": wantEffectiveStart, "endNs": wantEffectiveEnd,
			}, effective)
		})

		t.Run("getMetricAggregateView/"+tc.name, func(t *testing.T) {
			result, err := handler.Handle(context.Background(), createRequest(
				"getMetricAggregateView", metricRangeParams(metricRef, tc)))
			require.NoError(t, err)
			require.JSONEq(t,
				`{"aggregate":null,"scalarAggregate":{"selected":[],"all":[]}}`,
				string(result.(json.RawMessage)))
		})
	}
}

// TestMetricHandlersAcceptEveryParameter pins both contiguous positional
// contracts and their distinct final arities.
func TestMetricHandlersAcceptEveryParameter(t *testing.T) {
	handler := setupHandlerWithMetrics(t)

	summaryResult, err := handler.Handle(context.Background(), createRequest(
		"searchMetricSummaries", []string{"0", strconv.FormatInt(1<<63-1, 10)}))
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summaryResult.(json.RawMessage), &summaries))
	require.NotEmpty(t, summaries)
	metricRef := summaries[0]["metricRef"].(string)
	maxTime := strconv.FormatInt(1<<63-1, 10)

	detail := []any{
		metricRef,        // 1 Metric
		"0",              // 2 start
		maxTime,          // 3 end
		"100",            // 4 targetBuckets
		[]any{},          // 5 seriesRefs
		[]any{0.5, 0.95}, // 6 quantiles
		"0",              // 7 tzOffsetNs
		"120",            // 8 viewBuckets
		"64",             // 9 sparklineBuckets
		[]any{},          // 10 selectedSeriesRefs
		"Europe/London",  // 11 tzName
		[]any{},          // 12 datapointSeriesRefs
		"10",             // 13 datapointSeriesLimit
	}
	aggregate := []any{
		metricRef,        // 1 Metric
		"0",              // 2 start
		maxTime,          // 3 end
		"100",            // 4 targetBuckets
		[]any{},          // 5 seriesRefs
		[]any{0.5, 0.95}, // 6 quantiles
		"0",              // 7 tzOffsetNs
		"120",            // 8 viewBuckets
		[]any{},          // 9 selectedSeriesRefs
		"Europe/London",  // 10 tzName
	}

	for method, full := range map[string][]any{
		"getMetricView": detail, "getMetricAggregateView": aggregate,
	} {
		t.Run(method, func(t *testing.T) {
			for n := 3; n <= len(full); n++ {
				t.Run(fmt.Sprintf("%d params", n), func(t *testing.T) {
					result, err := handler.Handle(context.Background(),
						createRequest(method, full[:n]))
					require.NoErrorf(t, err, "%s with %d parameters", method, n)
					require.NotNil(t, result)
				})
			}

			_, err := handler.Handle(context.Background(),
				createRequest(method, append(append([]any{}, full...), "extra")))
			assert.Error(t, err, "a parameter beyond the known list must be refused")
		})
	}
}

func TestMetricNamedParamContracts(t *testing.T) {
	require.Equal(t, []string{"metricRef"}, methodParamNames["getMetric"])
	require.Equal(t, []string{"metricRef", "seriesRef", "startTime", "endTime", "includeDatapoints"}, methodParamNames["getMetricSeries"])
	require.Equal(t, []string{
		"metricRef", "startTime", "endTime", "targetBuckets", "seriesRefs",
		"quantiles", "tzOffsetNs", "viewBuckets", "sparklineBuckets",
		"selectedSeriesRefs", "tzName", "datapointSeriesRefs", "datapointSeriesLimit",
	}, methodParamNames["getMetricView"])
	require.Equal(t, []string{
		"metricRef", "startTime", "endTime", "targetBuckets", "seriesRefs",
		"quantiles", "tzOffsetNs", "viewBuckets", "selectedSeriesRefs", "tzName", "datapointRef",
	}, methodParamNames["getMetricAggregateView"])
}

// TestDeleteMetricTakesOneRef verifies that the method takes a bare reference,
// not a list.
func TestDeleteMetricTakesOneRef(t *testing.T) {
	got, err := normalizeParams("deleteMetric", json.RawMessage(`{"metricRef":"s1"}`))
	require.NoError(t, err)
	require.JSONEq(t, `["s1"]`, string(got))

	_, err = normalizeParams("deleteMetric", json.RawMessage(`{"metricRefs":["s1"]}`))
	require.Error(t, err, "the plural name must not silently work")
}

func TestParseGetMetricViewParamsLayouts(t *testing.T) {
	metricRef := "00000000-0000-0000-0000-000000000001"
	detailRaw := json.RawMessage(fmt.Sprintf(`[%q,null,20,4,[],[0.5],-3600,8,9,["selected"],"Europe/London",[],10]`, metricRef))
	detail, err := parseGetMetricViewParams(detailRaw, false)
	require.NoError(t, err)
	assert.Equal(t, metricRef, detail.metricRef)
	assert.NotNil(t, detail.seriesRefs)
	assert.Empty(t, detail.seriesRefs)
	assert.Equal(t, []float64{0.5}, detail.quantiles)
	assert.Equal(t, int64(-3600), detail.tzOffsetNs)
	assert.Equal(t, int64(9), detail.sparklineBuckets)
	assert.Equal(t, []string{"selected"}, detail.selectedSeriesRefs)
	assert.Equal(t, "Europe/London", detail.tzName)
	assert.NotNil(t, detail.datapointSeriesRefs)
	assert.Empty(t, detail.datapointSeriesRefs)
	assert.Equal(t, int64(10), detail.datapointSeriesLimit)

	aggregateRaw := json.RawMessage(fmt.Sprintf(`[%q,10,null,4,null,null,null,8,["selected"],"UTC"]`, metricRef))
	aggregate, err := parseGetMetricViewParams(aggregateRaw, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"selected"}, aggregate.selectedSeriesRefs)
	assert.Equal(t, "UTC", aggregate.tzName)
	assert.Zero(t, aggregate.sparklineBuckets)
	assert.Nil(t, aggregate.datapointSeriesRefs)
}

func TestParseGetMetricSeriesParams(t *testing.T) {
	metricRef := "00000000-0000-0000-0000-000000000001"
	seriesRef := "00000000-0000-0000-0000-000000000002"
	got, err := parseGetMetricSeriesParams(json.RawMessage(fmt.Sprintf(`[%q,%q,null,"18446744073709551615"]`, metricRef, seriesRef)))
	require.NoError(t, err)
	assert.Equal(t, metricRef, got.metricRef)
	assert.Equal(t, seriesRef, got.seriesRef)
	assert.Nil(t, got.timeRange.Start)
	require.NotNil(t, got.timeRange.End)
	assert.Equal(t, uint64(math.MaxUint64), *got.timeRange.End)
}

func TestParseOptionalQuantilesPreservesValuesOrderAndPresence(t *testing.T) {
	absent, err := parseOptionalQuantiles([]any{}, 0)
	require.NoError(t, err)
	assert.Nil(t, absent)

	empty, err := parseOptionalQuantiles([]any{[]any{}}, 0)
	require.NoError(t, err)
	assert.Nil(t, empty)

	values, err := parseOptionalQuantiles([]any{[]any{
		json.Number("0.12345678901234566"),
		json.Number("0.9876543210987654"),
		float64(0.5),
	}}, 0)
	require.NoError(t, err)
	assert.Equal(t, []float64{0.12345678901234566, 0.9876543210987654, 0.5}, values)
	assert.Equal(t, []uint64{
		math.Float64bits(0.12345678901234566),
		math.Float64bits(0.9876543210987654),
		math.Float64bits(0.5),
	}, []uint64{
		math.Float64bits(values[0]),
		math.Float64bits(values[1]),
		math.Float64bits(values[2]),
	})

	for _, params := range [][]any{
		{[]any{json.Number("1e999")}},
		{[]any{json.Number("1.1")}},
		{[]any{"0.5"}},
	} {
		_, err := parseOptionalQuantiles(params, 0)
		assert.ErrorIs(t, err, jsonrpc2.ErrInvalidParams)
	}
}
