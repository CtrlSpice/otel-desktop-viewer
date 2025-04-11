package telemetry

import "go.opentelemetry.io/collector/pdata/ptrace"

type LinkPayload struct {
	links ptrace.SpanLinkSlice
}

type LinkData struct {
	TraceID                string         `json:"traceID"`
	SpanID                 string         `json:"spanID"` 
	TraceState             string         `json:"traceState"`
	Attributes             map[string]any `json:"attributes"`
	DroppedAttributesCount uint32         `json:"droppedAttributesCount"`
}

func (payload *LinkPayload) ExtractLinks() []LinkData {
	linkDataSlice := []LinkData{}
	for i := range payload.links.Len() {
		linkDataSlice = append(linkDataSlice, aggregateLinkData(payload.links.At(i)))
	}

	return linkDataSlice
}

func aggregateLinkData(source ptrace.SpanLink) LinkData {
	return LinkData{
		TraceID:                source.TraceID().String(),
		SpanID:                 source.SpanID().String(),
		TraceState:             source.TraceState().AsRaw(),
		Attributes:             source.Attributes().AsRaw(),
		DroppedAttributesCount: source.DroppedAttributesCount(),
	}
}
