package logs

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/otlp"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
)

// GetLogOTLPProtobuf returns the same retained data as GetLogOTLP in a
// binary OTLP ExportLogsServiceRequest, preserving negative-zero doubles.
func GetLogOTLPProtobuf(ctx context.Context, db *sql.DB, logRef string) ([]byte, error) {
	raw, err := GetLogOTLP(ctx, db, logRef)
	if err != nil {
		return nil, err
	}
	encoded, err := otlp.MarshalProto(raw, &collectorlogs.ExportLogsServiceRequest{})
	if err != nil {
		return nil, fmt.Errorf("GetLogOTLPProtobuf: %w: %w", ErrLogsStoreInternal, err)
	}
	return encoded, nil
}
