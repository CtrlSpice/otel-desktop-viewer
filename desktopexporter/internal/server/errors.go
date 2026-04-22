package server

import (
	"errors"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"golang.org/x/exp/jsonrpc2"
)

// Custom JSON-RPC error codes
const (
	ErrCodeTraceNotFound                = -32001
	ErrCodeLogNotFound                  = -32002
	ErrCodeMetricNotFound               = -32003
	ErrCodeInvalidTraceID               = -32004
	ErrCodeInvalidLogID                 = -32005
	ErrCodeSpanNotFound                 = -32006
	ErrCodeInvalidQuery                 = -32007
	ErrCodeDatapointNotFound            = -32008
	ErrCodeQuantilesNotSupportedForType = -32009
)

// Custom JSON-RPC errors
var (
	ErrTraceNotFound                = jsonrpc2.NewError(ErrCodeTraceNotFound, "Trace not found")
	ErrLogsNotFound                 = jsonrpc2.NewError(ErrCodeLogNotFound, "Log not found")
	ErrMetricNotFound               = jsonrpc2.NewError(ErrCodeMetricNotFound, "Metric not found")
	ErrInvalidTraceID               = jsonrpc2.NewError(ErrCodeInvalidTraceID, "Invalid trace ID")
	ErrInvalidLogID                 = jsonrpc2.NewError(ErrCodeInvalidLogID, "Invalid log ID")
	ErrSpanNotFound                 = jsonrpc2.NewError(ErrCodeSpanNotFound, "Span not found")
	ErrInvalidQuery                 = jsonrpc2.NewError(ErrCodeInvalidQuery, "Invalid query")
	ErrDatapointNotFound            = jsonrpc2.NewError(ErrCodeDatapointNotFound, "Datapoint not found")
	ErrQuantilesNotSupportedForType = jsonrpc2.NewError(ErrCodeQuantilesNotSupportedForType, "Quantiles are only supported for Histogram and ExponentialHistogram datapoints")
)

// mapStoreError maps store-layer sentinel errors to JSON-RPC errors.
// Returns jsonrpc2.ErrInternal for unknown or internal errors.
func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, spans.ErrTraceIDNotFound):
		return ErrTraceNotFound
	case errors.Is(err, spans.ErrSpanIDNotFound):
		return ErrSpanNotFound
	case errors.Is(err, logs.ErrLogIDNotFound):
		return ErrLogsNotFound
	case errors.Is(err, metrics.ErrMetricIDNotFound):
		return ErrMetricNotFound
	case errors.Is(err, metrics.ErrDatapointIDNotFound):
		return ErrDatapointNotFound
	case errors.Is(err, metrics.ErrQuantilesNotSupportedForType):
		return ErrQuantilesNotSupportedForType
	case errors.Is(err, spans.ErrInvalidTraceQuery), errors.Is(err, logs.ErrInvalidLogQuery),
		errors.Is(err, metrics.ErrInvalidMetricQuery), errors.Is(err, search.ErrInvalidQuery):
		return ErrInvalidQuery
	case errors.Is(err, store.ErrStoreConnectionClosed):
		return jsonrpc2.ErrInternal
	default:
		return jsonrpc2.ErrInternal
	}
}
