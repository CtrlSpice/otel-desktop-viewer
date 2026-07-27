package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/stats"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

type JSONRPCHandler struct {
	store  *store.Store
	logger *zap.Logger
}

func NewJSONRPCHandler(store *store.Store, logger *zap.Logger) *JSONRPCHandler {
	return &JSONRPCHandler{store: store, logger: logger}
}

func (h *JSONRPCHandler) Handle(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	switch req.Method {
	case "searchTraces":
		return h.searchTraces(ctx, req)
	case "searchSpans":
		return h.searchSpans(ctx, req)
	case "searchLogs":
		return h.searchLogs(ctx, req)
	case "getLog":
		return h.getLog(ctx, req)
	case "searchMetricSummaries":
		return h.searchMetricSummaries(ctx, req)
	case "getMetric":
		return h.getMetric(ctx, req)
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
	case "getTraceAttributes":
		return h.getTraceAttributes(ctx, req)
	case "getLogAttributes":
		return h.getLogAttributes(ctx, req)
	case "getMetricAttributes":
		return h.getMetricAttributes(ctx, req)
	case "getAttributesByTraceID":
		return h.getAttributesByTraceID(ctx, req)
	case "getStats":
		return h.getStats(ctx)
	case "getTraceSpanCount":
		return h.getTraceSpanCount(ctx, req)
	default:
		return nil, jsonrpc2.ErrMethodNotFound
	}
}

func (h *JSONRPCHandler) searchTraces(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) < 2 || len(params) > 3 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "2-3"))
		return nil, jsonrpc2.ErrInvalidParams
	}

	startTime, err := h.parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}

	endTime, err := h.parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}

	var query any
	if len(params) == 3 {
		query = params[2]
	}

	summaries, err := spans.SearchTraces(ctx, h.store.DB(), startTime, endTime, query)
	if err != nil {
		h.logger.Error("searching traces", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return summaries, nil
}

func (h *JSONRPCHandler) searchSpans(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) < 1 || len(params) > 2 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "1-2"))
		return nil, jsonrpc2.ErrInvalidParams
	}

	traceID, err := h.parseIDParam(params[0], ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	var query any
	if len(params) == 2 {
		query = params[1]
	}

	result, err := spans.SearchSpans(ctx, h.store.DB(), traceID, query)
	if err != nil {
		h.logger.Error("searching spans", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) clearTraces(ctx context.Context) (any, error) {
	err := spans.Clear(ctx, h.store.DB())
	if err != nil {
		h.logger.Error("clearing traces", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return "Traces cleared successfully", nil
}

func (h *JSONRPCHandler) searchLogs(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) < 2 || len(params) > 3 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "2-3"))
		return nil, jsonrpc2.ErrInvalidParams
	}
	startTime, err := h.parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}
	endTime, err := h.parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}
	var query any
	if len(params) == 3 {
		query = params[2]
	}
	result, err := logs.Search(ctx, h.store.DB(), startTime, endTime, query)
	if err != nil {
		h.logger.Error("searching logs", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) clearLogs(ctx context.Context) (any, error) {
	err := logs.Clear(ctx, h.store.DB())
	if err != nil {
		h.logger.Error("clearing logs", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return "Logs cleared successfully", nil
}

// getLog returns the full LogData for a single log row identified by
// its tool-minted UUID (the same id returned in Search summaries).
func (h *JSONRPCHandler) getLog(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) != 1 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "1"))
		return nil, jsonrpc2.ErrInvalidParams
	}
	logID, err := h.parseIDParam(params[0], ErrInvalidLogID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	result, err := logs.Get(ctx, h.store.DB(), logID)
	if err != nil {
		h.logger.Error("getting log", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) searchMetricSummaries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) < 2 || len(params) > 3 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "2-3"))
		return nil, jsonrpc2.ErrInvalidParams
	}
	startTime, err := h.parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}
	endTime, err := h.parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}
	var query any
	if len(params) == 3 {
		query = params[2]
	}
	summaries, err := metrics.SearchSummaries(ctx, h.store.DB(), startTime, endTime, query)
	if err != nil {
		h.logger.Error("searching metric summaries", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return summaries, nil
}

func (h *JSONRPCHandler) getMetric(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) != 3 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "3: streamId, startTime, endTime"))
		return nil, jsonrpc2.ErrInvalidParams
	}
	streamID, err := h.parseIDParam(params[0], ErrInvalidStreamID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	startTime, err := h.parseTimestampParam(params[1], "startTime")
	if err != nil {
		return nil, err
	}
	endTime, err := h.parseTimestampParam(params[2], "endTime")
	if err != nil {
		return nil, err
	}
	result, err := metrics.GetMetric(ctx, h.store.DB(), streamID, startTime, endTime)
	if err != nil {
		h.logger.Error("getting metric", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) clearMetrics(ctx context.Context) (any, error) {
	err := metrics.Clear(ctx, h.store.DB())
	if err != nil {
		h.logger.Error("clearing metrics", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return "Metrics cleared successfully", nil
}

// deleteSpansByTraceID deletes all spans for one or more traces.
func (h *JSONRPCHandler) deleteSpansByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceIDs, err := h.parseIDParams(req, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := spans.DeleteSpansByTraceIDs(ctx, h.store.DB(), traceIDs); err != nil {
		h.logger.Error("deleting spans by trace IDs", zap.Error(err))
		return nil, mapStoreError(err)
	}

	return map[string]any{
		"message": "Spans deleted successfully",
		"count":   len(traceIDs),
	}, nil
}

// deleteSpanByID deletes one or more specific spans by their IDs.
func (h *JSONRPCHandler) deleteSpanByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	spanIDs, err := h.parseIDParams(req, ErrInvalidSpanID, normalizeSpanID)
	if err != nil {
		return nil, err
	}

	if err := spans.DeleteSpansByIDs(ctx, h.store.DB(), spanIDs); err != nil {
		h.logger.Error("deleting spans by IDs", zap.Error(err))
		return nil, mapStoreError(err)
	}

	return map[string]any{
		"message": "Spans deleted successfully",
		"count":   len(spanIDs),
	}, nil
}

// deleteLogByID deletes one or more specific logs by their IDs.
func (h *JSONRPCHandler) deleteLogByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	logIDs, err := h.parseIDParams(req, ErrInvalidLogID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := logs.DeleteLogsByIDs(ctx, h.store.DB(), logIDs); err != nil {
		h.logger.Error("deleting logs by IDs", zap.Error(err))
		return nil, mapStoreError(err)
	}

	return map[string]any{
		"message": "Logs deleted successfully",
		"count":   len(logIDs),
	}, nil
}

func (h *JSONRPCHandler) getTraceAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 2 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "2"))
		return nil, jsonrpc2.ErrInvalidParams
	}

	startTime, err := h.parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}

	endTime, err := h.parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}

	attributes, err := spans.GetTraceAttributes(ctx, h.store.DB(), startTime, endTime)
	if err != nil {
		h.logger.Error("getting trace attributes", zap.Error(err))
		return nil, mapStoreError(err)
	}

	return attributes, nil
}

func (h *JSONRPCHandler) getLogAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 2 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "2"))
		return nil, jsonrpc2.ErrInvalidParams
	}

	startTime, err := h.parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}

	endTime, err := h.parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}

	attributes, err := logs.GetLogAttributes(ctx, h.store.DB(), startTime, endTime)
	if err != nil {
		h.logger.Error("getting log attributes", zap.Error(err))
		return nil, mapStoreError(err)
	}

	return attributes, nil
}

func (h *JSONRPCHandler) getMetricAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 2 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "2"))
		return nil, jsonrpc2.ErrInvalidParams
	}

	startTime, err := h.parseTimestampParam(params[0], "startTime")
	if err != nil {
		return nil, err
	}

	endTime, err := h.parseTimestampParam(params[1], "endTime")
	if err != nil {
		return nil, err
	}

	attributes, err := metrics.GetMetricAttributes(ctx, h.store.DB(), startTime, endTime)
	if err != nil {
		h.logger.Error("getting metric attributes", zap.Error(err))
		return nil, mapStoreError(err)
	}

	return attributes, nil
}

func (h *JSONRPCHandler) getAttributesByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 1 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", len(params)), zap.String("expected", "1"))
		return nil, jsonrpc2.ErrInvalidParams
	}

	traceID, err := h.parseIDParam(params[0], ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	attributes, err := spans.GetAttributesByTraceID(ctx, h.store.DB(), traceID)
	if err != nil {
		h.logger.Error("getting attributes by trace ID", zap.Error(err))
		return nil, mapStoreError(err)
	}

	return attributes, nil
}

func (h *JSONRPCHandler) getTraceSpanCount(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}
	traceID, err := h.parseIDParam(params[0], ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	count, err := stats.GetTraceSpanCount(ctx, h.store.DB(), traceID)
	if err != nil {
		h.logger.Error("getting trace span count", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return count, nil
}

func (h *JSONRPCHandler) getStats(ctx context.Context) (any, error) {
	sizeBytes, err := h.store.SizeBytes(ctx)
	if err != nil {
		h.logger.Error("measuring store size", zap.Error(err))
		return nil, mapStoreError(err)
	}

	result, err := stats.GetStats(ctx, h.store.DB(), sizeBytes, h.store.RetentionCap())
	if err != nil {
		h.logger.Error("getting stats", zap.Error(err))
		return nil, mapStoreError(err)
	}
	return result, nil
}

// parseIDParams unmarshals a request's params as a non-empty array of entity
// IDs, validating and normalizing each element with the given normalize
// function. A malformed array returns ErrInvalidParams; a malformed element
// returns invalidIDErr (the signal-specific -3200x code). Previously these
// params went straight into SQL, where a non-string or non-UUID value became
// a DB cast error reported as a generic internal error.
func (h *JSONRPCHandler) parseIDParams(req *jsonrpc2.Request, invalidIDErr error, normalize func(string) (string, error)) ([]any, error) {
	var params []any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		h.logger.Debug("invalid RPC params", zap.Error(err))
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) == 0 {
		h.logger.Debug("invalid RPC parameter count", zap.Int("count", 0), zap.String("expected", "at least 1 ID"))
		return nil, jsonrpc2.ErrInvalidParams
	}

	ids := make([]any, 0, len(params))
	for _, param := range params {
		s, ok := param.(string)
		if !ok {
			h.logger.Debug("invalid ID type", zap.String("expected", "string"), zap.Any("param", param))
			return nil, invalidIDErr
		}
		id, err := normalize(s)
		if err != nil {
			h.logger.Debug("invalid ID", zap.String("id", s), zap.Error(err))
			return nil, invalidIDErr
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// parseIDParam validates and normalizes a single entity ID param (read
// paths: searchSpans, getLog, getMetric, getAttributesByTraceID,
// getTraceSpanCount). Like parseIDParams, a bad value returns the
// signal-specific -3200x code instead of reaching SQL as a cast error.
func (h *JSONRPCHandler) parseIDParam(param any, invalidIDErr error, normalize func(string) (string, error)) (string, error) {
	s, ok := param.(string)
	if !ok {
		h.logger.Debug("invalid ID type", zap.String("expected", "string"), zap.Any("param", param))
		return "", invalidIDErr
	}
	id, err := normalize(s)
	if err != nil {
		h.logger.Debug("invalid ID", zap.String("id", s), zap.Error(err))
		return "", invalidIDErr
	}
	return id, nil
}

// normalizeUUID validates a 128-bit entity ID (trace IDs: 32-char hex on the
// wire; log IDs: tool-minted dashed UUIDs; both stored in uuid columns) and
// returns it in canonical dashed form. Accepts 32-char hex and
// UUID-with-dashes. The length gate is NOT redundant with uuid.Parse: it
// exists to reject the braced {...} and urn:uuid: forms Parse would
// otherwise accept, keeping the API surface at exactly the two shapes we
// serve.
func normalizeUUID(s string) (string, error) {
	if len(s) != 32 && len(s) != 36 {
		return "", fmt.Errorf("ID must be 32-char hex or a dashed UUID, got %d chars", len(s))
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// normalizeSpanID additionally accepts the 16-char hex span ID the API serves
// in span payloads (OTLP span IDs are 8 bytes). Ingest zero-pads those into
// the low bytes of the span_id uuid column, so the same padding is applied
// here before the lookup.
func normalizeSpanID(s string) (string, error) {
	if len(s) == 16 {
		return normalizeUUID("0000000000000000" + s)
	}
	return normalizeUUID(s)
}

// parseTimestampParam parses a timestamp parameter that must be a JSON string
// containing a base-10 int64. Large integers travel as strings to avoid
// float64 precision loss in JSON.
func (h *JSONRPCHandler) parseTimestampParam(param any, paramName string) (int64, error) {
	s, ok := param.(string)
	if !ok {
		h.logger.Debug("invalid timestamp param type", zap.String("param", paramName), zap.Any("value", param))
		return 0, jsonrpc2.ErrInvalidParams
	}
	parsed, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		h.logger.Debug("invalid timestamp param", zap.String("param", paramName), zap.Error(err))
		return 0, jsonrpc2.ErrInvalidParams
	}
	return parsed, nil
}
