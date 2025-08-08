package metrics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/errors"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

type MetricsPayload struct {
	Metrics pmetric.Metrics
}

type MetricData struct {
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Unit        string                 `json:"unit,omitempty"`
	DataPoints  DataPoints             `json:"dataPoints"`
	Resource    *resource.ResourceData `json:"resource"`
	Scope       *scope.ScopeData       `json:"scope"`
	Received    int64                  `json:"-"`
}

func NewMetricsPayload(m pmetric.Metrics) *MetricsPayload {
	return &MetricsPayload{Metrics: m}
}

func (payload *MetricsPayload) ExtractMetrics() []MetricData {
	metricsDataSlice := []MetricData{}

	for _, resourceMetrics := range payload.Metrics.ResourceMetrics().All() {
		resourceData := resource.AggregateResourceData(resourceMetrics.Resource())

		for _, scopeMetrics := range resourceMetrics.ScopeMetrics().All() {
			scopeData := scope.AggregateScopeData(scopeMetrics.Scope())

			for _, metric := range scopeMetrics.Metrics().All() {
				metricsDataSlice = append(metricsDataSlice, aggregateMetricsData(metric, scopeData, resourceData))
			}
		}
	}
	return metricsDataSlice
}

func aggregateMetricsData(source pmetric.Metric, scopeData *scope.ScopeData, resourceData *resource.ResourceData) MetricData {
	metricsData := MetricData{
		Name:        source.Name(),
		Description: source.Description(),
		Unit:        source.Unit(),
		Received:    time.Now().UnixNano(),
		Resource:    resourceData,
		Scope:       scopeData,
	}

	switch source.Type() {
	case pmetric.MetricTypeEmpty:
		metricsData.DataPoints = DataPoints{Type: MetricTypeEmpty, Points: []MetricDataPoint{}}
	case pmetric.MetricTypeGauge:
		metricsData.DataPoints = DataPoints{Type: MetricTypeGauge, Points: extractGaugeDataPoints(source.Gauge())}
	case pmetric.MetricTypeSum:
		metricsData.DataPoints = DataPoints{Type: MetricTypeSum, Points: extractSumDataPoints(source.Sum())}
	case pmetric.MetricTypeHistogram:
		metricsData.DataPoints = DataPoints{Type: MetricTypeHistogram, Points: extractHistogramDataPoints(source.Histogram())}
	case pmetric.MetricTypeExponentialHistogram:
		metricsData.DataPoints = DataPoints{Type: MetricTypeExponentialHistogram, Points: extractExponentialHistogramDataPoints(source.ExponentialHistogram())}
	case pmetric.MetricTypeSummary:
		log.Printf("%v", errors.WarnSummaryMetricsDeprecated)
	default:
		log.Printf(errors.ErrUnknownMetricType, source.Type().String())
	}

	return metricsData
}

func (metricData MetricData) MarshalJSON() ([]byte, error) {
	type Alias MetricData // Avoid recursive MarshalJSON calls
	return json.Marshal(&struct {
		Alias
		Received string `json:"received"`
	}{
		Alias:    Alias(metricData),
		Received: strconv.FormatInt(metricData.Received, 10),
	})
}

func (metricData MetricData) ID() string {
	// Get resource name from attributes
	resourceName := ""
	if metricData.Resource != nil && metricData.Resource.Attributes != nil {
		if name, ok := metricData.Resource.Attributes["service.name"].(string); ok {
			resourceName = name
		}
	}

	hash := sha256.New()
	buf := make([]byte, 0, 256)
	buf = fmt.Appendf(buf, "%s|%s|%s|%s",
		metricData.Name,
		resourceName,
		metricData.DataPoints.Type,
		strconv.FormatInt(metricData.Received, 10),
	)
	hash.Write(buf)
	return hex.EncodeToString(hash.Sum(nil))
}
