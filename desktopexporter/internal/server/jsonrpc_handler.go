package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	storequery "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/query"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/stats"
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

// storeRead runs a query that returns a value, under the store's read lock.
// Every read path in this file goes through here so no handler reaches the
// pool unordered against ingest and retention.
func storeRead[T any](s *store.Store, fn func(db *sql.DB) (T, error)) (T, error) {
	var out T
	err := s.WithDBRead(func(db *sql.DB) error {
		var err error
		out, err = fn(db)
		return err
	})
	return out, err
}

func handlerRead[T any](ctx context.Context, h *JSONRPCHandler, fn func(db *sql.DB) (T, error)) (any, error) {
	result, err := storeRead(h.store, fn)
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
	case "searchTraces":
		return h.searchTraces(ctx, req)
	case "searchSpans":
		return h.searchSpans(ctx, req)
	case "getTrace":
		return h.getTrace(ctx, req)
	case "getSpan":
		return h.getSpan(ctx, req)
	case "searchLogs":
		return h.searchLogs(ctx, req)
	case "getTraceLogs":
		return h.getTraceLogs(ctx, req)
	case "getLog":
		return h.getLog(ctx, req)
	case "searchMetricSummaries":
		return h.searchMetricSummaries(ctx, req)
	case "getMetric":
		return h.getMetric(ctx, req)
	case "getMetricAggregate":
		return h.getMetricAggregate(ctx, req)
	case "clearTraces":
		return h.clearTraces(ctx)
	case "clearLogs":
		return h.clearLogs(ctx)
	case "clearMetrics":
		return h.clearMetrics(ctx)
	case "deleteMetricStream":
		return h.deleteMetricStream(ctx, req)
	case "deleteSpansByTraceID":
		return h.deleteSpansByTraceID(ctx, req)
	case "deleteLogByID":
		return h.deleteLogByID(ctx, req)
	case "getTraceAttributes":
		return h.getTraceAttributes(ctx, req)
	case "getLogAttributes":
		return h.getLogAttributes(ctx, req)
	case "getMetricAttributes":
		return h.getMetricAttributes(ctx, req)
	case "getFieldValues":
		return h.getFieldValues(ctx, req)
	case "searchAttributes":
		return h.searchAttributes(ctx, req)
	case "getAttributesByTraceID":
		return h.getAttributesByTraceID(ctx, req)
	case "getStats":
		return h.getStats(ctx)
	case "getTraceSpanCount":
		return h.getTraceSpanCount(ctx, req)
	case "query":
		return h.query(ctx, req)
	default:
		return nil, jsonrpc2.ErrMethodNotFound
	}
}

func (h *JSONRPCHandler) query(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseQueryParams(req.Params)
	if err != nil {
		return nil, err
	}

	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		conn, err := db.Conn(ctx)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		return storequery.Execute(ctx, conn, params.statement, params.limit)
	})
	if err == nil {
		return result, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, h.handleStoreError(ctx, err)
	}
	if errors.Is(err, storequery.ErrReadOnly) {
		return nil, fmt.Errorf("query rejected: %v: %w", err, jsonrpc2.ErrInvalidParams)
	}
	return nil, fmt.Errorf("query failed: %v: %w", err, ErrInvalidQuery)
}

func (h *JSONRPCHandler) searchTraces(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseSearchParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchTracesWithOptions(ctx, db, params.timeRange, params.query, params.options)
	})
}

func (h *JSONRPCHandler) searchSpans(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := decodePositionalParams(req.Params, 1, 2)
	if err != nil {
		return nil, err
	}
	traceID, err := parseIDParam(params[0], ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	var query any
	if len(params) == 2 {
		query = params[1]
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, traceID, query)
	})
}

type compactTraceResult struct {
	Trace compactTraceSummary `json:"trace"`
	Spans []compactTraceSpan  `json:"spans"`
	Logs  []compactTraceLog   `json:"logs"`
}

type compactTraceSummary struct {
	TraceID    string `json:"traceID"`
	SpanCount  int64  `json:"spanCount"`
	LogCount   int    `json:"logCount"`
	StartTime  string `json:"startTime"`
	DurationNs string `json:"durationNs"`
}

type compactTraceSpan struct {
	SpanID        string  `json:"spanID"`
	ParentSpanID  *string `json:"parentSpanID"`
	Service       string  `json:"service"`
	Name          string  `json:"name"`
	StartOffsetNs string  `json:"startOffsetNs"`
	DurationNs    string  `json:"durationNs"`
}

type traceLogSummary struct {
	Timestamp      string      `json:"timestamp"`
	SpanID         *string     `json:"spanID"`
	SeverityText   string      `json:"severityText"`
	SeverityNumber json.Number `json:"severityNumber"`
	ServiceName    string      `json:"serviceName"`
	EventName      string      `json:"eventName"`
	BodyPreview    string      `json:"bodyPreview"`
}

type compactTraceLog struct {
	Timestamp string  `json:"timestamp"`
	SpanID    *string `json:"spanID"`
	Severity  string  `json:"severity"`
	Service   string  `json:"service"`
	EventName string  `json:"eventName"`
	Body      string  `json:"body"`
}

func (h *JSONRPCHandler) getTrace(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceID, err := parseSingleIDParam(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (compactTraceResult, error) {
		traceRaw, err := spans.GetTrace(ctx, db, traceID)
		if err != nil {
			return compactTraceResult{}, err
		}
		var result compactTraceResult
		if err := json.Unmarshal(traceRaw, &result); err != nil {
			return compactTraceResult{}, err
		}
		logsRaw, err := logs.GetTraceLogs(ctx, db, traceID)
		if err != nil {
			return compactTraceResult{}, err
		}
		result.Logs, err = compactTraceLogs(logsRaw)
		if err != nil {
			return compactTraceResult{}, err
		}
		result.Trace.LogCount = len(result.Logs)
		return result, nil
	})
}

func compactTraceLogs(raw json.RawMessage) ([]compactTraceLog, error) {
	var summaries []traceLogSummary
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&summaries); err != nil {
		return nil, err
	}
	result := make([]compactTraceLog, len(summaries))
	for i, summary := range summaries {
		severity, err := logSeverityLabel(summary.SeverityText, summary.SeverityNumber)
		if err != nil {
			return nil, err
		}
		result[i] = compactTraceLog{
			Timestamp: summary.Timestamp, SpanID: summary.SpanID,
			Severity: severity,
			Service:  summary.ServiceName, EventName: summary.EventName, Body: summary.BodyPreview,
		}
	}
	return result, nil
}

func logSeverityLabel(text string, number json.Number) (string, error) {
	if text != "" {
		return text, nil
	}
	value, err := number.Int64()
	if err != nil {
		return "", fmt.Errorf("decode log severity number: %w", err)
	}
	switch {
	case value == 0:
		return "UNSPECIFIED", nil
	case value >= 1 && value <= 4:
		return "TRACE", nil
	case value >= 5 && value <= 8:
		return "DEBUG", nil
	case value >= 9 && value <= 12:
		return "INFO", nil
	case value >= 13 && value <= 16:
		return "WARN", nil
	case value >= 17 && value <= 20:
		return "ERROR", nil
	case value >= 21 && value <= 24:
		return "FATAL", nil
	}
	return fmt.Sprintf("Unknown (%d)", value), nil
}

type spanNotFoundResult struct {
	Status  string  `json:"status"`
	SpanID  string  `json:"spanID"`
	TraceID *string `json:"traceID"`
}

type spanAmbiguousResult struct {
	Status     string          `json:"status"`
	SpanID     string          `json:"spanID"`
	MatchCount int64           `json:"matchCount"`
	Summaries  json.RawMessage `json:"summaries"`
	Truncated  bool            `json:"truncated"`
}

type spanFoundResult struct {
	Status  string          `json:"status"`
	TraceID string          `json:"traceID"`
	Span    json.RawMessage `json:"span"`
	Logs    json.RawMessage `json:"logs"`
}

func (h *JSONRPCHandler) getSpan(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := decodePositionalParams(req.Params, 1, 3)
	if err != nil {
		return nil, err
	}
	spanID, spanValue, err := parseSpanIDParam(params[0])
	if err != nil {
		return nil, err
	}
	var requestedTraceID *string
	if len(params) >= 2 && params[1] != nil {
		traceID, err := parseIDParam(params[1], ErrInvalidTraceID, normalizeUUID)
		if err != nil {
			return nil, err
		}
		requestedTraceID = &traceID
	}
	limit := int64(25)
	if len(params) == 3 && params[2] != nil {
		parsed, err := parseTimestampParam(params[2], "limit")
		if err != nil || parsed < 1 || parsed > 25 {
			return nil, jsonrpc2.ErrInvalidParams
		}
		limit = parsed
	}

	return handlerRead(ctx, h, func(db *sql.DB) (any, error) {
		traceID := requestedTraceID
		if traceID == nil {
			summaries, matchCount, err := spans.GetSpanSummaries(ctx, db, spanValue, limit+1)
			if err != nil {
				return nil, err
			}
			var rows []json.RawMessage
			if err := json.Unmarshal(summaries, &rows); err != nil {
				return nil, err
			}
			switch matchCount {
			case 0:
				return spanNotFoundResult{Status: "notFound", SpanID: spanID}, nil
			case 1:
				var summary struct {
					TraceID string `json:"traceID"`
				}
				if len(rows) != 1 || json.Unmarshal(rows[0], &summary) != nil || summary.TraceID == "" {
					return nil, errors.New("invalid span summary result")
				}
				traceID = &summary.TraceID
			default:
				truncated := int64(len(rows)) > limit
				if truncated {
					rows = rows[:limit]
				}
				bounded, err := json.Marshal(rows)
				if err != nil {
					return nil, err
				}
				return spanAmbiguousResult{
					Status: "ambiguous", SpanID: spanID,
					MatchCount: matchCount, Summaries: bounded, Truncated: truncated,
				}, nil
			}
		}

		span, err := spans.GetSpan(ctx, db, *traceID, spanValue)
		if err != nil {
			return nil, err
		}
		if span == nil {
			resultTraceID := strings.ReplaceAll(*traceID, "-", "")
			return spanNotFoundResult{Status: "notFound", SpanID: spanID, TraceID: &resultTraceID}, nil
		}
		spanLogs, err := logs.GetSpanLogs(ctx, db, *traceID, spanValue)
		if err != nil {
			return nil, err
		}
		return spanFoundResult{
			Status: "found", TraceID: strings.ReplaceAll(*traceID, "-", ""),
			Span: span, Logs: spanLogs,
		}, nil
	})
}

func parseSpanIDParam(value any) (string, uint64, error) {
	text, ok := value.(string)
	if !ok || len(text) != 16 {
		return "", 0, jsonrpc2.ErrInvalidParams
	}
	parsed, err := strconv.ParseUint(text, 16, 64)
	if err != nil {
		return "", 0, jsonrpc2.ErrInvalidParams
	}
	return strings.ToLower(text), parsed, nil
}

func (h *JSONRPCHandler) clearTraces(ctx context.Context) (any, error) {
	return h.clearSignal(ctx, spans.Clear, "Traces cleared successfully")
}

func (h *JSONRPCHandler) searchLogs(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseSearchParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.SearchWithOptions(ctx, db, params.timeRange, params.query, params.options)
	})
}

func (h *JSONRPCHandler) getTraceLogs(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceID, err := parseSingleIDParam(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetTraceLogs(ctx, db, traceID)
	})
}

func (h *JSONRPCHandler) clearLogs(ctx context.Context) (any, error) {
	return h.clearSignal(ctx, logs.Clear, "Logs cleared successfully")
}

// getLog returns the full LogData for a single log row identified by
// its tool-minted UUID (the same id returned in Search summaries).
func (h *JSONRPCHandler) getLog(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	logID, err := parseSingleIDParam(req.Params, ErrInvalidLogID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.Get(ctx, db, logID)
	})
}

func (h *JSONRPCHandler) searchMetricSummaries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseSearchParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummariesWithOptions(ctx, db, params.timeRange, params.query, params.options)
	})
}

func (h *JSONRPCHandler) getMetric(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	args, err := parseGetMetricParams(req.Params, false)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, args.streamID, args.timeRange,
			args.targetBuckets, args.seriesIDs, args.quantiles, args.tzOffsetNs, args.viewBuckets, args.sparklineBuckets, args.selectedSeriesIDs, args.tzName,
			args.datapointSeriesIDs, args.datapointSeriesLimit)
	})
}

func (h *JSONRPCHandler) clearMetrics(ctx context.Context) (any, error) {
	return h.clearSignal(ctx, metrics.Clear, "Metrics cleared successfully")
}

func (h *JSONRPCHandler) clearSignal(
	ctx context.Context,
	clear func(context.Context, *sql.DB) error,
	successMessage string,
) (any, error) {
	// Clear deletes the signal's own rows but never the dictionary: attribute,
	// resource and scope rows are shared across signals, so only a sweep can
	// prove one is unreferenced. It has to run here rather than being left to
	// retention -- retention is size-driven and does not run at all when the
	// cap is disabled, which would leak every orphaned row until restart.
	err := h.store.WithDBWrite(func(db *sql.DB) error {
		if err := clear(ctx, db); err != nil {
			return err
		}
		return ingest.SweepOrphans(ctx, db, h.store.FlushedIDs())
	})
	if err != nil {
		return nil, h.handleStoreError(ctx, err)
	}
	return successMessage, nil
}

// deleteMetricStream deletes one metric stream and everything hanging off it.
// It takes a single ID rather than an array like deleteLogByID and
// deleteSpansByTraceID: metrics identify a stream by one UUID everywhere else
// in this handler (see getMetric), and the delete cascade in the store is keyed
// on a single stream_id.
func (h *JSONRPCHandler) deleteMetricStream(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	streamID, err := parseSingleIDParam(req.Params, ErrInvalidStreamID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := h.store.WithDBWrite(func(db *sql.DB) error {
		return metrics.DeleteMetricStream(ctx, db, streamID)
	}); err != nil {
		return nil, h.handleStoreError(ctx, err)
	}

	return "Metric stream deleted successfully", nil
}

// deleteSpansByTraceID deletes all spans for one or more traces.
func (h *JSONRPCHandler) deleteSpansByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceIDs, err := parseIDParams(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := h.store.WithDBWrite(func(db *sql.DB) error {
		return spans.DeleteSpansByTraceIDs(ctx, db, traceIDs)
	}); err != nil {
		return nil, h.handleStoreError(ctx, err)
	}

	return map[string]any{
		"message": "Spans deleted successfully",
		"count":   len(traceIDs),
	}, nil
}

// deleteLogByID deletes one or more specific logs by their IDs.
func (h *JSONRPCHandler) deleteLogByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	logIDs, err := parseIDParams(req.Params, ErrInvalidLogID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := h.store.WithDBWrite(func(db *sql.DB) error {
		return logs.DeleteLogsByIDs(ctx, db, logIDs)
	}); err != nil {
		return nil, h.handleStoreError(ctx, err)
	}

	return map[string]any{
		"message": "Logs deleted successfully",
		"count":   len(logIDs),
	}, nil
}

// searchAttributes answers "which attribute keys hold this text?" across every
// signal at once, from the dictionary alone.
//
// Deliberately not scoped to a signal or a time window, unlike the
// getXAttributes discovery methods. The dictionary is shared -- one row per
// distinct (key, value, type, scope) for the whole store -- so narrowing to one
// signal would mean unnesting owner arrays to find out which signals reference
// a row, which is the cost this avoids. The scope on each result says which
// signals can carry it.
func (h *JSONRPCHandler) searchAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	term, err := parseSingleStringParam(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return attributes.Search(ctx, db, term)
	})
}

func (h *JSONRPCHandler) getFieldValues(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseFieldValuesParams(req.Params)
	if err != nil {
		return nil, err
	}

	// A map rather than a switch: the named-params coverage test reads
	// Handle's dispatch cases out of this file by pattern, and a case-shaped
	// signal dispatch would show up in it as three phantom methods.
	bySignal := map[string]func(context.Context, *sql.DB, string, string, int64) (json.RawMessage, error){
		"traces":  spans.GetFieldValues,
		"logs":    logs.GetFieldValues,
		"metrics": metrics.GetFieldValues,
	}
	get, ok := bySignal[params.signal]
	if !ok {
		return nil, fmt.Errorf("%w: unknown signal %q", jsonrpc2.ErrInvalidParams, params.signal)
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return get(ctx, db, params.field, params.term, params.limit)
	})
}

func (h *JSONRPCHandler) getTraceAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := validateNoParams(req.Params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceAttributes(ctx, db)
	})
}

func (h *JSONRPCHandler) getLogAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := validateNoParams(req.Params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetLogAttributes(ctx, db)
	})
}

// getMetricAggregate takes the common metric parameters, viewBuckets,
// selectedSeriesIDs and tzName, and returns only the cross-series aggregate.
// Separate method rather than a flag on getMetric:
// the two are fetched on different triggers -- metric selection versus legend
// selection -- so they are different requests, not one request in two modes.
func (h *JSONRPCHandler) getMetricAggregate(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	args, err := parseGetMetricParams(req.Params, true)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregate(ctx, db, args.streamID, args.timeRange,
			args.targetBuckets, args.seriesIDs, args.quantiles, args.tzOffsetNs,
			args.viewBuckets, args.selectedSeriesIDs, args.tzName)
	})
}

func (h *JSONRPCHandler) getMetricAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := validateNoParams(req.Params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAttributes(ctx, db)
	})
}

func (h *JSONRPCHandler) getAttributesByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceID, err := parseSingleIDParam(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetAttributesByTraceID(ctx, db, traceID)
	})
}

func (h *JSONRPCHandler) getTraceSpanCount(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceID, err := parseSingleIDParam(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (int64, error) {
		return stats.GetTraceSpanCount(ctx, db, traceID)
	})
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
