package server

import (
	"context"
	"encoding/json"
	"log"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
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
	case "getTraceSummaries":
		return h.getTraceSummaries(ctx)
	case "getTraceByID":
		return h.getTraceByID(ctx, req)
	case "getLogs":
		return h.getLogs(ctx)
	case "getLogByID":
		return h.getLogByID(ctx, req)
	case "getLogsByTraceID":
		return h.getLogsByTraceID(ctx, req)
	case "getMetrics":
		return h.getMetrics(ctx)
	case "loadSampleData":
		return h.loadSampleData(ctx)
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
	case "checkSampleDataExists":
		return h.checkSampleDataExists(ctx)
	case "clearSampleData":
		return h.clearSampleData(ctx)
	default:
		return nil, jsonrpc2.ErrMethodNotFound
	}
}

func (h *JSONRPCHandler) getTraceSummaries(ctx context.Context) (any, error) {
	summaries, err := h.store.GetTraceSummaries(ctx)
	if err != nil {
		log.Printf("Error getting trace summaries: %v", err)
		return nil, jsonrpc2.ErrInternal
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
	trace, err := h.store.GetTrace(ctx, traceID)
	if err != nil {
		log.Printf("Error getting trace by ID: %v", err)
		return nil, ErrTraceNotFound
	}

	return trace, nil
}

func (h *JSONRPCHandler) clearTraces(ctx context.Context) (any, error) {
	err := h.store.ClearTraces(ctx)
	if err != nil {
		log.Printf("Error clearing traces: %v", err)
		return nil, jsonrpc2.ErrInternal
	}
	return "Traces cleared successfully", nil
}

func (h *JSONRPCHandler) getLogs(ctx context.Context) (any, error) {
	logs, err := h.store.GetLogs(ctx)
	if err != nil {
		log.Printf("Error getting logs: %v", err)
		return nil, jsonrpc2.ErrInternal
	}
	return logs, nil
}

func (h *JSONRPCHandler) getLogByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	logID := params[0]
	logData, err := h.store.GetLog(ctx, logID)
	if err != nil {
		log.Printf("Error getting log by ID: %v", err)
		return nil, ErrLogsNotFound
	}
	return logData, nil
}

func (h *JSONRPCHandler) getLogsByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []string
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	traceID := params[0]
	logs, err := h.store.GetLogsByTrace(ctx, traceID)
	if err != nil {
		log.Printf("Error getting logs by trace ID: %v", err)
		return nil, ErrLogsNotFound
	}
	return logs, nil
}

func (h *JSONRPCHandler) clearLogs(ctx context.Context) (any, error) {
	err := h.store.ClearLogs(ctx)
	if err != nil {
		log.Printf("Error clearing logs: %v", err)
		return nil, jsonrpc2.ErrInternal
	}
	return "Logs cleared successfully", nil
}

func (h *JSONRPCHandler) getMetrics(ctx context.Context) (any, error) {
	metrics, err := h.store.GetMetrics(ctx)
	if err != nil {
		log.Printf("Error getting metrics: %v", err)
		return nil, jsonrpc2.ErrInternal
	}
	return metrics, nil
}

func (h *JSONRPCHandler) clearMetrics(ctx context.Context) (any, error) {
	err := h.store.ClearMetrics(ctx)
	if err != nil {
		log.Printf("Error clearing metrics: %v", err)
		return nil, jsonrpc2.ErrInternal
	}
	return "Metrics cleared successfully", nil
}

func (h *JSONRPCHandler) loadSampleData(ctx context.Context) (any, error) {
	sample := telemetry.NewSampleTelemetry()

	if err := h.store.AddSpans(ctx, sample.Spans); err != nil {
		log.Printf("Error adding sample spans: %v", err)
		return nil, jsonrpc2.ErrInternal
	}

	if err := h.store.AddLogs(ctx, sample.Logs); err != nil {
		log.Printf("Error adding sample logs: %v", err)
		return nil, jsonrpc2.ErrInternal
	}

	if err := h.store.AddMetrics(ctx, sample.Metrics); err != nil {
		log.Printf("Error adding sample metrics: %v", err)
		return nil, jsonrpc2.ErrInternal
	}

	return "Sample data loaded successfully", nil
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

	err := h.store.DeleteSpansByTraceIDs(ctx, params)
	if err != nil {
		log.Printf("Error deleting spans by trace IDs: %v", err)
		return nil, jsonrpc2.ErrInternal
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

	err := h.store.DeleteSpansByIDs(ctx, params)
	if err != nil {
		log.Printf("Error deleting spans by IDs: %v", err)
		return nil, jsonrpc2.ErrInternal
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

	err := h.store.DeleteLogsByIDs(ctx, params)
	if err != nil {
		log.Printf("Error deleting logs by IDs: %v", err)
		return nil, jsonrpc2.ErrInternal
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

	err := h.store.DeleteMetricsByIDs(ctx, params)
	if err != nil {
		log.Printf("Error deleting metrics by IDs: %v", err)
		return nil, jsonrpc2.ErrInternal
	}

	return map[string]any{
		"message": "Metrics deleted successfully",
		"count":   len(params),
	}, nil
}

func (h *JSONRPCHandler) checkSampleDataExists(ctx context.Context) (any, error) {
	exists, err := h.store.SampleDataExists(ctx)
	if err != nil {
		log.Printf("Error checking if sample data exists: %v", err)
		return nil, jsonrpc2.ErrInternal
	}

	return map[string]any{
		"exists": exists,
	}, nil
}

func (h *JSONRPCHandler) clearSampleData(ctx context.Context) (any, error) {
	err := h.store.ClearSampleData(ctx)
	if err != nil {
		log.Printf("Error clearing sample data: %v", err)
		return nil, jsonrpc2.ErrInternal
	}

	return "Sample data cleared successfully", nil
}
