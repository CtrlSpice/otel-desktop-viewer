package spans

import (
	"context"
	"database/sql"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/queries"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
)

func SearchArrow(ctx context.Context, db *sql.DB, timeRange timerange.TimeRange, criteria any, options search.ResultOptions) (*arrowread.Result, error) {
	query, args, err := traceSummariesSQL(timeRange, criteria, options, true)
	if err != nil {
		return nil, err
	}
	return arrowread.Read(ctx, db, query, args...)
}

func ViewArrow(ctx context.Context, db *sql.DB, traceID string, criteria any, spanIDs ...uint64) (*arrowread.Result, error) {
	return viewArrow(ctx, db, traceID, criteria, false, nil, spanIDs...)
}

// WaterfallArrow returns complete tree geometry without received nested payloads.
func WaterfallArrow(ctx context.Context, db *sql.DB, traceID string, criteria any, spanIDs ...uint64) (*arrowread.Result, error) {
	return viewArrow(ctx, db, traceID, criteria, true, nil, spanIDs...)
}

func WaterfallColumnsArrow(ctx context.Context, db *sql.DB, traceID string, criteria any, columns []WaterfallColumn) (*arrowread.Result, error) {
	return viewArrow(ctx, db, traceID, criteria, true, columns)
}

func viewArrow(ctx context.Context, db *sql.DB, traceID string, criteria any, lightweight bool, columns []WaterfallColumn, spanIDs ...uint64) (*arrowread.Result, error) {
	query, args, err := traceQueryColumns(queries.GetTraceView, traceID, criteria, true, lightweight, columns, spanIDs...)
	if err != nil {
		return nil, err
	}
	result, err := arrowread.Read(ctx, db, query, args...)
	if err != nil {
		return nil, err
	}
	if result.NumRows() == 0 {
		result.Close()
		return nil, ErrTraceIDNotFound
	}
	unplaced, err := result.SingleInt64("unplacedSpanCount")
	if err != nil {
		result.Close()
		return nil, err
	}
	if unplaced == 0 {
		return result, nil
	}
	query, args, err = traceQueryColumns(queries.SalvageSpans, traceID, criteria, true, lightweight, columns, spanIDs...)
	if err != nil {
		result.Close()
		return nil, err
	}
	salvaged, err := arrowread.Read(ctx, db, query, args...)
	if err != nil {
		if ctx.Err() != nil {
			result.Close()
			return nil, ctx.Err()
		}
		return result, nil
	}
	result.Close()
	return salvaged, nil
}
