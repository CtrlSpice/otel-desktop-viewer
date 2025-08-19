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
