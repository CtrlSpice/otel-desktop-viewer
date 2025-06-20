package logs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"go.opentelemetry.io/collector/pdata/plog"
)

type LogsPayload struct {
	Logs plog.Logs
}

type LogData struct {
	Timestamp         int64 `json:"-"`
	ObservedTimestamp int64 `json:"-"`

	TraceID string `json:"traceID,omitempty"`
	SpanID  string `json:"spanID,omitempty"`

	SeverityText   string `json:"severityText,omitempty"`
	SeverityNumber int32  `json:"severityNumber,omitempty"`

	Body                   any                    `json:"body,omitempty"`
	Resource               *resource.ResourceData `json:"resource"`
	Scope                  *scope.ScopeData       `json:"scope"`
	Attributes             map[string]any         `json:"attributes,omitempty"`
	DroppedAttributesCount uint32                 `json:"droppedAttributeCount,omitempty"`
	Flags                  uint32                 `json:"flags,omitempty"`
	EventName              string                 `json:"eventName,omitempty"`
}

// Logs wraps a slice of LogData for JSON marshaling
type Logs struct {
	Logs []LogData `json:"logs"`
}

func NewLogsPayload(l plog.Logs) *LogsPayload {
	return &LogsPayload{Logs: l}
}

func (payload *LogsPayload) ExtractLogs() []LogData {
	logData := []LogData{}

	for _, resourceLogs := range payload.Logs.ResourceLogs().All() {
		resourceData := resource.AggregateResourceData(resourceLogs.Resource())

		for _, scopeLogs := range resourceLogs.ScopeLogs().All() {
			scopeData := scope.AggregateScopeData(scopeLogs.Scope())

			for _, logRecord := range scopeLogs.LogRecords().All() {
				logData = append(logData, aggregateLogData(logRecord, scopeData, resourceData))
			}
		}
	}
	return logData
}

func aggregateLogData(source plog.LogRecord, scopeData *scope.ScopeData, resourceData *resource.ResourceData) LogData {
	return LogData{
		Timestamp:         source.Timestamp().AsTime().UnixNano(),
		ObservedTimestamp: source.ObservedTimestamp().AsTime().UnixNano(),

		TraceID: source.TraceID().String(),
		SpanID:  source.SpanID().String(),

		SeverityText:   source.SeverityText(),
		SeverityNumber: int32(source.SeverityNumber()),

		Body:                   source.Body().AsRaw(),
		Resource:               resourceData,
		Scope:                  scopeData,
		Attributes:             source.Attributes().AsRaw(),
		DroppedAttributesCount: source.DroppedAttributesCount(),
		Flags:                  uint32(source.Flags()),
		EventName:              source.EventName(),
	}
}

func (logData LogData) ID() string {
	// Use timestamp if available, otherwise fall back to observed timestamp
	var logTime int64
	if logData.Timestamp > 0 {
		logTime = logData.Timestamp
	} else {
		logTime = logData.ObservedTimestamp
	}

	// Get resource name from attributes
	resourceName := ""
	if logData.Resource != nil && logData.Resource.Attributes != nil {
		if name, ok := logData.Resource.Attributes["service.name"].(string); ok {
			resourceName = name
		}
	}

	// Convert body to string
	bodyStr := fmt.Sprintf("%v", logData.Body)

	hash := sha256.New()
	buf := make([]byte, 0, 256)
	buf = fmt.Appendf(buf, "%d|%s|%s|%s",
		logTime,
		resourceName,
		bodyStr,
		logData.EventName,
	)
	hash.Write(buf)
	return hex.EncodeToString(hash.Sum(nil))
}

func (logData LogData) MarshalJSON() ([]byte, error) {
	type Alias LogData // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		Timestamp         string `json:"timestamp"`
		ObservedTimestamp string `json:"observedTimestamp"`
	}{
		Alias:             Alias(logData),
		Timestamp:         strconv.FormatInt(logData.Timestamp, 10),
		ObservedTimestamp: strconv.FormatInt(logData.ObservedTimestamp, 10),
	})
}
