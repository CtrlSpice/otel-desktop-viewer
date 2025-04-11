package telemetry

import "go.opentelemetry.io/collector/pdata/pcommon"

type ResourceData struct {
	Attributes             map[string]any `json:"attributes"`
	DroppedAttributesCount uint32         `json:"droppedAttributesCount"`
}

func AggregateResourceData(source pcommon.Resource) *ResourceData {
	return &ResourceData{
		Attributes:             source.Attributes().AsRaw(),
		DroppedAttributesCount: source.DroppedAttributesCount(),
	}
}
