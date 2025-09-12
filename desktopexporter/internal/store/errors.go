package store

import (
	"errors"
)

// Common error messages
var (
	// Not found errors
	ErrLogIDNotFound    = errors.New("log ID not found")
	ErrTraceIDNotFound  = errors.New("trace ID not found")
	ErrSpanIDNotFound   = errors.New("span ID not found")
	ErrMetricIDNotFound = errors.New("metric ID not found")

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
	ErrAddLogs    = "failed to add logs: %w"
	ErrAddSpans   = "failed to add spans: %w"
	ErrAddMetrics = "failed to add metrics: %w"

	// Appender errors
	ErrCreateAppender = "failed to create appender: %w"
	ErrAppendRow      = "failed to append row: %w"
	ErrFlushAppender  = "failed to flush appender: %w"

	// Query errors
	ErrGetTrace           = "failed to get trace %s: %w"
	ErrGetTraceSummaries  = "failed to get trace summaries: %w"
	ErrGetLog             = "failed to get log %s: %w"
	ErrGetLogs            = "failed to get logs: %w"
	ErrGetLogsByTraceSpan = "failed to get logs for trace %s span %s: %w"
	ErrGetLogsByTrace     = "failed to get logs for trace %s: %w"
	ErrGetMetric          = "failed to get metric %s: %w"
	ErrGetMetrics         = "failed to get metrics: %w"

	// Deletion errors
	ErrClearTraces  = "failed to clear traces: %w"
	ErrClearLogs    = "failed to clear logs: %w"
	ErrClearMetrics = "failed to clear metrics: %w"

	// Targeted deletion errors
	ErrDeleteSpansByTraceID = "failed to delete spans by trace ID: %w"
	ErrDeleteSpanByID       = "failed to delete span by ID: %w"
	ErrDeleteLogByID        = "failed to delete log by ID: %w"
	ErrDeleteMetricByID     = "failed to delete metric by ID: %w"

	// Scan errors
	ErrScanLogRow    = "failed to scan log row: %w"
	ErrScanTraceRow  = "failed to scan trace row: %w"
	ErrScanMetricRow = "failed to scan metric row: %w"

	// Metric errors
	ErrUnknownMetricType  = "unknown metric type: %s"
	ErrMetricTypeMismatch = "expected %s but got %T, skipping data point"
)
