package logs_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

const (
	otlpLogTraceID = "fedcba98765432100123456789abcdef"
	otlpLogSpanID  = "ffffffffffffffff"
)

var (
	otlpLogHex16   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	otlpLogHex32   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	otlpLogDecimal = regexp.MustCompile(`^-?[0-9]+$`)
)

func TestGetLogOTLP(t *testing.T) {
	s, ctx := storetest.New(t)
	received := otlpLogFixture()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, received, s.FlushedIDs())
	}))
	primaryID := lookupLogID(t, s, ctx, "primary.event")

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetLogOTLP(ctx, db, primaryID)
	})
	require.NoError(t, err)
	require.NoError(t, validateLogOTLP(raw))

	direct := string(raw)
	assert.Contains(t, direct, `"timeUnixNano":"18446744073709551614"`)
	assert.Contains(t, direct, `"observedTimeUnixNano":"18446744073709551615"`)
	assert.Contains(t, direct, `"severityNumber":99`)
	assert.Contains(t, direct, `"traceId":"`+otlpLogTraceID+`"`)
	assert.Contains(t, direct, `"spanId":"`+otlpLogSpanID+`"`)
	assert.Contains(t, direct, `"intValue":"-9223372036854775808"`)
	assert.Contains(t, direct, `"doubleValue":-0.0`)
	assert.Contains(t, direct, `"doubleValue":"NaN"`)
	assert.Contains(t, direct, `"doubleValue":"Infinity"`)
	assert.Contains(t, direct, `"doubleValue":"-Infinity"`)
	assert.Contains(t, direct, `"bytesValue":"+/8="`)
	assert.Contains(t, direct, `"key":"MiXeD_snake\n\"é"`)
	assert.Contains(t, direct, `"key":"null"`)
	assert.NotContains(t, direct, `"traceID"`)
	assert.NotContains(t, direct, `:null`)

	decoded, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(raw)
	require.NoError(t, err)
	require.Equal(t, 1, decoded.ResourceLogs().Len())
	rl := decoded.ResourceLogs().At(0)
	assert.Equal(t, "https://example.test/resource/log/v2", rl.SchemaUrl())
	assert.Equal(t, uint32(7), rl.Resource().DroppedAttributesCount())
	assert.Equal(t, "checkout", mustMapValue(t, rl.Resource().Attributes(), "service.name").Str())
	require.Equal(t, 1, rl.ScopeLogs().Len())
	sl := rl.ScopeLogs().At(0)
	assert.Equal(t, "https://example.test/scope/log/v2", sl.SchemaUrl())
	assert.Equal(t, "log-fixture", sl.Scope().Name())
	assert.Equal(t, "2.0.0", sl.Scope().Version())
	assert.Equal(t, uint32(8), sl.Scope().DroppedAttributesCount())
	assert.False(t, mustMapValue(t, sl.Scope().Attributes(), "scope.enabled").Bool())
	require.Equal(t, 1, sl.LogRecords().Len())
	record := sl.LogRecords().At(0)
	assert.Equal(t, pcommon.Timestamp(math.MaxUint64-1), record.Timestamp())
	assert.Equal(t, pcommon.Timestamp(math.MaxUint64), record.ObservedTimestamp())
	assert.Equal(t, plog.SeverityNumber(99), record.SeverityNumber())
	assert.Equal(t, "CUSTOM", record.SeverityText())
	assert.Equal(t, pcommon.TraceID(mustDecodeTraceIDLogs(otlpLogTraceID)), record.TraceID())
	assert.Equal(t, pcommon.SpanID(mustDecodeSpanIDLogs(otlpLogSpanID)), record.SpanID())
	assert.Equal(t, uint32(9), record.DroppedAttributesCount())
	assert.Equal(t, uint32(0x101), uint32(record.Flags()))
	assert.Equal(t, "primary.event", record.EventName())
	assert.Equal(t, pcommon.ValueTypeEmpty, mustMapValue(t, record.Attributes(), "empty").Type())
	assert.Equal(t, int64(math.MinInt64), mustMapValue(t, record.Attributes(), "minimum").Int())
	assert.True(t, math.Signbit(mustMapValue(t, record.Attributes(), "negative.zero").Double()))
	assert.True(t, math.IsNaN(mustMapValue(t, record.Attributes(), "nan").Double()))
	assert.True(t, math.IsInf(mustMapValue(t, record.Attributes(), "positive.infinity").Double(), 1))
	assert.True(t, math.IsInf(mustMapValue(t, record.Attributes(), "negative.infinity").Double(), -1))
	assert.Equal(t, []byte{0xfb, 0xff}, mustMapValue(t, record.Attributes(), "bytes").Bytes().AsRaw())
	assert.Equal(t, "literal", mustMapValue(t, record.Attributes(), "null").Str())

	body := record.Body().Map()
	assert.Equal(t, "unchanged", mustMapValue(t, body, "MiXeD_snake\n\"é").Str())
	items := mustMapValue(t, body, "items").Slice()
	require.Equal(t, 4, items.Len())
	assert.Equal(t, pcommon.ValueTypeEmpty, items.At(0).Type())
	assert.False(t, items.At(1).Bool())
	assert.Equal(t, int64(math.MaxInt64), items.At(2).Int())
	assert.Equal(t, 0, items.At(3).Map().Len())
	deep := mustMapValue(t, body, "deep")
	for range 200 {
		require.Equal(t, pcommon.ValueTypeSlice, deep.Type())
		require.Equal(t, 1, deep.Slice().Len())
		deep = deep.Slice().At(0)
	}
	assert.Equal(t, "bottom", deep.Str())

	for name, mutated := range map[string]string{
		"wrong casing":   strings.Replace(direct, `"traceId":`, `"traceID":`, 1),
		"enum name":      strings.Replace(direct, `"severityNumber":99`, `"severityNumber":"SEVERITY_NUMBER_INFO"`, 1),
		"base64 ID":      strings.Replace(direct, `"traceId":"`+otlpLogTraceID+`"`, `"traceId":"/ty6mHZUMhABI0VniavN7w=="`, 1),
		"unquoted int64": strings.Replace(direct, `"timeUnixNano":"18446744073709551614"`, `"timeUnixNano":18446744073709551614`, 1),
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			require.Error(t, validateLogOTLP([]byte(mutated)))
		})
	}
}

func TestGetLogOTLPBoundLookupAndAbsentCorrelation(t *testing.T) {
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, otlpLogFixture(), s.FlushedIDs())
	}))
	secondaryID := lookupLogID(t, s, ctx, "secondary.event")

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetLogOTLP(ctx, db, secondaryID)
	})
	require.NoError(t, err)
	require.NoError(t, validateLogOTLP(raw))
	assert.Contains(t, string(raw), `"stringValue":"secondary-only"`)
	assert.NotContains(t, string(raw), "primary.event")
	assert.NotContains(t, string(raw), `"traceId"`)
	assert.NotContains(t, string(raw), `"spanId"`)

	decoded, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(raw)
	require.NoError(t, err)
	rl := decoded.ResourceLogs().At(0)
	assert.Equal(t, "https://example.test/resource/log/v1", rl.SchemaUrl())
	assert.Equal(t, "secondary", mustMapValue(t, rl.Resource().Attributes(), "service.name").Str())
	record := rl.ScopeLogs().At(0).LogRecords().At(0)
	assert.Equal(t, pcommon.Timestamp(11), record.Timestamp())
	assert.Equal(t, pcommon.Timestamp(22), record.ObservedTimestamp())
	assert.True(t, record.TraceID().IsEmpty())
	assert.True(t, record.SpanID().IsEmpty())
}

func TestGetLogOTLPNotFound(t *testing.T) {
	s, ctx := storetest.New(t)
	for _, logID := range []string{"00000000-0000-0000-0000-000000000000", "not-a-log-id"} {
		_, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.GetLogOTLP(ctx, db, logID)
		})
		assert.ErrorIs(t, err, logs.ErrLogIDNotFound)
	}
}

func TestGetLogOTLPRejectsStoredSQLNull(t *testing.T) {
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, otlpLogFixture(), s.FlushedIDs())
	}))
	primaryID := lookupLogID(t, s, ctx, "primary.event")

	err := s.WithDBRead(func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx, `update logs set severity_text = null where id = ?::uuid`, primaryID); err != nil {
			return err
		}
		_, err := logs.GetLogOTLP(ctx, db, primaryID)
		return err
	})
	assert.ErrorIs(t, err, logs.ErrLogsStoreInternal)
	assert.ErrorContains(t, err, "OTLP document contains SQL NULL")
}

func TestGetLogOTLPRejectsDanglingAttribute(t *testing.T) {
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, otlpLogFixture(), s.FlushedIDs())
	}))
	primaryID := lookupLogID(t, s, ctx, "primary.event")

	err := s.WithDBRead(func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx, `
			delete from attributes
			where id = (select unnest(attribute_ids) from logs where id = ?::uuid limit 1)`, primaryID); err != nil {
			return err
		}
		_, err := logs.GetLogOTLP(ctx, db, primaryID)
		return err
	})
	assert.ErrorIs(t, err, logs.ErrLogsStoreInternal)
	assert.ErrorContains(t, err, "stored OTel value is SQL NULL")
}

func lookupLogID(t testing.TB, s *store.Store, ctx context.Context, eventName string) string {
	t.Helper()
	var id string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `select id::varchar from logs where event_name = ?`, eventName).Scan(&id)
	}))
	return id
}

func mustMapValue(t *testing.T, values pcommon.Map, key string) pcommon.Value {
	t.Helper()
	value, ok := values.Get(key)
	require.True(t, ok, "missing value %q", key)
	return value
}

func otlpLogFixture() plog.Logs {
	data := plog.NewLogs()
	primaryResource := data.ResourceLogs().AppendEmpty()
	primaryResource.SetSchemaUrl("https://example.test/resource/log/v2")
	primaryResource.Resource().SetDroppedAttributesCount(7)
	primaryResource.Resource().Attributes().PutStr("service.name", "checkout")
	primaryResource.Resource().Attributes().PutStr("MiXeD_snake\n\"é", "resource-unchanged")
	primaryScope := primaryResource.ScopeLogs().AppendEmpty()
	primaryScope.SetSchemaUrl("https://example.test/scope/log/v2")
	primaryScope.Scope().SetName("log-fixture")
	primaryScope.Scope().SetVersion("2.0.0")
	primaryScope.Scope().SetDroppedAttributesCount(8)
	primaryScope.Scope().Attributes().PutBool("scope.enabled", false)

	record := primaryScope.LogRecords().AppendEmpty()
	record.SetTimestamp(pcommon.Timestamp(math.MaxUint64 - 1))
	record.SetObservedTimestamp(pcommon.Timestamp(math.MaxUint64))
	record.SetTraceID(mustDecodeTraceIDLogs(otlpLogTraceID))
	record.SetSpanID(mustDecodeSpanIDLogs(otlpLogSpanID))
	record.SetSeverityNumber(plog.SeverityNumber(99))
	record.SetSeverityText("CUSTOM")
	record.SetDroppedAttributesCount(9)
	record.SetFlags(plog.LogRecordFlags(0x101))
	record.SetEventName("primary.event")
	record.Attributes().PutEmpty("empty")
	record.Attributes().PutStr("empty.string", "")
	record.Attributes().PutEmptySlice("empty.array")
	record.Attributes().PutEmptyMap("empty.map")
	record.Attributes().PutInt("minimum", math.MinInt64)
	record.Attributes().PutDouble("negative.zero", math.Float64frombits(0x8000000000000000))
	record.Attributes().PutDouble("nan", math.NaN())
	record.Attributes().PutDouble("positive.infinity", math.Inf(1))
	record.Attributes().PutDouble("negative.infinity", math.Inf(-1))
	record.Attributes().PutEmptyBytes("bytes").FromRaw([]byte{0xfb, 0xff})
	record.Attributes().PutStr("MiXeD_snake\n\"é", "log-unchanged")
	record.Attributes().PutStr("null", "literal")
	body := record.Body().SetEmptyMap()
	body.PutStr("MiXeD_snake\n\"é", "unchanged")
	items := body.PutEmptySlice("items")
	items.AppendEmpty()
	items.AppendEmpty().SetBool(false)
	items.AppendEmpty().SetInt(math.MaxInt64)
	items.AppendEmpty().SetEmptyMap()
	deep := body.PutEmptySlice("deep")
	for range 199 {
		deep = deep.AppendEmpty().SetEmptySlice()
	}
	deep.AppendEmpty().SetStr("bottom")

	secondaryResource := data.ResourceLogs().AppendEmpty()
	secondaryResource.SetSchemaUrl("https://example.test/resource/log/v1")
	secondaryResource.Resource().Attributes().PutStr("service.name", "secondary")
	secondaryScope := secondaryResource.ScopeLogs().AppendEmpty()
	secondaryScope.SetSchemaUrl("https://example.test/scope/log/v1")
	secondaryScope.Scope().SetName("secondary-scope")
	secondary := secondaryScope.LogRecords().AppendEmpty()
	secondary.SetTimestamp(11)
	secondary.SetObservedTimestamp(22)
	secondary.SetEventName("secondary.event")
	secondary.Body().SetStr("secondary-only")

	return data
}

func validateLogOTLP(document []byte) error {
	if err := rejectDuplicateLogJSONKeys(document); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return validateLogOTLPValue(value, "$")
}

func validateLogOTLPValue(value any, path string) error {
	allowed := map[string]bool{
		"resourceLogs": true, "resource": true, "scopeLogs": true, "schemaUrl": true,
		"scope": true, "logRecords": true, "name": true, "version": true,
		"attributes": true, "droppedAttributesCount": true, "key": true, "value": true,
		"timeUnixNano": true, "observedTimeUnixNano": true, "severityNumber": true,
		"severityText": true, "body": true, "traceId": true, "spanId": true,
		"flags": true, "eventName": true, "stringValue": true, "boolValue": true,
		"intValue": true, "doubleValue": true, "arrayValue": true, "kvlistValue": true,
		"bytesValue": true, "values": true,
	}
	switch value := value.(type) {
	case nil:
		return fmt.Errorf("%s: serializer emitted null", path)
	case []any:
		for i, child := range value {
			if err := validateLogOTLPValue(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case map[string]any:
		oneofCount := 0
		for _, field := range []string{"stringValue", "boolValue", "intValue", "doubleValue", "arrayValue", "kvlistValue", "bytesValue"} {
			if _, ok := value[field]; ok {
				oneofCount++
			}
		}
		if oneofCount > 1 {
			return fmt.Errorf("%s: multiple AnyValue alternatives", path)
		}
		for key, child := range value {
			if !allowed[key] {
				return fmt.Errorf("%s: unknown or incorrectly cased schema key %q", path, key)
			}
			switch key {
			case "traceId":
				if text, ok := child.(string); !ok || !otlpLogHex32.MatchString(text) {
					return fmt.Errorf("%s.%s: not fixed-width trace hex", path, key)
				}
			case "spanId":
				if text, ok := child.(string); !ok || !otlpLogHex16.MatchString(text) {
					return fmt.Errorf("%s.%s: not fixed-width span hex", path, key)
				}
			case "timeUnixNano", "observedTimeUnixNano", "intValue":
				if text, ok := child.(string); !ok || !otlpLogDecimal.MatchString(text) {
					return fmt.Errorf("%s.%s: 64-bit integer is not a decimal string", path, key)
				}
			case "severityNumber", "flags", "droppedAttributesCount":
				if _, ok := child.(json.Number); !ok {
					return fmt.Errorf("%s.%s: 32-bit field is not numeric", path, key)
				}
			case "boolValue":
				if _, ok := child.(bool); !ok {
					return fmt.Errorf("%s.%s: bool is not boolean", path, key)
				}
			case "bytesValue":
				text, ok := child.(string)
				if !ok {
					return fmt.Errorf("%s.%s: bytes is not a string", path, key)
				}
				if _, err := base64.StdEncoding.Strict().DecodeString(text); err != nil {
					return fmt.Errorf("%s.%s: not padded base64: %w", path, key, err)
				}
			case "doubleValue":
				if text, ok := child.(string); ok {
					if text != "NaN" && text != "Infinity" && text != "-Infinity" {
						return fmt.Errorf("%s.%s: invalid non-finite double", path, key)
					}
				} else if _, ok := child.(json.Number); !ok {
					return fmt.Errorf("%s.%s: finite double is not numeric", path, key)
				}
			}
			if err := validateLogOTLPValue(child, path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectDuplicateLogJSONKeys(document []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key := keyToken.(string)
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		}
		return nil
	}
	return walk()
}

func BenchmarkGetLogOTLP(b *testing.B) {
	ctx := context.Background()
	s, err := store.NewStore(ctx, "", zap.NewNop())
	require.NoError(b, err)
	b.Cleanup(func() { s.Close() })
	data := plog.NewLogs()
	rl := data.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "benchmark")
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("benchmark")
	record := sl.LogRecords().AppendEmpty()
	record.SetEventName("benchmark")
	for i := 0; i < 64; i++ {
		record.Attributes().PutStr(fmt.Sprintf("attribute.%02d", i), strings.Repeat("v", 64))
	}
	body := record.Body().SetEmptyMap()
	for i := 0; i < 128; i++ {
		body.PutStr(fmt.Sprintf("field.%03d", i), strings.Repeat("body", 32))
	}
	deep := body.PutEmptySlice("deep")
	for range 99 {
		deep = deep.AppendEmpty().SetEmptySlice()
	}
	deep.AppendEmpty().SetInt(42)
	require.NoError(b, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data, s.FlushedIDs())
	}))
	logID := lookupLogID(b, s, ctx, "benchmark")
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.GetLogOTLP(ctx, db, logID)
	})
	require.NoError(b, err)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.GetLogOTLP(ctx, db, logID)
		}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(raw)), "output-bytes")
}
