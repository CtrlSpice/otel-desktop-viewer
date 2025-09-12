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

	// 4. Add frontend service resource and metrics
	frontendResourceMetric := payload.Metrics.ResourceMetrics().AppendEmpty()
	fillFrontendResource(frontendResourceMetric.Resource())

	frontendScopeMetric := frontendResourceMetric.ScopeMetrics().AppendEmpty()
	fillFrontendScope(frontendScopeMetric.Scope())

	frontendScopeMetric.Metrics().EnsureCapacity(3)

	// Frontend page load times
	pageLoadMetric := frontendScopeMetric.Metrics().AppendEmpty()
	fillPageLoadHistogramMetric(pageLoadMetric)

	// Frontend error rate
	errorRateMetric := frontendScopeMetric.Metrics().AppendEmpty()
	fillErrorRateSumMetric(errorRateMetric)

	// Frontend active users
	activeUsersMetric := frontendScopeMetric.Metrics().AppendEmpty()
	fillActiveUsersGaugeMetric(activeUsersMetric)

	// 5. Add payment service resource and metrics
	paymentResourceMetric := payload.Metrics.ResourceMetrics().AppendEmpty()
	fillPaymentResource(paymentResourceMetric.Resource())

	paymentScopeMetric := paymentResourceMetric.ScopeMetrics().AppendEmpty()
	fillPaymentScope(paymentScopeMetric.Scope())

	paymentScopeMetric.Metrics().EnsureCapacity(2)

	// Payment processing times
	paymentProcessingMetric := paymentScopeMetric.Metrics().AppendEmpty()
	fillPaymentProcessingHistogramMetric(paymentProcessingMetric)

	// Payment transaction amounts
	transactionAmountMetric := paymentScopeMetric.Metrics().AppendEmpty()
	fillTransactionAmountExponentialHistogramMetric(transactionAmountMetric)

	return payload.ExtractMetrics()
}

func fillGaugeMetric(metric pmetric.Metric) {
	metric.SetDescription("Current memory usage across different instances")
	metric.SetName("memory.usage")
	metric.SetUnit("bytes")
	gauge := metric.SetEmptyGauge()

	// Multiple data points for different instances
	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// Instance 1 - Heap memory
	pt1 := gauge.DataPoints().AppendEmpty()
	pt1.SetDoubleValue(1024.5)
	pt1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt1.Attributes().PutStr("memory.type", "heap")
	pt1.Attributes().PutStr("service.instance", "currencyservice-1")
	pt1.Attributes().PutStr("host.name", "currency-pod-1")

	// Instance 2 - Non-heap memory
	pt2 := gauge.DataPoints().AppendEmpty()
	pt2.SetDoubleValue(512.3)
	pt2.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(5 * time.Second)))
	pt2.Attributes().PutStr("memory.type", "non_heap")
	pt2.Attributes().PutStr("service.instance", "currencyservice-1")
	pt2.Attributes().PutStr("host.name", "currency-pod-1")

	// Instance 3 - Different service instance
	pt3 := gauge.DataPoints().AppendEmpty()
	pt3.SetDoubleValue(2048.7)
	pt3.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(10 * time.Second)))
	pt3.Attributes().PutStr("memory.type", "heap")
	pt3.Attributes().PutStr("service.instance", "currencyservice-2")
	pt3.Attributes().PutStr("host.name", "currency-pod-2")

	// Add exemplar to the first data point
	exemplar1 := pt1.Exemplars().AppendEmpty()
	exemplar1.SetDoubleValue(1024.5)
	exemplar1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	exemplar1.SetTraceID([16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})
	exemplar1.SetSpanID([8]byte{0xa1, 0xb2, 0xc3, 0xd4, 0xe5, 0xf6, 0x07, 0x18})
	exemplar1.FilteredAttributes().PutStr("operation", "gc_collection")
}

func fillSumMetric(metric pmetric.Metric) {
	metric.SetDescription("Total requests processed by endpoint")
	metric.SetName("requests.total")
	metric.SetUnit("requests")
	sum := metric.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// POST /convert endpoint
	pt1 := sum.DataPoints().AppendEmpty()
	pt1.SetDoubleValue(1500.0)
	pt1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt1.Attributes().PutStr("http.method", "POST")
	pt1.Attributes().PutInt("http.status_code", 200)
	pt1.Attributes().PutStr("http.route", "/convert")
	pt1.Attributes().PutStr("service.instance", "currencyservice-1")

	// GET /supported-currencies endpoint
	pt2 := sum.DataPoints().AppendEmpty()
	pt2.SetDoubleValue(850.0)
	pt2.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(5 * time.Second)))
	pt2.Attributes().PutStr("http.method", "GET")
	pt2.Attributes().PutInt("http.status_code", 200)
	pt2.Attributes().PutStr("http.route", "/supported-currencies")
	pt2.Attributes().PutStr("service.instance", "currencyservice-1")

	// Error responses
	pt3 := sum.DataPoints().AppendEmpty()
	pt3.SetDoubleValue(25.0)
	pt3.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(10 * time.Second)))
	pt3.Attributes().PutStr("http.method", "POST")
	pt3.Attributes().PutInt("http.status_code", 400)
	pt3.Attributes().PutStr("http.route", "/convert")
	pt3.Attributes().PutStr("service.instance", "currencyservice-2")

	// Add exemplars
	exemplar1 := pt1.Exemplars().AppendEmpty()
	exemplar1.SetDoubleValue(1500.0)
	exemplar1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	exemplar1.SetTraceID([16]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20})
	exemplar1.SetSpanID([8]byte{0xb1, 0xc2, 0xd3, 0xe4, 0xf5, 0x06, 0x17, 0x28})
	exemplar1.FilteredAttributes().PutStr("user.id", "user123")
}

func fillHistogramMetric(metric pmetric.Metric) {
	metric.SetDescription("Request duration distribution across different endpoints")
	metric.SetName("request.duration")
	metric.SetUnit("seconds")
	histogram := metric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// Convert endpoint histogram
	pt1 := histogram.DataPoints().AppendEmpty()
	pt1.SetCount(100)
	pt1.SetSum(25.5)
	pt1.SetMin(0.1)
	pt1.SetMax(2.5)
	pt1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt1.Attributes().PutStr("http.method", "GET")
	pt1.Attributes().PutStr("http.route", "/api/convert")
	pt1.Attributes().PutStr("service.instance", "currencyservice-1")
	pt1.BucketCounts().FromRaw([]uint64{10, 20, 30, 25, 15})
	pt1.ExplicitBounds().FromRaw([]float64{0.5, 1.0, 1.5, 2.0})

	// Add exemplars to histogram
	exemplar1 := pt1.Exemplars().AppendEmpty()
	exemplar1.SetDoubleValue(1.85)
	exemplar1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	exemplar1.SetTraceID([16]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30})
	exemplar1.SetSpanID([8]byte{0xc1, 0xd2, 0xe3, 0xf4, 0x05, 0x16, 0x27, 0x38})
	exemplar1.FilteredAttributes().PutStr("slow_query", "complex_conversion")

	exemplar2 := pt1.Exemplars().AppendEmpty()
	exemplar2.SetDoubleValue(0.15)
	exemplar2.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(2 * time.Second)))
	exemplar2.SetTraceID([16]byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40})
	exemplar2.SetSpanID([8]byte{0xd1, 0xe2, 0xf3, 0x04, 0x15, 0x26, 0x37, 0x48})
	exemplar2.FilteredAttributes().PutStr("cache_hit", "true")

	// Supported currencies endpoint histogram
	pt2 := histogram.DataPoints().AppendEmpty()
	pt2.SetCount(50)
	pt2.SetSum(7.5)
	pt2.SetMin(0.05)
	pt2.SetMax(0.8)
	pt2.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(5 * time.Second)))
	pt2.Attributes().PutStr("http.method", "GET")
	pt2.Attributes().PutStr("http.route", "/api/supported-currencies")
	pt2.Attributes().PutStr("service.instance", "currencyservice-2")
	pt2.BucketCounts().FromRaw([]uint64{15, 20, 10, 5, 0})
	pt2.ExplicitBounds().FromRaw([]float64{0.1, 0.25, 0.5, 0.75})
}

func fillExponentialHistogramMetric(metric pmetric.Metric) {
	metric.SetDescription("Response size distribution for different content types")
	metric.SetName("response.size")
	metric.SetUnit("bytes")
	expHistogram := metric.SetEmptyExponentialHistogram()
	expHistogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// JSON responses
	pt1 := expHistogram.DataPoints().AppendEmpty()
	pt1.SetCount(50)
	pt1.SetSum(10240.0)
	pt1.SetMin(100.0)
	pt1.SetMax(2048.0)
	pt1.SetScale(2)
	pt1.SetZeroCount(5)
	pt1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt1.Attributes().PutStr("content.type", "application/json")
	pt1.Attributes().PutStr("http.method", "POST")
	pt1.Attributes().PutStr("service.instance", "currencyservice-1")

	positive1 := pt1.Positive()
	positive1.SetOffset(1)
	positive1.BucketCounts().FromRaw([]uint64{5, 10, 15, 10, 5})

	negative1 := pt1.Negative()
	negative1.SetOffset(0)
	negative1.BucketCounts().FromRaw([]uint64{2, 3})

	// Add exemplars to exponential histogram
	exemplar1 := pt1.Exemplars().AppendEmpty()
	exemplar1.SetDoubleValue(1856.0)
	exemplar1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	exemplar1.SetTraceID([16]byte{0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f, 0x50})
	exemplar1.SetSpanID([8]byte{0xe1, 0xf2, 0x03, 0x14, 0x25, 0x36, 0x47, 0x58})
	exemplar1.FilteredAttributes().PutStr("large_response", "currency_list")

	// XML responses
	pt2 := expHistogram.DataPoints().AppendEmpty()
	pt2.SetCount(25)
	pt2.SetSum(15360.0)
	pt2.SetMin(200.0)
	pt2.SetMax(4096.0)
	pt2.SetScale(2)
	pt2.SetZeroCount(2)
	pt2.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(10 * time.Second)))
	pt2.Attributes().PutStr("content.type", "application/xml")
	pt2.Attributes().PutStr("http.method", "POST")
	pt2.Attributes().PutStr("service.instance", "currencyservice-2")

	positive2 := pt2.Positive()
	positive2.SetOffset(2)
	positive2.BucketCounts().FromRaw([]uint64{3, 8, 7, 5, 2})

	negative2 := pt2.Negative()
	negative2.SetOffset(1)
	negative2.BucketCounts().FromRaw([]uint64{1, 1})
}

// Additional service resources and scopes
func fillFrontendResource(resource pcommon.Resource) {
	resource.Attributes().PutStr("service.name", "frontend")
	resource.Attributes().PutStr("service.namespace", "shop")
	resource.Attributes().PutStr("service.version", "1.2.3")
	resource.Attributes().PutStr("service.instance.id", "frontend-7d4b9c8f6d-xyz123")
	resource.Attributes().PutStr("container.name", "frontend")
	resource.Attributes().PutStr("k8s.pod.name", "frontend-7d4b9c8f6d-xyz123")
	resource.Attributes().PutStr("k8s.namespace.name", "default")
	resource.Attributes().PutStr("k8s.deployment.name", "frontend")
	resource.Attributes().PutStr("host.name", "frontend-pod-1")
	resource.Attributes().PutStr("host.arch", "amd64")
	resource.Attributes().PutStr("os.type", "linux")
	resource.Attributes().PutBool("telemetry.sample", true)
}

func fillFrontendScope(scope pcommon.InstrumentationScope) {
	scope.SetName("frontend/http")
	scope.SetVersion("1.0.0")
	scope.Attributes().PutStr("instrumentation.provider", "opentelemetry")
}

func fillPaymentResource(resource pcommon.Resource) {
	resource.Attributes().PutStr("service.name", "paymentservice")
	resource.Attributes().PutStr("service.namespace", "shop")
	resource.Attributes().PutStr("service.version", "2.1.0")
	resource.Attributes().PutStr("service.instance.id", "payment-5c7a8b9e2f-abc456")
	resource.Attributes().PutStr("container.name", "paymentservice")
	resource.Attributes().PutStr("k8s.pod.name", "payment-5c7a8b9e2f-abc456")
	resource.Attributes().PutStr("k8s.namespace.name", "payments")
	resource.Attributes().PutStr("k8s.deployment.name", "paymentservice")
	resource.Attributes().PutStr("host.name", "payment-pod-1")
	resource.Attributes().PutStr("host.arch", "amd64")
	resource.Attributes().PutStr("os.type", "linux")
	resource.Attributes().PutBool("telemetry.sample", true)
}

func fillPaymentScope(scope pcommon.InstrumentationScope) {
	scope.SetName("paymentservice/grpc")
	scope.SetVersion("1.1.0")
	scope.Attributes().PutStr("instrumentation.provider", "opentelemetry")
}

// Additional metric functions
func fillPageLoadHistogramMetric(metric pmetric.Metric) {
	metric.SetDescription("Page load time distribution by page type")
	metric.SetName("page.load.duration")
	metric.SetUnit("milliseconds")
	histogram := metric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// Home page loads
	pt := histogram.DataPoints().AppendEmpty()
	pt.SetCount(200)
	pt.SetSum(4800.0)
	pt.SetMin(120.0)
	pt.SetMax(850.0)
	pt.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt.Attributes().PutStr("page.type", "home")
	pt.Attributes().PutStr("browser", "chrome")
	pt.Attributes().PutStr("device.type", "desktop")
	pt.BucketCounts().FromRaw([]uint64{25, 60, 80, 30, 5})
	pt.ExplicitBounds().FromRaw([]float64{200.0, 400.0, 600.0, 800.0})

	// Add exemplar for slow page load
	exemplar := pt.Exemplars().AppendEmpty()
	exemplar.SetDoubleValue(825.0)
	exemplar.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	exemplar.SetTraceID([16]byte{0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59, 0x5a, 0x5b, 0x5c, 0x5d, 0x5e, 0x5f, 0x60})
	exemplar.SetSpanID([8]byte{0xf1, 0x02, 0x13, 0x24, 0x35, 0x46, 0x57, 0x68})
	exemplar.FilteredAttributes().PutStr("slow_component", "product_recommendations")
}

func fillErrorRateSumMetric(metric pmetric.Metric) {
	metric.SetDescription("Frontend error count by error type")
	metric.SetName("frontend.errors.total")
	metric.SetUnit("errors")
	sum := metric.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)

	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// JavaScript errors
	pt1 := sum.DataPoints().AppendEmpty()
	pt1.SetDoubleValue(15.0)
	pt1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt1.Attributes().PutStr("error.type", "javascript")
	pt1.Attributes().PutStr("browser", "chrome")
	pt1.Attributes().PutStr("page.route", "/checkout")

	// Network errors
	pt2 := sum.DataPoints().AppendEmpty()
	pt2.SetDoubleValue(8.0)
	pt2.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(5 * time.Second)))
	pt2.Attributes().PutStr("error.type", "network")
	pt2.Attributes().PutStr("browser", "safari")
	pt2.Attributes().PutStr("page.route", "/product")
}

func fillActiveUsersGaugeMetric(metric pmetric.Metric) {
	metric.SetDescription("Current active users by session type")
	metric.SetName("frontend.active_users")
	metric.SetUnit("users")
	gauge := metric.SetEmptyGauge()

	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// Authenticated users
	pt1 := gauge.DataPoints().AppendEmpty()
	pt1.SetDoubleValue(1250.0)
	pt1.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt1.Attributes().PutStr("session.type", "authenticated")
	pt1.Attributes().PutStr("user.tier", "premium")

	// Anonymous users
	pt2 := gauge.DataPoints().AppendEmpty()
	pt2.SetDoubleValue(3480.0)
	pt2.SetTimestamp(pcommon.NewTimestampFromTime(baseTime.Add(5 * time.Second)))
	pt2.Attributes().PutStr("session.type", "anonymous")
	pt2.Attributes().PutStr("user.tier", "free")
}

func fillPaymentProcessingHistogramMetric(metric pmetric.Metric) {
	metric.SetDescription("Payment processing time by payment method")
	metric.SetName("payment.processing.duration")
	metric.SetUnit("milliseconds")
	histogram := metric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// Credit card payments
	pt := histogram.DataPoints().AppendEmpty()
	pt.SetCount(85)
	pt.SetSum(12750.0)
	pt.SetMin(80.0)
	pt.SetMax(450.0)
	pt.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt.Attributes().PutStr("payment.method", "credit_card")
	pt.Attributes().PutStr("payment.provider", "stripe")
	pt.Attributes().PutStr("currency", "USD")
	pt.BucketCounts().FromRaw([]uint64{15, 35, 25, 8, 2})
	pt.ExplicitBounds().FromRaw([]float64{100.0, 200.0, 300.0, 400.0})

	// Add exemplar for failed payment
	exemplar := pt.Exemplars().AppendEmpty()
	exemplar.SetDoubleValue(425.0)
	exemplar.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	exemplar.SetTraceID([16]byte{0x61, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a, 0x6b, 0x6c, 0x6d, 0x6e, 0x6f, 0x70})
	exemplar.SetSpanID([8]byte{0x01, 0x12, 0x23, 0x34, 0x45, 0x56, 0x67, 0x78})
	exemplar.FilteredAttributes().PutStr("payment.status", "retry_required")
}

func fillTransactionAmountExponentialHistogramMetric(metric pmetric.Metric) {
	metric.SetDescription("Transaction amount distribution by currency")
	metric.SetName("payment.transaction.amount")
	metric.SetUnit("currency_units")
	expHistogram := metric.SetEmptyExponentialHistogram()
	expHistogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	baseTime := time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)

	// USD transactions
	pt := expHistogram.DataPoints().AppendEmpty()
	pt.SetCount(120)
	pt.SetSum(24580.50)
	pt.SetMin(5.99)
	pt.SetMax(1299.99)
	pt.SetScale(1)
	pt.SetZeroCount(0)
	pt.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	pt.Attributes().PutStr("currency", "USD")
	pt.Attributes().PutStr("payment.method", "credit_card")
	pt.Attributes().PutStr("merchant.category", "retail")

	positive := pt.Positive()
	positive.SetOffset(3)
	positive.BucketCounts().FromRaw([]uint64{20, 35, 40, 20, 5})

	negative := pt.Negative()
	negative.SetOffset(0)
	negative.BucketCounts().FromRaw([]uint64{0})

	// Add exemplar for high-value transaction
	exemplar := pt.Exemplars().AppendEmpty()
	exemplar.SetDoubleValue(1299.99)
	exemplar.SetTimestamp(pcommon.NewTimestampFromTime(baseTime))
	exemplar.SetTraceID([16]byte{0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7a, 0x7b, 0x7c, 0x7d, 0x7e, 0x7f, 0x80})
	exemplar.SetSpanID([8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88})
	exemplar.FilteredAttributes().PutStr("transaction.type", "high_value")
}
