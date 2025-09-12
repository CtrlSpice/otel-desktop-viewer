package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
)

// AddMetrics appends a list of metrics to the store.
func (s *Store) AddMetrics(ctx context.Context, metricsData []metrics.MetricData) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrAddMetrics, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	appender, err := NewAppenderWrapper(s.conn, "", "", "metrics")
	if err != nil {
		return fmt.Errorf(ErrCreateAppender, err)
	}
	defer appender.Close()

	for i, metricData := range metricsData {
		err := appender.AppendRow(
			metricData.ID(),
			metricData.Name,
			metricData.Description,
			metricData.Unit,
			metricData.DataPoints,
			metricData.Resource.Attributes,
			metricData.Resource.DroppedAttributesCount,
			metricData.Scope.Name,
			metricData.Scope.Version,
			metricData.Scope.Attributes,
			metricData.Scope.DroppedAttributesCount,
			metricData.Received,
		)
		if err != nil {
			return fmt.Errorf(ErrAppendRow, err)
		}

		// Flush every 10 metrics to prevent buffer overflow
		if (i+1)%10 == 0 {
			err = appender.Flush()
			if err != nil {
				return fmt.Errorf(ErrFlushAppender, err)
			}
		}
	}

	return nil

}

func (s *Store) GetMetrics(ctx context.Context) ([]metrics.MetricData, error) {
	if err := s.checkConnection(); err != nil {
		return nil, fmt.Errorf(ErrGetMetrics, err)
	}

	metrics := []metrics.MetricData{}

	rows, err := s.db.QueryContext(ctx, SelectMetrics)
	if err != nil {
		return nil, fmt.Errorf(ErrGetMetrics, err)
	}
	defer rows.Close()

	for rows.Next() {
		metricData, err := scanMetricRow(rows)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metricData)
	}

	return metrics, nil
}

// ClearMetrics truncates the metrics table.
func (s *Store) ClearMetrics(ctx context.Context) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrClearMetrics, err)
	}

	if _, err := s.db.ExecContext(ctx, TruncateMetrics); err != nil {
		return fmt.Errorf(ErrClearMetrics, err)
	}
	return nil
}

// DeleteMetricByID deletes a specific metric by its ID.
func (s *Store) DeleteMetricByID(ctx context.Context, metricID string) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteMetricByID, err)
	}

	_, err := s.db.ExecContext(ctx, DeleteMetricByID, metricID)
	if err != nil {
		return fmt.Errorf(ErrDeleteMetricByID, err)
	}

	return nil
}

// DeleteMetricsByIDs deletes multiple metrics by their IDs.
func (s *Store) DeleteMetricsByIDs(ctx context.Context, metricIDs []any) error {
	if err := s.checkConnection(); err != nil {
		return fmt.Errorf(ErrDeleteMetricByID, err)
	}

	if len(metricIDs) == 0 {
		return nil // Nothing to delete
	}

	placeholders := buildPlaceholders(len(metricIDs))
	query := fmt.Sprintf(DeleteMetricsByIDs, placeholders)

	_, err := s.db.ExecContext(ctx, query, metricIDs...)
	if err != nil {
		return fmt.Errorf(ErrDeleteMetricByID, err)
	}

	return nil
}

func scanMetricRow(scanner interface{ Scan(dest ...any) error }) (metrics.MetricData, error) {
	metricData := metrics.MetricData{
		Resource: &resource.ResourceData{
			Attributes:             map[string]any{},
			DroppedAttributesCount: 0,
		},
		Scope: &scope.ScopeData{
			Name:                   "",
			Version:                "",
			Attributes:             map[string]any{},
			DroppedAttributesCount: 0,
		},
	}

	if err := scanner.Scan(
		&metricData.Name,
		&metricData.Description,
		&metricData.Unit,
		&metricData.DataPoints,
		&metricData.Resource.Attributes,
		&metricData.Resource.DroppedAttributesCount,
		&metricData.Scope.Name,
		&metricData.Scope.Version,
		&metricData.Scope.Attributes,
		&metricData.Scope.DroppedAttributesCount,
		&metricData.Received,
	); err != nil {
		if err == sql.ErrNoRows {
			return metricData, ErrMetricIDNotFound
		}
		return metricData, fmt.Errorf(ErrScanMetricRow, err)
	}

	return metricData, nil
}
