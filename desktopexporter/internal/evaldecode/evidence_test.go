package evaldecode

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
)

func TestEvidenceExactValuesAndOTLPSerialization(t *testing.T) {
	raw, err := os.ReadFile("testdata/promptfoo-v3.json")
	require.NoError(t, err)
	var parts []IngestPart
	require.NoError(t, ParseIngest(t.Context(), strings.NewReader(string(raw)), func(p IngestPart) error { parts = append(parts, p); return nil }))
	require.Len(t, parts, 2)
	for _, part := range parts {
		data, err := Encode(strings.Repeat("a", 64), part)
		require.NoError(t, err)
		pb, err := plogotlp.NewExportRequestFromLogs(data).MarshalProto()
		require.NoError(t, err)
		request := plogotlp.NewExportRequest()
		require.NoError(t, request.UnmarshalProto(pb))
		wire, err := (&plog.JSONMarshaler{}).MarshalLogs(request.Logs())
		require.NoError(t, err)
		replayed, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(wire)
		require.NoError(t, err)
		log := replayed.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
		require.True(t, log.TraceID().IsEmpty())
		require.True(t, log.SpanID().IsEmpty())
		require.Zero(t, log.Timestamp())
		got, err := Decode(log)
		require.NoError(t, err)
		// RawMessage comparison preserves lexemes; JSONEq would round numbers.
		var want, actual map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(part.Payload, &want))
		require.NoError(t, json.Unmarshal(got.Payload, &actual))
		for _, key := range []string{"large", "negativeZero", "decimal", "null", "stringNumber", "success"} {
			require.Equal(t, want[key], actual[key], key)
		}
		require.NotContains(t, actual, "missing")
	}
}

func TestEvidenceRejectsChangedTypeMetadataAndNumericLeaves(t *testing.T) {
	for _, mutate := range []func(plog.LogRecord){
		func(l plog.LogRecord) { l.Body().Map().PutStr("version", "2") },
		func(l plog.LogRecord) { l.Body().Map().Remove("numberPaths") },
		func(l plog.LogRecord) { l.Body().Map().PutStr("numberPathsHash", "wrong") },
		func(l plog.LogRecord) { v, _ := l.Body().Map().Get("source"); v.Map().PutStr("value", "1 trailing") },
		func(l plog.LogRecord) { v, _ := l.Body().Map().Get("source"); v.Map().Remove("value") },
		func(l plog.LogRecord) { l.Body().Map().PutStr("ordinal", "01") },
	} {
		data, err := Encode(strings.Repeat("b", 64), IngestPart{Row: 0, Payload: json.RawMessage(`{"value":-0}`)})
		require.NoError(t, err)
		log := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
		mutate(log)
		_, err = Decode(log)
		require.Error(t, err)
	}
}

func TestNumberPathMetadataIsLinearUnderLongSharedKeys(t *testing.T) {
	key := strings.Repeat("a", 65536)
	part := IngestPart{Payload: json.RawMessage(`{"` + key + `":[` + strings.Repeat("0,", 10000) + `0]}`)}
	data, err := Encode(strings.Repeat("c", 64), part)
	require.NoError(t, err)
	wire, err := plogotlp.NewExportRequestFromLogs(data).MarshalProto()
	require.NoError(t, err)
	require.Less(t, len(wire), 1<<20)
	_, err = Decode(data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0))
	require.NoError(t, err)
}

func TestDenseNumericArrayFitsExistingReceiverLimit(t *testing.T) {
	// The smallest JSON numbers are the worst case for per-leaf wire overhead.
	// Positional array metadata avoids repeating millions of index strings.
	raw := []byte(`{"values":[` + strings.Repeat("0,", 100000) + `0]}`)
	data, err := Encode(strings.Repeat("e", 64), IngestPart{Payload: raw})
	require.NoError(t, err)
	wire, err := plogotlp.NewExportRequestFromLogs(data).MarshalProto()
	require.NoError(t, err)
	require.Less(t, len(wire), 5*len(raw))
	_, err = Decode(data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0))
	require.NoError(t, err)
}
