package scope

import (
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

type ScopeData struct {
	Name                   string                `json:"name"`
	Version                string                `json:"version"`
	Attributes             attributes.Attributes `json:"attributes"`
	DroppedAttributesCount uint32                `json:"droppedAttributesCount"`
}

func AggregateScopeData(source pcommon.InstrumentationScope) *ScopeData {
	return &ScopeData{
		Name:                   source.Name(),
		Version:                source.Version(),
		Attributes:             attributes.Attributes(source.Attributes().AsRaw()),
		DroppedAttributesCount: source.DroppedAttributesCount(),
	}
}
