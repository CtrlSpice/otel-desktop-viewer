package telemetry

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pmetric"
)

type MetricsPayload struct {
	metrics pmetric.Metrics
}

type MetricsData struct {
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Unit        string             `json:"unit,omitempty"`
	Type        pmetric.MetricType `json:"type,omitempty"`
	// add datapoints
	Resource *ResourceData `json:"resource"`
	Scope    *ScopeData    `json:"scope"`
	Received time.Time     `json:"-"`
}

func (payload *MetricsPayload) ExtractMetrics() []MetricsData {
	metricsDataSlice := []MetricsData{}

	for rmi := 0; rmi < payload.metrics.ResourceMetrics().Len(); rmi++ {
		resourceMetrics := payload.metrics.ResourceMetrics().At(rmi)
		resourceData := AggregateResourceData(resourceMetrics.Resource())

		for smi := 0; smi < resourceMetrics.ScopeMetrics().Len(); smi++ {
			scopeMetrics := resourceMetrics.ScopeMetrics().At(smi)
			scopeData := AggregateScopeData(scopeMetrics.Scope())

			for si := 0; si < scopeMetrics.Metrics().Len(); si++ {
				metric := scopeMetrics.Metrics().At(si)
				metricsDataSlice = append(metricsDataSlice, aggregateMetricsData(metric, scopeData, resourceData))
			}
		}
	}
	return metricsDataSlice
}

func aggregateMetricsData(source pmetric.Metric, scopeData *ScopeData, resourceData *ResourceData) MetricsData {
	return MetricsData{
		Name:        source.Name(),
		Description: source.Description(),
		Unit:        source.Unit(),
		Type:        source.Type(),
		Received:    time.Now(),
		// TODO: add other fields
		Resource: resourceData,
		Scope:    scopeData,
	}
}

func (metricData MetricsData) ID() string {
	// may need to consider additional fields to uniquely identify
	// a metric, for example different resources could potentially
	// send the same data at the same time and create collisions
	return metricData.Name + metricData.Received.String()
}
