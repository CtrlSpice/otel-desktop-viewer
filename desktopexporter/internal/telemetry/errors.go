package telemetry

import (
	"errors"
)

var ErrEmptySpansSlice = errors.New("slice of spans associated with this traceID must not be empty")
var ErrTraceIDNotFound = errors.New("traceID not found")
var ErrTraceIDMismatch = errors.New("traceID mismatch between TraceStore.traceMap and TraceStore.traceQueue")

var WarningMissingRootSpan = errors.New("warning: trace is incomplete - no root span found")
var WarningInvalidServiceName = errors.New("warning: Resource.Attributes['service.name'] must be a string value that helps to distinguish a group of services")
