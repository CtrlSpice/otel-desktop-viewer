package traces

import (
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
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

// Scan converts a DuckDB struct representation back to LinkData
func (linkData *LinkData) Scan(src any) error {
	switch v := src.(type) {
	case map[string]any:
		if traceID, ok := v["TraceID"].(string); ok {
			linkData.TraceID = traceID
		}
		if spanID, ok := v["SpanID"].(string); ok {
			linkData.SpanID = spanID
		}
		if traceState, ok := v["TraceState"].(string); ok {
			linkData.TraceState = traceState
		}
		if droppedCount, ok := v["DroppedAttributesCount"].(uint32); ok {
			linkData.DroppedAttributesCount = droppedCount
		}
		// Handle Attributes conversion using its Scan method
		if attrsRaw, ok := v["Attributes"]; ok {
			if err := linkData.Attributes.Scan(attrsRaw); err != nil {
				return fmt.Errorf("failed to scan Attributes: %w", err)
			}
		}
		return nil
	case nil:
		*linkData = LinkData{}
		return nil
	default:
		return fmt.Errorf("LinkData: cannot scan from %T", src)
	}
}

// Scan converts a DuckDB array representation back to Links
func (links *Links) Scan(src any) error {
	switch v := src.(type) {
	case []any:
		linkSlice := make([]LinkData, len(v))
		for i, item := range v {
			var link LinkData
			if err := link.Scan(item); err != nil {
				return fmt.Errorf("failed to scan link %d: %w", i, err)
			}
			linkSlice[i] = link
		}
		*links = Links(linkSlice)
		return nil
	case nil:
		*links = Links{}
		return nil
	default:
		return fmt.Errorf("Links: cannot scan from %T", src)
	}
}
