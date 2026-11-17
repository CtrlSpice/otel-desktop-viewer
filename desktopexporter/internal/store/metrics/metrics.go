package metrics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/nested"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var (
	ErrInvalidMetricQuery    = errors.New("invalid metric search query")
	ErrInvalidMetricLimit    = errors.New("invalid metric search limit")
	ErrMetricIDNotFound      = errors.New("metric ID not found")
	ErrUnsupportedMetricType = nested.ErrUnsupportedMetricType
	ErrMetricsStoreInternal  = errors.New("metrics store internal error")
)

// Ingest appends received metric records, points and exemplars atomically.
func Ingest(ctx context.Context, conn driver.Conn, m pmetric.Metrics) error {
	_, err := IngestReport(ctx, conn, m)
	return err
}

// IngestReport reports rejected Metrics while retaining accepted records.
func IngestReport(ctx context.Context, conn driver.Conn, m pmetric.Metrics) (rejected ingest.Rejected, err error) {
	defer func() {
		err = ingest.InterruptedContextError(ctx, err)
		if rejected.Count() == 0 {
			rejected = ingest.Rejected{}
		}
	}()
	return nested.IngestMetrics(ctx, conn, m)
}

// GetMetric returns the exact stored Metric identity and a catalogue of its
// generated database-local series. Datapoint counts and first/last datapoint
// timestamps are computed from all retained points; no datapoints are returned.
func GetMetric(ctx context.Context, db *sql.DB, metricID string) (json.RawMessage, error) {
	return getReceivedMetric(ctx, db, queries.GetMetric, "GetMetric", metricID)
}

// GetMetricSeries returns one exact series' retained received datapoints in the
// requested timestamp window.
// includeDatapoints=false returns complete received context with an empty
// datapoints array and skips point/exemplar reconstruction. The default is true.
func GetMetricSeries(ctx context.Context, db *sql.DB, metricID, seriesID string, timeRange timerange.TimeRange, includeDatapoints ...bool) (json.RawMessage, error) {
	metricType, err := receivedMetricType(ctx, db, "GetMetricSeries", metricID)
	if err != nil {
		return nil, err
	}
	query, err := queries.Render(queries.GetMetricSeries, metricSeriesShape(includeDatapoints))
	if err != nil {
		return nil, fmt.Errorf("GetMetricSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	var startTime, endTime any
	if timeRange.Start != nil {
		startTime = []uint64{*timeRange.Start}
	}
	if timeRange.End != nil {
		endTime = []uint64{*timeRange.End}
	}
	var queriedMetricType string
	var raw []byte
	if err := db.QueryRowContext(ctx, query, metricID, seriesID, startTime, endTime).Scan(&queriedMetricType, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricSeries: %w", ErrMetricIDNotFound)
		}
		return nil, fmt.Errorf("GetMetricSeries: %w: %w", ErrMetricsStoreInternal, err)
	}
	if queriedMetricType != metricType {
		return nil, fmt.Errorf("GetMetricSeries: %w: Metric type changed during query", ErrMetricsStoreInternal)
	}
	if raw == nil || string(raw) == "null" {
		return nil, fmt.Errorf("GetMetricSeries: %w: query returned null", ErrMetricsStoreInternal)
	}
	return json.RawMessage(raw), nil
}

func metricSeriesShape(includeDatapoints []bool) struct{ ContextOnly bool } {
	return struct{ ContextOnly bool }{ContextOnly: len(includeDatapoints) > 0 && !includeDatapoints[0]}
}

func receivedMetricType(ctx context.Context, db *sql.DB, operation, metricID string) (string, error) {
	var metricType string
	if err := db.QueryRowContext(ctx,
		`select metric_type from metrics where id = ?::uuid`, metricID).Scan(&metricType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%s: %w", operation, ErrMetricIDNotFound)
		}
		return "", fmt.Errorf("%s: %w: %w", operation, ErrMetricsStoreInternal, err)
	}
	if !supportedReceivedMetricType(metricType) {
		return "", fmt.Errorf("%s: %w: %s", operation, ErrUnsupportedMetricType, metricType)
	}
	return metricType, nil
}

func getReceivedMetric(ctx context.Context, db *sql.DB, queryName queries.Name, operation, metricID string) (json.RawMessage, error) {
	query, err := queries.Render(queryName, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %w", operation, ErrMetricsStoreInternal, err)
	}
	var metricType string
	var raw []byte
	if err := db.QueryRowContext(ctx, query, metricID).Scan(&metricType, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w", operation, ErrMetricIDNotFound)
		}
		return nil, fmt.Errorf("%s: %w: %w", operation, ErrMetricsStoreInternal, err)
	}
	if !supportedReceivedMetricType(metricType) {
		return nil, fmt.Errorf("%s: %w: %s", operation, ErrUnsupportedMetricType, metricType)
	}
	if raw == nil || string(raw) == "null" {
		return nil, fmt.Errorf("%s: %w: query returned null", operation, ErrMetricsStoreInternal)
	}
	return json.RawMessage(raw), nil
}

func supportedReceivedMetricType(metricType string) bool {
	switch metricType {
	case "Gauge", "Sum", "Histogram", "ExponentialHistogram":
		return true
	default:
		return false
	}
}

// GetMetricOTLP returns a Metric as a standard OTLP JSON object. Callers
// must transport the returned bytes unchanged because re-encoding can lose -0.0.
func GetMetricOTLP(ctx context.Context, db *sql.DB, metricID string) (json.RawMessage, error) {
	return GetMetricSelectionOTLP(ctx, db, metricID, "", "", true)
}

// GetMetricSelectionOTLP exports only the exact selected stored series or datapoint.
// Empty selectors export the whole retained Metric. Selection never uses chart reductions.
// When includeDatapoints is false, it omits dataPoints and skips their reconstruction.
func GetMetricSelectionOTLP(ctx context.Context, db *sql.DB, metricID, seriesID, datapointID string, includeDatapoints bool) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetMetricOTLP, nil)
	if err != nil {
		return nil, fmt.Errorf("GetMetricOTLP: %w: %w", ErrMetricsStoreInternal, err)
	}

	var metricType string
	var raw []byte
	if err := db.QueryRowContext(ctx, query, seriesID, datapointID, includeDatapoints, metricID).Scan(&metricType, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetMetricOTLP: %w", ErrMetricIDNotFound)
		}
		return nil, fmt.Errorf("GetMetricOTLP: %w: %w", ErrMetricsStoreInternal, err)
	}
	if raw == nil {
		switch metricType {
		case "Gauge", "Sum", "Histogram", "ExponentialHistogram":
			return nil, fmt.Errorf("GetMetricOTLP: %w: query returned null for %s", ErrMetricsStoreInternal, metricType)
		default:
			return nil, fmt.Errorf("GetMetricOTLP: %w: %s", ErrUnsupportedMetricType, metricType)
		}
	}
	return json.RawMessage(raw), nil
}

// Clear removes every Metric, series, datapoint, and exemplar. DuckDB does not
// cascade foreign keys, so rows are deleted explicitly from child to parent.
func Clear(ctx context.Context, db *sql.DB) error {
	return nested.Mutate(ctx, db, `delete from metric_records`)
}

// DeleteMetric removes a Metric, its series, datapoints, and exemplars.
// ingest.SweepOrphans collects unreferenced histogram bounds separately.
//
// We still can't wrap this in a transaction: DuckDB issue #13819 still
// fires "phantom" FK violations for in-tx cascades. The pinned-conn
// auto-commit pattern works around it. A retry of DeleteMetric
// completes any remaining Metric-table cascade; orphaned bounds
// are collected by a later SweepOrphans call after Clear or during retention.
//
// Returns nil if the Metric does not exist (idempotent delete).
func DeleteMetric(ctx context.Context, db *sql.DB, metricID string) error {
	return nested.Mutate(ctx, db, `delete from metric_records where id=?::uuid`, metricID)
}

func metricDatapointWhere(timeRange timerange.TimeRange) string {
	predicate, _ := search.TimePredicate("d.timestamp", timeRange.Start, timeRange.End)
	if predicate == "" {
		return ""
	}
	return "where " + predicate
}
