package traces

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type EventPayload struct {
	Events ptrace.SpanEventSlice
}

type EventData struct {
	Name                   string                `json:"name"`
	Timestamp              int64                 `json:"-"`
	Attributes             attributes.Attributes `json:"attributes"`
	DroppedAttributesCount uint32                `json:"droppedAttributesCount"`
}

// Events is a slice of EventData with a Scan method for DuckDB
type Events []EventData

func (payload *EventPayload) extractEvents() Events {
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
		Attributes:             attributes.Attributes(source.Attributes().AsRaw()),
		DroppedAttributesCount: source.DroppedAttributesCount(),
	}
}

// MarshalJSON implements custom JSON marshaling for EventData
func (eventData EventData) MarshalJSON() ([]byte, error) {
	type Alias EventData // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		Timestamp string `json:"timestamp"`
	}{
		Alias:     Alias(eventData),
		Timestamp: strconv.FormatInt(eventData.Timestamp, 10),
	})
}

// Scan converts a DuckDB struct representation back to EventData
func (eventData *EventData) Scan(src any) error {
	switch v := src.(type) {
	case map[string]any:
		if name, ok := v["Name"].(string); ok {
			eventData.Name = name
		}
		if timestamp, ok := v["Timestamp"].(int64); ok {
			eventData.Timestamp = timestamp
		}
		if droppedCount, ok := v["DroppedAttributesCount"].(uint32); ok {
			eventData.DroppedAttributesCount = droppedCount
		}
		// Handle Attributes conversion using its Scan method
		if attrsRaw, ok := v["Attributes"]; ok {
			if err := eventData.Attributes.Scan(attrsRaw); err != nil {
				return fmt.Errorf("failed to scan Attributes: %w", err)
			}
		}
		return nil
	case nil:
		*eventData = EventData{}
		return nil
	default:
		return fmt.Errorf("EventData: cannot scan from %T", src)
	}
}

// Scan converts a DuckDB array representation back to Events
func (eventDataSlice *Events) Scan(src any) error {
	switch v := src.(type) {
	case []any:
		events := make([]EventData, len(v))
		for i, item := range v {
			var event EventData
			if err := event.Scan(item); err != nil {
				return fmt.Errorf("failed to scan event %d: %w", i, err)
			}
			events[i] = event
		}
		*eventDataSlice = Events(events)
		return nil
	case nil:
		*eventDataSlice = Events{}
		return nil
	default:
		return fmt.Errorf("Events: cannot scan from %T", src)
	}
}
