package errors

import (
	"errors"
)

// Trace error messages
var (
	ErrInvalidServiceName = errors.New("Resource.Attributes['service.name'] must be a string value that helps to distinguish a group of services")
	WarnMissingRootSpan   = errors.New("warning: trace is incomplete - no root span found")
)

// Metric error messages
const (
	ErrUnknownMetricType         = "unknown metric type: %s"
	WarnSummaryMetricsDeprecated = "summary metrics are deprecated in OpenTelemetry and not supported"
)

// Attribute error messages
const (
	WarnUnsupportedAttributeType = "unsupported attribute type was converted to string: name=%s type=%T value=%v"
	WarnUnsupportedListAttribute = "unsupported list attribute was converted to []string: name=%s error=%v"
	WarnUint64Overflow           = "uint64 attribute exceeds int64 range and was converted to string: name=%s value=%d"
	WarnUint64SliceOverflow      = "[]uint64 attribute contains values exceeding int64 range and was converted to []string: name=%s"

	ErrMixedTypesPrefix      = "list attribute contains mixed types: "
	ErrNilValue              = ErrMixedTypesPrefix + "list contains nil value"
	ErrIncompatibleType      = ErrMixedTypesPrefix + "incompatible type %T"
	ErrIncompatibleIntType   = ErrMixedTypesPrefix + "incompatible type %T in integer list"
	ErrIncompatibleFloatType = ErrMixedTypesPrefix + "incompatible type %T in float list"

	ErrUint64Overflow      = "uint64 value %d exceeds int64 range"
	ErrNilFirstElement     = "nil value in list attribute"
	ErrUnsupportedListType = "unsupported list attribute type: %T"
)

// JSON error messages
var (
	WarnJSONMarshal   = "failed to marshal to JSON, body was converted to string: type=%T value=%v"
	WarnJSONUnmarshal = "failed to unmarshal JSON body: %v"
	ErrJSONValueType  = "expected string for JSON value, got %T"
)
