package metrics

import (
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func GenerateSampleMetrics() []MetricData {
	payload := NewMetricsPayload(pmetric.NewMetrics())

	// 1. Set up currencyservice resource
	currencyResourceMetric := payload.Metrics.ResourceMetrics().AppendEmpty()
	resource.FillCurrencyResource(currencyResourceMetric.Resource())

	// 2. Add currencyservice scope to currencyservice resource
	currencyScopeMetric := currencyResourceMetric.ScopeMetrics().AppendEmpty()
	scope.FillCurrencyScope(currencyScopeMetric.Scope())

	// 3. Add different types of metrics to currencyservice scope
	currencyScopeMetric.Metrics().EnsureCapacity(4)

	// Gauge metric
	gaugeMetric := currencyScopeMetric.Metrics().AppendEmpty()
	fillGaugeMetric(gaugeMetric)

	// Sum metric
	sumMetric := currencyScopeMetric.Metrics().AppendEmpty()
	fillSumMetric(sumMetric)

	// Histogram metric
	histogramMetric := currencyScopeMetric.Metrics().AppendEmpty()
	fillHistogramMetric(histogramMetric)

	// Exponential Histogram metric
	exponentialHistogramMetric := currencyScopeMetric.Metrics().AppendEmpty()
	fillExponentialHistogramMetric(exponentialHistogramMetric)

	return payload.ExtractMetrics()
}

func fillGaugeMetric(metric pmetric.Metric) {
	metric.SetDescription("Current memory usage")
	metric.SetName("memory.usage")
	metric.SetUnit("bytes")
	gauge := metric.SetEmptyGauge()
	pt := gauge.DataPoints().AppendEmpty()
	pt.SetDoubleValue(1024.5)
	pt.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)))
	pt.Attributes().PutStr("memory.type", "heap")
	pt.Attributes().PutStr("service.instance", "currencyservice-1")
}

func fillSumMetric(metric pmetric.Metric) {
	metric.SetDescription("Total requests processed")
	metric.SetName("requests.total")
	metric.SetUnit("requests")
	sum := metric.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	pt := sum.DataPoints().AppendEmpty()
	pt.SetDoubleValue(1500.0)
	pt.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)))
	pt.Attributes().PutStr("http.method", "POST")
	pt.Attributes().PutInt("http.status_code", 200)
}

func fillHistogramMetric(metric pmetric.Metric) {
	metric.SetDescription("Request duration distribution")
	metric.SetName("request.duration")
	metric.SetUnit("seconds")
	histogram := metric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	pt := histogram.DataPoints().AppendEmpty()
	pt.SetCount(100)
	pt.SetSum(25.5)
	pt.SetMin(0.1)
	pt.SetMax(2.5)
	pt.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)))
	pt.Attributes().PutStr("http.method", "GET")
	pt.Attributes().PutStr("http.route", "/api/convert")

	// Set bucket counts and bounds
	pt.BucketCounts().FromRaw([]uint64{10, 20, 30, 25, 15})
	pt.ExplicitBounds().FromRaw([]float64{0.5, 1.0, 1.5, 2.0})
}

func fillExponentialHistogramMetric(metric pmetric.Metric) {
	metric.SetDescription("Response size distribution")
	metric.SetName("response.size")
	metric.SetUnit("bytes")
	expHistogram := metric.SetEmptyExponentialHistogram()
	expHistogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	pt := expHistogram.DataPoints().AppendEmpty()
	pt.SetCount(50)
	pt.SetSum(10240.0)
	pt.SetMin(100.0)
	pt.SetMax(2048.0)
	pt.SetScale(2)
	pt.SetZeroCount(5)
	pt.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)))
	pt.Attributes().PutStr("content.type", "application/json")
	pt.Attributes().PutStr("http.method", "POST")

	// Set positive and negative buckets
	positive := pt.Positive()
	positive.SetOffset(1)
	positive.BucketCounts().FromRaw([]uint64{5, 10, 15, 10, 5})

	negative := pt.Negative()
	negative.SetOffset(0)
	negative.BucketCounts().FromRaw([]uint64{2, 3})
}
