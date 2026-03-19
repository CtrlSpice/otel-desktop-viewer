package server

import (
	"context"
	"encoding/json"
	"log"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/stats"
	"golang.org/x/exp/jsonrpc2"
)

type JSONRPCHandler struct {
	store *store.Store
}

func NewJSONRPCHandler(store *store.Store) *JSONRPCHandler {
	return &JSONRPCHandler{store: store}
}

func (h *JSONRPCHandler) Handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	switch req.Method {
	case "searchTraces":
		return h.searchTraces(ctx, req)
	case "getTraceByID":
		return h.getTraceByID(ctx, req)
	case "searchLogs":
		return h.searchLogs(ctx, req)
	case "searchMetrics", "getMetrics":
		return h.searchMetrics(ctx, req)
	case "clearTraces":
		return h.clearTraces(ctx)
	case "clearLogs":
		return h.clearLogs(ctx)
	case "clearMetrics":
		return h.clearMetrics(ctx)
	case "deleteSpansByTraceID":
		return h.deleteSpansByTraceID(ctx, req)
	case "deleteSpanByID":
		return h.deleteSpanByID(ctx, req)
	case "deleteLogByID":
		return h.deleteLogByID(ctx, req)
	case "deleteMetricByID":
		return h.deleteMetricByID(ctx, req)
	case "getTraceAttributes":
		return h.getTraceAttributes(ctx, req)
	case "getStats":
		return h.getStats(ctx)
	default:
		return nil, jsonrpc2.ErrMethodNotFound
	}
}

func (h *JSONRPCHandler) searchTraces(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Failed to unmarshal params: %v", err)
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) < 2 || len(params) > 3 {
		log.Printf("Invalid parameter count: %d (expected 2-3)", len(params))
		return nil, jsonrpc2.ErrInvalidParams
	}

	startTime, err := parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}

	endTime, err := parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}

	var query any
	if len(params) == 3 {
		query = params[2]
	}

	log.Printf("searchTraces query parameter: %+v", query)
	summaries, err := spans.SearchTraces(ctx, h.store.DB(), startTime, endTime, query)
	if err != nil {
		log.Printf("Error searching traces: %v", err)
		return nil, mapStoreError(err)
	}
	return summaries, nil
}

func (h *JSONRPCHandler) getTraceByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	traceID := params[0]
	trace, err := spans.GetTrace(ctx, h.store.DB(), traceID)
	if err != nil {
		log.Printf("Error getting trace by ID: %v", err)
		return nil, mapStoreError(err)
	}

	return trace, nil
}

func (h *JSONRPCHandler) clearTraces(ctx context.Context) (any, error) {
	err := spans.Clear(ctx, h.store.DB())
	if err != nil {
		log.Printf("Error clearing traces: %v", err)
		return nil, mapStoreError(err)
	}
	return "Traces cleared successfully", nil
}

func (h *JSONRPCHandler) searchLogs(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Failed to unmarshal params: %v", err)
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) < 2 || len(params) > 3 {
		log.Printf("Invalid parameter count: %d (expected 2-3)", len(params))
		return nil, jsonrpc2.ErrInvalidParams
	}
	startTime, err := parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}
	endTime, err := parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}
	var query any
	if len(params) == 3 {
		query = params[2]
	}
	result, err := logs.Search(ctx, h.store.DB(), startTime, endTime, query)
	if err != nil {
		log.Printf("Error searching logs: %v", err)
		return nil, mapStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) clearLogs(ctx context.Context) (any, error) {
	err := logs.Clear(ctx, h.store.DB())
	if err != nil {
		log.Printf("Error clearing logs: %v", err)
		return nil, mapStoreError(err)
	}
	return "Logs cleared successfully", nil
}

func (h *JSONRPCHandler) searchMetrics(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Failed to unmarshal params: %v", err)
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) < 2 || len(params) > 3 {
		log.Printf("Invalid parameter count: %d (expected 2-3)", len(params))
		return nil, jsonrpc2.ErrInvalidParams
	}
	startTime, err := parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}
	endTime, err := parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}
	var query any
	if len(params) == 3 {
		query = params[2]
	}
	result, err := metrics.Search(ctx, h.store.DB(), startTime, endTime, query)
	if err != nil {
		log.Printf("Error searching metrics: %v", err)
		return nil, mapStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) clearMetrics(ctx context.Context) (any, error) {
	err := metrics.Clear(ctx, h.store.DB())
	if err != nil {
		log.Printf("Error clearing metrics: %v", err)
		return nil, mapStoreError(err)
	}
	return "Metrics cleared successfully", nil
}

// deleteSpansByTraceID deletes all spans for one or more traces.
func (h *JSONRPCHandler) deleteSpansByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) == 0 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	err := spans.DeleteSpansByTraceIDs(ctx, h.store.DB(), params)
	if err != nil {
		log.Printf("Error deleting spans by trace IDs: %v", err)
		return nil, mapStoreError(err)
	}

	return map[string]any{
		"message": "Spans deleted successfully",
		"count":   len(params),
	}, nil
}

// deleteSpanByID deletes one or more specific spans by their IDs.
func (h *JSONRPCHandler) deleteSpanByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) == 0 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	err := spans.DeleteSpansByIDs(ctx, h.store.DB(), params)
	if err != nil {
		log.Printf("Error deleting spans by IDs: %v", err)
		return nil, mapStoreError(err)
	}

	return map[string]any{
		"message": "Spans deleted successfully",
		"count":   len(params),
	}, nil
}

// deleteLogByID deletes one or more specific logs by their IDs.
func (h *JSONRPCHandler) deleteLogByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) == 0 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	err := logs.DeleteLogsByIDs(ctx, h.store.DB(), params)
	if err != nil {
		log.Printf("Error deleting logs by IDs: %v", err)
		return nil, mapStoreError(err)
	}

	return map[string]any{
		"message": "Logs deleted successfully",
		"count":   len(params),
	}, nil
}

// deleteMetricByID deletes one or more specific metrics by their IDs.
func (h *JSONRPCHandler) deleteMetricByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) == 0 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	err := metrics.DeleteMetricsByIDs(ctx, h.store.DB(), params)
	if err != nil {
		log.Printf("Error deleting metrics by IDs: %v", err)
		return nil, mapStoreError(err)
	}

	return map[string]any{
		"message": "Metrics deleted successfully",
		"count":   len(params),
	}, nil
}

func (h *JSONRPCHandler) getTraceAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		log.Printf("Failed to unmarshal params: %v", err)
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 2 {
		log.Printf("Invalid parameter count: %d (expected 2)", len(params))
		return nil, jsonrpc2.ErrInvalidParams
	}

	startTime, err := parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}

	endTime, err := parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}

	attributes, err := spans.GetTraceAttributes(ctx, h.store.DB(), startTime, endTime)
	if err != nil {
		log.Printf("Error getting trace attributes: %v", err)
		return nil, mapStoreError(err)
	}

	return attributes, nil
}

func (h *JSONRPCHandler) getStats(ctx context.Context) (any, error) {
	result, err := stats.GetStats(ctx, h.store.DB())
	if err != nil {
		log.Printf("Error getting stats: %v", err)
		return nil, mapStoreError(err)
	}
	return result, nil
}

// parseTimestampParam parses a timestamp parameter that must be a JSON string
// containing a base-10 int64. Large integers travel as strings to avoid
// float64 precision loss in JSON.
func parseTimestampParam(param any, paramName string) (int64, error) {
	s, ok := param.(string)
	if !ok {
		log.Printf("Invalid %s type: %T, value: %v (expected string)", paramName, param, param)
		return 0, jsonrpc2.ErrInvalidParams
	}
	parsed, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Printf("Invalid %s string: %v", paramName, err)
		return 0, jsonrpc2.ErrInvalidParams
	}
	return parsed, nil
}
