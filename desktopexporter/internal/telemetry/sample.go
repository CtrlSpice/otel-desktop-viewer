package telemetry

import (
	"encoding/hex"
	"log"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type SampleTelemetry struct {
	Spans   []SpanData
	Logs    []LogData
	Metrics []MetricsData
}

func NewSampleTelemetry() SampleTelemetry {
	sample := SampleTelemetry{}
	sample.generateTraces()
	sample.generateLogs()
	sample.generateMetrics()
	return sample
}

func (sample *SampleTelemetry) generateTraces() {
	payload := SpanPayload{ptrace.NewTraces()}
	payload.traces.ResourceSpans().EnsureCapacity(3)

	// Generate sample currency conversion trace:
	// 1. Set up currencyservice resource
	currencyResourceSpan := payload.traces.ResourceSpans().AppendEmpty()
	fillCurrencyResource(currencyResourceSpan.Resource())

	// 2. Add currencyservice scope to currencyservice resource
	currencyScopeSpan := currencyResourceSpan.ScopeSpans().AppendEmpty()
	fillCurrencyScope(currencyScopeSpan.Scope())

	// 3. Add CurrencyService/Convert span to currencyservice scope
	currencySpan := currencyScopeSpan.Spans().AppendEmpty()
	fillCurrencySpan(currencySpan)

	// Generate sample HTTP POST trace:
	// 1. Set up loadgenerator resource
	loadGeneratorResourceSpan := payload.traces.ResourceSpans().AppendEmpty()
	fillLoadGeneratorResource(loadGeneratorResourceSpan.Resource())

	// 2. Set up frontend resource
	frontEndResourceSpan := payload.traces.ResourceSpans().AppendEmpty()
	fillFrontEndResource(frontEndResourceSpan.Resource())

	// 3. Add requests and urllib3 scopes to loadgenerator resource
	loadGeneratorResourceSpan.ScopeSpans().EnsureCapacity(2)
	requestsScopeSpan := loadGeneratorResourceSpan.ScopeSpans().AppendEmpty()
	fillRequestsScope(requestsScopeSpan.Scope())

	urlLib3ScopeSpan := loadGeneratorResourceSpan.ScopeSpans().AppendEmpty()
	fillUrlLib3Scope(urlLib3ScopeSpan.Scope())

	// 4. Add http scope to frontend resource
	httpScopeSpan := frontEndResourceSpan.ScopeSpans().AppendEmpty()
	fillHttpScope(httpScopeSpan.Scope())

	// 5. Add HTTP POST span 1 to requests scope
	httpPostSpan1 := requestsScopeSpan.Spans().AppendEmpty()
	fillHttpPostSpan1(httpPostSpan1)

	// 6. Add HTTP POST span 2 to urllib3 scope
	httpPostSpan2 := urlLib3ScopeSpan.Spans().AppendEmpty()
	fillHttpPostSpan2(httpPostSpan2)

	// 7. Add HTTP POST span 3 to http scope
	httpPostSpan3 := httpScopeSpan.Spans().AppendEmpty()
	fillHttpPostSpan3(httpPostSpan3)

	sample.Spans = payload.ExtractSpans()
}

func (sample *SampleTelemetry) generateLogs() {
	payload := LogsPayload{plog.NewLogs()}

	// Generate sample currency conversion trace:
	// 1. Set up currencyservice resource
	currencyResourceLog := payload.logs.ResourceLogs().AppendEmpty()
	fillCurrencyResource(currencyResourceLog.Resource())

	// 2. Add currencyservice scope to currencyservice resource
	currencyScopeLog := currencyResourceLog.ScopeLogs().AppendEmpty()
	fillCurrencyScope(currencyScopeLog.Scope())

	// 3. Add CurrencyService/Convert log to currencyservice scope
	fillCurrencyLog(currencyScopeLog.LogRecords().AppendEmpty())

	sample.Logs = payload.ExtractLogs()
}

func (sample *SampleTelemetry) generateMetrics() {
	payload := MetricsPayload{pmetric.NewMetrics()}

	// Generate sample currency conversion trace:
	// 1. Set up currencyservice resource
	currencyResourceMetric := payload.metrics.ResourceMetrics().AppendEmpty()
	fillCurrencyResource(currencyResourceMetric.Resource())

	// 2. Add currencyservice scope to currencyservice resource
	currencyScopeMetric := currencyResourceMetric.ScopeMetrics().AppendEmpty()
	fillCurrencyScope(currencyScopeMetric.Scope())

	// 3. Add CurrencyService/Convert span to currencyservice scope
	currencyMetric := currencyScopeMetric.Metrics().AppendEmpty()
	fillCurrencyMetric(currencyMetric)

	// TODO: add different kinds of metrics

	sample.Metrics = payload.ExtractMetrics()
}

func fillCurrencyMetric(metric pmetric.Metric) {
	metric.SetDescription("amount requested")
	metric.SetName("amount")
	metric.SetUnit("dollar")
	sum := metric.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	pt := sum.DataPoints().AppendEmpty()
	pt.SetDoubleValue(1.9)
}

func fillCurrencyLog(log plog.LogRecord) {
	log.Attributes().PutBool("bool-attr", true)
	log.SetDroppedAttributesCount(99)
	log.SetFlags(plog.DefaultLogRecordFlags)
	log.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	log.SetSeverityNumber(plog.SeverityNumberError)
	log.SetSeverityText("ERROR")
	log.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	log.SetTraceID(encodeTraceID("7979cec4d1c04222fa9a3c7c97c0a99c"))
	log.SetSpanID(encodeSpanID("2c1ae93af4d3f887"))
	log.Body().SetStr("something with currency happened")
}

// currencyservice resource data
func fillCurrencyResource(resource pcommon.Resource) {
	resource.SetDroppedAttributesCount(0)
	resource.Attributes().PutStr("service.name", "sample-currencyservice")
	resource.Attributes().PutStr("telemetry.sdk.language", "cpp")
	resource.Attributes().PutStr("telemetry.sdk.name", "opentelemetry")
	resource.Attributes().PutStr("telemetry.sdk.version", "1.5.0")
}

// loadgenerator resource data
func fillLoadGeneratorResource(resource pcommon.Resource) {
	resource.SetDroppedAttributesCount(0)
	resource.Attributes().PutStr("service.name", "sample-loadgenerator")
	resource.Attributes().PutStr("telemetry.sdk.language", "python")
	resource.Attributes().PutStr("telemetry.sdk.name", "opentelemetry")
	resource.Attributes().PutStr("telemetry.sdk.version", "1.9.1")
}

// frontend resource data
func fillFrontEndResource(resource pcommon.Resource) {
	resource.SetDroppedAttributesCount(0)
	resource.Attributes().PutStr("service.name", "sample-frontend")
	resource.Attributes().PutStr("process.command", "/app/server.js")
	resource.Attributes().PutStr("process.command_line", "/usr/local/bin/node /app/server.js")
	resource.Attributes().PutStr("process.executable.name", "node")
	resource.Attributes().PutInt("process.pid", 17)
	resource.Attributes().PutStr("process.runtime.description", "Node.js")
	resource.Attributes().PutStr("process.runtime.name", "nodejs")
	resource.Attributes().PutStr("process.runtime.version", "18.12.1")
	resource.Attributes().PutStr("telemetry.sdk.language", "nodejs")
	resource.Attributes().PutStr("telemetry.sdk.name", "opentelemetry")
	resource.Attributes().PutStr("telemetry.sdk.version", "1.7.0")
}

// currencyservice scope data
func fillCurrencyScope(scope pcommon.InstrumentationScope) {
	scope.SetDroppedAttributesCount(0)
	scope.SetName("sample-currencyservice")
	scope.SetVersion("v1.2.3")
}

// requests scope data
func fillRequestsScope(scope pcommon.InstrumentationScope) {
	scope.SetDroppedAttributesCount(0)
	scope.SetName("sample-opentelemetry.instrumentation.requests")
	scope.SetVersion("0.28b1")
}

// urllib3 scope data
func fillUrlLib3Scope(scope pcommon.InstrumentationScope) {
	scope.SetDroppedAttributesCount(0)
	scope.SetName("sample-opentelemetry.instrumentation.urllib3")
	scope.SetVersion("0.28b1")
}

// http scope data
func fillHttpScope(scope pcommon.InstrumentationScope) {
	scope.SetDroppedAttributesCount(0)
	scope.SetName("sample-@opentelemetry/instrumentation-http")
	scope.SetVersion("0.32.0")
}

// CurrencyService/Convert span data
func fillCurrencySpan(span ptrace.Span) {
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetDroppedLinksCount(0)
	span.SetName("sample-CurrencyService/Convert")
	span.SetKind(ptrace.SpanKindServer)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179498174, time.UTC)))
	span.Status().SetCode(ptrace.StatusCodeOk)
	span.SetTraceID(encodeTraceID("7979cec4d1c04222fa9a3c7c97c0a99c"))
	span.SetSpanID(encodeSpanID("2c1ae93af4d3f887"))
	span.Attributes().PutStr("currency.conversion.from", "USD")
	span.Attributes().PutStr("currency.conversion.to", "CAD")
	span.Attributes().PutStr("rpc.system", "grpc")

	// Event data for CurrencyService/Convert
	conversionRequestEvent := span.Events().AppendEmpty()
	conversionRequestEvent.SetDroppedAttributesCount(0)
	conversionRequestEvent.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179475132, time.UTC)))
	conversionRequestEvent.SetName("sample-Processing currency conversion request")

	conversionSuccessEvent := span.Events().AppendEmpty()
	conversionSuccessEvent.SetDroppedAttributesCount(0)
	conversionSuccessEvent.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179479924, time.UTC)))
	conversionSuccessEvent.SetName("sample-Conversion successful. Response sent back.")
}

// HTTP POST span data (client, root)
func fillHttpPostSpan1(span ptrace.Span) {
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetDroppedLinksCount(0)
	span.SetName("sample-HTTP POST")
	span.SetKind(ptrace.SpanKindClient)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 803511676, time.UTC)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 817351051, time.UTC)))
	span.Status().SetCode(ptrace.StatusCodeUnset)
	span.SetTraceID(encodeTraceID("42957c7c2fca940a0d32a0cdd38c06a4"))
	span.SetSpanID(encodeSpanID("37fd1349bf83d330"))
	span.Attributes().PutStr("http.method", "POST")
	span.Attributes().PutInt("http.status_code", 200)
	span.Attributes().PutStr("http.url", "http://frontend:8080/api/cart")
}

// HTTP POST span data (client, child)
func fillHttpPostSpan2(span ptrace.Span) {
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetDroppedLinksCount(0)
	span.SetName("sample-HTTP POST")
	span.SetKind(ptrace.SpanKindClient)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 804417635, time.UTC)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 816959885, time.UTC)))
	span.Status().SetCode(ptrace.StatusCodeUnset)
	span.SetTraceID(encodeTraceID("42957c7c2fca940a0d32a0cdd38c06a4"))
	span.SetParentSpanID(encodeSpanID("37fd1349bf83d330"))
	span.SetSpanID(encodeSpanID("a24ac1588d52a6fc"))
	span.Attributes().PutStr("http.method", "POST")
	span.Attributes().PutInt("http.status_code", 200)
	span.Attributes().PutStr("http.url", "http://frontend:8080/api/cart")
}

// HTTP POST span data (server, child)
func fillHttpPostSpan3(span ptrace.Span) {
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetDroppedLinksCount(0)
	span.SetName("sample-HTTP POST")
	span.SetKind(ptrace.SpanKindServer)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 805039872, time.UTC)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 816274688, time.UTC)))
	span.Status().SetCode(ptrace.StatusCodeUnset)
	span.SetTraceID(encodeTraceID("42957c7c2fca940a0d32a0cdd38c06a4"))
	span.SetParentSpanID(encodeSpanID("a24ac1588d52a6fc"))
	span.SetSpanID(encodeSpanID("355dc9bea1ec64d8"))
	span.Attributes().PutStr("http.flavor", "1.1")
	span.Attributes().PutStr("http.host", "frontend:8080")
	span.Attributes().PutStr("http.method", "POST")
	span.Attributes().PutInt("http.request_length", 102)
	span.Attributes().PutInt("http.status_code", 200)
	span.Attributes().PutStr("http.status_text", "Ok")
	span.Attributes().PutStr("http.target", "/api/cart")
	span.Attributes().PutStr("http.url", "http://frontend:8080/api/cart")
	span.Attributes().PutStr("http.user_agent", "python-requests/2.27.1")
	span.Attributes().PutStr("net.host.ip", "::ffff:172.24.0.22")
	span.Attributes().PutStr("net.host.name", "frontend")
	span.Attributes().PutInt("net.host.port", 8080)
	span.Attributes().PutStr("net.peer.ip", "::ffff:172.24.0.23")
	span.Attributes().PutInt("net.peer.port", 46054)
	span.Attributes().PutStr("net.transport", "ip_tcp")
}

func encodeTraceID(traceID string) [16]byte {
	var byteArray [16]byte
	byteSlice, err := hex.DecodeString(traceID)
	if err != nil {
		log.Fatal(err)
	}
	copy(byteArray[:], byteSlice[:16])
	return byteArray
}

func encodeSpanID(spanID string) [8]byte {
	var byteArray [8]byte
	byteSlice, err := hex.DecodeString(spanID)
	if err != nil {
		log.Fatal(err)
	}
	copy(byteArray[:], byteSlice[:8])
	return byteArray
}
