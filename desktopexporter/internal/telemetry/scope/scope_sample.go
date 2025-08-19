package scope

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// currencyservice scope data
func FillCurrencyScope(scope pcommon.InstrumentationScope) {
	scope.SetDroppedAttributesCount(2)
	scope.SetName("sample.currencyservice")
	scope.SetVersion("v1.2.3")
	scope.Attributes().PutStr("owner.name", "Mila Ardath")
	scope.Attributes().PutStr("owner.contact", "github.com/CtrlSpice")
}

// requests scope data
func FillRequestsScope(scope pcommon.InstrumentationScope) {
	scope.SetDroppedAttributesCount(0)
	scope.SetName("sample.opentelemetry.instrumentation.requests")
	scope.SetVersion("0.28b1")
}

// urllib3 scope data
func FillUrlLib3Scope(scope pcommon.InstrumentationScope) {
	scope.SetDroppedAttributesCount(0)
	scope.SetName("sample.opentelemetry.instrumentation.urllib3")
	scope.SetVersion("0.28b1")
}

// http scope data
func FillHttpScope(scope pcommon.InstrumentationScope) {
	scope.SetDroppedAttributesCount(0)
	scope.SetName("sample.@opentelemetry/instrumentation-http")
	scope.SetVersion("0.32.0")
}
