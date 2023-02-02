package desktopexporter

import (
	"context"
	"encoding/hex"
	"log"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func GenerateSampleData(ctx context.Context) []SpanData {
	traceData := ptrace.NewTraces()

	traceData.ResourceSpans().EnsureCapacity(1)

	// Resource data for currencyservice
	currencyResourceSpan := traceData.ResourceSpans().AppendEmpty()
	currencyResource := currencyResourceSpan.Resource()
	currencyResource.SetDroppedAttributesCount(0)
	currencyResource.Attributes().PutStr("service.name", "SAMPLE: currencyservice")
	currencyResource.Attributes().PutStr("telemetry.sdk.language", "cpp")
	currencyResource.Attributes().PutStr("telemetry.sdk.name", "opentelemetry")
	currencyResource.Attributes().PutStr("telemetry.sdk.version", "1.5.0")

	// Scope data for currencyservice
	currencyResourceSpan.ScopeSpans().EnsureCapacity(1)
	currencyScopeSpan := currencyResourceSpan.ScopeSpans().AppendEmpty()
	currencyScope := currencyScopeSpan.Scope()
	currencyScope.SetDroppedAttributesCount(0)
	currencyScope.SetName("SAMPLE: currencyservice")
	currencyScope.SetVersion("v1.2.3")

	// Span data for CurrencyService/Convert
	currencyScopeSpan.Spans().EnsureCapacity(1)
	currencySpan := currencyScopeSpan.Spans().AppendEmpty()
	currencySpan.SetDroppedAttributesCount(0)
	currencySpan.SetDroppedEventsCount(0)
	currencySpan.SetDroppedLinksCount(0)
	currencySpan.SetName("SAMPLE: CurrencyService/Convert")
	currencySpan.SetKind(ptrace.SpanKindServer)
	currencySpan.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)))
	currencySpan.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179498174, time.UTC)))
	currencySpan.Status().SetCode(ptrace.StatusCodeOk)
	currencySpan.SetTraceID(encodeTraceID("7979cec4d1c04222fa9a3c7c97c0a99c"))
	currencySpan.SetSpanID(encodeSpanID("2c1ae93af4d3f887"))
	currencySpan.Attributes().PutStr("currency.conversion.from", "USD")
	currencySpan.Attributes().PutStr("currency.conversion.to", "CAD")
	currencySpan.Attributes().PutStr("rpc.system", "grpc")

	// Event data for CurrencyService/Convert
	conversionRequestEvent := currencySpan.Events().AppendEmpty()
	conversionRequestEvent.SetDroppedAttributesCount(0)
	conversionRequestEvent.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179475132, time.UTC)))
	conversionRequestEvent.SetName("SAMPLE: Processing currency conversion request")

	conversionSuccessEvent := currencySpan.Events().AppendEmpty()
	conversionSuccessEvent.SetDroppedAttributesCount(0)
	conversionSuccessEvent.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179479924, time.UTC)))
	conversionSuccessEvent.SetName("SAMPLE: Conversion successful. Response sent back.")

	spanData := extractSpans(ctx, traceData)
	return spanData
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
