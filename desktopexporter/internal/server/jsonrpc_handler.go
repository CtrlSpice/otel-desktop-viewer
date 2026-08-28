package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
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

// handleStoreError maps store errors to JSON-RPC codes and logs only unexpected
// failures (those that become -32603). Expected outcomes like not-found,
// invalid query, or a caller that went away are returned without logging.
func (h *JSONRPCHandler) handleStoreError(err error) error {
	if err == nil {
		return nil
	}
	mapped := mapStoreError(err)
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
	case "getSpanNames":
		return h.getSpanNames(ctx, req)
	case "searchAttributes":
		return h.searchAttributes(ctx, req)
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
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) < 2 || len(params) > 3 {
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

	summaries, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchTraces(ctx, db, startTime, endTime, query)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return summaries, nil
}

func (h *JSONRPCHandler) searchSpans(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) < 1 || len(params) > 2 {
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

	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, traceID, query)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) clearTraces(ctx context.Context) (any, error) {
	// Clear deletes the signal's own rows but never the dictionary: attribute,
	// resource and scope rows are shared across signals, so only a sweep can
	// prove one is unreferenced. It has to run here rather than being left to
	// retention -- retention is size-driven and does not run at all when the
	// cap is disabled, which would leak every orphaned row until restart.
	err := h.store.WithDBWrite(func(db *sql.DB) error {
		if err := spans.Clear(ctx, db); err != nil {
			return err
		}
		return ingest.SweepOrphans(ctx, db, h.store.FlushedIDs())
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return "Traces cleared successfully", nil
}

func (h *JSONRPCHandler) searchLogs(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) < 2 || len(params) > 3 {
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
	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return logs.Search(ctx, db, startTime, endTime, query)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) clearLogs(ctx context.Context) (any, error) {
	// Clear deletes the signal's own rows but never the dictionary: attribute,
	// resource and scope rows are shared across signals, so only a sweep can
	// prove one is unreferenced. It has to run here rather than being left to
	// retention -- retention is size-driven and does not run at all when the
	// cap is disabled, which would leak every orphaned row until restart.
	err := h.store.WithDBWrite(func(db *sql.DB) error {
		if err := logs.Clear(ctx, db); err != nil {
			return err
		}
		return ingest.SweepOrphans(ctx, db, h.store.FlushedIDs())
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return "Logs cleared successfully", nil
}

// getLog returns the full LogData for a single log row identified by
// its tool-minted UUID (the same id returned in Search summaries).
func (h *JSONRPCHandler) getLog(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}
	logID, err := h.parseIDParam(params[0], ErrInvalidLogID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return logs.Get(ctx, db, logID)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) searchMetricSummaries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) < 2 || len(params) > 3 {
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
	summaries, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummaries(ctx, db, startTime, endTime, query)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return summaries, nil
}

// getMetricParams is the parameter set getMetric and getMetricAggregate share.
// They ask the same question of the same window; only the shape of the answer
// differs, so the parsing lives once.
type getMetricParams struct {
	streamID             string
	startTime, endTime   int64
	targetBuckets        int64
	seriesIDs            []string
	quantiles            []float64
	tzOffsetNs           int64
	fitToData            bool
	viewBuckets          int64
	sparklineBuckets     int64
	selectedSeriesIDs    []string
	datapointSeriesIDs   []string
	datapointSeriesLimit int64
	tzName               string
}

func (h *JSONRPCHandler) parseGetMetricParams(req *jsonrpc2.Request) (getMetricParams, error) {
	var out getMetricParams
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return out, jsonrpc2.ErrInvalidParams
	}
	// Everything past the third is optional and additive: targetBuckets,
	// seriesIDs, quantiles, tzOffsetNs, fitToData, viewBuckets,
	// sparklineBuckets, selectedSeriesIDs. A caller that predates any of them
	// sends fewer and gets the old behaviour.
	//
	// The upper bound was 4 and stayed 4 while four more parameters were added
	// below it, so every request carrying them was rejected before it reached
	// them. Nothing caught it: the store tests call GetMetric directly, and the
	// handler tests only ever sent three. TestGetMetricAcceptsEveryParameter
	// now sends a full set, so the bound cannot fall behind again silently.
	if len(params) < 3 || len(params) > 14 {
		return out, jsonrpc2.ErrInvalidParams
	}
	streamID, err := h.parseIDParam(params[0], ErrInvalidStreamID, normalizeUUID)
	if err != nil {
		return out, err
	}
	startTime, err := h.parseTimestampParam(params[1], "startTime")
	if err != nil {
		return out, err
	}
	endTime, err := h.parseTimestampParam(params[2], "endTime")
	if err != nil {
		return out, err
	}
	// How many time buckets to reduce the window to. The client knows its
	// chart width; the store cannot.
	var targetBuckets int64
	if len(params) >= 4 {
		targetBuckets, err = h.parseTimestampParam(params[3], "targetBuckets")
		if err != nil {
			return out, err
		}
		if targetBuckets < 0 {
			return out, jsonrpc2.ErrInvalidParams
		}
	}

	// Which series to return. Absent or empty means all of them, which is what
	// every caller predating this parameter sends.
	var seriesIDs []string
	if len(params) >= 5 && params[4] != nil {
		raw, ok := params[4].([]any)
		if !ok {
			return out, jsonrpc2.ErrInvalidParams
		}
		// Non-nil before the loop: an empty array means "no series", and
		// appending nothing to a nil slice would leave it indistinguishable
		// from the parameter being absent.
		seriesIDs = []string{}
		for _, v := range raw {
			id, ok := v.(string)
			if !ok {
				return out, jsonrpc2.ErrInvalidParams
			}
			seriesIDs = append(seriesIDs, id)
		}
	}

	// Which quantiles to compute per histogram datapoint. Absent or empty
	// means none, so a caller drawing no overlays does not pay for them.
	var quantiles []float64
	if len(params) >= 6 && params[5] != nil {
		raw, ok := params[5].([]any)
		if !ok {
			return out, jsonrpc2.ErrInvalidParams
		}
		for _, v := range raw {
			// Quantiles are genuinely fractional, so float64 is the right type
			// here -- unlike timestamps. They arrive as json.Number because
			// params are decoded with UseNumber; float64 stays accepted so a
			// caller that decoded params some other way still works.
			var q float64
			switch n := v.(type) {
			case json.Number:
				f, err := n.Float64()
				if err != nil {
					return out, fmt.Errorf("quantiles must be numbers, got %q: %w",
						n.String(), jsonrpc2.ErrInvalidParams)
				}
				q = f
			case float64:
				q = n
			default:
				return out, fmt.Errorf("quantiles must be numbers, got %T: %w",
					v, jsonrpc2.ErrInvalidParams)
			}
			if q < 0 || q > 1 {
				return out, fmt.Errorf("quantiles must be between 0 and 1, got %v: %w",
					q, jsonrpc2.ErrInvalidParams)
			}
			quantiles = append(quantiles, q)
		}
	}

	// The viewer's UTC offset in nanoseconds, so day boundaries fall where the
	// reader's calendar puts them. Absent means UTC.
	var tzOffsetNs int64
	if len(params) >= 7 && params[6] != nil {
		tzOffsetNs, err = h.parseTimestampParam(params[6], "tzOffsetNs")
		if err != nil {
			return out, err
		}
	}
	// Whether the window is a request or the absence of one. Absent means it is
	// a request, which is what every caller predating this parameter meant.
	// Resolution for the scalar views. Absent means none, which is what a
	// caller predating the parameter meant.
	var viewBuckets int64
	if len(params) >= 9 && params[8] != nil {
		viewBuckets, err = h.parseTimestampParam(params[8], "viewBuckets")
		if err != nil {
			return out, err
		}
		if viewBuckets < 0 {
			return out, jsonrpc2.ErrInvalidParams
		}
	}

	// Resolution for the per-row sparklines. Absent means none, and a series
	// then carries a null sparkline rather than the store guessing a width for a
	// list row it cannot see.
	var sparklineBuckets int64
	if len(params) >= 10 && params[9] != nil {
		sparklineBuckets, err = h.parseTimestampParam(params[9], "sparklineBuckets")
		if err != nil {
			return out, err
		}
		if sparklineBuckets < 0 {
			return out, jsonrpc2.ErrInvalidParams
		}
	}

	// Which series the reader has checked, for the Selected cross-series line.
	// Absent or null means none are checked -- a real state, in which the chart
	// draws the All line by itself.
	var selectedSeriesIDs []string
	if len(params) >= 11 && params[10] != nil {
		raw, ok := params[10].([]any)
		if !ok {
			return out, jsonrpc2.ErrInvalidParams
		}
		selectedSeriesIDs = []string{}
		for _, v := range raw {
			id, ok := v.(string)
			if !ok {
				return out, jsonrpc2.ErrInvalidParams
			}
			selectedSeriesIDs = append(selectedSeriesIDs, id)
		}
	}

	// Which series ship their datapoints, and how many when the caller cannot
	// name them. Absent means every series does, which is what every caller
	// predating these parameters expects.
	var datapointSeriesIDs []string
	if len(params) >= 12 && params[11] != nil {
		raw, ok := params[11].([]any)
		if !ok {
			return out, jsonrpc2.ErrInvalidParams
		}
		// Non-nil before the loop: an empty array names no series and ships no
		// datapoints, which is a different answer from the parameter being absent.
		datapointSeriesIDs = []string{}
		for _, v := range raw {
			id, ok := v.(string)
			if !ok {
				return out, jsonrpc2.ErrInvalidParams
			}
			datapointSeriesIDs = append(datapointSeriesIDs, id)
		}
	}

	var datapointSeriesLimit int64
	if len(params) >= 13 && params[12] != nil {
		datapointSeriesLimit, err = h.parseTimestampParam(params[12], "datapointSeriesLimit")
		if err != nil {
			return out, err
		}
		if datapointSeriesLimit < 0 {
			return out, jsonrpc2.ErrInvalidParams
		}
	}

	// The IANA zone the viewer is displaying in. tzOffsetNs above is a single
	// offset captured at one instant, so it is wrong on the other side of a DST
	// transition; a zone name lets the query resolve the offset per datapoint.
	// A caller that sends no zone keeps the single-offset behaviour.
	var tzName string
	if len(params) >= 14 && params[13] != nil {
		name, ok := params[13].(string)
		if !ok {
			return out, jsonrpc2.ErrInvalidParams
		}
		tzName = name
	}

	var fitToData bool
	if len(params) >= 8 && params[7] != nil {
		b, ok := params[7].(bool)
		if !ok {
			return out, jsonrpc2.ErrInvalidParams
		}
		fitToData = b
	}
	out.streamID = streamID
	out.startTime = startTime
	out.endTime = endTime
	out.targetBuckets = targetBuckets
	out.seriesIDs = seriesIDs
	out.quantiles = quantiles
	out.tzOffsetNs = tzOffsetNs
	out.fitToData = fitToData
	out.viewBuckets = viewBuckets
	out.sparklineBuckets = sparklineBuckets
	out.selectedSeriesIDs = selectedSeriesIDs
	out.datapointSeriesIDs = datapointSeriesIDs
	out.datapointSeriesLimit = datapointSeriesLimit
	out.tzName = tzName
	return out, nil
}

func (h *JSONRPCHandler) getMetric(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	args, err := h.parseGetMetricParams(req)
	if err != nil {
		return nil, err
	}
	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, args.streamID, args.startTime, args.endTime,
			args.targetBuckets, args.seriesIDs, args.quantiles, args.tzOffsetNs, args.fitToData, args.viewBuckets, args.sparklineBuckets, args.selectedSeriesIDs, args.tzName,
			args.datapointSeriesIDs, args.datapointSeriesLimit)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) clearMetrics(ctx context.Context) (any, error) {
	// Clear deletes the signal's own rows but never the dictionary: attribute,
	// resource and scope rows are shared across signals, so only a sweep can
	// prove one is unreferenced. It has to run here rather than being left to
	// retention -- retention is size-driven and does not run at all when the
	// cap is disabled, which would leak every orphaned row until restart.
	err := h.store.WithDBWrite(func(db *sql.DB) error {
		if err := metrics.Clear(ctx, db); err != nil {
			return err
		}
		return ingest.SweepOrphans(ctx, db, h.store.FlushedIDs())
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return "Metrics cleared successfully", nil
}

// deleteMetricStream deletes one metric stream and everything hanging off it.
// It takes a single ID rather than an array like deleteLogByID and
// deleteSpansByTraceID: metrics identify a stream by one UUID everywhere else
// in this handler (see getMetric), and the delete cascade in the store is keyed
// on a single stream_id.
func (h *JSONRPCHandler) deleteMetricStream(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	streamID, err := h.parseIDParam(params[0], ErrInvalidStreamID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := h.store.WithDBWrite(func(db *sql.DB) error {
		return metrics.DeleteMetricStream(ctx, db, streamID)
	}); err != nil {
		return nil, h.handleStoreError(err)
	}

	return "Metric stream deleted successfully", nil
}

// deleteSpansByTraceID deletes all spans for one or more traces.
func (h *JSONRPCHandler) deleteSpansByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceIDs, err := h.parseIDParams(req, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := h.store.WithDBWrite(func(db *sql.DB) error {
		return spans.DeleteSpansByTraceIDs(ctx, db, traceIDs)
	}); err != nil {
		return nil, h.handleStoreError(err)
	}

	return map[string]any{
		"message": "Spans deleted successfully",
		"count":   len(traceIDs),
	}, nil
}

// deleteLogByID deletes one or more specific logs by their IDs.
func (h *JSONRPCHandler) deleteLogByID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	logIDs, err := h.parseIDParams(req, ErrInvalidLogID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := h.store.WithDBWrite(func(db *sql.DB) error {
		return logs.DeleteLogsByIDs(ctx, db, logIDs)
	}); err != nil {
		return nil, h.handleStoreError(err)
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
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}
	term, ok := params[0].(string)
	if !ok {
		return nil, jsonrpc2.ErrInvalidParams
	}

	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return attributes.Search(ctx, db, term)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return result, nil
}

// getSpanNames returns distinct span names matching a term, most frequent
// first, for value completion in the traces search box. The limit is clamped
// rather than trusted: the caller wants a dropdown, not a dump.
func (h *JSONRPCHandler) getSpanNames(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) != 2 {
		return nil, jsonrpc2.ErrInvalidParams
	}
	term, ok := params[0].(string)
	if !ok {
		return nil, jsonrpc2.ErrInvalidParams
	}
	// Same decoder targetBuckets uses: accepts a JSON number or a string.
	limit, err := h.parseTimestampParam(params[1], "limit")
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetSpanNames(ctx, db, term, limit)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) getTraceAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 2 {
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

	attributes, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceAttributes(ctx, db, startTime, endTime)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}

	return attributes, nil
}

func (h *JSONRPCHandler) getLogAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 2 {
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

	attributes, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetLogAttributes(ctx, db, startTime, endTime)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}

	return attributes, nil
}

// getMetricAggregate takes the same parameters as getMetric and returns only
// the cross-series aggregate. Separate method rather than a flag on getMetric:
// the two are fetched on different triggers -- metric selection versus legend
// selection -- so they are different requests, not one request in two modes.
func (h *JSONRPCHandler) getMetricAggregate(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	args, err := h.parseGetMetricParams(req)
	if err != nil {
		return nil, err
	}
	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		// sparklineBuckets is parsed but not forwarded: this method returns the
		// aggregate envelopes alone, and GetMetricAggregate pins the parameter to
		// 0 for that reason. Accepting it keeps one parser for both methods.
		return metrics.GetMetricAggregate(ctx, db, args.streamID, args.startTime, args.endTime,
			args.targetBuckets, args.seriesIDs, args.quantiles, args.tzOffsetNs, args.fitToData,
			args.viewBuckets, args.selectedSeriesIDs, args.tzName)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return result, nil
}

func (h *JSONRPCHandler) getMetricAttributes(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 2 {
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

	attributes, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAttributes(ctx, db, startTime, endTime)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}

	return attributes, nil
}

func (h *JSONRPCHandler) getAttributesByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}

	traceID, err := h.parseIDParam(params[0], ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	attributes, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetAttributesByTraceID(ctx, db, traceID)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}

	return attributes, nil
}

func (h *JSONRPCHandler) getTraceSpanCount(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	var params []any
	if err := decodeParams(req.Params, &params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}
	if len(params) != 1 {
		return nil, jsonrpc2.ErrInvalidParams
	}
	traceID, err := h.parseIDParam(params[0], ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	count, err := storeRead(h.store, func(db *sql.DB) (int64, error) {
		return stats.GetTraceSpanCount(ctx, db, traceID)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
	}
	return count, nil
}

func (h *JSONRPCHandler) getStats(ctx context.Context) (any, error) {
	retentionCap := h.store.RetentionCap()

	result, err := storeRead(h.store, func(db *sql.DB) (json.RawMessage, error) {
		// SizeBytesWithDB, not SizeBytes: we already hold the read lock.
		sizeBytes, err := h.store.SizeBytesWithDB(ctx, db)
		if err != nil {
			return nil, err
		}
		return stats.GetStats(ctx, db, sizeBytes, retentionCap)
	})
	if err != nil {
		return nil, h.handleStoreError(err)
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
	if err := decodeParams(req.Params, &params); err != nil {
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
// paths: searchSpans, getLog, getMetric, getAttributesByTraceID,
// getTraceSpanCount). Like parseIDParams, a bad value returns the
// signal-specific -3200x code instead of reaching SQL as a cast error.
func (h *JSONRPCHandler) parseIDParam(param any, invalidIDErr error, normalize func(string) (string, error)) (string, error) {
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

// parseTimestampParam parses a timestamp parameter that must be a JSON string
// containing a base-10 int64. Large integers travel as strings to avoid
// float64 precision loss in JSON.
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
func (h *JSONRPCHandler) parseTimestampParam(param any, paramName string) (int64, error) {
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
