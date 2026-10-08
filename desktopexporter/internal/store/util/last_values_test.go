package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestEncodeValueSelectsLastKeysRecursivelyWithoutChangingInput(t *testing.T) {
	raw := []byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"kvlistValue":{"values":[
{"key":"x","value":{"stringValue":"old"}},
{"key":"x","value":{"intValue":"9223372036854775807"}},
{"key":"X","value":{"boolValue":true}},
{"key":"nested","value":{"arrayValue":{"values":[
{"kvlistValue":{"values":[{"key":"k","value":{"intValue":"1"}},{"key":"k","value":{"doubleValue":-0.0}}]}},
{"intValue":"2"},{"intValue":"2"}]}}}
]}}}]}]}]}`)
	logs, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(raw)
	require.NoError(t, err)
	before, err := (&plog.JSONMarshaler{}).MarshalLogs(logs)
	require.NoError(t, err)
	value := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body()
	encoded, err := EncodeValue(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"map","value":[
{"key":"X","value":{"kind":"bool","value":true}},
{"key":"nested","value":{"kind":"array","value":[
{"kind":"map","value":[{"key":"k","value":{"kind":"double","value":"0x8000000000000000"}}]},
{"kind":"int64","value":"2"},{"kind":"int64","value":"2"}]}},
{"key":"x","value":{"kind":"int64","value":"9223372036854775807"}}]}`, string(encoded))
	after, err := (&plog.JSONMarshaler{}).MarshalLogs(logs)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	last, found := LastValue(value.Map(), "x")
	require.True(t, found)
	assert.Equal(t, int64(9223372036854775807), last.Int())
	_, found = LastValue(value.Map(), "absent")
	assert.False(t, found)
}
