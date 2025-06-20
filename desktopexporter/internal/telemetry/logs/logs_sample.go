package logs

import (
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/resource"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/scope"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry/util"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func GenerateSampleLogs() []LogData {
	payload := NewLogsPayload(plog.NewLogs())

	// Generate currency service logs
	currencyResourceLog := payload.Logs.ResourceLogs().AppendEmpty()
	resource.FillCurrencyResource(currencyResourceLog.Resource())
	currencyScopeLog := currencyResourceLog.ScopeLogs().AppendEmpty()
	scope.FillCurrencyScope(currencyScopeLog.Scope())
	fillCurrencyLog(currencyScopeLog.LogRecords().AppendEmpty())

	// Generate HTTP service logs
	httpResourceLog := payload.Logs.ResourceLogs().AppendEmpty()
	resource.FillLoadGeneratorResource(httpResourceLog.Resource())
	httpScopeLog := httpResourceLog.ScopeLogs().AppendEmpty()
	scope.FillRequestsScope(httpScopeLog.Scope())
	fillHttpLog(httpScopeLog.LogRecords().AppendEmpty())

	// Generate system logs
	systemResourceLog := payload.Logs.ResourceLogs().AppendEmpty()
	resource.FillFrontEndResource(systemResourceLog.Resource())
	systemScopeLog := systemResourceLog.ScopeLogs().AppendEmpty()
	scope.FillHttpScope(systemScopeLog.Scope())
	fillSystemLog(systemScopeLog.LogRecords().AppendEmpty())

	return payload.ExtractLogs()
}

func fillCurrencyLog(log plog.LogRecord) {
	log.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179472007, time.UTC)))
	log.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 01, 20, 25, 36, 179498174, time.UTC)))
	log.SetTraceID(util.ToTraceID("7979cec4d1c04222fa9a3c7c97c0a99c"))
	log.SetSpanID(util.ToSpanID("2c1ae93af4d3f887"))
	log.SetSeverityNumber(plog.SeverityNumberError)
	log.SetSeverityText("ERROR")
	log.SetEventName("currency.conversion.failed")
	log.Body().SetStr("Currency conversion failed: invalid amount")
	log.Attributes().PutStr("currency.from", "USD")
	log.Attributes().PutStr("currency.to", "CAD")
	log.Attributes().PutDouble("currency.amount", 100.50)
	log.SetDroppedAttributesCount(0)
	log.SetFlags(plog.LogRecordFlags(1))
}

func fillHttpLog(log plog.LogRecord) {
	log.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 803511676, time.UTC)))
	log.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 02, 18, 17, 54, 817351051, time.UTC)))
	log.SetTraceID(util.ToTraceID("42957c7c2fca940a0d32a0cdd38c06a4"))
	log.SetSpanID(util.ToSpanID("37fd1349bf83d330"))
	log.SetSeverityNumber(plog.SeverityNumberInfo)
	log.SetSeverityText("INFO")
	log.SetEventName("http.request.completed")
	log.Body().SetStr("HTTP request completed")
	log.Attributes().PutStr("http.method", "POST")
	log.Attributes().PutInt("http.status_code", 200)
	log.Attributes().PutStr("http.url", "http://frontend:8080/api/cart")
	log.SetDroppedAttributesCount(0)
	log.SetFlags(plog.DefaultLogRecordFlags)
}

func fillSystemLog(log plog.LogRecord) {
	log.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 03, 10, 15, 30, 0, time.UTC)))
	log.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Date(2023, 02, 03, 10, 15, 30, 100000, time.UTC)))
	log.SetSeverityNumber(plog.SeverityNumberWarn)
	log.SetSeverityText("WARN")
	log.SetEventName("system.memory.high")
	log.Body().SetStr("High memory usage detected")
	log.Attributes().PutDouble("system.memory.used", 85.5)
	log.Attributes().PutDouble("system.memory.total", 100.0)
	log.Attributes().PutStr("system.component", "cache")
	log.SetDroppedAttributesCount(0)
	log.SetFlags(plog.DefaultLogRecordFlags)
}
