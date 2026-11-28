package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"golang.org/x/exp/jsonrpc2"
)

func (h *Handler) searchMetricSummaries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	params, err := parseSearchParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.SearchSummariesWithOptions(ctx, db, params.timeRange, params.query, params.options)
	})
}

func (h *Handler) getMetric(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	metricRef, err := parseSingleIDParam(req.Params, ErrInvalidMetricRef, normalizeUUID)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetric(ctx, db, metricRef)
	})
}

func (h *Handler) getMetricSeries(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	args, err := parseGetMetricSeriesParams(req.Params)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricSeries(ctx, db, args.metricRef, args.seriesRef, args.timeRange, args.includeDatapoints)
	})
}

func (h *Handler) getMetricView(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	args, err := parseGetMetricViewParams(req.Params, false)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(ctx, db, args.metricRef, args.timeRange,
			args.targetBuckets, args.seriesRefs, args.quantiles, args.tzOffsetNs, args.viewBuckets, args.sparklineBuckets, args.selectedSeriesRefs, args.tzName,
			args.datapointSeriesRefs, args.datapointSeriesLimit)
	})
}

func (h *Handler) clearMetrics(ctx context.Context) (any, error) {
	return h.clearSignal(ctx, metrics.Clear, "Metrics cleared successfully")
}

// deleteMetric deletes one Metric and its dependent rows.
// It takes one Metric reference because the store cascade is keyed by one
// metric_id.
func (h *Handler) deleteMetric(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	metricRef, err := parseSingleIDParam(req.Params, ErrInvalidMetricRef, normalizeUUID)
	if err != nil {
		return nil, err
	}

	if err := h.store.WithDBWrite(func(db *sql.DB) error {
		return metrics.DeleteMetric(ctx, db, metricRef)
	}); err != nil {
		return nil, h.handleStoreError(ctx, err)
	}

	return "Metric deleted successfully", nil
}

// getMetricAggregateView takes the common metric chart parameters, viewBuckets,
// selectedSeriesRefs and tzName, and returns only the cross-series aggregate.
// Separate method rather than a flag on getMetricView:
// the two are fetched on different triggers -- metric selection versus legend
// selection -- so they are different requests, not one request in two modes.
func (h *Handler) getMetricAggregateView(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	args, err := parseGetMetricViewParams(req.Params, true)
	if err != nil {
		return nil, err
	}
	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAggregateView(ctx, db, args.metricRef, args.timeRange,
			args.targetBuckets, args.seriesRefs, args.quantiles, args.tzOffsetNs,
			args.viewBuckets, args.selectedSeriesRefs, args.tzName, args.datapointRef)
	})
}

func (h *Handler) getMetricAttributeDefinitions(ctx context.Context, req *jsonrpc2.Request) (any, error) {
	if err := validateNoParams(req.Params); err != nil {
		return nil, jsonrpc2.ErrInvalidParams
	}

	return handlerRead(ctx, h, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricAttributeDefinitions(ctx, db)
	})
}

// getMetricViewParams holds the common chart request plus detail-only fields.
type getMetricViewParams struct {
	metricRef            string
	timeRange            store.TimeRange
	targetBuckets        int64
	seriesRefs           []string
	quantiles            []float64
	tzOffsetNs           int64
	viewBuckets          int64
	sparklineBuckets     int64
	selectedSeriesRefs   []string
	datapointSeriesRefs  []string
	datapointSeriesLimit int64
	datapointRef         string
	tzName               string
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

func parseGetMetricViewParams(raw json.RawMessage, aggregateOnly bool) (getMetricViewParams, error) {
	maxParams := 13
	if aggregateOnly {
		maxParams = 11
	}
	params, err := decodePositionalParams(raw, 3, maxParams)
	if err != nil {
		return getMetricViewParams{}, err
	}
	metricRef, err := parseIDParam(params[0], ErrInvalidMetricRef, normalizeUUID)
	if err != nil {
		return getMetricViewParams{}, err
	}
	timeRange, err := parseTimeRange(params[1], params[2])
	if err != nil {
		return getMetricViewParams{}, err
	}
	targetBuckets, err := parseOptionalNonNegativeInt(params, 3, "targetBuckets")
	if err != nil {
		return getMetricViewParams{}, err
	}
	seriesRefs, err := parseOptionalStringList(params, 4)
	if err != nil {
		return getMetricViewParams{}, err
	}
	quantiles, err := parseOptionalQuantiles(params, 5)
	if err != nil {
		return getMetricViewParams{}, err
	}
	tzOffsetNs, err := parseOptionalInt(params, 6, "tzOffsetNs")
	if err != nil {
		return getMetricViewParams{}, err
	}
	viewBuckets, err := parseOptionalNonNegativeInt(params, 7, "viewBuckets")
	if err != nil {
		return getMetricViewParams{}, err
	}

	selectedSeriesIndex, tzNameIndex := 9, 10
	var sparklineBuckets int64
	if aggregateOnly {
		selectedSeriesIndex, tzNameIndex = 8, 9
	} else {
		sparklineBuckets, err = parseOptionalNonNegativeInt(params, 8, "sparklineBuckets")
		if err != nil {
			return getMetricViewParams{}, err
		}
	}
	selectedSeriesRefs, err := parseOptionalStringList(params, selectedSeriesIndex)
	if err != nil {
		return getMetricViewParams{}, err
	}
	tzName, err := parseOptionalString(params, tzNameIndex)
	if err != nil {
		return getMetricViewParams{}, err
	}

	var datapointSeriesRefs []string
	var datapointSeriesLimit int64
	if !aggregateOnly {
		datapointSeriesRefs, err = parseOptionalStringList(params, 11)
		if err != nil {
			return getMetricViewParams{}, err
		}
		datapointSeriesLimit, err = parseOptionalNonNegativeInt(params, 12, "datapointSeriesLimit")
		if err != nil {
			return getMetricViewParams{}, err
		}
	}

	var datapointRef string
	if aggregateOnly && len(params) > 10 && params[10] != nil {
		datapointRef, err = parseIDParam(params[10], jsonrpc2.ErrInvalidParams, normalizeUUID)
		if err != nil {
			return getMetricViewParams{}, err
		}
	}
	return getMetricViewParams{
		metricRef: metricRef, timeRange: timeRange, targetBuckets: targetBuckets,
		seriesRefs: seriesRefs, quantiles: quantiles, tzOffsetNs: tzOffsetNs,
		viewBuckets: viewBuckets, sparklineBuckets: sparklineBuckets,
		selectedSeriesRefs: selectedSeriesRefs, datapointSeriesRefs: datapointSeriesRefs,
		datapointSeriesLimit: datapointSeriesLimit, tzName: tzName, datapointRef: datapointRef,
	}, nil
}

type getMetricSeriesParams struct {
	metricRef         string
	seriesRef         string
	timeRange         store.TimeRange
	includeDatapoints bool
}

type metricSeriesOrderParams struct {
	metricRef  string
	seriesRefs []string
	key        string
	timeRange  store.TimeRange
	descending bool
}

func parseMetricSeriesOrderParams(raw json.RawMessage) (metricSeriesOrderParams, error) {
	var result metricSeriesOrderParams
	params, err := decodePositionalParams(raw, 6, 6)
	if err != nil {
		return result, err
	}
	result.metricRef, err = parseIDParam(params[0], ErrInvalidMetricRef, normalizeUUID)
	if err != nil {
		return result, err
	}
	result.timeRange, err = parseTimeRange(params[1], params[2])
	if err != nil {
		return result, err
	}
	if params[3] == nil {
		return result, jsonrpc2.ErrInvalidParams
	}
	result.seriesRefs, err = parseOptionalStringList(params, 3)
	if err != nil {
		return result, err
	}
	for i, ref := range result.seriesRefs {
		result.seriesRefs[i], err = normalizeUUID(ref)
		if err != nil {
			return result, ErrInvalidMetricRef
		}
	}
	key, ok := params[4].(string)
	if !ok {
		return result, jsonrpc2.ErrInvalidParams
	}
	result.key = key
	direction, ok := params[5].(string)
	if !ok || (direction != "asc" && direction != "desc") {
		return result, jsonrpc2.ErrInvalidParams
	}
	result.descending = direction == "desc"
	return result, nil
}

func parseGetMetricSeriesParams(raw json.RawMessage) (getMetricSeriesParams, error) {
	params, err := decodePositionalParams(raw, 4, 5)
	if err != nil {
		return getMetricSeriesParams{}, err
	}
	metricRef, err := parseIDParam(params[0], ErrInvalidMetricRef, normalizeUUID)
	if err != nil {
		return getMetricSeriesParams{}, err
	}
	seriesRef, err := parseIDParam(params[1], ErrInvalidMetricRef, normalizeUUID)
	if err != nil {
		return getMetricSeriesParams{}, err
	}
	timeRange, err := parseTimeRange(params[2], params[3])
	if err != nil {
		return getMetricSeriesParams{}, err
	}
	includeDatapoints := true
	if len(params) > 4 && params[4] != nil {
		var ok bool
		includeDatapoints, ok = params[4].(bool)
		if !ok {
			return getMetricSeriesParams{}, jsonrpc2.ErrInvalidParams
		}
	}
	return getMetricSeriesParams{metricRef: metricRef, seriesRef: seriesRef, timeRange: timeRange, includeDatapoints: includeDatapoints}, nil
}
