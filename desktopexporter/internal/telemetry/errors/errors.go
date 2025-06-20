package errors

import (
	"errors"
)

// Common error messages
var (
	ErrInvalidServiceName = errors.New("Resource.Attributes['service.name'] must be a string value that helps to distinguish a group of services")
	WarnMissingRootSpan   = errors.New("warning: trace is incomplete - no root span found")
)

// Warning messages for logging
const (
	ErrUnknownMetricType         = "unknown metric type: %s"
	WarnSummaryMetricsDeprecated = "summary metrics are deprecated in OpenTelemetry and not supported"
)
