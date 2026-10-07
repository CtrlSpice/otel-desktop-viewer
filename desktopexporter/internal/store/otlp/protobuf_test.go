package otlp_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/otlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
)

func TestMarshalProtoKeepsTypedValuesAndLiteralIDKeys(t *testing.T) {
	raw := json.RawMessage(`{"resourceLogs":[{"schemaUrl":"resource-schema","scopeLogs":[{"schemaUrl":"scope-schema","logRecords":[{"traceId":"FEDCBA98765432100123456789ABCDEF","spanId":"FFFFFFFFFFFFFFFF","timeUnixNano":"18446744073709551615","severityNumber":-99,"body":{"kvlistValue":{"values":[{"key":"traceId","value":{"stringValue":"not a hexadecimal identifier"}},{"key":"parentSpanId","value":{"arrayValue":{"values":[{"intValue":"9223372036854775807"},{"doubleValue":-0.0},{"doubleValue":"NaN"},{"doubleValue":"Infinity"},{"doubleValue":"-Infinity"}]} }},{"key":"traceId","value":{"bytesValue":"+/8="}}]}}}]}]}]}`)
	original := string(raw)
	wire, err := otlp.MarshalProto(raw, &collectorlogs.ExportLogsServiceRequest{})
	require.NoError(t, err)
	assert.Equal(t, original, string(raw), "JSON export bytes must remain unchanged")
	decoded, err := (&plog.ProtoUnmarshaler{}).UnmarshalLogs(wire)
	require.NoError(t, err)
	rl := decoded.ResourceLogs().At(0)
	assert.Equal(t, "resource-schema", rl.SchemaUrl())
	sl := rl.ScopeLogs().At(0)
	assert.Equal(t, "scope-schema", sl.SchemaUrl())
	record := sl.LogRecords().At(0)
	assert.Equal(t, "fedcba98765432100123456789abcdef", record.TraceID().String())
	assert.Equal(t, "ffffffffffffffff", record.SpanID().String())
	assert.Equal(t, uint64(math.MaxUint64), uint64(record.Timestamp()))
	assert.Equal(t, plog.SeverityNumber(-99), record.SeverityNumber())
	assert.Equal(t, 3, record.Body().Map().Len(), "duplicate attribute keys remain entries")
	values, ok := record.Body().Map().Get("parentSpanId")
	require.True(t, ok)
	assert.Equal(t, int64(math.MaxInt64), values.Slice().At(0).Int())
	assert.Equal(t, uint64(1<<63), math.Float64bits(values.Slice().At(1).Double()))
	assert.True(t, math.IsNaN(values.Slice().At(2).Double()))
	assert.True(t, math.IsInf(values.Slice().At(3).Double(), 1))
	assert.True(t, math.IsInf(values.Slice().At(4).Double(), -1))
}

func TestMarshalProtoRejectsInvalidReconstruction(t *testing.T) {
	for _, raw := range []string{
		``,
		`{"resourceLogs":[`,
		`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"traceId":"zz"}]}]}]}`,
		`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"spanId":42}]}]}]}`,
		`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"18446744073709551616"}]}]}]}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := otlp.MarshalProto(json.RawMessage(raw), &collectorlogs.ExportLogsServiceRequest{})
			require.Error(t, err)
		})
	}
}
