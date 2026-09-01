package server

import (
	"context"
	"errors"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"golang.org/x/exp/jsonrpc2"
)

// Custom JSON-RPC error codes.
//
// Not-found convention: a request for a specific entity that does not exist
// returns the matching *NotFound error below -- never a null result. The
// frontend service layer decides how each surfaces in the UI.
const (
	ErrCodeTraceNotFound   = -32001
	ErrCodeLogNotFound     = -32002
	ErrCodeMetricNotFound  = -32003
	ErrCodeInvalidTraceID  = -32004
	ErrCodeInvalidLogID    = -32005
	ErrCodeInvalidQuery    = -32007
	ErrCodeInvalidStreamID = -32009
	ErrCodeRequestCanceled = -32010
)

// Custom JSON-RPC errors
var (
	ErrTraceNotFound   = jsonrpc2.NewError(ErrCodeTraceNotFound, "Trace not found")
	ErrLogsNotFound    = jsonrpc2.NewError(ErrCodeLogNotFound, "Log not found")
	ErrMetricNotFound  = jsonrpc2.NewError(ErrCodeMetricNotFound, "Metric not found")
	ErrInvalidTraceID  = jsonrpc2.NewError(ErrCodeInvalidTraceID, "Invalid trace ID")
	ErrInvalidLogID    = jsonrpc2.NewError(ErrCodeInvalidLogID, "Invalid log ID")
	ErrInvalidQuery    = jsonrpc2.NewError(ErrCodeInvalidQuery, "Invalid query")
	ErrInvalidStreamID = jsonrpc2.NewError(ErrCodeInvalidStreamID, "Invalid metric stream ID")

	// ErrRequestCanceled covers a query abandoned by the caller -- the UI
	// navigating away mid-poll, or a browser tab closing. DuckDB surfaces the
	// interrupt as "INTERRUPT Error: Interrupted!" wrapped around
	// context.Canceled. Nobody is left to receive this, so it exists to keep
	// cancellation out of the internal-error bucket rather than to be shown.
	ErrRequestCanceled = jsonrpc2.NewError(ErrCodeRequestCanceled, "Request canceled")
)

// mapStoreError maps store-layer sentinel errors to JSON-RPC errors.
// Returns jsonrpc2.ErrInternal for unknown or internal errors.
func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	// Checked first: a cancelled query can fail anywhere, so the wrapped
	// error may also match a store sentinel on its way out.
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ErrRequestCanceled
	case errors.Is(err, spans.ErrTraceIDNotFound):
		return ErrTraceNotFound
	case errors.Is(err, logs.ErrLogIDNotFound):
		return ErrLogsNotFound
	case errors.Is(err, metrics.ErrStreamIDNotFound):
		return ErrMetricNotFound
	case errors.Is(err, spans.ErrInvalidTraceQuery), errors.Is(err, logs.ErrInvalidLogQuery),
		errors.Is(err, metrics.ErrInvalidMetricQuery), errors.Is(err, search.ErrInvalidQuery):
		return ErrInvalidQuery
	case errors.Is(err, spans.ErrInvalidTraceLimit), errors.Is(err, logs.ErrInvalidLogLimit),
		errors.Is(err, metrics.ErrInvalidMetricLimit), errors.Is(err, search.ErrInvalidSort):
		return jsonrpc2.ErrInvalidParams
	case errors.Is(err, store.ErrStoreConnectionClosed):
		return jsonrpc2.ErrInternal
	default:
		return jsonrpc2.ErrInternal
	}
}
