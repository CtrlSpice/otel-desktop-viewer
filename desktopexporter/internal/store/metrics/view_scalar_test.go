package metrics_test

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestMetricViewRetainsIdentityResourceOutsideDatapointWindow(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	timestamp := int64(1_700_000_000_000_000_000)
	md := buildInstanceMetrics(t, map[string]string{"host.name": "pod-a"}, 0, timestamp)
	rm := md.ResourceMetrics().At(0)
	rm.SetSchemaUrl("https://opentelemetry.io/schemas/1.29.0")
	rm.Resource().SetDroppedAttributesCount(7)
	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	sm.SetSchemaUrl("https://opentelemetry.io/schemas/1.30.0")
	sm.Scope().SetDroppedAttributesCount(5)
	sm.Scope().Attributes().PutStr("scope.build", "blue")
	metricPayload := sm.Metrics().At(0)
	metricPayload.SetDescription("received description")
	metricPayload.Metadata().PutStr("metadata.key", "metadata value")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string),
			store.BoundedTimeRange(timestamp+1, timestamp+2),
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	resource := metric["resource"].(map[string]any)
	attributeValues := map[string]any{}
	for _, rawAttribute := range resource["attributes"].([]any) {
		attribute := rawAttribute.(map[string]any)
		attributeValues[attribute["key"].(string)] = attribute["value"].(map[string]any)["value"]
	}
	assert.Equal(t, "checkout", attributeValues["service.name"])
	assert.Equal(t, "pod-a", attributeValues["host.name"])
	assert.Equal(t, "otelhttp", metric["scopeName"])
	assert.Equal(t, "1.2.0", metric["scopeVersion"])
	assert.Equal(t, "https://opentelemetry.io/schemas/1.29.0", metric["resourceSchemaUrl"])
	assert.Equal(t, "https://opentelemetry.io/schemas/1.30.0", metric["scopeSchemaUrl"])
	assert.Equal(t, "received description", metric["description"])
	assert.Equal(t, float64(7), metric["resourceDroppedAttributesCount"])
	assert.Equal(t, float64(5), metric["scopeDroppedAttributesCount"])
	metadata := metric["metadata"].([]any)
	require.Len(t, metadata, 1)
	assert.Equal(t, "metadata.key", metadata[0].(map[string]any)["key"])
	scope := metric["scope"].(map[string]any)
	scopeAttributeValues := map[string]any{}
	for _, rawAttribute := range scope["attributes"].([]any) {
		attribute := rawAttribute.(map[string]any)
		scopeAttributeValues[attribute["key"].(string)] = attribute["value"].(map[string]any)["value"]
	}
	assert.Equal(t, "blue", scopeAttributeValues["scope.build"])
	assert.Empty(t, metric["timeseries"])
}

func TestMetricViewProjectsMonotonicityOnlyForSum(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	md := pmetric.NewMetrics()
	sm := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()
	const timestamp = pcommon.Timestamp(100)

	gauge := sm.Metrics().AppendEmpty()
	gauge.SetName("gauge")
	gauge.SetEmptyGauge().DataPoints().AppendEmpty().SetTimestamp(timestamp)

	sum := sm.Metrics().AppendEmpty()
	sum.SetName("sum")
	sumData := sum.SetEmptySum()
	sumData.SetIsMonotonic(true)
	sumData.DataPoints().AppendEmpty().SetTimestamp(timestamp)

	histogram := sm.Metrics().AppendEmpty()
	histogram.SetName("histogram")
	histogram.SetEmptyHistogram().DataPoints().AppendEmpty().SetTimestamp(timestamp)

	exponential := sm.Metrics().AppendEmpty()
	exponential.SetName("exponential")
	exponential.SetEmptyExponentialHistogram().DataPoints().AppendEmpty().SetTimestamp(timestamp)

	summaryMetric := sm.Metrics().AppendEmpty()
	summaryMetric.SetName("summary")
	summaryMetric.SetEmptySummary().DataPoints().AppendEmpty().SetTimestamp(timestamp)

	var rejected ingest.Rejected
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		var err error
		rejected, err = metrics.IngestReport(ctx, conn, md)
		return err
	}))
	require.Equal(t, 1, rejected.Count())
	require.ErrorIs(t, rejected.Reason(), metrics.ErrUnsupportedMetricType)

	for _, summary := range searchMetricsAll(t, s, ctx) {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, summary["metricRef"].(string),
				store.BoundedTimeRange(101, 102), 0, nil, nil, 0, 0, 0, nil, "", nil, 0)

		})
		require.NoError(t, err)
		var metric map[string]any
		require.NoError(t, json.Unmarshal(raw, &metric))
		if summary["name"] == "sum" {
			assert.Equal(t, true, metric["isMonotonic"])
		} else {
			assert.Nil(t, metric["isMonotonic"], "%s monotonicity is not applicable", summary["name"])
		}
		if summary["name"] == "sum" || summary["name"] == "histogram" || summary["name"] == "exponential" {
			assert.Equal(t, float64(0), metric["aggregationTemporalityCode"])
		} else {
			assert.Nil(t, metric["aggregationTemporalityCode"], "%s temporality is not applicable", summary["name"])
			assert.Nil(t, metric["aggregationTemporality"], "%s temporality is not applicable", summary["name"])
		}
	}
	require.Equal(t, 0, storetest.CountRows(t, s, ctx, `select count(*) from metrics where name = 'summary'`))
}

// TestGetMetricView_MergedSeriesKeepTheirLabels verifies that reduced histogram
// series retain the attributes that identify them.
func TestGetMetricView_MergedSeriesKeepTheirLabels(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// Two series, told apart only by their labels, each with two datapoints
	// close enough to merge.
	dp := func(d time.Duration, driver string) histTestDP {
		return histTestDP{
			timestamp: base.Add(d),
			attrs:     map[string]string{"driver": driver},
			bounds:    []float64{1, 2},
			counts:    []uint64{1, 0, 0},
			count:     1, sum: 0.5, min: 0.5, max: 0.5,
		}
	}
	fixture := makeHistogramFixtureT("labelled.duration", pmetric.AggregationTemporalityDelta, []histTestDP{
		dp(0, "ALO"), dp(time.Second, "ALO"),
		dp(0, "VER"), dp(time.Second, "VER"),
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Target 1 forces the path under test to merge each series.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.BoundedTimeRange(
			base.Add(-time.Hour).UnixNano(), base.Add(time.Hour).UnixNano()),

			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	series, _ := metric["timeseries"].([]any)
	require.Len(t, series, 2, "two series")

	drivers := map[string]bool{}
	for _, entry := range series {
		attrs, _ := entry.(map[string]any)["labelPreviews"].([]any)
		require.NotEmpty(t, attrs,
			"a merged series must carry the previews that identify it")
		for _, a := range attrs {
			m := a.(map[string]any)
			if m["key"] == "driver" {
				drivers[m["text"].(string)] = true
			}
		}
	}
	assert.Equal(t, map[string]bool{"ALO": true, "VER": true}, drivers,
		"each merged series keeps its own label, not a blank or a neighbour's")
}

// TestGetMetricView_ScalarViewBuckets covers the grid the Sum / Average / Rate
// views aggregate on. Every series uses the same absolute ladder boundaries.
//
// The other half is what an empty bucket means. It comes back null rather than
// zero, because Sum and Rate read a gap as no activity while Average has to
// skip it -- the mean of nothing is not nought. Emitting 0 would bake one
// view's reading into all three.
func TestGetMetricView_ScalarViewBuckets(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// A counter climbing by one a minute, with a five-minute silence in the
	// middle and a late start, so the trimming and the interior gap are both
	// exercised.
	var dps []sumTestDP
	early := map[string]string{"pod": "early"}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			value:     float64(i + 1),
			attrs:     early,
		})
	}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(8+i) * time.Minute),
			value:     float64(4 + i),
			attrs:     early,
		})
	}
	// A second series that only starts halfway through. Its buckets must begin
	// where *it* begins, not where the first series did -- the trimming is per
	// series, and a response-wide extent would pad this one with empty buckets
	// for time before it existed.
	late := map[string]string{"pod": "late"}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(8+i) * time.Minute),
			value:     float64(i + 1),
			attrs:     late,
		})
	}
	fixture := makeSumFixtureT("climb.total", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// With no requested bounds, the grid comes from the ten minutes that hold
	// datapoints rather than an arbitrary wider range.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			0, nil, nil, 0, 12, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	series, _ := metric["timeseries"].([]any)
	require.Len(t, series, 2, "two series")

	byPod := map[string][]any{}
	for _, entry := range series {
		ts := entry.(map[string]any)
		var pod string
		for _, a := range ts["labelPreviews"].([]any) {
			if a.(map[string]any)["key"] == "pod" {
				pod = a.(map[string]any)["text"].(string)
			}
		}
		v, _ := ts["views"].([]any)
		byPod[pod] = v
	}
	require.NotEmpty(t, byPod["early"], "a scalar series carries its view buckets")
	require.NotEmpty(t, byPod["late"])

	// The late series covers the last three minutes only. Padding it back to
	// the early series' start would roughly triple its bucket count.
	assert.Less(t, len(byPod["late"]), len(byPod["early"]),
		"each series is trimmed to its own extent, not the response's")
	assert.Positive(t, byPod["late"][0].(map[string]any)["sampleCount"],
		"the late series starts at its own first bucket")

	views := byPod["early"]

	first := views[0].(map[string]any)
	last := views[len(views)-1].(map[string]any)

	// Leading and trailing empties are trimmed: the first and last bucket both
	// hold samples, even though the window is twelve hours wide.
	assert.Positive(t, first["sampleCount"], "no empty buckets before the data")
	assert.Positive(t, last["sampleCount"], "no empty buckets after it")

	// The interior gap survives, and is null rather than zero.
	var empties int
	for _, v := range views {
		b := v.(map[string]any)
		if b["sampleCount"].(float64) != 0 {
			continue
		}
		empties++
		assert.Nil(t, b["sum"], "an empty bucket has no sum, not a sum of zero")
		assert.Nil(t, b["avg"], "nor a mean")
		assert.Nil(t, b["rate"], "nor a rate")
	}
	assert.Positive(t, empties, "the silence in the middle is still a bucket")

	// The first bucket of a cumulative series has no predecessor, so it
	// describes no interval and its rate is unknown rather than zero.
	assert.Nil(t, first["rate"],
		"the first bucket has no earlier reading to difference against")

	// Sum and Average read the running totals, not the increments: summing
	// across series at time t means adding the counters. Only Rate differences.
	assert.NotNil(t, first["sum"])
	assert.NotNil(t, first["avg"])
}

// TestGetMetricView_RateSlopeAndStats verifies slopes and extrema use the same
// drawn sequence: empty buckets draw zero, while sampled buckets without a rate
// draw nothing.
func TestGetMetricView_RateSlopeAndStats(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// A counter climbing a minute at a time with a five-minute silence: the
	// gap draws zeros, and the slope into and out of the gap spans them.
	var dps []sumTestDP
	attrs := map[string]string{"pod": "a"}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			value:     float64(i + 1),
			attrs:     attrs,
		})
	}
	for i := range 3 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(8+i) * time.Minute),
			value:     float64(10 * (i + 1)),
			attrs:     attrs,
		})
	}
	fixture := makeSumFixtureT("slope.total", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			0, nil, nil, 0, 12, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	series := m["timeseries"].([]any)
	require.Len(t, series, 1)
	ts := series[0].(map[string]any)
	views := ts["views"].([]any)
	require.NotEmpty(t, views)

	// Recompute the drawn sequence from the response's own buckets and check
	// every slope against it. The test may do arithmetic; the client may not.
	type drawnPoint struct {
		startNs float64
		value   float64
	}
	var drawn []drawnPoint
	var emptyBuckets, firstRateNull int
	for _, v := range views {
		b := v.(map[string]any)
		start, err := strconv.ParseFloat(b["bucketStart"].(string), 64)
		require.NoError(t, err)
		sampleCount := b["sampleCount"].(float64)
		rate, hasRate := b["rate"].(float64)
		slope, hasSlope := b["slope"].(float64)

		if sampleCount > 0 && !hasRate {
			// A series' first bucket: no predecessor, nothing drawn, no slope.
			firstRateNull++
			assert.False(t, hasSlope, "an undrawn bucket has no slope")
			continue
		}
		value := 0.0
		if sampleCount > 0 {
			value = rate
		} else {
			emptyBuckets++
		}
		if len(drawn) == 0 {
			assert.False(t, hasSlope, "the first drawn point has no arriving segment")
		} else {
			prev := drawn[len(drawn)-1]
			want := (value - prev.value) / ((start - prev.startNs) / 1e9)
			require.True(t, hasSlope, "every drawn point after the first carries a slope")
			assert.InDelta(t, want, slope, 1e-9,
				"slope is Δrate over the seconds since the previous drawn point")
		}
		drawn = append(drawn, drawnPoint{start, value})
	}
	require.Positive(t, emptyBuckets, "the silence draws zeros, and they are in the sequence")
	require.Positive(t, firstRateNull, "the cumulative first bucket is undrawn")

	// The badges' numbers are the drawn line's, gap zeros included.
	rs := ts["rateStats"].(map[string]any)
	minWant, maxWant, sum := drawn[0].value, drawn[0].value, 0.0
	for _, d := range drawn {
		minWant, maxWant, sum = min(minWant, d.value), max(maxWant, d.value), sum+d.value
	}
	assert.InDelta(t, minWant, rs["min"], 1e-9, "gap zeros pull the minimum to the floor the chart shows")
	assert.InDelta(t, maxWant, rs["max"], 1e-9)
	assert.InDelta(t, sum/float64(len(drawn)), rs["avg"], 1e-9)

	// The pools carry slope by the same rule; one bucket proves the field.
	pools := m["scalarAggregate"].(map[string]any)
	all := pools["all"].([]any)
	require.NotEmpty(t, all)
	var poolSlopes int
	for _, v := range all {
		if _, ok := v.(map[string]any)["slope"].(float64); ok {
			poolSlopes++
		}
	}
	assert.Positive(t, poolSlopes, "the pooled line's segments carry slopes too")
}

// TestGetMetricView_DatapointLimitMatchesResponseOrder verifies that the first N
// response series are the N series that include datapoints.
//
// A first request cannot name series before receiving them, so its limit and
// the response ordering must use the same rank.
func TestGetMetricView_DatapointLimitMatchesResponseOrder(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	bounds := []float64{10, 20, 30}
	// Staggered start times, so raw recency and bucket recency disagree: every
	// series' last datapoint lands in the same merged bucket, but their raw
	// timestamps differ by seconds.
	var dps []histTestDP
	for series := range 8 {
		attrs := map[string]string{"pod": fmt.Sprintf("pod-%d", series)}
		for i := range 4 {
			dps = append(dps, histTestDP{
				timestamp: base.
					Add(time.Duration(i) * time.Minute).
					Add(time.Duration(series) * time.Second),
				attrs:  attrs,
				bounds: bounds,
				counts: []uint64{1, 1, 0, 0},
				count:  2, sum: 25, min: 5, max: 15,
			})
		}
	}
	fixture := makeHistogramFixtureT("order.hist", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	const limit = 3
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			20, nil, nil, 0, 0, 0, nil, "", nil, limit)

	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	series := m["timeseries"].([]any)
	require.Len(t, series, 8)

	firstN := map[string]bool{}
	for _, entry := range series[:limit] {
		firstN[entry.(map[string]any)["seriesRef"].(string)] = true
	}
	shipped := map[string]bool{}
	for _, entry := range series {
		ts := entry.(map[string]any)
		if len(ts["datapoints"].([]any)) > 0 {
			shipped[ts["seriesRef"].(string)] = true
		}
	}
	require.Len(t, shipped, limit, "the limit ships exactly N series")
	assert.Equal(t, firstN, shipped,
		"the series the store ships datapoints for are the first N the response "+
			"lists, so the client's \"check the first N\" checks series that have data")
}

// TestGetMetricView_ViewGridRespectsCadence covers the grid the Sum, Average and
// Rate views are drawn on: it may not divide finer than the data arrives.
//
// The cadence cap prevents empty buckets caused only by a requested grid finer
// than the reporting interval. The result remains a named ladder width.
func TestGetMetricView_ViewGridRespectsCadence(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// Once a minute for twenty minutes, with no gaps: every bucket the grid
	// produces should hold a reading.
	var dps []sumTestDP
	attrs := map[string]string{"pod": "a"}
	for i := range 20 {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			value:     float64(i + 1),
			attrs:     attrs,
		})
	}
	fixture := makeSumFixtureT("sparse.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			0, nil, nil, 0, 120, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	views := m["timeseries"].([]any)[0].(map[string]any)["views"].([]any)
	require.NotEmpty(t, views)

	var empty int
	var starts []int64
	for _, v := range views {
		b := v.(map[string]any)
		if b["sampleCount"].(float64) == 0 {
			empty++
		}
		start, err := strconv.ParseInt(b["bucketStart"].(string), 10, 64)
		require.NoError(t, err)
		starts = append(starts, start)
	}

	assert.Zero(t, empty,
		"a series reporting on a steady cadence with no gaps should produce no "+
			"empty buckets: every empty one here would draw as a zero the data "+
			"does not contain")
	assert.LessOrEqual(t, len(views), 20,
		"no more buckets than the series has reporting intervals")

	// The width is still a ladder rung, which is what makes a boundary nameable.
	require.Greater(t, len(starts), 1)
	width := starts[1] - starts[0]
	assert.Equal(t, int64(time.Minute), width,
		"one minute, the rung at the reporting cadence -- not the raw median gap")

	// A dense series is not coarsened by the cap: it asks for fewer buckets than
	// its cadence allows, so the ladder answers unchanged.
	dense := []sumTestDP{}
	for i := range 600 {
		dense = append(dense, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Second),
			value:     1,
			attrs:     map[string]string{"pod": "dense"},
		})
	}
	denseFixture := makeSumFixtureT("dense.total", pmetric.AggregationTemporalityDelta, dense)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, denseFixture)
	}))
	all := searchMetricsAll(t, s, ctx)
	var denseID string
	for _, sum := range all {
		if sum["name"] == "dense.total" {
			denseID = sum["metricRef"].(string)
		}
	}
	require.NotEmpty(t, denseID)
	denseRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, denseID, store.TimeRange{},
			0, nil, nil, 0, 120, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var dm map[string]any
	require.NoError(t, json.Unmarshal(denseRaw, &dm))
	denseViews := dm["timeseries"].([]any)[0].(map[string]any)["views"].([]any)
	assert.Greater(t, len(denseViews), 20,
		"a series reporting every second keeps the resolution it can support")
}

// TestGetMetricView_CountsDescribeTheWindow distinguishes retained datapoint
// counts from the narrowed or reduced datapoints carried in one response.
func TestGetMetricView_CountsDescribeTheWindow(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	var dps []sumTestDP
	for _, pod := range []string{"a", "b", "c", "d"} {
		for i := range 25 {
			dps = append(dps, sumTestDP{
				timestamp: base.Add(time.Duration(i) * time.Minute),
				value:     float64(i),
				attrs:     map[string]string{"pod": pod},
			})
		}
	}
	fixture := makeSumFixtureT("counts.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// Narrowed hard: datapoints for one series of four.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			0, nil, nil, 0, 0, 0, nil, "", nil, 1)

	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	series := m["timeseries"].([]any)
	require.Len(t, series, 4)

	var shipped, counted int
	for _, entry := range series {
		ts := entry.(map[string]any)
		shipped += len(ts["datapoints"].([]any))
		counted += int(ts["datapointCount"].(float64))

		// Every series knows its own total and when it last reported, whether
		// or not this response carried a single one of its datapoints.
		assert.Equal(t, float64(25), ts["datapointCount"],
			"each series holds 25 datapoints in the window")
		assert.NotNil(t, ts["lastSeenNs"],
			"including the ones whose datapoints were narrowed away")
	}

	assert.Equal(t, 100, counted, "the per-series counts describe the window")
	assert.Equal(t, 25, shipped, "while the response carried one series' worth")
	assert.Equal(t, float64(100), m["datapointCount"],
		"and the metric's own total agrees with the sum of its series")

	// The metric's last-seen does not depend on which series shipped.
	lastSeen, ok := m["lastSeenNs"].(string)
	require.True(t, ok, "last seen is present even under narrowing")
	ns, err := strconv.ParseInt(lastSeen, 10, 64)
	require.NoError(t, err)
	assert.Equal(t, base.Add(24*time.Minute).UnixNano(), ns,
		"the most recent datapoint in the window, across every series")

	// A reduced histogram is where "the window" and "the response" diverge
	// most: the merge replaces a bucket's datapoints with one merged row, so
	// counting the rows the projection carries reports buckets and calls them
	// datapoints. The count has to come from before the merge.
	bounds := []float64{10, 20, 30}
	var hdps []histTestDP
	for i := range 30 {
		hdps = append(hdps, histTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			attrs:     map[string]string{"pod": "h"},
			bounds:    bounds,
			counts:    []uint64{1, 1, 0, 0},
			count:     2, sum: 25, min: 5, max: 15,
		})
	}
	hfixture := makeHistogramFixtureT("counts.hist", pmetric.AggregationTemporalityDelta, hdps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, hfixture)
	}))
	var histID string
	for _, sum := range searchMetricsAll(t, s, ctx) {
		if sum["name"] == "counts.hist" {
			histID = sum["metricRef"].(string)
		}
	}
	require.NotEmpty(t, histID)

	hraw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		// Reduced to a handful of buckets, so the merge collapses 30 datapoints
		// into far fewer rows.
		return metrics.GetMetricView(ctx, db, histID, store.TimeRange{},
			5, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var hm map[string]any
	require.NoError(t, json.Unmarshal(hraw, &hm))
	hts := hm["timeseries"].([]any)[0].(map[string]any)
	merged := len(hts["datapoints"].([]any))

	assert.Equal(t, float64(30), hts["datapointCount"],
		"the datapoints the window holds, not the merged rows standing in for them")
	assert.Less(t, merged, 30,
		"the merge really did collapse them, or this proves nothing")
	assert.Equal(t, float64(30), hm["datapointCount"],
		"and the metric's total counts datapoints too")
}

// TestGetMetricView_Deterministic verifies byte-identical responses when
// timestamps and values tie. IDs provide the election and ordering tie-breaks.
func TestGetMetricView_Deterministic(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	var dps []sumTestDP
	// Twelve rows on one instant at one value: the election's worst case.
	flat := map[string]string{"pod": "flat"}
	for range 12 {
		dps = append(dps, sumTestDP{timestamp: base, value: 5, attrs: flat})
	}
	// A climb with a duplicated timestamp, which OTLP permits: two readings at
	// t1 with different values. The lag and the delta join must not care.
	dup := map[string]string{"pod": "dup"}
	for _, v := range []struct {
		min   int
		value float64
	}{{0, 1}, {1, 2}, {1, 3}, {2, 4}, {3, 5}} {
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(v.min) * time.Minute),
			value:     v.value,
			attrs:     dup,
		})
	}
	fixture := makeSumFixtureT("ties.total", pmetric.AggregationTemporalityCumulative, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	get := func(targetBuckets int64) json.RawMessage {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.TimeRange{},
				targetBuckets, nil, nil, 0, 4, 2, nil, "", nil, 0)

		})
		require.NoError(t, err)
		return raw
	}

	// Responses are byte-identical across runs, reduced and unreduced.
	for _, tb := range []int64{0, 1} {
		first := get(tb)
		for range 3 {
			require.Equal(t, string(first), string(get(tb)),
				"the same request must return the same bytes (targetBuckets=%d)", tb)
		}
	}

	seriesByPod := func(raw json.RawMessage) map[string][]map[string]any {
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		out := map[string][]map[string]any{}
		for _, entry := range m["timeseries"].([]any) {
			ts := entry.(map[string]any)
			var pod string
			for _, a := range ts["labelPreviews"].([]any) {
				if a.(map[string]any)["key"] == "pod" {
					pod = a.(map[string]any)["text"].(string)
				}
			}
			for _, dp := range ts["datapoints"].([]any) {
				out[pod] = append(out[pod], dp.(map[string]any))
			}
		}
		return out
	}

	full := seriesByPod(get(0))
	require.Len(t, full["flat"], 12)
	require.Len(t, full["dup"], 5)

	// Duplicate timestamps must not duplicate rows in the delta join.
	for pod, list := range full {
		seen := map[string]bool{}
		for _, dp := range list {
			id := dp["id"].(string)
			assert.False(t, seen[id], "%s: datapoint %s shipped twice", pod, id)
			seen[id] = true
		}
	}

	// Rows an ORDER BY cannot separate by timestamp arrive in id order --
	// verified on the flat series, where every row shares the instant.
	for i := 1; i < len(full["flat"]); i++ {
		assert.Less(t, full["flat"][i-1]["id"].(string), full["flat"][i]["id"].(string),
			"tied timestamps must order by id")
	}

	// The election's tie contract: among rows sharing a value, min and max
	// elect by id, and on the flat series first/last collapse onto the same
	// rows -- so exactly the smallest and largest id survive. DuckDB orders
	// UUIDs as their text sorts (verified against the CLI), so the expectation
	// is computable here.
	ids := make([]string, 0, 12)
	for _, dp := range full["flat"] {
		ids = append(ids, dp["id"].(string))
	}
	slices.Sort(ids)
	reduced := seriesByPod(get(1))
	var got []string
	for _, dp := range reduced["flat"] {
		got = append(got, dp["id"].(string))
	}
	slices.Sort(got)
	assert.Equal(t, []string{ids[0], ids[len(ids)-1]}, got,
		"a fully tied bucket elects the smallest and largest id, nothing else")
}

// TestGetMetricView_DatapointNarrowing covers the narrowest of the three series
// parameters: which series ship their datapoints.
//
// Narrowing datapoints must preserve every series row, stats, view buckets, and
// sparkline. The All aggregate still includes series whose datapoints are not
// shipped.
func TestGetMetricView_DatapointNarrowing(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	var dps []sumTestDP
	for _, pod := range []string{"a", "b", "c"} {
		for i := range 20 {
			dps = append(dps, sumTestDP{
				timestamp: base.Add(time.Duration(i) * time.Minute),
				value:     float64(i + 1),
				attrs:     map[string]string{"pod": pod},
			})
		}
	}
	fixture := makeSumFixtureT("narrow.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	get := func(dpSeries []string, limit int64) []any {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.TimeRange{},
				0, nil, nil, 0, 12, 8, nil, "", dpSeries, limit)

		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m["timeseries"].([]any)
	}

	// Everything ships when neither parameter is given.
	full := get(nil, 0)
	require.Len(t, full, 3)
	for _, entry := range full {
		assert.NotEmpty(t, entry.(map[string]any)["datapoints"])
	}

	// Name one series: it alone carries datapoints, and the other two arrive
	// complete in every other respect.
	first := full[0].(map[string]any)["seriesRef"].(string)
	narrowed := get([]string{first}, 0)
	require.Len(t, narrowed, 3, "narrowing datapoints drops no series")
	var withPoints, withoutPoints int
	for _, entry := range narrowed {
		ts := entry.(map[string]any)
		if len(ts["datapoints"].([]any)) > 0 {
			withPoints++
			assert.Equal(t, first, ts["seriesRef"])
		} else {
			withoutPoints++
		}
		// The parts that must survive regardless.
		assert.NotNil(t, ts["stats"], "stats describe the window, not the sample")
		assert.NotEmpty(t, ts["views"], "view buckets feed the All aggregate")
		assert.NotEmpty(t, ts["sparkline"], "the row still draws a shape")
		assert.NotEmpty(t, ts["labelPreviews"], "and still names itself")
	}
	assert.Equal(t, 1, withPoints)
	assert.Equal(t, 2, withoutPoints)

	// An empty list is not the same as an absent one: it names no series.
	none := get([]string{}, 0)
	require.Len(t, none, 3)
	for _, entry := range none {
		ts := entry.(map[string]any)
		assert.Empty(t, ts["datapoints"], "an empty list ships no datapoints")
		assert.NotEmpty(t, ts["views"], "and still every view bucket")
	}

	// The limit is for a caller that cannot name the series, because it picks
	// them from this response. It takes them in the response's own order.
	limited := get(nil, 2)
	require.Len(t, limited, 3)
	var limitedWith []string
	for _, entry := range limited {
		ts := entry.(map[string]any)
		if len(ts["datapoints"].([]any)) > 0 {
			limitedWith = append(limitedWith, ts["seriesRef"].(string))
		}
	}
	assert.Len(t, limitedWith, 2, "the first two series in response order")
	var wantFirstTwo []string
	for _, entry := range limited[:2] {
		wantFirstTwo = append(wantFirstTwo, entry.(map[string]any)["seriesRef"].(string))
	}
	assert.ElementsMatch(t, wantFirstTwo, limitedWith,
		"and they are the first two as the response lists them, so the caller's "+
			"\"first N\" and the store's mean the same series")

	// A named list wins over a limit, since that caller does know.
	both := get([]string{first}, 2)
	var bothWith int
	for _, entry := range both {
		if len(entry.(map[string]any)["datapoints"].([]any)) > 0 {
			bothWith++
		}
	}
	assert.Equal(t, 1, bothWith, "the list decides when it is given")
}

// TestGetMetricView_ScalarPoolAggregate verifies that cross-series lines use
// the per-series grid and average all samples rather than series averages.
func TestGetMetricView_ScalarPoolAggregate(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// Deliberately lopsided: "dense" reports ten times a minute at value 1,
	// "sparse" once a minute at value 100. Mean-of-means would call that 50.5;
	// the pooled mean is (10*1 + 100) / 11 = 10, and only one of those is the
	// average of what was observed.
	var dps []sumTestDP
	dense := map[string]string{"pod": "dense"}
	sparse := map[string]string{"pod": "sparse"}
	for minute := range 5 {
		for tick := range 10 {
			dps = append(dps, sumTestDP{
				timestamp: base.Add(time.Duration(minute)*time.Minute + time.Duration(tick)*time.Second),
				value:     1,
				attrs:     dense,
			})
		}
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(minute) * time.Minute),
			value:     100,
			attrs:     sparse,
		})
	}
	fixture := makeSumFixtureT("pool.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	get := func(selected []string) map[string]any {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.TimeRange{},
				0, nil, nil, 0, 12, 0, selected, "", nil, 0)

		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m
	}

	// Series ids, so one of the two can be named as the checked pool.
	all := get(nil)
	idByPod := map[string]string{}
	for _, entry := range all["timeseries"].([]any) {
		ts := entry.(map[string]any)
		for _, a := range ts["labelPreviews"].([]any) {
			attr := a.(map[string]any)
			if attr["key"] == "pod" {
				idByPod[attr["text"].(string)] = ts["seriesRef"].(string)
			}
		}
	}
	require.Len(t, idByPod, 2)

	pools := all["scalarAggregate"].(map[string]any)
	allBuckets := pools["all"].([]any)
	require.NotEmpty(t, allBuckets, "the All pool folds every series")
	assert.Empty(t, pools["selected"],
		"nothing checked is a real state: the Selected pool is empty and the "+
			"chart draws All by itself")

	// Pool and per-series buckets share absolute boundaries.
	seriesBucketStarts := map[string]bool{}
	for _, entry := range all["timeseries"].([]any) {
		views, _ := entry.(map[string]any)["views"].([]any)
		for _, v := range views {
			seriesBucketStarts[v.(map[string]any)["bucketStart"].(string)] = true
		}
	}
	for _, b := range allBuckets {
		start := b.(map[string]any)["bucketStart"].(string)
		assert.True(t, seriesBucketStarts[start],
			"pool bucket %s is on the shared grid", start)
	}

	// The pooled mean, on a bucket holding both series.
	var checked int
	for _, b := range allBuckets {
		bucket := b.(map[string]any)
		if bucket["sampleCount"].(float64) != 11 {
			continue
		}
		checked++
		assert.InDelta(t, 110.0, bucket["sum"], 1e-9, "ten 1s and one 100")
		assert.InDelta(t, 10.0, bucket["avg"], 1e-9,
			"pooled over every sample; the mean of the two series' means would "+
				"be 50.5 and would let one sparse series outvote ten samples")
	}
	assert.Positive(t, checked, "at least one bucket holds both series")

	// Checking one series folds that series alone.
	one := get([]string{idByPod["sparse"]})
	selBuckets := one["scalarAggregate"].(map[string]any)["selected"].([]any)
	require.NotEmpty(t, selBuckets)
	var populated, empty int
	for _, b := range selBuckets {
		bucket := b.(map[string]any)
		if bucket["sampleCount"].(float64) == 0 {
			// The sparse series reports once a minute onto a finer grid, so most
			// of its buckets hold nothing. They are carried rather than dropped:
			// an interior empty bucket is zero activity for Sum and Rate, and a
			// line that skipped it would draw straight through the gap.
			empty++
			assert.Nil(t, bucket["sum"], "an empty bucket has no sum, not zero")
			continue
		}
		populated++
		assert.Equal(t, 1.0, bucket["sampleCount"],
			"only the sparse series is in the checked pool")
		assert.InDelta(t, 100.0, bucket["sum"], 1e-9)
	}
	assert.Positive(t, populated, "the checked series contributes samples")
	assert.Positive(t, empty, "and its gaps survive the fold")
	// The All pool is unmoved by what is checked -- that is what makes it "all".
	assert.Equal(t, len(allBuckets),
		len(one["scalarAggregate"].(map[string]any)["all"].([]any)),
		"the All pool does not narrow with the selection")
}

// TestGetMetricView_Sparkline verifies that row-resolution reduction is bounded
// by sparkline width and preserves extremes instead of averaging them away.
func TestGetMetricView_Sparkline(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	// A single spike distinguishes extrema reduction from bucket averaging.
	const spike = 500.0
	var dps []sumTestDP
	attrs := map[string]string{"pod": "a"}
	for i := range 200 {
		value := 1.0
		if i == 100 {
			value = spike
		}
		dps = append(dps, sumTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute),
			value:     value,
			attrs:     attrs,
		})
	}
	fixture := makeSumFixtureT("spiky.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	const buckets = 8
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, metricID, store.TimeRange{},
			1, nil, nil, 0, 0, buckets, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var metric map[string]any
	require.NoError(t, json.Unmarshal(raw, &metric))

	series, _ := metric["timeseries"].([]any)
	require.Len(t, series, 1)
	points, _ := series[0].(map[string]any)["sparkline"].([]any)
	require.NotEmpty(t, points, "a scalar series carries a sparkline")

	// The preview simplifies the main chart's M4 election, not all 200 inputs.
	assert.LessOrEqual(t, len(points), 2*buckets,
		"at most a min and a max per bucket")
	assert.LessOrEqual(t, len(points), 4, "only main-chart elected samples feed the preview")
	retained := map[string]bool{}
	for _, p := range series[0].(map[string]any)["datapoints"].([]any) {
		retained[p.(map[string]any)["timestamp"].(string)] = true
	}

	var maxValue float64
	var prev int64
	for i, p := range points {
		b := p.(map[string]any)
		assert.True(t, retained[b["timestamp"].(string)], "preview samples must come from the main chart")
		v := b["value"].(float64)
		if v > maxValue {
			maxValue = v
		}
		// Timestamps travel as strings, like every other ns value on the wire.
		ts, err := strconv.ParseInt(b["timestamp"].(string), 10, 64)
		require.NoError(t, err)
		if i > 0 {
			assert.Greater(t, ts, prev,
				"points are ordered by time, and a bucket that elected the same "+
					"row as both its min and its max contributes it once")
		}
		prev = ts
	}

	// The reason this is not read off the views: the spike is one sample in a
	// bucket holding twenty-five, so an average would report about 21.
	assert.Equal(t, spike, maxValue,
		"the spike survives the reduction rather than being averaged away")

	// Asking for no buckets is how a caller that draws no sparklines avoids
	// paying for them, and it has to be legible as "absent" rather than empty.
	rawNone, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, metricID, store.TimeRange{},
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var none map[string]any
	require.NoError(t, json.Unmarshal(rawNone, &none))
	noneSeries, _ := none["timeseries"].([]any)
	require.Len(t, noneSeries, 1)
	assert.Nil(t, noneSeries[0].(map[string]any)["sparkline"],
		"zero buckets means no sparkline, not an empty one")
}

// TestGetMetricView_SeriesFilter covers the parameter that makes server-side
// cross-series aggregation possible: the caller names the series it wants and
// the store narrows before the reduction, so asking for two of ten costs two.
//
// Nil means all; an empty list means no series.
func TestGetMetricView_SeriesFilter(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Now().Add(-time.Minute)
	var dps []expHistTestDP
	for si := 0; si < 3; si++ {
		for j := 0; j < 4; j++ {
			dps = append(dps, expHistTestDP{
				timestamp: base.Add(time.Duration(j) * time.Second),
				attrs:     map[string]string{"route": fmt.Sprintf("/r%d", si)},
				scale:     0, zeroCount: 1, zeroThreshold: 1,
				posOffset: 0, posCounts: []uint64{1, 2},
				count: 4, sum: 10,
			})
		}
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn,
			makeExpHistogramFixtureT("filter.me", pmetric.AggregationTemporalityDelta, dps))
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)
	end := time.Now().UnixNano() + int64(time.Hour)

	seriesKeys := func(ids []string) []string {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(0, end), 0, ids, nil, 0, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		var out []string
		for _, e := range m["timeseries"].([]any) {
			out = append(out, e.(map[string]any)["seriesRef"].(string))
		}
		return out
	}

	all := seriesKeys(nil)
	require.Len(t, all, 3, "nil returns every series")

	assert.Equal(t, []string{all[1]}, seriesKeys([]string{all[1]}),
		"asking for one series returns exactly that series")

	got := seriesKeys([]string{all[0], all[2]})
	assert.ElementsMatch(t, []string{all[0], all[2]}, got,
		"asking for two returns exactly those two")

	// The three states are distinct, and the empty one is the reachable
	// mistake: a user who unticks every series must see nothing, not
	// everything. nil and []string{} are different questions.
	assert.Empty(t, seriesKeys([]string{}),
		"an empty selection means no series, not every series")
}

// TestGetMetricView_TimezoneAlignedBuckets verifies calendar-aligned bucket
// boundaries.
//
// Bucketing shifts by the UTC offset, floors, and shifts back. floor_div is
// required because integer division truncates pre-epoch timestamps toward zero.
func TestGetMetricView_TimezoneAlignedBuckets(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// Two datapoints either side of midnight UTC. Under a -5h offset they fall
	// on the same local day; under UTC they straddle two.
	utcMidnight := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	fixture := makeExpHistogramFixtureT("tz.duration", pmetric.AggregationTemporalityDelta, []expHistTestDP{
		{
			timestamp: utcMidnight.Add(-2 * time.Hour), scale: 0,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{4}, count: 4, sum: 8,
		},
		{
			timestamp: utcMidnight.Add(2 * time.Hour), scale: 0,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{6}, count: 6, sum: 12,
		},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	// A window a few days wide, so the bucket ladder lands on day-sized widths.
	start := utcMidnight.Add(-36 * time.Hour).UnixNano()
	end := utcMidnight.Add(36 * time.Hour).UnixNano()

	bucketCount := func(offsetNs int64) int {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(start, end), 3, nil, nil, offsetNs, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		agg, _ := m["aggregate"].([]any)
		return len(agg)
	}

	utcBuckets := bucketCount(0)
	shifted := bucketCount(-5 * int64(time.Hour))

	// The datapoints are 4h apart, so whether they share a bucket depends
	// entirely on where the boundary falls -- which is the thing under test.
	assert.Positive(t, utcBuckets, "UTC alignment still produces buckets")
	assert.Positive(t, shifted, "a shifted offset still produces buckets")

	// The offset must actually reach the bucketing. Identical counts for every
	// offset would mean it was ignored, which is the failure this guards.
	var distinct = map[int]bool{}
	for _, off := range []int64{0, -5 * int64(time.Hour), 9 * int64(time.Hour), 13 * int64(time.Hour)} {
		distinct[bucketCount(off)] = true
	}
	assert.Greater(t, len(distinct), 1,
		"bucket boundaries must move with the offset, or tz_offset_ns is not reaching bucket_start")
}

// TestGetMetricView_NullBoundsSpanTheData verifies that null bounds use the
// data extent while explicit bounds continue to use the requested window.
func TestGetMetricView_NullBoundsSpanTheData(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// The explicit window collapses these ten minutes into one bucket, while an
	// unbounded request derives a minute-scale effective window.
	base := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	dps := make([]expHistTestDP, 0, 10)
	for i := range 10 {
		dps = append(dps, expHistTestDP{
			timestamp: base.Add(time.Duration(i) * time.Minute), scale: 0,
			zeroCount: 0, zeroThreshold: 0,
			posOffset: 0, posCounts: []uint64{uint64(i + 1)}, count: uint64(i + 1), sum: float64(i + 1),
		})
	}
	fixture := makeExpHistogramFixtureT("fit.duration", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	start := int64(0)
	end := base.Add(48 * time.Hour).UnixNano()

	get := func(timeRange store.TimeRange) map[string]any {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, metricID, timeRange, 8, nil, nil, 0, 0, 0, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m
	}
	buckets := func(m map[string]any) int {
		agg, _ := m["aggregate"].([]any)
		return len(agg)
	}

	bounded := get(store.BoundedTimeRange(start, end))
	unbounded := get(store.TimeRange{})

	// The explicit decades-wide window collapses the session into one bucket.
	assert.Equal(t, 1, buckets(bounded),
		"dividing a decades-wide window must still merge the session into one bucket")
	assert.Greater(t, buckets(unbounded), buckets(bounded),
		"null bounds must derive a data-sized effective window")

	boundedWindow := bounded["window"].(map[string]any)
	require.Equal(t, map[string]any{
		"startNs": "0", "endNs": strconv.FormatInt(end, 10),
	}, boundedWindow["requested"])
	require.Equal(t, boundedWindow["requested"], boundedWindow["effective"])

	unboundedWindow := unbounded["window"].(map[string]any)
	require.Equal(t, map[string]any{"startNs": nil, "endNs": nil}, unboundedWindow["requested"])
	require.Equal(t, map[string]any{
		"startNs": strconv.FormatInt(base.UnixNano(), 10),
		"endNs":   strconv.FormatInt(base.Add(9*time.Minute).UnixNano(), 10),
	}, unboundedWindow["effective"])
	require.ElementsMatch(t, []string{"requested", "effective"}, mapKeys(unboundedWindow),
		"the window contract has exactly its two explicit bound sets")

	// Explicit windows preserve empty time outside the data extent.
	tight := base.Add(-6 * time.Hour).UnixNano()
	rawTight, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(tight, end), 8, nil, nil, 0, 0, 0, nil, "", nil, 0)
	})
	require.NoError(t, err)
	var mTight map[string]any
	require.NoError(t, json.Unmarshal(rawTight, &mTight))
	assert.Positive(t, buckets(mTight), "an explicit window still buckets")
}

func TestGetMetricView_NullableBoundsDetermineEffectiveWindow(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("nullable.window")
	gauge := metric.SetEmptyGauge()
	for _, timestamp := range []int64{100, 200, 300} {
		dp := gauge.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(timestamp))
		dp.SetIntValue(timestamp)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))

	summariesRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, store.TimeRange{}, nil)
	})
	require.NoError(t, err)
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal(summariesRaw, &summaries))
	require.Len(t, summaries, 1)
	metricID := summaries[0]["metricRef"].(string)

	start, end := uint64(200), uint64(200)
	for _, tc := range []struct {
		name      string
		timeRange store.TimeRange
		wantReqS  any
		wantReqE  any
		wantStart string
		wantEnd   string
		wantCount float64
	}{
		{"unbounded", store.TimeRange{}, nil, nil, "100", "300", 3},
		{"start only", store.TimeRange{Start: &start}, "200", nil, "200", "300", 2},
		{"end only", store.TimeRange{End: &end}, nil, "200", "100", "200", 2},
		{"bounded", store.TimeRange{Start: &start, End: &end}, "200", "200", "200", "200", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricView(ctx, db, metricID, tc.timeRange,
					2, nil, nil, 0, 0, 0, nil, "", nil, 0)

			})
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Equal(t, tc.wantCount, got["datapointCount"])
			window := got["window"].(map[string]any)
			require.Equal(t, map[string]any{
				"startNs": tc.wantReqS, "endNs": tc.wantReqE,
			}, window["requested"])
			require.Equal(t, map[string]any{
				"startNs": tc.wantStart, "endNs": tc.wantEnd,
			}, window["effective"])

			aggregate, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricAggregateView(ctx, db, metricID, tc.timeRange,
					2, nil, nil, 0, 0, nil, "")

			})
			require.NoError(t, err, "nullable bounds must reach the aggregate reduction path")
			require.JSONEq(t,
				`{"aggregate":null,"scalarAggregate":{"selected":[],"all":[]}}`,
				string(aggregate))
		})
	}

	after, before := uint64(400), uint64(50)
	for _, tc := range []struct {
		name      string
		timeRange store.TimeRange
		wantReqS  any
		wantReqE  any
		wantStart any
		wantEnd   any
	}{
		{"empty after start", store.TimeRange{Start: &after}, "400", nil, "400", nil},
		{"empty before end", store.TimeRange{End: &before}, nil, "50", nil, "50"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricView(ctx, db, metricID, tc.timeRange,
					2, nil, nil, 0, 2, 2, nil, "", nil, 0)

			})
			require.NoError(t, err, "an empty effective extent must not perform invalid null arithmetic")
			var got map[string]any
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Equal(t, float64(0), got["datapointCount"])
			require.Empty(t, got["timeseries"])
			window := got["window"].(map[string]any)
			require.Equal(t, map[string]any{
				"startNs": tc.wantReqS, "endNs": tc.wantReqE,
			}, window["requested"])
			require.Equal(t, map[string]any{
				"startNs": tc.wantStart, "endNs": tc.wantEnd,
			}, window["effective"])

			aggregate, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricAggregateView(ctx, db, metricID, tc.timeRange,
					2, nil, nil, 0, 2, nil, "")

			})
			require.NoError(t, err, "aggregate reduction must tolerate an empty effective window")
			require.JSONEq(t,
				`{"aggregate":null,"scalarAggregate":{"selected":[],"all":[]}}`,
				string(aggregate))
		})
	}
}

// TestGetMetricView_BucketsFollowTheZoneAcrossDST verifies that bucketing uses
// the named timezone's offset at each boundary.
//
// London changes from UTC+1 to UTC+0 on 2026-10-25T01:00Z, making that local
// day 25 hours long. A single fixed offset splits readings from the same day.
func TestGetMetricView_BucketsFollowTheZoneAcrossDST(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	// Both readings fall on local Sunday 25 October 2026 in London: the first
	// half an hour into it while BST is still in force, the second half an hour
	// before it ends, by which point the clocks have gone back.
	morning := time.Date(2026, 10, 24, 23, 30, 0, 0, time.UTC)
	evening := time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC)
	// And one on the Monday after, by which point the offset has changed for
	// good. Its bucket has to start at local midnight under the *new* offset.
	monday := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	attrs := map[string]string{"pod": "a"}
	fixture := makeSumFixtureT("dst.total", pmetric.AggregationTemporalityDelta, []sumTestDP{
		{timestamp: morning, value: 1, attrs: attrs},
		{timestamp: evening, value: 2, attrs: attrs},
		{timestamp: monday, value: 3, attrs: attrs},
	})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// One view bucket over a ~25h window selects the one-day rung, so the grid
	// asks exactly the question this test is about: which local day is each
	// reading on? Read the views rather than the datapoints -- the datapoint
	// reduction elects a bucket's first *and* last point, so a bucket holding
	// both readings still emits two, and the count would say nothing.
	dayBuckets := func(tzName string, tzOffsetNs int64) []any {
		t.Helper()
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
				0, nil, nil, tzOffsetNs, 2, 0, nil, tzName, nil, 0)

		})
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		return m["timeseries"].([]any)[0].(map[string]any)["views"].([]any)
	}

	// The offset a London browser reports while BST is in force. Sent alone it
	// is right for the first reading and an hour wrong for the second.
	const bstOffsetNs = int64(time.Hour)

	// Each bucket as (start, samples). The count alone cannot tell the two
	// answers apart -- both produce two buckets -- because the offset does not
	// lose a day, it puts the boundary in the wrong place and moves one reading
	// across it.
	shape := func(views []any) []string {
		t.Helper()
		var out []string
		for _, v := range views {
			b := v.(map[string]any)
			out = append(out, fmt.Sprintf("%s/%d",
				b["bucketStart"].(string), int(b["sampleCount"].(float64))))
		}
		return out
	}
	utc := func(day, hour int) string {
		return strconv.FormatInt(
			time.Date(2026, 10, day, hour, 0, 0, 0, time.UTC).UnixNano(), 10)
	}

	assert.Equal(t,
		// Local midnight on each day, 25 hours apart: that Sunday is 25 hours
		// long, and its boundaries are BST at the start and GMT at the end.
		[]string{utc(24, 23) + "/2", utc(26, 0) + "/1"},
		shape(dayBuckets("Europe/London", bstOffsetNs)),
		"both Sunday readings belong to the Sunday, and each bucket has to begin "+
			"at local midnight under the offset in force there")

	assert.Equal(t,
		[]string{utc(24, 23) + "/1", utc(25, 23) + "/2"},
		shape(dayBuckets("", bstOffsetNs)),
		"with no zone named the single offset still applies to the whole window, "+
			"so the Sunday ends an hour early and takes its last reading into "+
			"the Monday -- the behaviour a zone-less caller keeps, and the bug")
}

// TestGetMetricView_ExemplarsAreCappedPerBucket verifies that exemplar carriers
// preserve trace links without making reduced responses unbounded.
func TestGetMetricView_ExemplarsAreCappedPerBucket(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	// One bucket's worth of readings, every one of them carrying an exemplar.
	const n = 60
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "exemplar-cap")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("cap-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName("capped.total")
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	for i := range n {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base.Add(time.Duration(i) * time.Second).UnixNano()))
		dp.SetDoubleValue(float64(i % 7))
		dp.Attributes().PutStr("pod", "a")
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(dp.Timestamp())
		ex.SetDoubleValue(float64(i))
		ex.SetTraceID(pcommon.TraceID([16]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
		ex.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// One target bucket, so the whole run reduces to a single bucket and the
	// per-bucket ceiling is the whole response's ceiling.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)

	// Four elected plus at most two exemplar carriers, and the two sets
	// overlap, so six is the ceiling rather than the expected count.
	assert.LessOrEqual(t, len(dps), 6,
		"a bucket yields at most the four M4 elects plus two exemplar carriers; "+
			"without the cap all %d readings come back and the reduction does nothing", n)
	assert.Greater(t, len(dps), 0, "the bucket should still yield its elected points")

	// Trace correlation survives: the point of retaining carriers at all.
	var withExemplars int
	for _, d := range dps {
		if ex, _ := d.(map[string]any)["exemplars"].([]any); len(ex) > 0 {
			withExemplars++
		}
	}
	assert.Greater(t, withExemplars, 0,
		"capping must not become dropping -- a reader still needs a way into a trace")
}

// TestGetMetricView_ExemplarListIsCappedAndCounted covers the other direction the
// response grew in: how many exemplars one datapoint carries, which OTel does
// not limit. The list is capped and the true count travels beside it, so a
// client can say "5 of 60" rather than silently showing five.
func TestGetMetricView_ExemplarListIsCappedAndCounted(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	const perDatapoint = 60
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "exemplar-fanout")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("fanout-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName("fanout.total")
	g := m.SetEmptyGauge()
	dp := g.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(base.UnixNano()))
	dp.SetDoubleValue(1)
	dp.Attributes().PutStr("pod", "a")
	for i := range perDatapoint {
		ex := dp.Exemplars().AppendEmpty()
		// Staggered, so the (timestamp, id) order the cap keeps a prefix of is
		// a real order rather than an accident of insertion.
		ex.SetTimestamp(pcommon.Timestamp(base.Add(time.Duration(i) * time.Millisecond).UnixNano()))
		ex.SetDoubleValue(float64(i))
		ex.SetTraceID(pcommon.TraceID([16]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
		ex.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1)
	only := dps[0].(map[string]any)

	assert.Len(t, only["exemplars"].([]any), 5,
		"one datapoint ships at most five exemplars, whatever the SDK attached")
	assert.Equal(t, float64(perDatapoint), only["exemplarCount"],
		"and reports how many it actually holds, so the cap is visible rather "+
			"than a silent loss of trace links")
}

// TestGetMetricView_ExemplarSelectionKeepsBothExtremes verifies that capped
// exemplars span the value range rather than forming a time-ordered prefix.
func TestGetMetricView_ExemplarSelectionKeepsBothExtremes(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "exemplar-extremes")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("extremes-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName("extremes.gauge")
	g := m.SetEmptyGauge()
	dp := g.DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(base.UnixNano()))
	dp.SetDoubleValue(1)
	dp.Attributes().PutStr("pod", "a")
	// Values 100 through 119 distinguish range ranking from a time-order prefix.
	const n = 20
	for i := range n {
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(pcommon.Timestamp(base.Add(time.Duration(i) * time.Millisecond).UnixNano()))
		ex.SetDoubleValue(float64(100 + i))
		ex.SetTraceID(pcommon.TraceID([16]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
		ex.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1)

	var values []float64
	for _, e := range dps[0].(map[string]any)["exemplars"].([]any) {
		values = append(values, e.(map[string]any)["doubleValue"].(float64))
	}
	sort.Float64s(values)

	// Five kept, working inward from both ends: 100, 101, 102 from the bottom
	// and 118, 119 from the top -- or the mirror of that, which is the same
	// spread. What matters is that both extremes are present and the middle is
	// not.
	require.Len(t, values, 5)
	assert.Equal(t, 100.0, values[0],
		"the lowest exemplar is one a reader would want and must survive the cap")
	assert.Equal(t, 119.0, values[len(values)-1],
		"so is the highest -- time-order selection would have dropped it")
	assert.NotContains(t, values, 110.0,
		"and the middle of the range is what the cap should be spending its "+
			"budget on last")
}

// Adjacent int64 values at this magnitude all have the same float64
// representation. Give their rows ids in ascending value order: a ranking that
// compared only the rounded DOUBLE would deterministically keep the first five
// and drop the true maximum.
func TestGetMetricView_IntegerExemplarSelectionRemainsExactAbove2Pow53(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	const n = 20
	const baseValue int64 = 1 << 62
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "integer-exemplar-extremes")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("integer_exemplar_extremes")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(100)
	dp.SetDoubleValue(1)
	for i := range n {
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(pcommon.Timestamp(i + 1))
		ex.SetIntValue(baseValue + int64(i))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		for i := range n {
			id := fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
			if _, err := db.Exec(`update metric_records set datapoints=list_transform(datapoints,lambda d:struct_update(d,exemplars:=list_transform(d.exemplars,lambda e:case when e.int_value=$2 then struct_update(e,id:=$1::uuid) else e end)))`,
				id, baseValue+int64(i)); err != nil {
				return err
			}
		}
		return nil
	}))

	got := getMetricFullByName(t, s, ctx, "integer_exemplar_extremes")
	dps := metricDatapoints(got)
	require.Len(t, dps, 1)
	var values []string
	for _, raw := range dps[0].(map[string]any)["exemplars"].([]any) {
		values = append(values, raw.(map[string]any)["intValue"].(string))
	}
	require.Len(t, values, 5)
	assert.Contains(t, values, strconv.FormatInt(baseValue, 10))
	assert.Contains(t, values, strconv.FormatInt(baseValue+n-1, 10),
		"the exact maximum must survive even though all values share one float64 approximation")
	assert.NotContains(t, values, strconv.FormatInt(baseValue+n/2, 10),
		"the middle of the exact integer range should be ranked after both extremes")
}

// Doubles and adjacent integers at this magnitude share one float64
// approximation. Deterministic ids favor the earliest rows, so ranking on the
// approximation alone would fill the cap before reaching the true integer max.
func TestGetMetricView_MixedExemplarSelectionUsesExactIntegerTieBreak(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	const n = 20
	const baseValue int64 = 1 << 62
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "mixed-exemplar-extremes")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("mixed_exemplar_extremes")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(100)
	dp.SetDoubleValue(1)
	for i := range n {
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(pcommon.Timestamp(i + 1))
		if i%2 == 0 {
			ex.SetDoubleValue(float64(baseValue + int64(i)))
		} else {
			ex.SetIntValue(baseValue + int64(i))
		}
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		for i := range n {
			id := fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
			if _, err := db.Exec(`update metric_records set datapoints=list_transform(datapoints,lambda d:struct_update(d,exemplars:=list_transform(d.exemplars,lambda e:case when e.timestamp=$2 then struct_update(e,id:=$1::uuid) else e end)))`, id, i+1); err != nil {
				return err
			}
		}
		return nil
	}))

	got := getMetricFullByName(t, s, ctx, "mixed_exemplar_extremes")
	dps := metricDatapoints(got)
	require.Len(t, dps, 1)
	exemplars := dps[0].(map[string]any)["exemplars"].([]any)
	require.Len(t, exemplars, 5)

	var sawDoubleMinimum, sawIntegerMaximum bool
	for _, raw := range exemplars {
		ex := raw.(map[string]any)
		switch ex["valueType"] {
		case "Double":
			sawDoubleMinimum = sawDoubleMinimum || ex["doubleValue"] == float64(baseValue)
		case "Int":
			sawIntegerMaximum = sawIntegerMaximum || ex["intValue"] == strconv.FormatInt(baseValue+n-1, 10)
		}
	}
	assert.True(t, sawDoubleMinimum, "the shared double minimum must survive the cap")
	assert.True(t, sawIntegerMaximum,
		"the exact integer maximum must survive rows with the same float64 approximation")
}

// TestGetMetricView_ExemplarCarriersAreTheExtremeOnes verifies that each bucket
// retains carriers for the lowest and highest exemplar values.
func TestGetMetricView_ExemplarCarriersAreTheExtremeOnes(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "carrier-extremes")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("carrier-scope")
	m := sm.Metrics().AppendEmpty()
	m.SetName("carriers.total")
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	// The exemplar extremes sit in the middle of the run, at i=5 and i=6, while
	// the datapoint values rise monotonically. M4 therefore elects different rows
	// and cannot supply the exemplar extremes by itself.
	const n = 12
	for i := range n {
		dp := sum.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(base.Add(time.Duration(i) * time.Second).UnixNano()))
		dp.SetDoubleValue(float64(i))
		dp.Attributes().PutStr("pod", "a")
		value := 55.0
		switch i {
		case 5:
			value = 10
		case 6:
			value = 99
		}
		ex := dp.Exemplars().AppendEmpty()
		ex.SetTimestamp(dp.Timestamp())
		ex.SetDoubleValue(value)
		ex.SetTraceID(pcommon.TraceID([16]byte{byte(i), 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
		ex.SetSpanID(pcommon.SpanID([8]byte{byte(i), 2, 3, 4, 5, 6, 7, 8}))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, md)
	}))
	summaries := searchMetricsAll(t, s, ctx)
	require.Len(t, summaries, 1)

	// One bucket for the whole run, so the per-bucket carrier cap is what
	// decides the answer.
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)

	var shipped []float64
	for _, d := range dps {
		for _, e := range d.(map[string]any)["exemplars"].([]any) {
			shipped = append(shipped, e.(map[string]any)["doubleValue"].(float64))
		}
	}
	sort.Float64s(shipped)
	require.NotEmpty(t, shipped)

	assert.Contains(t, shipped, 99.0,
		"the bucket's most extreme exemplar is the one a reader is hunting, and "+
			"no elected reading carries it -- only the carrier cap can")
	assert.Contains(t, shipped, 10.0,
		"and the other end matters too -- a request that returned instantly is "+
			"as diagnostic as one that hung")
}

// TestSeriesCountsAreWindowAndLifetime verifies that seriesCount reports the
// requested window while seriesCardinality reports the metric's lifetime.
func TestSeriesCountsAreWindowAndLifetime(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	// Twelve series on the Metric; only three of them report in the first
	// minute, which is the window the narrow search below asks about.
	var dps []sumTestDP
	for i := range 12 {
		pod := map[string]string{"pod": string(rune('a' + i))}
		at := base.Add(time.Duration(i) * 10 * time.Minute)
		if i < 3 {
			at = base.Add(time.Duration(i) * 10 * time.Second)
		}
		// Unequal per-series counts distinguish datapoint totals from series cardinality.
		for j := range i + 1 {
			dps = append(dps, sumTestDP{timestamp: at.Add(time.Duration(j) * time.Second), value: float64(i), attrs: pod})
		}
	}
	fixture := makeSumFixtureT("cardinality.total", pmetric.AggregationTemporalityDelta, dps)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, fixture)
	}))

	summarise := func(from, to time.Time) map[string]any {
		t.Helper()
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(from.UnixNano(), to.UnixNano()), nil)
		})
		require.NoError(t, err)
		var rows []map[string]any
		require.NoError(t, json.Unmarshal(raw, &rows))
		require.Len(t, rows, 1)
		return rows[0]
	}

	whole := summarise(base.Add(-time.Hour), base.Add(4*time.Hour))
	assert.Equal(t, float64(12), whole["seriesCount"],
		"every series reported somewhere in an unbounded window")
	assert.Equal(t, float64(12), whole["seriesCardinality"],
		"and the lifetime total agrees there, which is why one number hid this")
	assert.EqualValues(t, 78, whole["dataPointCount"])

	narrow := summarise(base.Add(-time.Second), base.Add(time.Minute))
	assert.Equal(t, float64(3), narrow["seriesCount"],
		"three series reported in this minute, so that is what the window count says")
	assert.Equal(t, float64(12), narrow["seriesCardinality"],
		"the Metric still has twelve series; narrowing the window did not delete nine")
	assert.EqualValues(t, 6, narrow["dataPointCount"])
}

func TestScalarCounterArithmetic(t *testing.T) {
	s, ctx := storetest.New(t)
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "scalar-counter-test")
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("test.counter")
	sum := metric.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	appendInt := func(series string, timestamp uint64, value int64) {
		dp := sum.DataPoints().AppendEmpty()
		dp.Attributes().PutStr("series", series)
		dp.SetTimestamp(pcommon.Timestamp(timestamp))
		dp.SetIntValue(value)
	}
	appendDouble := func(series string, timestamp uint64, value float64) {
		dp := sum.DataPoints().AppendEmpty()
		dp.Attributes().PutStr("series", series)
		dp.SetTimestamp(pcommon.Timestamp(timestamp))
		dp.SetDoubleValue(value)
	}

	appendInt("above-2^53", 100, 1<<53)
	appendInt("above-2^53", 200, 1<<53+1)
	appendInt("max-int64", 100, math.MaxInt64-1)
	appendInt("max-int64", 200, math.MaxInt64)
	appendInt("full-signed-span", 100, math.MinInt64)
	appendInt("full-signed-span", 200, math.MaxInt64)
	appendInt("reset", 100, 100)
	appendInt("reset", 200, 7)
	appendInt("zero", 100, 0)
	appendInt("zero", 200, 0)
	appendInt("negative", 100, -5)
	appendInt("negative", 200, -4)
	appendInt("first", 100, 42)
	appendInt("tie", 100, 10)
	appendInt("tie", 100, 11)
	appendDouble("double", 100, 1.25)
	appendDouble("double", 200, 2.5)
	appendDouble("double-integral", 100, 1)
	appendDouble("double-integral", 200, 2)
	appendDouble("double-reset", 100, 100)
	appendDouble("double-reset", 200, 7)
	appendDouble("double-negative-reset", 100, -1.5)
	appendDouble("double-negative-reset", 200, -2.25)
	appendDouble("double-signed-zero", 100, math.Copysign(0, -1))
	appendDouble("double-signed-zero", 200, 0)
	appendInt("mixed-double-after-int", 100, 1<<53+1)
	appendDouble("mixed-double-after-int", 200, float64(1<<53))
	appendDouble("mixed-int-after-double", 100, float64(1<<53))
	appendInt("mixed-int-after-double", 200, 1<<53+1)
	appendInt("mixed-fraction-down", 100, 3)
	appendDouble("mixed-fraction-down", 200, 2.5)
	appendDouble("mixed-fraction-up", 100, 2.5)
	appendInt("mixed-fraction-up", 200, 3)
	appendInt("mixed-max-to-double", 100, math.MaxInt64)
	appendDouble("mixed-max-to-double", 200, 9223372036854775808.0)
	appendDouble("mixed-double-to-max", 100, 9223372036854775808.0)
	appendInt("mixed-double-to-max", 200, math.MaxInt64)
	appendInt("mixed-min-to-double", 100, math.MinInt64)
	appendDouble("mixed-min-to-double", 200, -9223372036854775808.0)
	appendDouble("mixed-double-to-min-plus-one", 100, -9223372036854775808.0)
	appendInt("mixed-double-to-min-plus-one", 200, math.MinInt64+1)
	appendInt("mixed-above-max", 100, math.MaxInt64)
	appendDouble("mixed-above-max", 200, 9223372036854777856.0)
	appendDouble("mixed-above-max-reset", 100, 9223372036854777856.0)
	appendInt("mixed-above-max-reset", 200, math.MaxInt64)
	appendDouble("mixed-below-min", 100, -9223372036854777856.0)
	appendInt("mixed-below-min", 200, math.MinInt64)
	appendInt("mixed-below-min-reset", 100, math.MinInt64)
	appendDouble("mixed-below-min-reset", 200, -9223372036854777856.0)
	appendDouble("mixed-hugeint-overflow", 100, -math.Ldexp(1, 127))
	appendInt("mixed-hugeint-overflow", 200, math.MaxInt64)
	appendInt("mixed-hugeint-reset", 100, math.MaxInt64)
	appendDouble("mixed-hugeint-reset", 200, -math.Ldexp(1, 127))
	appendDouble("double-overflow", 100, -math.MaxFloat64)
	appendDouble("double-overflow", 200, math.MaxFloat64)

	empty := sum.DataPoints().AppendEmpty()
	empty.Attributes().PutStr("series", "empty-then-int")
	empty.SetTimestamp(100)
	appendInt("empty-then-int", 200, 1)
	appendInt("int-empty-int", 100, 10)
	emptyBetween := sum.DataPoints().AppendEmpty()
	emptyBetween.Attributes().PutStr("series", "int-empty-int")
	emptyBetween.SetTimestamp(200)
	appendInt("int-empty-int", 300, 15)

	nonmonotonicMetric := rm.ScopeMetrics().At(0).Metrics().AppendEmpty()
	nonmonotonicMetric.SetName("test.nonmonotonic")
	nonmonotonic := nonmonotonicMetric.SetEmptySum()
	nonmonotonic.SetIsMonotonic(false)
	nonmonotonic.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	for i, value := range []int64{math.MaxInt64, math.MinInt64} {
		dp := nonmonotonic.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(100 + i*100))
		dp.SetIntValue(value)
	}
	nonmonotonicMixedMetric := rm.ScopeMetrics().At(0).Metrics().AppendEmpty()
	nonmonotonicMixedMetric.SetName("test.nonmonotonic-mixed-overflow")
	nonmonotonicMixed := nonmonotonicMixedMetric.SetEmptySum()
	nonmonotonicMixed.SetIsMonotonic(false)
	nonmonotonicMixed.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	firstMixed := nonmonotonicMixed.DataPoints().AppendEmpty()
	firstMixed.SetTimestamp(100)
	firstMixed.SetIntValue(math.MaxInt64)
	secondMixed := nonmonotonicMixed.DataPoints().AppendEmpty()
	secondMixed.SetTimestamp(200)
	secondMixed.SetDoubleValue(-math.Ldexp(1, 127))
	nonmonotonicEmptyMetric := rm.ScopeMetrics().At(0).Metrics().AppendEmpty()
	nonmonotonicEmptyMetric.SetName("test.nonmonotonic-empty")
	nonmonotonicEmpty := nonmonotonicEmptyMetric.SetEmptySum()
	nonmonotonicEmpty.SetIsMonotonic(false)
	nonmonotonicEmpty.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	for i, value := range []int64{10, 15} {
		dp := nonmonotonicEmpty.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(100 + i*200))
		dp.SetIntValue(value)
	}
	emptyNonmonotonic := nonmonotonicEmpty.DataPoints().AppendEmpty()
	emptyNonmonotonic.SetTimestamp(200)

	nonfiniteMetric := rm.ScopeMetrics().At(0).Metrics().AppendEmpty()
	nonfiniteMetric.SetName("test.nonfinite")
	nonfinite := nonfiniteMetric.SetEmptySum()
	nonfinite.SetIsMonotonic(true)
	nonfinite.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	for i, value := range []float64{1, math.Float64frombits(0x7ff8000000000001), math.Float64frombits(0xfff8000000000002), math.Inf(1), math.Inf(-1), 3} {
		dp := nonfinite.DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(100 + i*100))
		dp.SetDoubleValue(value)
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, data)
	}))
	raw := getMetricFullByNameInRange(t, s, ctx, "test.counter", store.BoundedTimeRange(0, 400))

	bySeries := make(map[string]map[string]any)
	for _, rawSeries := range raw["timeseries"].([]any) {
		series := rawSeries.(map[string]any)
		attribute := series["labelPreviews"].([]any)[0].(map[string]any)
		name := attribute["text"].(string)
		bySeries[name] = series
	}
	latestFor := func(name string) map[string]any {
		return bySeries[name]["datapoints"].([]any)[0].(map[string]any)
	}

	require.Equal(t, "1", latestFor("above-2^53")["delta"])
	require.Equal(t, "1", latestFor("max-int64")["delta"])
	require.Equal(t, "18446744073709551615", latestFor("full-signed-span")["delta"])
	require.Equal(t, "7", latestFor("reset")["delta"])
	require.Equal(t, true, latestFor("reset")["isReset"])
	require.Equal(t, "0", latestFor("zero")["delta"])
	require.Equal(t, "1", latestFor("negative")["delta"])
	require.Equal(t, 1.25, latestFor("double")["delta"])
	require.Equal(t, float64(1), latestFor("double-integral")["delta"])
	require.Equal(t, float64(7), latestFor("double-reset")["delta"])
	require.Equal(t, true, latestFor("double-reset")["isReset"])
	require.Equal(t, -2.25, latestFor("double-negative-reset")["delta"])
	require.Equal(t, true, latestFor("double-negative-reset")["isReset"])
	require.Equal(t, float64(0), latestFor("double-signed-zero")["delta"])
	require.Equal(t, "1", latestFor("mixed-int-after-double")["delta"])
	require.Equal(t, false, latestFor("mixed-int-after-double")["isReset"])
	require.Equal(t, "9007199254740992", latestFor("mixed-double-after-int")["delta"])
	require.Equal(t, true, latestFor("mixed-double-after-int")["isReset"])
	require.Equal(t, 2.5, latestFor("mixed-fraction-down")["delta"])
	require.Equal(t, true, latestFor("mixed-fraction-down")["isReset"])
	require.Equal(t, 0.5, latestFor("mixed-fraction-up")["delta"])
	require.Equal(t, false, latestFor("mixed-fraction-up")["isReset"])
	require.Equal(t, "1", latestFor("mixed-max-to-double")["delta"])
	require.Equal(t, false, latestFor("mixed-max-to-double")["isReset"])
	require.Equal(t, "9223372036854775807", latestFor("mixed-double-to-max")["delta"])
	require.Equal(t, true, latestFor("mixed-double-to-max")["isReset"])
	require.Equal(t, "0", latestFor("mixed-min-to-double")["delta"])
	require.Equal(t, "1", latestFor("mixed-double-to-min-plus-one")["delta"])
	require.Equal(t, "2049", latestFor("mixed-above-max")["delta"])
	require.Equal(t, "9223372036854775807", latestFor("mixed-above-max-reset")["delta"])
	require.Equal(t, "2048", latestFor("mixed-below-min")["delta"])
	require.Equal(t, "-9223372036854777856", latestFor("mixed-below-min-reset")["delta"])
	require.Equal(t, math.Ldexp(1, 127), latestFor("mixed-hugeint-overflow")["delta"])
	require.Equal(t, -math.Ldexp(1, 127), latestFor("mixed-hugeint-reset")["delta"])
	require.Equal(t, "0x7ff0000000000000", latestFor("double-overflow")["delta"])
	require.NotContains(t, latestFor("first"), "delta")
	require.NotContains(t, latestFor("first"), "isReset")
	require.NotContains(t, latestFor("empty-then-int"), "delta")
	require.NotContains(t, latestFor("int-empty-int"), "delta")
	require.NotContains(t, latestFor("int-empty-int"), "isReset")

	tied := bySeries["tie"]["datapoints"].([]any)
	require.Len(t, tied, 2)
	require.NotContains(t, tied[0].(map[string]any), "delta")
	require.Contains(t, tied[1].(map[string]any), "delta")

	metricID := findMetricRef(t, s, ctx, "test.counter")
	withViews, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, metricID, store.BoundedTimeRange(0, 300),
			0, nil, nil, 0, 2, 2, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var viewsMetric map[string]any
	require.NoError(t, json.Unmarshal(withViews, &viewsMetric))
	for _, rawSeries := range viewsMetric["timeseries"].([]any) {
		series := rawSeries.(map[string]any)
		attribute := series["labelPreviews"].([]any)[0].(map[string]any)
		switch attribute["text"] {
		case "above-2^53":
			views := series["views"].([]any)
			require.NotEmpty(t, views)
			require.Positive(t, views[len(views)-1].(map[string]any)["rate"].(float64))
		case "int-empty-int":
			var sampleCount float64
			for _, rawView := range series["views"].([]any) {
				sampleCount += rawView.(map[string]any)["sampleCount"].(float64)
			}
			require.Equal(t, float64(2), sampleCount)
			sparkline := series["sparkline"].([]any)
			require.Len(t, sparkline, 2)
			for _, rawPoint := range sparkline {
				require.NotNil(t, rawPoint.(map[string]any)["value"])
			}
		}
	}

	nonmonotonicRaw := getMetricFullByNameInRange(t, s, ctx, "test.nonmonotonic", store.BoundedTimeRange(0, 300))
	nonmonotonicDatapoints := metricDatapoints(nonmonotonicRaw)
	require.Len(t, nonmonotonicDatapoints, 2)
	latest := nonmonotonicDatapoints[0].(map[string]any)
	require.Equal(t, "-18446744073709551615", latest["delta"])
	require.Equal(t, false, latest["isReset"])

	nonmonotonicMixedRaw := getMetricFullByNameInRange(t, s, ctx, "test.nonmonotonic-mixed-overflow", store.BoundedTimeRange(0, 300))
	nonmonotonicMixedDatapoints := metricDatapoints(nonmonotonicMixedRaw)
	require.Len(t, nonmonotonicMixedDatapoints, 2)
	latestMixed := nonmonotonicMixedDatapoints[0].(map[string]any)
	require.Equal(t, -math.Ldexp(1, 127), latestMixed["delta"])
	require.Equal(t, false, latestMixed["isReset"])

	nonmonotonicEmptyRaw := getMetricFullByNameInRange(t, s, ctx, "test.nonmonotonic-empty", store.BoundedTimeRange(0, 400))
	nonmonotonicEmptyDatapoints := metricDatapoints(nonmonotonicEmptyRaw)
	require.Len(t, nonmonotonicEmptyDatapoints, 3)
	latestAfterEmpty := nonmonotonicEmptyDatapoints[0].(map[string]any)
	require.NotContains(t, latestAfterEmpty, "delta")
	require.NotContains(t, latestAfterEmpty, "isReset")

	// Non-finite observations are excluded from arithmetic, so the finite points
	// span a delta of two. Detail still carries every received value through the
	// hexadecimal double wire form where JSON numbers cannot represent it.
	nonfiniteID := findMetricRef(t, s, ctx, "test.nonfinite")
	nonfiniteRaw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, nonfiniteID, store.BoundedTimeRange(0, 700),
			0, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	require.True(t, json.Valid(nonfiniteRaw))
	require.Contains(t, string(nonfiniteRaw), `"doubleValue":"0x7ff8000000000001"`)
	require.Contains(t, string(nonfiniteRaw), `"doubleValue":"0xfff8000000000002"`)
	require.Contains(t, string(nonfiniteRaw), `"doubleValue":"0x7ff0000000000000"`)
	require.Contains(t, string(nonfiniteRaw), `"doubleValue":"0xfff0000000000000"`)
	require.Contains(t, string(nonfiniteRaw), `"delta":2.0`)
}

func TestScalarMetricViewIncludesInactiveValueNulls(t *testing.T) {
	for _, metricCase := range []struct {
		name        string
		temporality pmetric.AggregationTemporality
	}{
		{name: "Gauge"},
		{name: "Sum.Delta", temporality: pmetric.AggregationTemporalityDelta},
		{name: "Sum.Cumulative", temporality: pmetric.AggregationTemporalityCumulative},
	} {
		for _, valueType := range []string{"Int", "Double", "Empty"} {
			t.Run(metricCase.name+"/"+valueType, func(t *testing.T) {
				s, ctx := storetest.New(t)
				data := pmetric.NewMetrics()
				metric := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
				metric.SetName("scalar.nulls")
				var points pmetric.NumberDataPointSlice
				if metricCase.name == "Gauge" {
					points = metric.SetEmptyGauge().DataPoints()
				} else {
					sum := metric.SetEmptySum()
					sum.SetAggregationTemporality(metricCase.temporality)
					sum.SetIsMonotonic(true)
					points = sum.DataPoints()
				}
				dp := points.AppendEmpty()
				dp.SetTimestamp(100)
				switch valueType {
				case "Int":
					dp.SetIntValue(math.MaxInt64)
				case "Double":
					dp.SetDoubleValue(math.Copysign(0, -1))
				}
				require.NoError(t, s.WithConn(func(conn driver.Conn) error {
					return metrics.Ingest(ctx, conn, data)
				}))
				view := getMetricFullByNameInRange(t, s, ctx, "scalar.nulls", store.TimeRange{})
				datapoints := metricDatapoints(view)
				require.Len(t, datapoints, 1)
				point := datapoints[0].(map[string]any)
				require.Contains(t, point, "intValue", "an inactive field must be present, not merely compare equal to nil")
				require.Contains(t, point, "doubleValue")
				assert.Equal(t, valueType, point["valueType"])
				switch valueType {
				case "Int":
					assert.Equal(t, "9223372036854775807", point["intValue"])
					assert.Nil(t, point["doubleValue"])
				case "Double":
					assert.Nil(t, point["intValue"])
					assert.Equal(t, "0x8000000000000000", point["doubleValue"])
				case "Empty":
					assert.Nil(t, point["intValue"])
					assert.Nil(t, point["doubleValue"])
				}
				assert.NotContains(t, point, "exemplarCount", "no exemplars were withheld")
				if metricCase.name != "Gauge" {
					assert.Equal(t, float64(metricCase.temporality), point["aggregationTemporalityCode"])
					assert.Equal(t, true, point["isMonotonic"])
				}
				assert.NotContains(t, point, "delta", "Delta and first Cumulative readings have no computed delta")
				if metricCase.temporality != pmetric.AggregationTemporalityCumulative {
					assert.NotContains(t, point, "isReset")
				}
			})
		}
	}
}

func TestGaugeGapsPreserveRealBoundaries(t *testing.T) {
	s, ctx := storetest.New(t)
	base := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).UnixNano()
	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("gaps")
	points := m.SetEmptyGauge().DataPoints()
	add := func(series string, second int64, value int64) {
		p := points.AppendEmpty()
		p.SetTimestamp(pcommon.Timestamp(base + second*int64(time.Second)))
		p.SetIntValue(value)
		p.Attributes().PutStr("series", series)
	}
	for _, p := range [][2]int64{{0, 0}, {1, -10}, {2, 100}, {3, 42}, {40, 43}, {40, 44}, {41, -5}, {42, 99}, {49, 0}} {
		add("interrupted", p[0], p[1])
	}
	for second := int64(1); second < 60; second += 10 {
		add("continuous", second, 0)
	}
	add("singleton", 50, 1)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	id := findMetricRef(t, s, ctx, "gaps")
	window := store.BoundedTimeRange(base, base+int64(time.Minute)-1)
	type point struct {
		ID          string  `json:"id"`
		Timestamp   string  `json:"timestamp"`
		IntValue    string  `json:"intValue"`
		Value       float64 `json:"value"`
		BreakBefore *bool   `json:"breakBefore"`
	}
	type series struct {
		Datapoints []point `json:"datapoints"`
		Sparkline  []point `json:"sparkline"`
	}
	read := func(buckets, sparkBuckets int64) []series {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricView(ctx, db, id, window, buckets, nil, nil, 0, 0, sparkBuckets, nil, "", nil, 0)
		})
		require.NoError(t, err)
		var view struct {
			Timeseries []series `json:"timeseries"`
		}
		require.NoError(t, json.Unmarshal(raw, &view))
		return view.Timeseries
	}
	stamp := func(second int64) string { return strconv.FormatInt(base+second*int64(time.Second), 10) }
	rows := read(6, 6)
	require.Len(t, rows, 3)
	breaks, sparkBreaks := 0, 0
	for _, row := range rows {
		for i, p := range row.Datapoints {
			if p.BreakBefore == nil {
				continue
			}
			breaks++
			require.True(t, *p.BreakBefore)
			require.Equal(t, stamp(40), p.Timestamp)
			// Newest-first transport: this is the first point after the gap in
			// reverse traversal, including when timestamps are duplicated.
			require.Less(t, i+1, len(row.Datapoints))
			require.Equal(t, stamp(3), row.Datapoints[i+1].Timestamp)
			require.Equal(t, "42", row.Datapoints[i+1].IntValue)
		}
		for i, p := range row.Sparkline {
			if p.BreakBefore == nil {
				continue
			}
			sparkBreaks++
			require.True(t, *p.BreakBefore)
			// The preview keeps extrema, not extra exact gap boundaries.
			require.Equal(t, stamp(41), p.Timestamp)
			require.Greater(t, i, 0)
			require.Equal(t, stamp(2), row.Sparkline[i-1].Timestamp)
			require.Equal(t, float64(100), row.Sparkline[i-1].Value)
			require.Equal(t, float64(-5), p.Value)
		}
	}
	require.Equal(t, 1, breaks, "other series cannot fill a series' empty buckets")
	require.Equal(t, 1, sparkBreaks, "sparkline extrema inherit the main chart's break")
	for _, buckets := range []int64{0, 1} {
		for _, row := range read(buckets, 1) {
			for _, p := range append(row.Datapoints, row.Sparkline...) {
				require.Nil(t, p.BreakBefore, "no empty M4 bucket means no inferred break")
			}
		}
	}
	for _, row := range read(6, 1) {
		require.LessOrEqual(t, len(row.Sparkline), 2, "gaps must not add preview points beyond its budget")
		for _, p := range row.Sparkline {
			require.Nil(t, p.BreakBefore, "a coarse sparkline need not show the main chart's gap")
		}
	}
	for _, row := range read(6, 0) {
		require.Nil(t, row.Sparkline, "disabled sparklines remain absent")
	}
	// The native Arrow path carries true/null, never false. Both transports
	// retain the same break on main-chart and sparkline points.
	_, err := storetest.Read(s, func(db *sql.DB) (int, error) {
		result, err := metrics.ViewArrow(ctx, db, id, window, 6, nil, nil, 0, 0, 6, nil, "", nil, 0, false, "")
		if err != nil {
			return 0, err
		}
		defer result.Close()
		var wire bytes.Buffer
		if err := result.Write(ctx, &wire); err != nil {
			return 0, err
		}
		reader, err := ipc.NewReader(&wire)
		if err != nil {
			return 0, err
		}
		defer reader.Release()
		require.True(t, reader.Next())
		record := reader.RecordBatch()
		rows := record.Column(record.Schema().FieldIndices("timeseries")[0]).(*array.List).ListValues().(*array.Struct)
		field := func(rows *array.Struct, name string) arrow.Array {
			index, ok := rows.DataType().(*arrow.StructType).FieldIdx(name)
			require.True(t, ok, name)
			return rows.Field(index)
		}
		for _, name := range []string{"datapoints", "sparkline"} {
			points := field(rows, name).(*array.List).ListValues().(*array.Struct)
			flags := field(points, "breakBefore").(*array.Boolean)
			count := 0
			for i := 0; i < flags.Len(); i++ {
				if flags.IsNull(i) {
					continue
				}
				require.True(t, flags.Value(i))
				count++
			}
			require.Equal(t, 1, count, name)
		}
		return 0, reader.Err()
	})
	require.NoError(t, err)
}

func TestSparklineGapsSurviveNumericFiltering(t *testing.T) {
	for _, tc := range []struct {
		name           string
		value          float64
		empty          bool
		beforeInvalid  bool
		fillBuckets    bool
		invalidSection bool
	}{
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
		{name: "empty", empty: true},
		{name: "both boundaries invalid", value: math.NaN(), beforeInvalid: true},
		{name: "invalid-only section", value: math.NaN(), invalidSection: true},
		{name: "invalid readings occupy buckets", value: math.NaN(), fillBuckets: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx := storetest.New(t)
			base := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).UnixNano()
			data := pmetric.NewMetrics()
			m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			m.SetName("sparkline-filtering")
			points := m.SetEmptyGauge().DataPoints()
			for _, sample := range []struct {
				second int64
				value  float64
			}{
				{0, 0}, {1, -10}, {2, 100}, {3, 42}, {40, 44}, {41, 43}, {41, 44}, {42, 99}, {49, 0},
			} {
				p := points.AppendEmpty()
				p.SetTimestamp(pcommon.Timestamp(base + sample.second*int64(time.Second)))
				if sample.second == 40 || (sample.second == 3 && tc.beforeInvalid) {
					if !tc.empty {
						p.SetDoubleValue(tc.value)
					}
				} else {
					p.SetDoubleValue(sample.value)
				}
			}
			for _, second := range []int64{10, 20, 30} {
				if tc.fillBuckets || (tc.invalidSection && second == 20) {
					p := points.AppendEmpty()
					p.SetTimestamp(pcommon.Timestamp(base + second*int64(time.Second)))
					if second != 10 {
						p.SetDoubleValue(math.NaN())
					}
				}
			}
			require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
			id := findMetricRef(t, s, ctx, "sparkline-filtering")
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				// Preview breaks come from the main chart, before numeric filtering.
				return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(base, base+int64(time.Minute)-1),
					6, nil, nil, 0, 0, 6, nil, "", nil, 0)
			})
			require.NoError(t, err)
			var view struct {
				Timeseries []struct {
					Sparkline []struct {
						Timestamp   string  `json:"timestamp"`
						Value       float64 `json:"value"`
						BreakBefore *bool   `json:"breakBefore"`
					} `json:"sparkline"`
				} `json:"timeseries"`
			}
			require.NoError(t, json.Unmarshal(raw, &view))
			require.Len(t, view.Timeseries, 1)
			breaks := 0
			for i, p := range view.Timeseries[0].Sparkline {
				if p.BreakBefore == nil {
					continue
				}
				breaks++
				require.True(t, *p.BreakBefore)
				require.Equal(t, strconv.FormatInt(base+42*int64(time.Second), 10), p.Timestamp)
				require.Greater(t, i, 0)
				require.Equal(t, strconv.FormatInt(base+2*int64(time.Second), 10), view.Timeseries[0].Sparkline[i-1].Timestamp)
			}
			if tc.fillBuckets {
				require.Zero(t, breaks)
			} else {
				require.Equal(t, 1, breaks)
			}
		})
	}
}

func TestGaugeGapsUseBucketOccupancyDuringRepeatedHours(t *testing.T) {
	for _, tc := range []struct {
		name          string
		clocks        []string
		occupiedClock string
		buckets       int64
		boundaries    map[string]string // following sample -> preceding sample, in UTC
	}{
		{
			name:          "half-hour buckets",
			clocks:        []string{"04:15", "04:45", "05:15", "06:15", "07:15", "07:45"},
			occupiedClock: "05:45", buckets: 8,
			boundaries: map[string]string{"07:15": "06:15"},
		},
		{
			name:          "both passes of the repeated hour",
			clocks:        []string{"04:45", "05:00", "05:30", "05:45", "06:00", "06:30", "06:45", "07:00", "07:15", "07:30", "07:45"},
			occupiedClock: "05:15", buckets: 16,
			boundaries: map[string]string{"05:30": "05:00", "06:30": "06:00"},
		},
	} {
		for _, missing := range []bool{false, true} {
			t.Run(tc.name+"/missing="+strconv.FormatBool(missing), func(t *testing.T) {
				s, ctx := storetest.New(t)
				data := pmetric.NewMetrics()
				m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
				m.SetName("calendar-occupancy")
				points := m.SetEmptyGauge().DataPoints()
				stamp := func(clock string) int64 {
					timestamp, err := time.Parse(time.RFC3339, "2026-11-01T"+clock+":00Z")
					require.NoError(t, err)
					return timestamp.UnixNano()
				}
				clocks := append([]string{}, tc.clocks...)
				if !missing {
					clocks = append(clocks, tc.occupiedClock)
				}
				for _, clock := range clocks {
					p := points.AppendEmpty()
					p.SetTimestamp(pcommon.Timestamp(stamp(clock)))
					p.SetIntValue(0)
				}
				require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
				id := findMetricRef(t, s, ctx, "calendar-occupancy")
				start := time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC)
				raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
					return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(start.UnixNano(), start.Add(4*time.Hour).UnixNano()-1),
						tc.buckets, nil, nil, 0, 0, tc.buckets, nil, "America/New_York", nil, 0)
				})
				require.NoError(t, err)
				type point struct {
					Timestamp   string `json:"timestamp"`
					BreakBefore *bool  `json:"breakBefore"`
				}
				var view struct {
					Timeseries []struct {
						Datapoints []point `json:"datapoints"`
						Sparkline  []point `json:"sparkline"`
					} `json:"timeseries"`
				}
				require.NoError(t, json.Unmarshal(raw, &view))
				require.Len(t, view.Timeseries, 1)
				want := map[string]string{}
				if missing {
					for after, before := range tc.boundaries {
						want[strconv.FormatInt(stamp(after), 10)] = strconv.FormatInt(stamp(before), 10)
					}
				}
				for name, points := range map[string][]point{
					"main":      view.Timeseries[0].Datapoints,
					"sparkline": view.Timeseries[0].Sparkline,
				} {
					got := map[string]string{}
					for i, p := range points {
						if p.BreakBefore == nil {
							continue
						}
						require.True(t, *p.BreakBefore)
						previous := i - 1
						if name == "main" {
							previous = i + 1
						}
						require.GreaterOrEqual(t, previous, 0)
						require.Less(t, previous, len(points))
						got[p.Timestamp] = points[previous].Timestamp
					}
					require.Equal(t, want, got, name)
				}
			})
		}
	}
}

func TestGaugeGapsFollowCalendarBuckets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		times   []string
		buckets int64
		breaks  int
		zone    string
	}{
		{"spring skipped hour", []string{"2026-03-08T06:30:00Z", "2026-03-08T07:30:00Z", "2026-03-08T08:30:00Z"}, 2, 0, "America/New_York"},
		{"spring real empty hour", []string{"2026-03-08T06:30:00Z", "2026-03-08T07:30:00Z", "2026-03-08T09:30:00Z"}, 3, 1, "America/New_York"},
		{"fall 25-hour day", []string{"2026-10-31T16:00:00Z", "2026-11-01T17:00:00Z", "2026-11-02T17:00:00Z"}, 3, 0, "America/New_York"},
		{"Apia skipped day", []string{"2011-12-29T22:00:00Z", "2011-12-30T22:00:00Z", "2011-12-31T22:00:00Z"}, 3, 0, "Pacific/Apia"},
		{"Apia real empty day", []string{"2011-12-29T22:00:00Z", "2011-12-30T22:00:00Z", "2012-01-02T22:00:00Z"}, 4, 1, "Pacific/Apia"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ctx := storetest.New(t)
			data := pmetric.NewMetrics()
			m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			m.SetName("calendar")
			points := m.SetEmptyGauge().DataPoints()
			for _, text := range tc.times {
				timestamp, err := time.Parse(time.RFC3339, text)
				require.NoError(t, err)
				p := points.AppendEmpty()
				p.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
				p.SetIntValue(0)
			}
			require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
			id := findMetricRef(t, s, ctx, "calendar")
			raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
				return metrics.GetMetricView(ctx, db, id, store.TimeRange{}, tc.buckets, nil, nil, 0, 0, 0, nil, tc.zone, nil, 0)
			})
			require.NoError(t, err)
			require.Equal(t, tc.breaks, bytes.Count(raw, []byte(`"breakBefore":true`)))
		})
	}
}

func TestGaugeGapsAcrossClockChanges(t *testing.T) {
	for _, tc := range []struct {
		name, zone, start, outage string
		windowHours               int
		buckets                   int64
		aliasOnly                 bool
	}{
		{"Chatham spring", "Pacific/Chatham", "2026-09-26T12:00:00Z", "2026-09-26T15:15:00Z", 4, 8, false},
		{"Chatham spring UI resolution", "Pacific/Chatham", "2026-09-26T12:00:00Z", "2026-09-26T15:15:00Z", 172, 500, false},
		{"Chatham skipped label only", "Pacific/Chatham", "2026-09-26T12:00:00Z", "2026-09-26T15:15:00Z", 4, 8, true},
		{"Chatham fall", "Pacific/Chatham", "2026-04-04T12:00:00Z", "2026-04-04T15:15:00Z", 4, 8, false},
		{"Lord Howe half-hour spring", "Australia/Lord_Howe", "2026-10-03T14:00:00Z", "2026-10-03T17:00:00Z", 4, 8, false},
		{"Lord Howe half-hour fall", "Australia/Lord_Howe", "2026-04-04T14:00:00Z", "2026-04-04T17:00:00Z", 4, 8, false},
		{"New York spring", "America/New_York", "2026-03-08T05:00:00Z", "2026-03-08T08:00:00Z", 4, 8, false},
		{"Kathmandu quarter-hour change", "Asia/Kathmandu", "1985-12-31T17:00:00Z", "1985-12-31T20:15:00Z", 4, 8, false},
		{"Kathmandu quarter-hour buckets", "Asia/Kathmandu", "1985-12-31T17:00:00Z", "1985-12-31T20:15:00Z", 4, 16, false},
		{"Monrovia seconds offset change", "Africa/Monrovia", "1972-01-06T23:00:00Z", "1972-01-07T02:00:00Z", 4, 8, false},
		{"Apia skipped date", "Pacific/Apia", "2011-12-30T08:00:00Z", "2011-12-30T11:00:00Z", 4, 8, false},
	} {
		for _, missing := range []bool{false, true} {
			t.Run(tc.name+"/missing="+strconv.FormatBool(missing), func(t *testing.T) {
				parse := func(text string) time.Time {
					timestamp, err := time.Parse(time.RFC3339, text)
					require.NoError(t, err)
					return timestamp
				}
				start, outage := parse(tc.start), parse(tc.outage)
				s, ctx := storetest.New(t)
				data := pmetric.NewMetrics()
				m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
				m.SetName("clock-change")
				points := m.SetEmptyGauge().DataPoints()
				var before, after time.Time
				for timestamp := start; timestamp.Before(start.Add(4 * time.Hour)); timestamp = timestamp.Add(5 * time.Minute) {
					// Keep the skipped 03:30 label's samples, but omit all samples
					// labelled 04:30 that resolve to the same UTC bucket. Choosing
					// max(source label) must not substitute for resolving the bucket.
					if tc.aliasOnly && !timestamp.Before(start.Add(165*time.Minute)) && timestamp.Before(start.Add(195*time.Minute)) {
						continue
					}
					if missing && !timestamp.Before(outage) && timestamp.Before(outage.Add(30*time.Minute)) {
						continue
					}
					if timestamp.Before(outage) {
						before = timestamp
					} else if after.IsZero() {
						after = timestamp
					}
					p := points.AppendEmpty()
					p.SetTimestamp(pcommon.NewTimestampFromTime(timestamp))
					p.SetIntValue(0)
				}
				require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
				id := findMetricRef(t, s, ctx, "clock-change")
				raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
					window := store.BoundedTimeRange(start.UnixNano(), start.Add(time.Duration(tc.windowHours)*time.Hour).UnixNano()-1)
					return metrics.GetMetricView(ctx, db, id, window, tc.buckets, nil, nil, 0, 0, 0, nil, tc.zone, nil, 0)
				})
				require.NoError(t, err)
				var view struct {
					Timeseries []struct {
						Datapoints []struct {
							Timestamp   string `json:"timestamp"`
							BreakBefore *bool  `json:"breakBefore"`
						} `json:"datapoints"`
					} `json:"timeseries"`
				}
				require.NoError(t, json.Unmarshal(raw, &view))
				require.Len(t, view.Timeseries, 1)
				got := map[string]string{}
				for i, p := range view.Timeseries[0].Datapoints {
					if p.BreakBefore == nil {
						continue
					}
					require.True(t, *p.BreakBefore)
					require.Less(t, i+1, len(view.Timeseries[0].Datapoints))
					got[p.Timestamp] = view.Timeseries[0].Datapoints[i+1].Timestamp
				}
				want := map[string]string{}
				if missing {
					want[strconv.FormatInt(after.UnixNano(), 10)] = strconv.FormatInt(before.UnixNano(), 10)
				}
				require.Equal(t, want, got, "breaks must retain the actual samples surrounding only the empty bucket")
			})
		}
	}
}

func TestGaugeGapsAcrossSkippedDateAtMillisecondResolution(t *testing.T) {
	s, ctx := storetest.New(t)
	transition := time.Date(2011, 12, 30, 10, 0, 0, 0, time.UTC)
	start := transition.Add(-time.Second)
	data := pmetric.NewMetrics()
	m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("skipped-date-fine-grid")
	points := m.SetEmptyGauge().DataPoints()
	for i := 0; i < 2000; i++ {
		p := points.AppendEmpty()
		p.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Duration(i) * time.Millisecond)))
		p.SetIntValue(0)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data) }))
	id := findMetricRef(t, s, ctx, "skipped-date-fine-grid")
	raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, id, store.BoundedTimeRange(start.UnixNano(), start.Add(2*time.Second).UnixNano()-1),
			500, nil, nil, 0, 0, 0, nil, "Pacific/Apia", nil, 0)
	})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"timestamp"`)
	require.NotContains(t, string(raw), `"breakBefore":true`)
}
