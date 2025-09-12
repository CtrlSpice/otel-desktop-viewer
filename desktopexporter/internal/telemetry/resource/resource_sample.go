package resource

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// currencyservice resource data
func FillCurrencyResource(resource pcommon.Resource) {
	resource.SetDroppedAttributesCount(0)
	resource.Attributes().PutStr("service.name", "sample.currencyservice")
	resource.Attributes().PutStr("telemetry.sdk.language", "cpp")
	resource.Attributes().PutStr("telemetry.sdk.name", "opentelemetry")
	resource.Attributes().PutStr("telemetry.sdk.version", "1.5.0")
	resource.Attributes().PutBool("telemetry.sample", true)
	attr := resource.Attributes().PutEmptySlice("array.example")
	attr.EnsureCapacity(3)
	attr.AppendEmpty().SetStr("example1")
	attr.AppendEmpty().SetStr("example2")
	attr.AppendEmpty().SetStr("example3")
}

// loadgenerator resource data
func FillLoadGeneratorResource(resource pcommon.Resource) {
	resource.SetDroppedAttributesCount(0)
	resource.Attributes().PutStr("service.name", "sample-loadgenerator")
	resource.Attributes().PutStr("telemetry.sdk.language", "python")
	resource.Attributes().PutStr("telemetry.sdk.name", "opentelemetry")
	resource.Attributes().PutStr("telemetry.sdk.version", "1.9.1")
	resource.Attributes().PutBool("telemetry.sample", true)
}

// frontend resource data
func FillFrontEndResource(resource pcommon.Resource) {
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
	resource.Attributes().PutBool("telemetry.sample", true)
}
