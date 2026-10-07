package metrics

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/otlp"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

// GetMetricOTLPProtobuf returns the same retained data as GetMetricOTLP in a
// binary OTLP ExportMetricsServiceRequest, preserving negative-zero doubles.
func GetMetricOTLPProtobuf(ctx context.Context, db *sql.DB, metricID string) ([]byte, error) {
	raw, err := GetMetricOTLP(ctx, db, metricID)
	if err != nil {
		return nil, err
	}
	encoded, err := otlp.MarshalProto(raw, &collectormetrics.ExportMetricsServiceRequest{})
	if err != nil {
		return nil, fmt.Errorf("GetMetricOTLPProtobuf: %w: %w", ErrMetricsStoreInternal, err)
	}
	return encoded, nil
}
