package metrics

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
)

// GetMetricSeriesColumn returns one exact attribute for each retained member of
// the requested cohort. Missing attributes have an empty map; missing series
// have no row. Metric ownership is enforced before selecting series.
func GetMetricSeriesColumn(ctx context.Context, db *sql.DB, metricID string, seriesIDs []string, key string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetMetricSeriesColumn, nil)
	if err != nil {
		return nil, err
	}
	var result []byte
	if err := db.QueryRowContext(ctx, query, metricID, seriesIDs, key).Scan(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func SeriesColumnArrow(ctx context.Context, db *sql.DB, metricID string, seriesIDs []string, key string) (*arrowread.Result, error) {
	query, err := queries.RenderArrow(queries.GetMetricSeriesColumn, nil)
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query, metricID, seriesIDs, key)
}
