package telemetry

import (
	"fmt"
)

// Get the service name of a span with respect to OTEL semanic conventions:
// service.name must be a string value having a meaning that helps to distinguish a group of services.
// Read more here: (https://opentelemetry.io/docs/reference/specification/resource/semantic_conventions/#service)
func (spanData *SpanData) GetServiceName() string {
	serviceName, ok := spanData.Resource.Attributes["service.name"].(string)
	if !ok {
		fmt.Println("traceID:", spanData.TraceID, "spanID:", spanData.SpanID, WarningInvalidServiceName)
		return ""
	}
	return serviceName
}
