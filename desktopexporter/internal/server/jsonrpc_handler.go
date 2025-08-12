package server

import (
	"context"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"golang.org/x/exp/jsonrpc2"
)

type JSONRPCHandler struct {
	store *store.Store
}

func NewJSONRPCHandler(dbPath string) *JSONRPCHandler {
	store := store.NewStore(context.Background(), dbPath)
	return &JSONRPCHandler{store: store}
}

func (handler *JSONRPCHandler) Handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	switch req.Method {
	case "getTraceSummaries":
		return handler.getTraceSummaries(ctx, req)
	case "getTraceByID":
		return handler.getTraceByID(ctx, req)
	case "clearTraces":
		return handler.clearTraces(ctx, req)
	case "getLogs":
		return handler.getLogs(ctx, req)
	case "getLogsByTraceID":
		return handler.getLogsByTraceID(ctx, req)
	case "clearLogs":
		return handler.clearLogs(ctx, req)
	case "getMetrics":
		return handler.getMetrics(ctx, req)
	case "clearMetrics":
		return handler.clearMetrics(ctx, req)
	case "getSampleData":
		return handler.getSampleData(ctx, req)
	default:
		return nil, jsonrpc2.ErrMethodNotFound
	}
}

func (handler *JSONRPCHandler) getTraceSummaries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
}
