package telemetry

import (
	"encoding/json"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

type EventPayload struct {
	Events ptrace.SpanEventSlice
}

type EventData struct {
	Name                   string         `json:"name"`
	Timestamp              int64          `json:"-"`
	Attributes             map[string]any `json:"attributes"`
	DroppedAttributesCount uint32         `json:"droppedAttributesCount"`
}

func (payload *EventPayload) extractEvents() []EventData {
	eventDataSlice := []EventData{}

	for _, event := range payload.Events.All() {
		eventDataSlice = append(eventDataSlice, aggregateEventData(event))
	}

	return eventDataSlice
}

func aggregateEventData(source ptrace.SpanEvent) EventData {
	return EventData{
		Name:                   source.Name(),
		Timestamp:              source.Timestamp().AsTime().UnixNano(),
		Attributes:             source.Attributes().AsRaw(),
		DroppedAttributesCount: source.DroppedAttributesCount(),
	}
}

// MarshalJSON implements custom JSON marshaling for EventData
func (eventData EventData) MarshalJSON() ([]byte, error) {
	type Alias EventData // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		Timestamp PreciseTimestamp `json:"timestamp"`
	}{
		Alias:     Alias(eventData),
		Timestamp: NewPreciseTimestamp(eventData.Timestamp),
	})
}
