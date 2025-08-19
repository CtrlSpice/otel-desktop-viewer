package traces

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"github.com/mitchellh/mapstructure"
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

// Scan converts a DuckDB array representation back to Events
func (eventDataSlice *Events) Scan(src any) error {
	switch v := src.(type) {
	case []any:
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: attributes.AttributesDecodeHook,
			Result:     eventDataSlice,
		})
		if err != nil {
			return fmt.Errorf("failed to create decoder: %w", err)
		}

		if err := decoder.Decode(v); err != nil {
			return fmt.Errorf("failed to decode Events: %w", err)
		}

		return nil
	case nil:
		*eventDataSlice = Events{}
		return nil
	default:
		return fmt.Errorf("Events: cannot scan from %T", src)
	}
}
