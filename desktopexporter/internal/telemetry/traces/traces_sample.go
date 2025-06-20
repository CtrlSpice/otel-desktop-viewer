package traces

import (
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/util"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func GenerateSampleTraces() []SpanData {
	payload := NewSpanPayload(ptrace.NewTraces())
	payload.Traces.ResourceSpans().EnsureCapacity(3)

	// Generate sample currency conversion trace:
	// 1. Set up currencyservice resource
	currencyResourceSpan := payload.Traces.ResourceSpans().AppendEmpty()
	resource.FillCurrencyResource(currencyResourceSpan.Resource())

	// 2. Add currencyservice scope to currencyservice resource
	currencyScopeSpan := currencyResourceSpan.ScopeSpans().AppendEmpty()
	scope.FillCurrencyScope(currencyScopeSpan.Scope())

	// 3. Add CurrencyService/Convert span to currencyservice scope
	currencySpan := currencyScopeSpan.Spans().AppendEmpty()
	fillCurrencySpan(currencySpan)

	// Generate sample HTTP POST trace:
	// 1. Set up loadgenerator resource
	loadGeneratorResourceSpan := payload.Traces.ResourceSpans().AppendEmpty()
	resource.FillLoadGeneratorResource(loadGeneratorResourceSpan.Resource())

	// 2. Set up frontend resource
	frontEndResourceSpan := payload.Traces.ResourceSpans().AppendEmpty()
	resource.FillFrontEndResource(frontEndResourceSpan.Resource())

	// 3. Add requests and urllib3 scopes to loadgenerator resource
	loadGeneratorResourceSpan.ScopeSpans().EnsureCapacity(2)
	requestsScopeSpan := loadGeneratorResourceSpan.ScopeSpans().AppendEmpty()
	scope.FillRequestsScope(requestsScopeSpan.Scope())

	urlLib3ScopeSpan := loadGeneratorResourceSpan.ScopeSpans().AppendEmpty()
	scope.FillUrlLib3Scope(urlLib3ScopeSpan.Scope())

	// 4. Add http scope to frontend resource
	httpScopeSpan := frontEndResourceSpan.ScopeSpans().AppendEmpty()
	scope.FillHttpScope(httpScopeSpan.Scope())

	// 5. Add HTTP POST span 1 to requests scope
	httpPostSpan1 := requestsScopeSpan.Spans().AppendEmpty()
	fillHttpPostSpan1(httpPostSpan1)

	// 6. Add HTTP POST span 2 to urllib3 scope
	httpPostSpan2 := urlLib3ScopeSpan.Spans().AppendEmpty()
	fillHttpPostSpan2(httpPostSpan2)

	// 7. Add HTTP POST span 3 to http scope
	httpPostSpan3 := httpScopeSpan.Spans().AppendEmpty()
	fillHttpPostSpan3(httpPostSpan3)

	return payload.ExtractSpans()
}

// CurrencyService/Convert span data
func fillCurrencySpan(span ptrace.Span) {
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetDroppedLinksCount(0)
	span.SetName("sample.CurrencyService/Convert")
	span.SetKind(ptrace.SpanKindServer)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179498174, time.UTC)))
	span.Status().SetCode(ptrace.StatusCodeOk)

	span.SetTraceID(util.ToTraceID("7979cec4d1c04222fa9a3c7c97c0a99c"))
	span.SetSpanID(util.ToSpanID("2c1ae93af4d3f887"))

	span.Attributes().PutStr("currency.conversion.from", "USD")
	span.Attributes().PutStr("currency.conversion.to", "CAD")
	span.Attributes().PutStr("rpc.system", "grpc")

	// Event data for CurrencyService/Convert
	conversionRequestEvent := span.Events().AppendEmpty()
	conversionRequestEvent.SetDroppedAttributesCount(0)
	conversionRequestEvent.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179475132, time.UTC)))
	conversionRequestEvent.SetName("Processing currency conversion request")
	conversionRequestEvent.Attributes().PutStr("event.class", "sample")

	conversionSuccessEvent := span.Events().AppendEmpty()
	conversionSuccessEvent.SetDroppedAttributesCount(1)
	conversionSuccessEvent.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179479924, time.UTC)))
	conversionSuccessEvent.SetName("Conversion successful. Response sent back.")
	conversionSuccessEvent.Attributes().PutStr("event.class", "sample")
	conversionSuccessEvent.Attributes().PutInt("event.priority", 1)
}

// HTTP POST span data (client, root)
func fillHttpPostSpan1(span ptrace.Span) {
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetDroppedLinksCount(0)
	span.SetName("SAMPLE HTTP POST")
	span.SetKind(ptrace.SpanKindClient)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 803511676, time.UTC)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 817351051, time.UTC)))
	span.Status().SetCode(ptrace.StatusCodeUnset)

	span.SetTraceID(util.ToTraceID("42957c7c2fca940a0d32a0cdd38c06a4"))
	span.SetSpanID(util.ToSpanID("37fd1349bf83d330"))

	span.Attributes().PutStr("http.method", "POST")
	span.Attributes().PutInt("http.status_code", 200)
	span.Attributes().PutStr("http.url", "http://frontend:8080/api/cart")
}

// HTTP POST span data (client, child)
func fillHttpPostSpan2(span ptrace.Span) {
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetDroppedLinksCount(0)
	span.SetName("SAMPLE HTTP POST")
	span.SetKind(ptrace.SpanKindClient)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 804417635, time.UTC)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 816959885, time.UTC)))
	span.Status().SetCode(ptrace.StatusCodeUnset)

	span.SetTraceID(util.ToTraceID("42957c7c2fca940a0d32a0cdd38c06a4"))
	span.SetParentSpanID(util.ToSpanID("37fd1349bf83d330"))
	span.SetSpanID(util.ToSpanID("a24ac1588d52a6fc"))

	span.Attributes().PutStr("http.method", "POST")
	span.Attributes().PutInt("http.status_code", 200)
	span.Attributes().PutStr("http.url", "http://frontend:8080/api/cart")

	link := span.Links().AppendEmpty()
	link.SetSpanID(util.ToSpanID("2c1ae93af4d3f887"))
	link.SetTraceID(util.ToTraceID("7979cec4d1c04222fa9a3c7c97c0a99c"))
	link.SetDroppedAttributesCount(5)
	link.Attributes().PutStr("relationship", "in-cart currency conversion")
}

// HTTP POST span data (server, child)
func fillHttpPostSpan3(span ptrace.Span) {
	span.SetDroppedAttributesCount(0)
	span.SetDroppedEventsCount(0)
	span.SetDroppedLinksCount(0)
	span.SetName("SAMPLE HTTP POST")
	span.SetKind(ptrace.SpanKindServer)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 805039872, time.UTC)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 816274688, time.UTC)))
	span.Status().SetCode(ptrace.StatusCodeUnset)

	span.SetTraceID(util.ToTraceID("42957c7c2fca940a0d32a0cdd38c06a4"))
	span.SetParentSpanID(util.ToSpanID("a24ac1588d52a6fc"))
	span.SetSpanID(util.ToSpanID("355dc9bea1ec64d8"))

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
	// Add array.example attribute to span
	// I realise this looks funky, but that's how pcommon.Value is implemented
	attr := span.Attributes().PutEmptySlice("array.example")
	attr.EnsureCapacity(3)
	attr.AppendEmpty().SetDouble(1.1)
	attr.AppendEmpty().SetDouble(1.2)
	attr.AppendEmpty().SetDouble(1.3)
}
