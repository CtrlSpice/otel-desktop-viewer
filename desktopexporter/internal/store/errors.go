package store

import (
	"errors"
)

// Common error messages
var (
	// Not found errors
	ErrLogIDNotFound   = errors.New("log ID not found")
	ErrTraceIDNotFound = errors.New("trace ID not found")

	// Connection errors
	ErrStoreConnectionClosed = errors.New("store connection is closed")
)

// Error format strings for wrapping errors with context
const (
	// Initialization errors (fail-fast with log.Fatal, thus no error wrapping)
	ErrInitConnector     = "failed to initialize connector: %v"
	ErrInitConnection    = "failed to connect to database: %v"
	ErrInitAttributeType = "failed to create attribute type: %v"
	ErrInitEventType     = "failed to create event type: %v"
	ErrInitLinkType      = "failed to create link type: %v"
	ErrInitBodyType      = "failed to create body type: %v"
	ErrInitSpansTable    = "failed to create spans table: %v"
	ErrInitLogsTable     = "failed to create logs table: %v"

	// Addition errors
	ErrAddLogs = "failed to add logs: %w"
	ErrAddSpans = "failed to add spans: %w"

	// Appender errors
	ErrCreateAppender = "failed to create appender: %w"
	ErrAppendRow      = "failed to append row: %w"
	ErrFlushAppender  = "failed to flush appender: %w"

	// Query errors
	ErrGetTrace          = "failed to get trace %s: %w"
	ErrGetTraceSummaries = "failed to get trace summaries: %w"
	ErrGetLog            = "failed to get log %s: %w"
	ErrGetLogs           = "failed to get logs: %w"
	ErrGetLogsByTraceSpan = "failed to get logs for trace %s span %s: %w"
	ErrGetLogsByTrace    = "failed to get logs for trace %s: %w"

	// Deletion errors
	ErrClearTraces = "failed to clear traces: %w"
	ErrClearLogs   = "failed to clear logs: %w"

	// Scan errors
	ErrScanLogRow = "failed to scan log row: %w"
)

// Warning messages for logging
const (
	WarnUnsupportedAttributeType = "unsupported attribute type was converted to string: name=%s type=%T value=%v"
	WarnUnsupportedListAttribute = "unsupported list attribute was converted to []string: name=%s error=%v"
	WarnUint64Overflow          = "uint64 attribute exceeds int64 range and was converted to string: name=%s value=%d"
	WarnUint64SliceOverflow     = "[]uint64 attribute contains values exceeding int64 range and was converted to []string: name=%s"
	WarnJSONMarshal             = "failed to marshal to JSON, body was converted to string: type=%T value=%v"
	WarnJSONUnmarshal           = "failed to unmarshal JSON body: %v"
)

// Attribute type validation error messages
const (
	errMixedTypesPrefix      = "list attribute contains mixed types: "
	errNilValue              = errMixedTypesPrefix + "list contains nil value"
	errIncompatibleType      = errMixedTypesPrefix + "incompatible type %T"
	errIncompatibleIntType   = errMixedTypesPrefix + "incompatible type %T in integer list"
	errIncompatibleFloatType = errMixedTypesPrefix + "incompatible type %T in float list"
	errUint64Overflow        = "uint64 value %d exceeds int64 range"
	errNilFirstElement       = "nil value in list attribute"
	errUnsupportedListType   = "unsupported list attribute type: %T"
	errJSONValueType 		 = "expected string for JSON value, got %T"
)
