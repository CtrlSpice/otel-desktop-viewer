package spans

import (
	"context"
	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/duckdb/duckdb-go/v2"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// appendSpansBisecting writes every span it can and isolates the ones it
// cannot, one span being the unit of blame.
//
// All the policy lives in ingest.BisectingWrite; this only supplies the two
// things it asks for -- how many spans there are, and how to attempt a
// contiguous range of them atomically.
func appendSpansBisecting(
	ctx context.Context,
	conn driver.Conn,
	traces ptrace.Traces,
	resourceIDs map[int]duckdb.UUID,
	scopeIDs map[scopeKey]duckdb.UUID,
	spanAttrs, eventAttrs, linkAttrs [][]duckdb.UUID,
) (ingest.Rejected, error) {
	return ingest.BisectingWrite(ctx, len(spanAttrs), func(lo, hi int) error {
		return ingest.InTransaction(ctx, conn, func() error {
			return appendPass(ctx, conn, traces, resourceIDs, scopeIDs,
				spanAttrs, eventAttrs, linkAttrs,
				func(ordinal int) bool { return ordinal >= lo && ordinal < hi })
		})
	})
}
