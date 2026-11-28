package rpc

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"golang.org/x/exp/jsonrpc2"
)

func (h *Handler) searchLogSummaries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseSearchParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.SearchSummariesWithOptions(ctx, db, params.timeRange, params.query, params.options)
	})
}

func (h *Handler) searchLogPage(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseLogPageParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.SearchPage(ctx, db, params.timeRange, params.query, params.options, params.page)
	})
}

type logPageParams struct {
	searchParams
	page logs.PageOptions
}

func parseLogPageParams(raw json.RawMessage) (*logPageParams, error) {
	values, err := decodePositionalParams(raw, 2, 8)
	if err != nil {
		return nil, err
	}
	params, err := searchParamsFromValues(values)
	if err != nil {
		return nil, err
	}
	size := int64(100)
	if len(values) > 5 && values[5] != nil {
		size, err = parseTimestampParam(values[5], "pageSize")
		if err != nil {
			return nil, err
		}
	}
	offset, err := parseOptionalNonNegativeInt(values, 6, "offset")
	if err != nil {
		return nil, err
	}
	if size < 1 || size > 250 {
		return nil, jsonrpc2.ErrInvalidParams
	}
	var columns []logs.Column
	if len(values) > 7 && values[7] != nil {
		list, ok := values[7].([]any)
		if !ok {
			return nil, jsonrpc2.ErrInvalidParams
		}
		for _, value := range list {
			column, ok := value.(map[string]any)
			if !ok || len(column) != 3 {
				return nil, jsonrpc2.ErrInvalidParams
			}
			source, sourceOK := column["source"].(string)
			kind, kindOK := column["kind"].(string)
			key, keyOK := column["key"].(string)
			if !sourceOK || !kindOK || !keyOK {
				return nil, jsonrpc2.ErrInvalidParams
			}
			columns = append(columns, logs.Column{Source: source, Kind: kind, Key: key})
		}
		if logs.ValidateColumns(columns) != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
	}
	return &logPageParams{searchParams: params, page: logs.PageOptions{Size: size, Offset: offset, Columns: columns}}, nil
}

func (h *Handler) getLogErrorNavigation(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	values, err := decodePositionalParams(req.Params, 2, 6)
	if err != nil {
		return nil, err
	}
	params, err := searchParamsFromValues(values)
	if err != nil {
		return nil, err
	}
	selectedID := ""
	if len(values) > 5 && values[5] != nil {
		value, ok := values[5].(string)
		if !ok {
			return nil, jsonrpc2.ErrInvalidParams
		}
		selectedID, err = normalizeUUID(value)
		if err != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetErrorNavigation(ctx, db, params.timeRange, params.query, params.options, selectedID)
	})
}

func (h *Handler) getTraceLogSummaries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceID, err := parseSingleIDParam(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogSummaries(ctx, db, traceID)
	})
}

func (h *Handler) clearLogs(ctx context.Context) (any, error) {
	return h.clearSignal(ctx, logs.Clear, "Logs cleared successfully")
}

// getLog returns the full LogData for a single log row identified by
// its tool-minted UUID (the same id returned in Search summaries).
func (h *Handler) getLog(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	logRef, err := parseSingleIDParam(req.Params, ErrInvalidLogRef, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.Get(ctx, db, logRef)
	})
}

// deleteLogsByRefs deletes one or more specific logs by their IDs.
func (h *Handler) deleteLogsByRefs(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	logRefs, err := parseIDParams(req.Params, ErrInvalidLogRef, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := h.store.WithDBWrite(func(db *sql.DB) error {
		return logs.DeleteLogsByRefs(ctx, db, logRefs)
	}); err != nil {
		return nil, h.handleStoreError(ctx, err)
	}

	return map[string]any{
		"message": "Logs deleted successfully",
		"count":   len(logRefs),
	}, nil
}

func (h *Handler) getLogAttributeDefinitions(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := validateNoParams(req.Params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetLogAttributeDefinitions(ctx, db)
	})
}
