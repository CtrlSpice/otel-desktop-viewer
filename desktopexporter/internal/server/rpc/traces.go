package rpc

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/stats"
	"golang.org/x/exp/jsonrpc2"
)

func (h *Handler) searchTraceSummaries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseSearchParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchTraceSummariesWithOptions(ctx, db, params.timeRange, params.query, params.options)
	})
}

func (h *Handler) getTraceView(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseTraceViewParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceView(ctx, db, params.traceID, params.query, params.spanIDs...)
	})
}

type traceViewParams struct {
	traceID string
	query   any
	spanIDs []uint64
	columns []spans.WaterfallColumn
}

func parseTraceViewParams(raw json.RawMessage) (*traceViewParams, error) {
	params, err := decodePositionalParams(raw, 1, 4)
	if err != nil {
		return nil, err
	}
	traceID, err := parseIDParam(params[0], ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	var query any
	if len(params) >= 2 {
		query = params[1]
	}
	var spanIDs []uint64
	if len(params) >= 3 && params[2] != nil {
		values, ok := params[2].([]any)
		if !ok || len(values) == 0 || len(values) > 256 {
			return nil, fmt.Errorf("spanIDs requires 1 to 256 span IDs: %w", jsonrpc2.ErrInvalidParams)
		}
		for _, value := range values {
			_, id, err := parseSpanIDParam(value)
			if err != nil {
				return nil, err
			}
			spanIDs = append(spanIDs, id)
		}
	}

	var columns []spans.WaterfallColumn
	if len(params) >= 4 && params[3] != nil {
		encoded, err := json.Marshal(params[3])
		if err != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
		if err := json.Unmarshal(encoded, &columns); err != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
		if err := spans.ValidateWaterfallColumns(columns); err != nil {
			return nil, jsonrpc2.ErrInvalidParams
		}
	}
	return &traceViewParams{traceID: traceID, query: query, spanIDs: spanIDs, columns: columns}, nil
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

func (h *Handler) getTraceOverview(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceID, err := parseSingleIDParam(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerSnapshotRead(ctx, h, func(db *sql.Tx) (compactTraceResult, error) {
		traceRaw, err := spans.GetTraceOverview(ctx, db, traceID)
		if err != nil {
			return compactTraceResult{}, err
		}
		var result compactTraceResult
		if err := json.Unmarshal(traceRaw, &result); err != nil {
			return compactTraceResult{}, err
		}
		logsRaw, err := logs.GetTraceLogSummaries(ctx, db, traceID)
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

func (h *Handler) getSpan(ctx context.Context, req *jsonrpc2.Request) (any, error) {
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
		if err != nil || parsed < 1 || parsed == math.MaxInt64 {
			return nil, jsonrpc2.ErrInvalidParams
		}
		limit = parsed
	}

	return handlerSnapshotRead(ctx, h, func(db *sql.Tx) (any, error) {
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

func (h *Handler) clearTraces(ctx context.Context) (any, error) {
	return h.clearSignal(ctx, spans.Clear, "Traces cleared successfully")
}

// deleteSpansByTraceID deletes all spans for one or more traces.
func (h *Handler) deleteSpansByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
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

func (h *Handler) getTraceAttributeDefinitions(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := validateNoParams(req.Params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceAttributeDefinitions(ctx, db)
	})
}

func (h *Handler) getTraceAttributeDefinitionsByTraceID(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceID, err := parseSingleIDParam(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceAttributeDefinitionsByTraceID(ctx, db, traceID)
	})
}

func (h *Handler) getTraceSpanCount(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	traceID, err := parseSingleIDParam(req.Params, ErrInvalidTraceID, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (int64, error) {
		return stats.GetTraceSpanCount(ctx, db, traceID)
	})
}
