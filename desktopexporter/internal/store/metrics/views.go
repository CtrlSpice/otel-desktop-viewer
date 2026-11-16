package metrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
)

// GetMetricView returns the chart/UI projection for a Metric in the time window.
// An unknown metricID returns ErrMetricIDNotFound; a known Metric with no
// datapoints in the window returns valid MetricViewData with an empty timeseries
// list (the two are distinct: only the former is a "not found").
// targetBuckets is how many time buckets to reduce the window to; 0 means no
// reduction, and the caller gets every datapoint. Reduction is opt-in because
// it changes which datapoints exist in the response, and a caller that has not
// asked for it should not have to discover that.
// seriesIDs narrows the response to those series; nil returns every series and
// an empty slice returns none. The filter is applied before the reduction, so
// a caller asking for two of ten series pays for two.
// quantiles are computed per histogram datapoint and returned keyed by the
// quantile; empty skips the work.
// viewBuckets is the resolution the Sum / Average / Rate views aggregate onto,
// which is a different question from targetBuckets: the election thins while
// keeping the line's shape, the views bucket for a chart.
// sparklineBuckets is the resolution of the per-series sparkline, a third
// question again: it fits a list row rather than a chart, and the reduction
// keeps each bucket's min and max so a spike survives at that size. 0 skips it
// and every series comes back with a null sparkline. It is computed for every
// series in the response, including unselected ones, because the sparkline is
// how a user decides which series to select.
// selectedSeriesIDs names the pool the Selected cross-series line folds. It
// narrows nothing else: the All line keeps folding every series in the Metric,
// and nil means nothing is checked, so only the All line is drawn.
// datapointSeriesIDs and datapointSeriesLimit decide which series ship their
// datapoints -- almost the whole payload. Every series keeps its row, stats,
// view buckets and sparkline regardless, so the panel can still list the ones
// nobody is drawing and the All aggregate can still fold them. The limit is
// for a caller that cannot name the series it wants because it picks them from
// this very response; it takes the first N in the response's own order. Nil
// and 0 mean every series ships them.
// Series attributes are empty; attributesComplete=false marks this projection.
// labelPreviews contain bounded distinguishing-column text. GetMetricSeriesOrder
// sorts the timestamp-clamped cohort in the database; GetMetricSeries returns
// complete received context on demand.
// A null endpoint is omitted from filtering and derived from the filtered data
// extent. A concrete endpoint remains the effective endpoint.
func GetMetricView(ctx context.Context, db *sql.DB, metricID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, sparklineBuckets int64, selectedSeriesIDs []string, tzName string, datapointSeriesIDs []string, datapointSeriesLimit int64) (json.RawMessage, error) {
	return getMetricView(ctx, db, metricViewShapeFor(ctx, db, metricID), metricID, timeRange, targetBuckets, seriesIDs, quantiles, tzOffsetNs, viewBuckets, sparklineBuckets, selectedSeriesIDs, tzName, datapointSeriesIDs, datapointSeriesLimit)
}

// getMetricView runs the chart query in whichever shape params asks for. Both shapes take
// the same arguments and the same CTEs; only the projection differs.
func getMetricView(ctx context.Context, db *sql.DB, params getMetricViewParams, metricID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, sparklineBuckets int64, selectedSeriesIDs []string, tzName string, datapointSeriesIDs []string, datapointSeriesLimit int64) (json.RawMessage, error) {
	query, args, err := metricViewSQL(params, metricID, timeRange, targetBuckets, seriesIDs, quantiles, tzOffsetNs, viewBuckets, sparklineBuckets, selectedSeriesIDs, tzName, datapointSeriesIDs, datapointSeriesLimit, false)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricView: %w", ErrMetricIDNotFound)
		}
		return nil, fmt.Errorf("GetMetricView: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, fmt.Errorf("GetMetricView: %w: query returned null", ErrMetricsStoreInternal)
	}
	return json.RawMessage(raw), nil
}

func metricViewSQL(params getMetricViewParams, metricID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, sparklineBuckets int64, selectedSeriesIDs []string, tzName string, datapointSeriesIDs []string, datapointSeriesLimit int64, native bool) (string, []any, error) {
	// Deduplicate the quantile list, keeping first-occurrence order.
	//
	// DuckDB map keys must be unique. Preserve first-occurrence request order.
	if len(quantiles) > 1 {
		seen := make(map[float64]bool, len(quantiles))
		deduped := quantiles[:0:0]
		for _, q := range quantiles {
			if !seen[q] {
				seen[q] = true
				deduped = append(deduped, q)
			}
		}
		quantiles = deduped
	}

	// Every view filters the owned datapoints by metric_id.
	params.HasStart = timeRange.Start != nil
	params.HasEnd = timeRange.End != nil

	// A nil slice binds as an empty array, not SQL NULL, so "all series" has
	// to travel as an untyped nil. Empty then keeps its own meaning: no series.
	var seriesArg any
	if seriesIDs != nil {
		seriesArg = seriesIDs
	}
	if quantiles == nil {
		quantiles = []float64{}
	}
	// Same untyped-nil dance as seriesArg, and for a different meaning: null is
	// "nothing checked", so the Selected pool is empty and the chart draws All
	// alone. An empty slice would bind as an empty array and say the same thing,
	// but going through nil keeps the two parameters' conventions identical.
	var selectedArg any
	if len(selectedSeriesIDs) > 0 {
		selectedArg = selectedSeriesIDs
	}
	// nil and empty are different questions here, as they are for seriesArg:
	// nil means the caller is not naming series, so the limit decides, while an
	// empty list names no series and ships no datapoints at all.
	// Empty means "no zone named", which the query reads as a null and answers
	// with the single offset instead -- so a caller that sends no zone keeps the
	// behaviour it had.
	var tzArg any
	if tzName != "" {
		tzArg = tzName
	}
	var datapointArg any
	if datapointSeriesIDs != nil {
		datapointArg = datapointSeriesIDs
	}
	var startTime, endTime any
	if timeRange.Start != nil {
		startTime = []uint64{*timeRange.Start}
	}
	if timeRange.End != nil {
		endTime = []uint64{*timeRange.End}
	}
	render := queries.RenderBound
	if native {
		render = queries.RenderBoundArrow
	}
	return render(queries.GetMetricView, params, map[string]any{
		"metric_id": metricID, "time_start": startTime, "time_end": endTime,
		"target_buckets": targetBuckets, "series_ids": seriesArg, "quantiles": quantiles,
		"tz_offset_ns": tzOffsetNs, "view_buckets": viewBuckets, "sparkline_buckets": sparklineBuckets,
		"selected_series_ids": selectedArg, "tz_name": tzArg, "datapoint_series_ids": datapointArg,
		"datapoint_series_limit": datapointSeriesLimit, "contribution_id": params.ContributionID,
	})
}

// GetMetricAggregateView returns only the cross-series aggregate: the selected
// series merged into one histogram per time bucket.
//
// The aggregate is fetched separately because it changes with legend selection
// while per-series data does not.
//
// A histogram merges the selected series into one histogram per bucket, so the
// caller narrows with seriesIDs and the merge sees only those.
//
// A scalar needs both selectedSeriesIDs and the complete metric so it can
// compute Selected and All without narrowing the latter.
func GetMetricAggregateView(ctx context.Context, db *sql.DB, metricID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, selectedSeriesIDs []string, tzName string, contributionID ...string) (json.RawMessage, error) {
	// The aggregate projection lets DuckDB prune per-series CTEs.
	shape := metricViewShapeFor(ctx, db, metricID)
	shape.AggregateOnly = true
	if len(contributionID) > 0 {
		shape.ContributionID = contributionID[0]
	}
	raw, err := getMetricView(ctx, db, shape,
		metricID, timeRange, targetBuckets, seriesIDs, quantiles,
		tzOffsetNs, viewBuckets, 0, selectedSeriesIDs, tzName,
		[]string{}, 0)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

type getMetricViewParams struct {
	HasStart bool
	HasEnd   bool
	// ContributionID selects interval activity after differencing, never before.
	ContributionID string
	// AggregateOnly emits just the cross-series aggregates, which is all
	// GetMetricAggregateView returns. It drops `timeseries` -- the field that
	// carries the per-series pipelines and most of the planning cost.
	AggregateOnly bool

	// NoHistogramMerge and NoScalarPools drop a chain whose output is already
	// known to be empty for this metric's shape, so the planner never builds
	// it. scalar_dps admits Gauge and Sum only, so a histogram's pools are
	// always []; the histogram merge needs bucket vectors, so a scalar's
	// aggregate is always null.
	//
	// Scalar detail also omits histogram reduction and quantile branches before
	// binding; an empty runtime result would still incur their planning cost.
	NoHistogramMerge bool
	NoScalarPools    bool
	// Explicit histograms have no exponential scale/offset alignment. Delta
	// histograms already contain interval activity and need no predecessor scan.
	ExplicitHistogram bool
	DeltaHistogram    bool
}

// metricViewShapeFor selects the query chains required by the metric type.
func metricViewShapeFor(ctx context.Context, db *sql.DB, metricID string) getMetricViewParams {
	p := getMetricViewParams{}
	var metricType string
	var temporality int
	err := db.QueryRowContext(ctx,
		`select metric_type, aggregation_temporality from metrics where id = ?::uuid`, metricID).Scan(&metricType, &temporality)
	if err != nil {
		// An unresolved Metric id is reported by the main query, so retain both
		// chains and let it return ErrMetricIDNotFound.
		return p
	}
	switch metricType {
	case "Histogram", "ExponentialHistogram":
		p.NoScalarPools = true
		p.ExplicitHistogram = metricType == "Histogram"
		p.DeltaHistogram = temporality == 1
	case "Gauge", "Sum":
		p.NoHistogramMerge = true
	}
	return p
}
