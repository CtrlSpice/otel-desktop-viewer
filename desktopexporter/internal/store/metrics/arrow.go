package metrics

import (
	"context"
	"database/sql"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
)

func SearchArrow(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (*arrowread.Result, error) {
	query, args, err := summariesSQL(timeRange, criteria, options, true)
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query, args...)
}

func GetArrow(ctx context.Context, db *sql.DB, metricID string) (*arrowread.Result, error) {
	if _, err := receivedMetricType(ctx, db, "GetMetric", metricID); err != nil {
		return nil, err
	}
	query, err := queries.RenderArrow(queries.GetMetric, nil)
	if err != nil {
		return nil, err
	}
	return readMetricArrow(ctx, db, query, metricID)
}

func SeriesArrow(ctx context.Context, db *sql.DB, metricID, seriesID string, timeRange timerange.TimeRange, includeDatapoints ...bool) (*arrowread.Result, error) {
	if _, err := receivedMetricType(ctx, db, "GetMetricSeries", metricID); err != nil {
		return nil, err
	}
	query, err := queries.RenderArrow(queries.GetMetricSeries, metricSeriesShape(includeDatapoints))
	if err != nil {
		return nil, err
	}
	var start, end any
	if timeRange.Start != nil {
		start = []uint64{*timeRange.Start}
	}
	if timeRange.End != nil {
		end = []uint64{*timeRange.End}
	}
	return readMetricArrow(ctx, db, query, metricID, seriesID, start, end)
}

func readMetricArrow(ctx context.Context, db *sql.DB, query string, args ...any) (*arrowread.Result, error) {
	result, err := arrowread.Read(ctx, db, query, args...)
	if err == nil && result.NumRows() == 0 {
		result.Close()
		return nil, ErrMetricIDNotFound
	}
	return result, err
}

func ViewArrow(ctx context.Context, db *sql.DB, metricID string, timeRange timerange.TimeRange, targetBuckets int64, seriesIDs []string, quantiles []float64, tzOffsetNs int64, viewBuckets int64, sparklineBuckets int64, selectedSeriesIDs []string, tzName string, datapointSeriesIDs []string, datapointSeriesLimit int64, aggregateOnly bool, contributionID string) (*arrowread.Result, error) {
	shape := metricViewShapeFor(ctx, db, metricID)
	shape.AggregateOnly = aggregateOnly
	shape.ContributionID = contributionID
	query, args, err := metricViewSQL(shape, metricID, timeRange, targetBuckets, seriesIDs, quantiles, tzOffsetNs, viewBuckets, sparklineBuckets, selectedSeriesIDs, tzName, datapointSeriesIDs, datapointSeriesLimit, true)
	if err != nil {
		return nil, err
	}
	return readMetricArrow(ctx, db, query, args...)
}
