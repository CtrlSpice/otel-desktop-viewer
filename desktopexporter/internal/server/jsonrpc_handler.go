package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/stats"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

type JSONRPCHandler struct {
	store        *store.Store
	databases    DatabaseManager
	logger       *zap.Logger
	otlpHTTPPort int
	otlpGRPCPort int
	homeValues   homeValueReferences
}

func NewJSONRPCHandler(store *store.Store, logger *zap.Logger) *JSONRPCHandler {
	return &JSONRPCHandler{store: store, logger: logger}
}

// handleStoreError maps store errors to JSON-RPC codes and logs only unexpected
// failures (those that become -32603). Expected outcomes like not-found,
// invalid query, or a caller that went away are returned without logging.
func (h *JSONRPCHandler) handleStoreError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	mapped := mapStoreError(ingest.InterruptedContextError(ctx, err))
	if mapped == jsonrpc2.ErrInternal {
		h.logger.Error("store error", zap.Error(err))
	}
	return mapped
}

// storeRead runs a query that returns a value under the store's read lock.
// The lock excludes pool mutation and close; ingest may run concurrently.
func storeRead[T any](s *store.Store, fn func(db *sql.DB) (T, error)) (T, error) {
	var out T
	err := s.WithDBRead(func(db *sql.DB) error {
		var err error
		out, err = fn(db)
		return err
	})
	return out, err
}

func storeSnapshotRead[T any](ctx context.Context, s *store.Store, fn func(tx *sql.Tx) (T, error)) (T, error) {
	return storeRead(s, func(db *sql.DB) (T, error) {
		var zero T
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return zero, err
		}
		defer tx.Rollback()
		result, err := fn(tx)
		if err != nil {
			return zero, err
		}
		if err := tx.Commit(); err != nil {
			return zero, err
		}
		return result, nil
	})
}

func handlerRead[T any](ctx context.Context, h *JSONRPCHandler, fn func(db *sql.DB) (T, error)) (any, error) {
	result, err := storeRead(h.store, fn)
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	return result, nil
}

func handlerSnapshotRead[T any](ctx context.Context, h *JSONRPCHandler, fn func(tx *sql.Tx) (T, error)) (any, error) {
	result, err := storeSnapshotRead(ctx, h.store, fn)
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	return result, nil
}

func (h *JSONRPCHandler) Handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	// Object params are rewritten into the positional array every handler
	// below already reads, so named and positional calls share one code path
	// and cannot drift apart. Array params pass through untouched.
	normalized, err := normalizeParams(req.Method, req.Params)
	if err != nil {
		return nil, err
	}
	req.Params = normalized

	switch req.Method {
	case "getServiceCount":
		return h.getServiceCount(ctx, req)
	case "getServiceLabels":
		return h.getServiceLabels(ctx, req)
	case "rpc.discover":
		if err := validateNoParams(req.Params); err != nil {
			return nil, err
		}
		return openRPCDocument(), nil
	case "getImportConfig":
		if h.otlpHTTPPort <= 0 {
			return nil, fmt.Errorf("%w: OTLP HTTP import is not configured", jsonrpc2.ErrInternal)
		}
		return struct {
			OTLPHTTPPort int `json:"otlpHttpPort"`
			OTLPGRPCPort int `json:"otlpGrpcPort,omitempty"`
		}{OTLPHTTPPort: h.otlpHTTPPort, OTLPGRPCPort: h.otlpGRPCPort}, nil
	case "searchTraceSummaries":
		return h.searchTraceSummaries(ctx, req)
	case "getTraceView":
		return h.getTraceView(ctx, req)
	case "getTraceOverview":
		return h.getTraceOverview(ctx, req)
	case "getSpan":
		return h.getSpan(ctx, req)
	case "searchLogSummaries":
		return h.searchLogSummaries(ctx, req)
	case "searchLogPage":
		return h.searchLogPage(ctx, req)
	case "getLogErrorNavigation":
		return h.getLogErrorNavigation(ctx, req)
	case "getTraceLogSummaries":
		return h.getTraceLogSummaries(ctx, req)
	case "getLog":
		return h.getLog(ctx, req)
	case "searchMetricSummaries":
		return h.searchMetricSummaries(ctx, req)
	case "getMetric":
		return h.getMetric(ctx, req)
	case "getMetricSeries":
		return h.getMetricSeries(ctx, req)
	case "getMetricSeriesOrder":
		args, err := parseMetricSeriesOrderParams(req.Params)
		if err != nil {
			return nil, err
		}
		return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricSeriesOrder(ctx, db, args.metricRef, args.timeRange, args.seriesRefs, args.key, args.descending)
		})
	case "getMetricView":
		return h.getMetricView(ctx, req)
	case "getMetricAggregateView":
		return h.getMetricAggregateView(ctx, req)
	case "clearTraces":
		return h.clearTraces(ctx)
	case "clearLogs":
		return h.clearLogs(ctx)
	case "clearMetrics":
		return h.clearMetrics(ctx)
	case "deleteMetric":
		return h.deleteMetric(ctx, req)
	case "deleteSpansByTraceID":
		return h.deleteSpansByTraceID(ctx, req)
	case "deleteLogsByRefs":
		return h.deleteLogsByRefs(ctx, req)
	case "getTraceAttributeDefinitions":
		return h.getTraceAttributeDefinitions(ctx, req)
	case "getLogAttributeDefinitions":
		return h.getLogAttributeDefinitions(ctx, req)
	case "getMetricAttributeDefinitions":
		return h.getMetricAttributeDefinitions(ctx, req)
	case "getFieldValueCompletions":
		return h.getFieldValueCompletions(ctx, req)
	case "searchAttributeMatches":
		return h.searchAttributeMatches(ctx, req)
	case "getTraceAttributeDefinitionsByTraceID":
		return h.getTraceAttributeDefinitionsByTraceID(ctx, req)
	case "getStats":
		return h.getStats(ctx)
	case "discoverTelemetry":
		return h.discoverTelemetry(ctx, req)
	case "getTraceSpanCount":
		return h.getTraceSpanCount(ctx, req)
	case "query":
		return h.query(ctx, req)
	case "listDatabases":
		return h.listDatabases(ctx, req)
	case "openDatabase":
		return h.openDatabase(ctx, req)
	case "newDatabase":
		return h.newDatabase(ctx, req)
	default:
		return nil, jsonrpc2.ErrMethodNotFound
	}
}

func (h *JSONRPCHandler) clearSignal(
	ctx context.Context,
	clear func(context.Context, *sql.DB) error,
	successMessage string,
) (any, error) {
	// Clear removes owning rows and their search catalogue atomically.
	err := h.store.WithDBWrite(func(db *sql.DB) error {
		return clear(ctx, db)
	})
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	return successMessage, nil
}

func (h *JSONRPCHandler) getStats(ctx context.Context) (any, error) {
	retentionCap := h.store.RetentionCap()

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		// SizeBytesWithDB, not SizeBytes: we already hold the read lock.
		sizeBytes, err := h.store.SizeBytesWithDB(ctx, db)
		if err != nil {
			return nil, err
		}
		return stats.GetStats(ctx, db, sizeBytes, retentionCap)
	})
}
