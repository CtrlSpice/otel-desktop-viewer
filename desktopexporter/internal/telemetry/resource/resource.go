package resource

import (
	"log"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/errors"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

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

// GetServiceName gets the service name from resource attributes with respect to OTEL semantic conventions:
// service.name must be a string value having a meaning that helps to distinguish a group of services.
// Read more here: (https://opentelemetry.io/docs/reference/specification/resource/semantic_conventions/#service)
func (resourceData *ResourceData) GetServiceName() string {
	serviceName, ok := resourceData.Attributes["service.name"]
	if !ok {
		log.Println(errors.ErrInvalidServiceName)
		return ""
	}
	return serviceName.(string)
}
