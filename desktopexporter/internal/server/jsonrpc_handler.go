package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	storequery "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/query"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
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

func decodePositionalParams(raw json.RawMessage, minParams, maxParams int) ([]any, error) {
	var params []any
	if err := decodeParams(raw, &params); err != nil || len(params) < minParams || len(params) > maxParams {
		return nil, jsonrpc2.ErrInvalidParams
	}
	return params, nil
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

type queryParams struct {
	statement string
	limit     uint64
}

func parseQueryParams(raw json.RawMessage) (queryParams, error) {
	params, err := decodePositionalParams(raw, 1, 2)
	if err != nil {
		return queryParams{}, err
	}
	statement, ok := params[0].(string)
	if !ok {
		return queryParams{}, fmt.Errorf("sql must be a string: %w", jsonrpc2.ErrInvalidParams)
	}
	limit := storequery.DefaultLimit
	if len(params) == 2 {
		number, ok := params[1].(json.Number)
		if !ok {
			return queryParams{}, fmt.Errorf("limit must be a non-negative whole number: %w", jsonrpc2.ErrInvalidParams)
		}
		parsed, err := strconv.ParseUint(number.String(), 10, 64)
		if err != nil {
			return queryParams{}, fmt.Errorf("limit must be a non-negative whole number: %w", jsonrpc2.ErrInvalidParams)
		}
		limit = parsed
	}
	return queryParams{statement: statement, limit: limit}, nil
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

type searchParams struct {
	timeRange store.TimeRange
	query     any
	options   search.ResultOptions
}

func parseSearchParams(raw json.RawMessage) (searchParams, error) {
	params, err := decodePositionalParams(raw, 2, 5)
	if err != nil {
		return searchParams{}, err
	}
	timeRange, err := parseTimeRange(params[0], params[1])
	if err != nil {
		return searchParams{}, err
	}
	var query any
	if len(params) >= 3 {
		query = params[2]
	}
	options, err := parseSearchOptions(params)
	if err != nil {
		return searchParams{}, err
	}
	return searchParams{timeRange: timeRange, query: query, options: options}, nil
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

func parseSearchLimit(params []any) (*int64, error) {
	if len(params) < 4 || params[3] == nil {
		return nil, nil
	}
	limit, err := parseTimestampParam(params[3], "limit")
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be positive: %w", jsonrpc2.ErrInvalidParams)
	}
	return &limit, nil
}

func parseSearchOptions(params []any) (search.ResultOptions, error) {
	limit, err := parseSearchLimit(params)
	if err != nil {
		return search.ResultOptions{}, err
	}
	if len(params) < 5 || params[4] == nil {
		return search.ResultOptions{Limit: limit}, nil
	}

	rawSort, ok := params[4].(map[string]any)
	if !ok {
		return search.ResultOptions{}, fmt.Errorf("sort must be an object: %w", jsonrpc2.ErrInvalidParams)
	}
	field, fieldOK := rawSort["field"].(string)
	direction, directionOK := rawSort["direction"].(string)
	if !fieldOK || !directionOK || len(rawSort) != 2 {
		return search.ResultOptions{}, fmt.Errorf("sort must contain only string field and direction properties: %w", jsonrpc2.ErrInvalidParams)
	}
	return search.ResultOptions{
		Limit: limit,
		Sort:  &search.Sort{Field: field, Direction: direction},
	}, nil
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

// getMetricParams holds the common metric request plus detail-only fields.
type getMetricParams struct {
	streamID             string
	timeRange            store.TimeRange
	targetBuckets        int64
	seriesIDs            []string
	quantiles            []float64
	tzOffsetNs           int64
	viewBuckets          int64
	sparklineBuckets     int64
	selectedSeriesIDs    []string
	datapointSeriesIDs   []string
	datapointSeriesLimit int64
	tzName               string
}

func parseOptionalInt(params []any, index int, name string) (int64, error) {
	if len(params) <= index || params[index] == nil {
		return 0, nil
	}
	return parseTimestampParam(params[index], name)
}

func parseOptionalNonNegativeInt(params []any, index int, name string) (int64, error) {
	value, err := parseOptionalInt(params, index, name)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, jsonrpc2.ErrInvalidParams
	}
	return value, nil
}

func parseOptionalStringList(params []any, index int) ([]string, error) {
	if len(params) <= index || params[index] == nil {
		return nil, nil
	}
	raw, ok := params[index].([]any)
	if !ok {
		return nil, jsonrpc2.ErrInvalidParams
	}
	values := make([]string, 0, len(raw))
	for _, value := range raw {
		text, ok := value.(string)
		if !ok {
			return nil, jsonrpc2.ErrInvalidParams
		}
		values = append(values, text)
	}
	return values, nil
}

func parseOptionalQuantiles(params []any, index int) ([]float64, error) {
	if len(params) <= index || params[index] == nil {
		return nil, nil
	}
	raw, ok := params[index].([]any)
	if !ok {
		return nil, jsonrpc2.ErrInvalidParams
	}
	var quantiles []float64
	for _, value := range raw {
		var quantile float64
		switch number := value.(type) {
		case json.Number:
			parsed, err := number.Float64()
			if err != nil {
				return nil, fmt.Errorf("quantiles must be numbers, got %q: %w", number.String(), jsonrpc2.ErrInvalidParams)
			}
			quantile = parsed
		case float64:
			quantile = number
		default:
			return nil, fmt.Errorf("quantiles must be numbers, got %T: %w", value, jsonrpc2.ErrInvalidParams)
		}
		if quantile < 0 || quantile > 1 {
			return nil, fmt.Errorf("quantiles must be between 0 and 1, got %v: %w", quantile, jsonrpc2.ErrInvalidParams)
		}
		quantiles = append(quantiles, quantile)
	}
	return quantiles, nil
}

func parseOptionalString(params []any, index int) (string, error) {
	if len(params) <= index || params[index] == nil {
		return "", nil
	}
	value, ok := params[index].(string)
	if !ok {
		return "", jsonrpc2.ErrInvalidParams
	}
	return value, nil
}

func parseGetMetricParams(raw json.RawMessage, aggregateOnly bool) (getMetricParams, error) {
	maxParams := 13
	if aggregateOnly {
		maxParams = 10
	}
	params, err := decodePositionalParams(raw, 3, maxParams)
	if err != nil {
		return getMetricParams{}, err
	}
	streamID, err := parseIDParam(params[0], ErrInvalidStreamID, normalizeUUID)
	if err != nil {
		return getMetricParams{}, err
	}
	timeRange, err := parseTimeRange(params[1], params[2])
	if err != nil {
		return getMetricParams{}, err
	}
	targetBuckets, err := parseOptionalNonNegativeInt(params, 3, "targetBuckets")
	if err != nil {
		return getMetricParams{}, err
	}
	seriesIDs, err := parseOptionalStringList(params, 4)
	if err != nil {
		return getMetricParams{}, err
	}
	quantiles, err := parseOptionalQuantiles(params, 5)
	if err != nil {
		return getMetricParams{}, err
	}
	tzOffsetNs, err := parseOptionalInt(params, 6, "tzOffsetNs")
	if err != nil {
		return getMetricParams{}, err
	}
	viewBuckets, err := parseOptionalNonNegativeInt(params, 7, "viewBuckets")
	if err != nil {
		return getMetricParams{}, err
	}

	selectedSeriesIndex, tzNameIndex := 9, 10
	var sparklineBuckets int64
	if aggregateOnly {
		selectedSeriesIndex, tzNameIndex = 8, 9
	} else {
		sparklineBuckets, err = parseOptionalNonNegativeInt(params, 8, "sparklineBuckets")
		if err != nil {
			return getMetricParams{}, err
		}
	}
	selectedSeriesIDs, err := parseOptionalStringList(params, selectedSeriesIndex)
	if err != nil {
		return getMetricParams{}, err
	}
	tzName, err := parseOptionalString(params, tzNameIndex)
	if err != nil {
		return getMetricParams{}, err
	}

	var datapointSeriesIDs []string
	var datapointSeriesLimit int64
	if !aggregateOnly {
		datapointSeriesIDs, err = parseOptionalStringList(params, 11)
		if err != nil {
			return getMetricParams{}, err
		}
		datapointSeriesLimit, err = parseOptionalNonNegativeInt(params, 12, "datapointSeriesLimit")
		if err != nil {
			return getMetricParams{}, err
		}
	}

	return getMetricParams{
		streamID: streamID, timeRange: timeRange, targetBuckets: targetBuckets,
		seriesIDs: seriesIDs, quantiles: quantiles, tzOffsetNs: tzOffsetNs,
		viewBuckets: viewBuckets, sparklineBuckets: sparklineBuckets,
		selectedSeriesIDs: selectedSeriesIDs, datapointSeriesIDs: datapointSeriesIDs,
		datapointSeriesLimit: datapointSeriesLimit, tzName: tzName,
	}, nil
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

// getFieldValues returns distinct values of one completable column, for
// value completion in the search box. Which fields complete is decided by
// each signal package's allowlist, so an unknown field is an invalid-params
// answer rather than an empty list -- a typo should look like one. The limit
// is clamped rather than trusted: the caller wants a dropdown, not a dump.
type fieldValuesParams struct {
	signal string
	field  string
	term   string
	limit  int64
}

func parseFieldValuesParams(raw json.RawMessage) (fieldValuesParams, error) {
	params, err := decodePositionalParams(raw, 4, 4)
	if err != nil {
		return fieldValuesParams{}, err
	}
	values := make([]string, 3)
	for index := range values {
		value, ok := params[index].(string)
		if !ok {
			return fieldValuesParams{}, jsonrpc2.ErrInvalidParams
		}
		values[index] = value
	}
	// Same decoder targetBuckets uses: accepts a JSON number or a string.
	limit, err := parseTimestampParam(params[3], "limit")
	if err != nil {
		return fieldValuesParams{}, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}
	return fieldValuesParams{signal: values[0], field: values[1], term: values[2], limit: limit}, nil
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

// parseIDParams unmarshals a request's params as a non-empty array of entity
// IDs, validating and normalizing each element with the given normalize
// function. A malformed array returns ErrInvalidParams; a malformed element
// returns invalidIDErr (the signal-specific -3200x code). Previously these
// params went straight into SQL, where a non-string or non-UUID value became
// a DB cast error reported as a generic internal error.
func parseIDParams(raw json.RawMessage, invalidIDErr error, normalize func(string) (string, error)) ([]any, error) {
	var params []any
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) == 0 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	ids := make([]any, 0, len(params))
	for _, param := range params {
		s, ok := param.(string)
		if !ok {
			return nil, invalidIDErr
		}
		id, err := normalize(s)
		if err != nil {
			return nil, invalidIDErr
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// parseIDParam validates and normalizes a single entity ID param (read
// paths: searchSpans, getTraceLogs, getLog, getMetric, getAttributesByTraceID,
// getTraceSpanCount). Like parseIDParams, a bad value returns the
// signal-specific -3200x code instead of reaching SQL as a cast error.
func parseIDParam(param any, invalidIDErr error, normalize func(string) (string, error)) (string, error) {
	s, ok := param.(string)
	if !ok {
		return "", invalidIDErr
	}
	id, err := normalize(s)
	if err != nil {
		return "", invalidIDErr
	}
	return id, nil
}

func parseSingleIDParam(raw json.RawMessage, invalidIDErr error, normalize func(string) (string, error)) (string, error) {
	params, err := decodePositionalParams(raw, 1, 1)
	if err != nil {
		return "", err
	}
	return parseIDParam(params[0], invalidIDErr, normalize)
}

func parseSingleStringParam(raw json.RawMessage) (string, error) {
	params, err := decodePositionalParams(raw, 1, 1)
	if err != nil {
		return "", err
	}
	value, ok := params[0].(string)
	if !ok {
		return "", jsonrpc2.ErrInvalidParams
	}
	return value, nil
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

// parseTimestampParam reads a whole number sent either as a JSON string or as
// a JSON number.
//
// Strings were the original and only accepted form, for a real reason:
// nanosecond timestamps are around 1.8e18, and JSON numbers decoded into `any`
// become float64, which is exact only to 2^53. Three of four realistic
// timestamps lose precision that way -- by up to 65ns -- which would move a
// query boundary silently rather than failing.
//
// So numbers are accepted, but only because params are decoded with
// UseNumber: a JSON number arrives as json.Number, which is text, and parses
// to int64 exactly. float64 is rejected outright rather than rounded, since
// reaching this function with one means the decoder was bypassed and the
// precision is already gone.
//
// The error says which parameter and what was wrong with it. The bare
// "invalid params" this used to return gave a caller nothing to act on, which
// costs time even when the caller can read this file.
func parseTimestampParam(param any, paramName string) (int64, error) {
	var text string
	switch v := param.(type) {
	case string:
		text = v
	case json.Number:
		text = v.String()
	case float64:
		return 0, fmt.Errorf(
			"%s decoded as float64, which cannot hold a nanosecond timestamp exactly: %w",
			paramName, jsonrpc2.ErrInvalidParams)
	default:
		return 0, fmt.Errorf(
			"%s must be a whole number, as a JSON number or a decimal string, got %T: %w",
			paramName, param, jsonrpc2.ErrInvalidParams)
	}

	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"%s must be a whole number, got %q: %w",
			paramName, text, jsonrpc2.ErrInvalidParams)
	}
	return parsed, nil
}

// parseOptionalTimestampParam is reserved for nullable time bounds. All other
// numeric parameters keep parseTimestampParam's strict non-null contract.
func parseOptionalTimestampParam(param any, paramName string) (*uint64, error) {
	if param == nil {
		return nil, nil
	}
	var text string
	switch v := param.(type) {
	case string:
		text = v
	case json.Number:
		text = v.String()
	case float64:
		return nil, fmt.Errorf(
			"%s decoded as float64, which cannot hold a nanosecond timestamp exactly: %w",
			paramName, jsonrpc2.ErrInvalidParams)
	default:
		return nil, fmt.Errorf(
			"%s must be an unsigned whole number, as a JSON number or a decimal string, got %T: %w",
			paramName, param, jsonrpc2.ErrInvalidParams)
	}
	parsed, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return nil, fmt.Errorf(
			"%s must be an unsigned whole number, got %q: %w",
			paramName, text, jsonrpc2.ErrInvalidParams)
	}
	return &parsed, nil
}

func parseTimeRange(startParam, endParam any) (store.TimeRange, error) {
	startTime, err := parseOptionalTimestampParam(startParam, "startTime")
	if err != nil {
		return store.TimeRange{}, err
	}
	endTime, err := parseOptionalTimestampParam(endParam, "endTime")
	if err != nil {
		return store.TimeRange{}, err
	}
	return store.TimeRange{Start: startTime, End: endTime}, nil
}

// decodeParams unmarshals a request's params with UseNumber, so JSON numbers
// arrive as json.Number rather than float64 and keep full integer precision.
//
// Every params decode in this file goes through here. Using json.Unmarshal
// directly would silently reintroduce the float64 rounding that
// parseTimestampParam exists to avoid.
func decodeParams(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(dst)
}

func validateNoParams(raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var params []any
	if err := decodeParams(raw, &params); err != nil || len(params) != 0 {
		return jsonrpc2.ErrInvalidParams
	}
	return nil
}
