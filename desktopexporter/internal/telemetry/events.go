package telemetry

import (
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

type EventPayload struct {
	Events ptrace.SpanEventSlice
}

type EventData struct {
	Name                   string         `json:"name"`
	Timestamp              time.Time      `json:"timestamp"`
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
		Timestamp:              source.Timestamp().AsTime(),
		Attributes:             source.Attributes().AsRaw(),
		DroppedAttributesCount: source.DroppedAttributesCount(),
	}
}
