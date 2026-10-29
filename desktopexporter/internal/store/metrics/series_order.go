package metrics

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
)

type seriesOrderShape struct{ Descending bool }

func seriesOrderArgs(metricID string, window timerange.TimeRange, seriesIDs []string, key string) []any {
	var start, end any
	if window.Start != nil {
		start = []uint64{*window.Start}
	}
	if window.End != nil {
		end = []uint64{*window.End}
	}
	return []any{metricID, start, end, seriesIDs, key}
}

// GetMetricSeriesOrder returns only IDs. The database clamps the metric's points
// to the requested inclusive window and explicit cohort before preparing keys.
func GetMetricSeriesOrder(ctx context.Context, db *sql.DB, metricID string, window timerange.TimeRange, seriesIDs []string, key string, descending bool) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetMetricSeriesOrder, seriesOrderShape{descending})
	if err != nil {
		return nil, err
	}
	var result []byte
	if err := db.QueryRowContext(ctx, query, seriesOrderArgs(metricID, window, seriesIDs, key)...).Scan(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func SeriesOrderArrow(ctx context.Context, db *sql.DB, metricID string, window timerange.TimeRange, seriesIDs []string, key string, descending bool) (*arrowread.Result, error) {
	query, err := queries.RenderArrow(queries.GetMetricSeriesOrder, seriesOrderShape{descending})
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query, seriesOrderArgs(metricID, window, seriesIDs, key)...)
}
