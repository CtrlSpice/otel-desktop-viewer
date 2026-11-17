package logs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
)

var (
	ErrInvalidLogQuery   = errors.New("invalid log search query")
	ErrInvalidLogLimit   = errors.New("invalid log search limit")
	ErrLogsStoreInternal = errors.New("logs store internal error")
	ErrLogRefNotFound    = errors.New("log ID not found")
)

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Get returns the full LogData for a single log identified by its
// tool-minted UUID. Used by the log-detail pane after a user clicks
// a card from Search results. Returns ErrLogRefNotFound when no log
// matches.
func Get(ctx context.Context, db *sql.DB, logRef string) (json.RawMessage, error) {
	// attrs_json resolves each of the row's three attribute arrays in place.
	query, err := queries.Render(queries.GetLog, nil)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, logRef).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("Get: %w", ErrLogRefNotFound)
		}
		return nil, fmt.Errorf("Get: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("Get: %w", ErrLogRefNotFound)
	}
	return json.RawMessage(raw), nil
}

// GetLogOTLP returns the stored log identified by logRef as one standard OTLP
// JSON document. Callers must transport the returned bytes unchanged: parsing
// and re-encoding in JavaScript loses negative zero.
func GetLogOTLP(ctx context.Context, db *sql.DB, logRef string) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetLogOTLP, nil)
	if err != nil {
		return nil, fmt.Errorf("GetLogOTLP: %w: %w", ErrLogsStoreInternal, err)
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, logRef).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetLogOTLP: %w", ErrLogRefNotFound)
		}
		return nil, fmt.Errorf("GetLogOTLP: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("GetLogOTLP: %w", ErrLogRefNotFound)
	}
	return json.RawMessage(raw), nil
}

// GetTraceLogSummaries returns every lightweight log summary carrying traceID. The
// trace boundary is the complete correlation boundary: timestamps only order
// rows, and nullable or dangling span IDs remain in the result for callers to
// classify. Full body and attribute detail remains owned by Get.
func GetTraceLogSummaries(ctx context.Context, db queryRower, traceID string) (json.RawMessage, error) {
	query, args, err := getTraceLogSummariesSQL(traceID)
	if err != nil {
		return nil, err
	}

	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetTraceLogSummaries: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

// GetSpanLogs returns every full log row associated with one exact composite
// span identity. The result is complete and uses one bulk query.
func GetSpanLogs(ctx context.Context, db queryRower, traceID string, spanID uint64) (json.RawMessage, error) {
	query, err := queries.Render(queries.GetSpanLogs, nil)
	if err != nil {
		return nil, fmt.Errorf("GetSpanLogs: %w: %w", ErrLogsStoreInternal, err)
	}
	var raw []byte
	if err := db.QueryRowContext(ctx, query, traceID, []uint64{spanID}).Scan(&raw); err != nil {
		return nil, fmt.Errorf("GetSpanLogs: %w: %w", ErrLogsStoreInternal, err)
	}
	if raw == nil {
		return json.RawMessage("[]"), nil
	}
	return json.RawMessage(raw), nil
}

func getTraceLogSummariesSQL(traceID string) (string, []any, error) {
	query, err := queries.Render(queries.GetTraceLogSummaries, nil)
	if err != nil {
		return "", nil, err
	}
	return query, []any{traceID}, nil
}

// Clear deletes logs and their search documents.
func Clear(ctx context.Context, db *sql.DB) error {
	if err := ingest.DeleteLogs(ctx, db, `truncate table logs`); err != nil {
		return fmt.Errorf("Clear: %w: %w", ErrLogsStoreInternal, err)
	}
	return nil
}

// DeleteLogsByRefs deletes multiple logs by their IDs.
func DeleteLogsByRefs(ctx context.Context, db *sql.DB, logRefs []any) error {
	if len(logRefs) == 0 {
		return nil
	}
	ids := util.ToStringList(logRefs)
	if err := ingest.DeleteLogs(ctx, db, `delete from logs where id in (select id from uuid_list(?))`, ids); err != nil {
		return fmt.Errorf("DeleteLogsByRefs: %w: %w", ErrLogsStoreInternal, err)
	}
	return nil
}
