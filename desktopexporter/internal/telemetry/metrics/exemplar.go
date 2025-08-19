package metrics

import (
	"encoding/json"
	"strconv"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/attributes"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// Exemplar represents a sample value that contributed to a metric data point
type Exemplar struct {
	Timestamp          int64                 `json:"-"`
	Value              float64               `json:"value"`
	TraceID            string                `json:"traceID,omitempty"`
	SpanID             string                `json:"spanID,omitempty"`
	FilteredAttributes attributes.Attributes `json:"filteredAttributes,omitempty"`
}

// extractExemplars extracts exemplars from OpenTelemetry exemplar slices
func extractExemplars(exemplars pmetric.ExemplarSlice) []Exemplar {
	result := make([]Exemplar, exemplars.Len())
	for i := range exemplars.Len() {
		exemplar := exemplars.At(i)
		result[i] = Exemplar{
			Timestamp:          exemplar.Timestamp().AsTime().UnixNano(),
			Value:              exemplar.DoubleValue(),
			TraceID:            exemplar.TraceID().String(),
			SpanID:             exemplar.SpanID().String(),
			FilteredAttributes: attributes.Attributes(exemplar.FilteredAttributes().AsRaw()),
		}
	}
	return result
}

func (exemplar Exemplar) MarshalJSON() ([]byte, error) {
	type Alias Exemplar // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		Timestamp string `json:"timestamp"`
	}{
		Alias:     Alias(exemplar),
		Timestamp: strconv.FormatInt(exemplar.Timestamp, 10),
	})
}
