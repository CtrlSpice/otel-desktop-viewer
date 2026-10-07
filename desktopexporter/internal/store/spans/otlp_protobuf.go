package spans

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/otlp"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// GetTraceOTLPProtobuf returns the same retained data as GetTraceOTLP in a
// binary OTLP ExportTraceServiceRequest, preserving negative-zero doubles.
func GetTraceOTLPProtobuf(ctx context.Context, db *sql.DB, traceID string) ([]byte, error) {
	raw, err := GetTraceOTLP(ctx, db, traceID)
	if err != nil {
		return nil, err
	}
	encoded, err := otlp.MarshalProto(raw, &collectortrace.ExportTraceServiceRequest{})
	if err != nil {
		return nil, fmt.Errorf("GetTraceOTLPProtobuf: %w: %w", ErrSpansStoreInternal, err)
	}
	return encoded, nil
}
