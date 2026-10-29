package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowread"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

const arrowStreamContentType = "application/vnd.apache.arrow.stream"

// Dataset requests contain only parameters. The existing typed parameter
// parsers and store search compiler are shared with RPC; no submitted SQL is
// executed by this endpoint.
func (s *Server) arrowHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	params, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRPCRequestBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		writeArrowError(w, jsonrpc2.ErrParse)
		return
	}
	if !json.Valid(params) {
		writeArrowError(w, jsonrpc2.ErrParse)
		return
	}
	result, err := s.jsonrpcHandler.readArrow(r.Context(), r.PathValue("method"), params)
	if err != nil {
		writeArrowError(w, err)
		return
	}
	defer result.Close()
	w.Header().Set("Content-Type", arrowStreamContentType)
	w.Header().Set("X-Arrow-Rows", strconv.FormatInt(result.NumRows(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := result.Write(r.Context(), w); err != nil {
		if r.Context().Err() == nil {
			s.logger.Error("writing Arrow response", zap.Error(err))
		}
		// net/http aborts the response stream. Sending a normal EOF after a
		// failed write could make a partial dataset appear to have succeeded.
		panic(http.ErrAbortHandler)
	}
}

func writeArrowError(w http.ResponseWriter, err error) {
	mapped, status := jsonrpc2.ErrInternal, http.StatusInternalServerError
	for _, candidate := range []error{jsonrpc2.ErrParse, jsonrpc2.ErrInvalidParams,
		ErrInvalidTraceID, ErrInvalidLogRef, ErrInvalidMetricRef, ErrInvalidQuery, ErrRequestCanceled} {
		if errors.Is(err, candidate) {
			mapped, status = candidate, http.StatusBadRequest
			break
		}
	}
	for _, candidate := range []error{jsonrpc2.ErrMethodNotFound, ErrTraceNotFound, ErrLogsNotFound, ErrMetricNotFound, ErrHomeValueUnavailable} {
		if errors.Is(err, candidate) {
			mapped, status = candidate, http.StatusNotFound
			break
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(mapped)
}

func (h *JSONRPCHandler) readArrow(ctx context.Context, method string, raw json.RawMessage) (*arrowread.Result, error) {
	paramMethod := method
	if method == "getSpanLogs" {
		paramMethod = "getSpan"
	}
	params, err := normalizeParams(paramMethod, raw)
	if err != nil {
		return nil, err
	}
	switch method {
	case "searchAttributeMatches":
		options, err := parseAttributeMatchParams(params)
		if err != nil {
			return nil, err
		}
		query, args, err := searchindex.DiscoverArrowSQL(*options)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			return arrowread.Read(ctx, db, query, args...)
		})
	case "getTraceAttributeDefinitions", "getLogAttributeDefinitions", "getMetricAttributeDefinitions", "getTraceAttributeDefinitionsByTraceID":
		traceID := ""
		if method == "getTraceAttributeDefinitionsByTraceID" {
			traceID, err = parseSingleIDParam(params, ErrInvalidTraceID, normalizeUUID)
		} else {
			err = validateNoParams(params)
		}
		if err != nil {
			return nil, err
		}
		signal := "span"
		if method == "getLogAttributeDefinitions" {
			signal = "log"
		} else if method == "getMetricAttributeDefinitions" {
			signal = "metric"
		}
		query, args, err := searchindex.DefinitionsArrowSQL(signal, traceID)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			return arrowread.Read(ctx, db, query, args...)
		})
	case "getFieldValueCompletions":
		p, err := parseFieldValuesParams(params)
		if err != nil {
			return nil, err
		}
		query, args, err := searchindex.FieldValuesSQL(p.signal, p.field, p.term, p.limit)
		if err != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) { return arrowread.Read(ctx, db, query, args...) })
	case "discoverTelemetry":
		if options, home, err := homeDiscoveryOptions(params); err != nil {
			return nil, err
		} else if home {
			return h.homeValueArrow(ctx, options)
		}
		plan, err := discoveryPlanFor(params, true)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			return arrowread.Read(ctx, db, plan.statement, plan.args...)
		})
	case "getMetricView", "getMetricAggregateView":
		aggregateOnly := method == "getMetricAggregateView"
		args, err := parseGetMetricViewParams(params, aggregateOnly)
		if err != nil {
			return nil, err
		}
		if aggregateOnly {
			args.datapointSeriesRefs = []string{}
			args.datapointSeriesLimit = 0
			args.sparklineBuckets = 0
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			return metrics.ViewArrow(ctx, db, args.metricRef, args.timeRange, args.targetBuckets, args.seriesRefs, args.quantiles,
				args.tzOffsetNs, args.viewBuckets, args.sparklineBuckets, args.selectedSeriesRefs, args.tzName,
				args.datapointSeriesRefs, args.datapointSeriesLimit, aggregateOnly, args.datapointRef)
		})
	case "getMetric":
		id, err := parseSingleIDParam(params, ErrInvalidMetricRef, normalizeUUID)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) { return metrics.GetArrow(ctx, db, id) })
	case "getMetricSeriesColumn":
		args, err := parseMetricSeriesColumnParams(params)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			return metrics.SeriesColumnArrow(ctx, db, args.metricRef, args.seriesRefs, args.key)
		})
	case "getMetricSeries":
		args, err := parseGetMetricSeriesParams(params)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			return metrics.SeriesArrow(ctx, db, args.metricRef, args.seriesRef, args.timeRange, args.includeDatapoints)
		})
	case "getTraceView":
		parsed, err := parseTraceViewParams(params)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			// Explicit bounded selections supply Evals' received evidence, not a
			// waterfall. Ordinary trace views never include complete payloads.
			if len(parsed.spanIDs) > 0 {
				return spans.ViewArrow(ctx, db, parsed.traceID, parsed.query, parsed.spanIDs...)
			}
			return spans.WaterfallColumnsArrow(ctx, db, parsed.traceID, parsed.query, parsed.columns)
		})
	case "getSpan":
		values, err := decodePositionalParams(params, 2, 2)
		if err != nil {
			return nil, err
		}
		_, spanID, err := parseSpanIDParam(values[0])
		if err != nil {
			return nil, err
		}
		traceID, err := parseIDParam(values[1], ErrInvalidTraceID, normalizeUUID)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) { return spans.GetArrow(ctx, db, traceID, spanID) })
	case "getTraceColumnPreviews":
		values, err := decodePositionalParams(params, 2, 2)
		if err != nil {
			return nil, err
		}
		traceID, err := parseIDParam(values[0], ErrInvalidTraceID, normalizeUUID)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(values[1])
		if err != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
		var columns []spans.WaterfallColumn
		if err := json.Unmarshal(encoded, &columns); err != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
		if err := spans.ValidateWaterfallColumns(columns); err != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			return spans.ColumnPreviewsArrow(ctx, db, traceID, columns)
		})
	case "searchLogPage":
		parsed, err := parseLogPageParams(params)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) {
			return logs.PageArrow(ctx, db, parsed.timeRange, parsed.query, parsed.options, parsed.page)
		})
	case "getLog":
		id, err := parseSingleIDParam(params, ErrInvalidLogRef, normalizeUUID)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) { return logs.GetArrow(ctx, db, id) })
	case "getTraceLogSummaries":
		id, err := parseSingleIDParam(params, ErrInvalidTraceID, normalizeUUID)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) { return logs.TraceSummariesArrow(ctx, db, id) })
	case "getSpanLogs":
		values, err := decodePositionalParams(params, 2, 2)
		if err != nil {
			return nil, err
		}
		_, spanID, err := parseSpanIDParam(values[0])
		if err != nil {
			return nil, err
		}
		traceID, err := parseIDParam(values[1], ErrInvalidTraceID, normalizeUUID)
		if err != nil {
			return nil, err
		}
		return h.arrowRead(ctx, func(db *sql.DB) (*arrowread.Result, error) { return logs.SpanLogsArrow(ctx, db, traceID, spanID) })
	case "searchTraceSummaries", "searchMetricSummaries", "searchLogSummaries":
		parsed, err := parseSearchParams(params)
		if err != nil {
			return nil, err
		}
		result, err := storeRead(h.store, func(db *sql.DB) (*arrowread.Result, error) {
			switch method {
			case "searchTraceSummaries":
				return spans.SearchArrow(ctx, db, parsed.timeRange, parsed.query, parsed.options)
			case "searchMetricSummaries":
				return metrics.SearchArrow(ctx, db, parsed.timeRange, parsed.query, parsed.options)
			default:
				return logs.SearchArrow(ctx, db, parsed.timeRange, parsed.query, parsed.options)
			}
		})
		return result, h.handleStoreError(ctx, err)
	default:
		return nil, jsonrpc2.ErrMethodNotFound
	}
}

func (h *JSONRPCHandler) arrowRead(ctx context.Context, read func(*sql.DB) (*arrowread.Result, error)) (*arrowread.Result, error) {
	result, err := storeRead(h.store, read)
	return result, h.handleStoreError(ctx, err)
}
