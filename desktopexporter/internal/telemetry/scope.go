package telemetry

import "go.opentelemetry.io/collector/pdata/pcommon"

type ScopeData struct {
	Name                   string                 `json:"name"`
	Version                string                 `json:"version"`
	Attributes             map[string]interface{} `json:"attributes"`
	DroppedAttributesCount uint32                 `json:"droppedAttributesCount"`
}

func AggregateScopeData(source pcommon.InstrumentationScope) *ScopeData {
	return &ScopeData{
		Name:                   source.Name(),
		Version:                source.Version(),
		Attributes:             source.Attributes().AsRaw(),
		DroppedAttributesCount: source.DroppedAttributesCount(),
	}
}
