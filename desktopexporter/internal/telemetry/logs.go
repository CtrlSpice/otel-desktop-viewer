package telemetry

import (
	"time"

	"go.opentelemetry.io/collector/pdata/plog"
)

type LogsPayload struct {
	logs plog.Logs
}

type LogData struct {
	Body                   string                 `json:"body,omitempty"`
	TraceID                string                 `json:"traceID,omitempty"`
	SpanID                 string                 `json:"spanID,omitempty"`
	Timestamp              time.Time              `json:"timestamp,omitempty"`
	ObservedTimestamp      time.Time              `json:"observedTimestamp,omitempty"`
	Attributes             map[string]interface{} `json:"attributes,omitempty"`
	SeverityText           string                 `json:"severityText,omitempty"`
	SeverityNumber         plog.SeverityNumber    `json:"severityNumber,omitempty"`
	DroppedAttributesCount uint32                 `json:"droppedAttributeCount,omitempty"`
	Flags                  plog.LogRecordFlags    `json:"flags,omitempty"`
	Resource               *ResourceData          `json:"resource"`
	Scope                  *ScopeData             `json:"scope"`
}

func NewLogsPayload(l plog.Logs) *LogsPayload {
	return &LogsPayload{logs: l}
}

func (payload *LogsPayload) ExtractLogs() []LogData {
	logData := []LogData{}

	for _, resourceLogs := range payload.logs.ResourceLogs().All() {
		resourceData := AggregateResourceData(resourceLogs.Resource())

		for _, scopeLogs := range resourceLogs.ScopeLogs().All() {
			scopeData := AggregateScopeData(scopeLogs.Scope())

			for _, logRecord := range scopeLogs.LogRecords().All() {
				logData = append(logData, aggregateLogData(logRecord, scopeData, resourceData))
			}
		}
	}
	return logData
}

func aggregateLogData(source plog.LogRecord, scopeData *ScopeData, resourceData *ResourceData) LogData {
	return LogData{
		Body:                   source.Body().AsString(),
		TraceID:                source.TraceID().String(),
		SpanID:                 source.SpanID().String(),
		ObservedTimestamp:      source.ObservedTimestamp().AsTime(),
		Timestamp:              source.Timestamp().AsTime(),
		Attributes:             source.Attributes().AsRaw(),
		Resource:               resourceData,
		Scope:                  scopeData,
		DroppedAttributesCount: source.DroppedAttributesCount(),
		SeverityText:           source.SeverityText(),
		SeverityNumber:         source.SeverityNumber(),
		Flags:                  source.Flags(),
	}
}

func (logData LogData) ID() string {
	// may need to consider additional fields to uniquely identify
	// a log, for example different resources could potentially
	// send the same data at the same time and create collisions
	return logData.Body + logData.Timestamp.String() + logData.ObservedTimestamp.String()
}
