package traces

import (
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"github.com/go-viper/mapstructure/v2"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type LinkPayload struct {
	links ptrace.SpanLinkSlice
}

type LinkData struct {
	TraceID                string                `json:"traceID"`
	SpanID                 string                `json:"spanID"`
	TraceState             string                `json:"traceState"`
	Attributes             attributes.Attributes `json:"attributes"`
	DroppedAttributesCount uint32                `json:"droppedAttributesCount"`
}

// Links is a slice of LinkData with a Scan method for DuckDB
// I was going to name it Zeldas, but through herculean restraint I did not do this thing.
// I really wanted to though...
type Links []LinkData

func (payload *LinkPayload) ExtractLinks() []LinkData {
	linkDataSlice := []LinkData{}
	for _, link := range payload.links.All() {
		linkDataSlice = append(linkDataSlice, aggregateLinkData(link))
	}

	return linkDataSlice
}

func aggregateLinkData(source ptrace.SpanLink) LinkData {
	return LinkData{
		TraceID:                source.TraceID().String(),
		SpanID:                 source.SpanID().String(),
		TraceState:             source.TraceState().AsRaw(),
		Attributes:             attributes.Attributes(source.Attributes().AsRaw()),
		DroppedAttributesCount: source.DroppedAttributesCount(),
	}
}

// Scan converts a DuckDB array representation back to Links
func (links *Links) Scan(src any) error {
	switch v := src.(type) {
	case []any:
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: attributes.AttributesDecodeHook,
			Result:     links,
		})
		if err != nil {
			return fmt.Errorf("failed to create decoder: %w", err)
		}

		if err := decoder.Decode(v); err != nil {
			return fmt.Errorf("failed to decode Links: %w", err)
		}

		return nil
	case nil:
		*links = Links{}
		return nil
	default:
		return fmt.Errorf("Links: cannot scan from %T", src)
	}
}
