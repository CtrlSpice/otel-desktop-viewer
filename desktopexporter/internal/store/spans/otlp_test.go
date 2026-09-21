package spans_test

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
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

const otlpTraceID = "fedcba98765432100123456789abcdef"

var (
	otlpHex16   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	otlpHex32   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	otlpDecimal = regexp.MustCompile(`^-?[0-9]+$`)
)

func TestGetTraceOTLP(t *testing.T) {
	s, ctx := storetest.New(t)
	input := otlpTraceFixture()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, input, s.FlushedIDs())
	}))

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetTraceOTLP(ctx, db, otlpTraceID)
	})
	require.NoError(t, err)
	require.NoError(t, validateTraceOTLP(raw))

	direct := string(raw)
	assert.Contains(t, direct, `"traceId":"`+otlpTraceID+`"`)
	assert.Contains(t, direct, `"spanId":"ffffffffffffffff"`)
	assert.Contains(t, direct, `"startTimeUnixNano":"18446744073709551614"`)
	assert.Contains(t, direct, `"endTimeUnixNano":"18446744073709551615"`)
	assert.Contains(t, direct, `"kind":99`)
	assert.Contains(t, direct, `"intValue":"-9223372036854775808"`)
	assert.Contains(t, direct, `"doubleValue":-0.0`)
	assert.Contains(t, direct, `"doubleValue":"NaN"`)
	assert.Contains(t, direct, `"bytesValue":"+/8="`)
	assert.Contains(t, direct, `"key":"MiXeD_snake\n\"é"`)
	assert.NotContains(t, direct, `"traceID"`)
	assert.NotContains(t, direct, `:null`)

	decoded, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(raw)
	require.NoError(t, err)
	assert.Equal(t, 3, decoded.ResourceSpans().Len())

	var spanCount, eventCount, linkCount int
	gotSpans := map[pcommon.SpanID]ptrace.Span{}
	resourceSchemas := map[string]bool{}
	for _, rs := range decoded.ResourceSpans().All() {
		resourceSchemas[rs.SchemaUrl()] = true
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				spanCount++
				eventCount += span.Events().Len()
				linkCount += span.Links().Len()
				gotSpans[span.SpanID()] = span
			}
		}
	}
	assert.Equal(t, 4, spanCount)
	assert.Equal(t, 2, eventCount)
	assert.Equal(t, 2, linkCount)
	assert.Equal(t, map[string]bool{
		"":                                 true,
		"https://example.test/resource/v1": true,
		"https://example.test/resource/v2": true,
	}, resourceSchemas)

	var primaryResource ptrace.ResourceSpans
	for _, rs := range decoded.ResourceSpans().All() {
		if rs.SchemaUrl() == "https://example.test/resource/v2" {
			primaryResource = rs
			break
		}
	}
	require.Equal(t, "https://example.test/resource/v2", primaryResource.SchemaUrl())
	assert.Equal(t, uint32(7), primaryResource.Resource().DroppedAttributesCount())
	resourceKey, ok := primaryResource.Resource().Attributes().Get("MiXeD_snake\n\"é")
	require.True(t, ok)
	assert.Equal(t, "unchanged", resourceKey.Str())
	require.Equal(t, 1, primaryResource.ScopeSpans().Len())
	primaryScope := primaryResource.ScopeSpans().At(0)
	assert.Equal(t, "https://example.test/scope/v2", primaryScope.SchemaUrl())
	assert.Equal(t, "trace-fixture", primaryScope.Scope().Name())
	assert.Equal(t, "2.0.0", primaryScope.Scope().Version())
	assert.Equal(t, uint32(8), primaryScope.Scope().DroppedAttributesCount())
	scopeEnabled, ok := primaryScope.Scope().Attributes().Get("scope.enabled")
	require.True(t, ok)
	assert.False(t, scopeEnabled.Bool())

	primary, found := gotSpans[mustDecodeSpanID("ffffffffffffffff")]
	require.True(t, found)
	assert.Equal(t, pcommon.Timestamp(math.MaxUint64-1), primary.StartTimestamp())
	assert.Equal(t, pcommon.Timestamp(math.MaxUint64), primary.EndTimestamp())
	assert.Equal(t, ptrace.SpanKind(99), primary.Kind())
	assert.Equal(t, ptrace.StatusCode(99), primary.Status().Code())
	assert.Equal(t, "unknown status retained", primary.Status().Message())
	assert.Equal(t, uint32(0x101), uint32(primary.Flags()))
	assert.Equal(t, uint32(9), primary.DroppedAttributesCount())
	assert.Equal(t, uint32(10), primary.DroppedEventsCount())
	assert.Equal(t, uint32(11), primary.DroppedLinksCount())
	assert.Equal(t, "vendor=opaque", primary.TraceState().AsRaw())
	assert.Equal(t, 2, primary.Events().Len())
	assert.Equal(t, 2, primary.Links().Len())
	assert.Equal(t, pcommon.SpanID(mustDecodeSpanID("0000000000000002")), primary.ParentSpanID())

	value, ok := primary.Attributes().Get("nested")
	require.True(t, ok)
	items, ok := value.Map().Get("items")
	require.True(t, ok)
	assert.Equal(t, 3, items.Slice().Len())
	assert.Equal(t, pcommon.ValueTypeEmpty, items.Slice().At(0).Type())
	assert.Equal(t, int64(math.MinInt64), items.Slice().At(1).Int())
	assert.True(t, math.Signbit(items.Slice().At(2).Double()))
	nan, ok := primary.Attributes().Get("nan")
	require.True(t, ok)
	assert.True(t, math.IsNaN(nan.Double()))
	bytesValue, ok := primary.Attributes().Get("bytes")
	require.True(t, ok)
	assert.Equal(t, []byte{0xfb, 0xff}, bytesValue.Bytes().AsRaw())
	assert.Equal(t, pcommon.ValueTypeEmpty, mustAttribute(t, primary, "empty").Type())
	assert.Equal(t, 0, mustAttribute(t, primary, "empty.array").Slice().Len())
	assert.Equal(t, 0, mustAttribute(t, primary, "empty.map").Map().Len())
	tree := mustAttribute(t, primary, "tree").Map()
	assert.Equal(t, "user-kind", treeValue(t, tree, "kind").Str())
	assert.Equal(t, "user-value", treeValue(t, tree, "value").Str())
	assert.Equal(t, 12, treeValue(t, tree, "broad").Slice().Len())
	deep := treeValue(t, tree, "deep")
	for range 200 {
		require.Equal(t, pcommon.ValueTypeSlice, deep.Type())
		require.Equal(t, 1, deep.Slice().Len())
		deep = deep.Slice().At(0)
	}
	assert.Equal(t, int64(42), deep.Int())
	var linked, empty ptrace.SpanLink
	for _, link := range primary.Links().All() {
		if link.TraceID().IsEmpty() {
			empty = link
		} else {
			linked = link
		}
	}
	assert.Equal(t, uint32(1), uint32(linked.Flags()))
	assert.Equal(t, mustDecodeTraceID("11111111111111112222222222222222"), [16]byte(linked.TraceID()))
	assert.Equal(t, mustDecodeSpanID("8000000000000000"), [8]byte(linked.SpanID()))
	assert.Equal(t, uint32(2), uint32(empty.Flags()))
	assert.True(t, empty.TraceID().IsEmpty())
	assert.True(t, empty.SpanID().IsEmpty())

	for name, mutated := range map[string]string{
		"wrong casing":   strings.Replace(direct, `"traceId":`, `"traceID":`, 1),
		"enum name":      strings.Replace(direct, `"kind":99`, `"kind":"SPAN_KIND_SERVER"`, 1),
		"base64 ID":      strings.Replace(direct, `"traceId":"`+otlpTraceID+`"`, `"traceId":"/ty6mHZUMhABI0VniavN7w=="`, 1),
		"unquoted int64": strings.Replace(direct, `"startTimeUnixNano":"18446744073709551614"`, `"startTimeUnixNano":18446744073709551614`, 1),
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			require.Error(t, validateTraceOTLP([]byte(mutated)))
		})
	}
}

func mustAttribute(t *testing.T, span ptrace.Span, key string) pcommon.Value {
	t.Helper()
	value, ok := span.Attributes().Get(key)
	require.True(t, ok, "missing attribute %q", key)
	return value
}

func treeValue(t *testing.T, values pcommon.Map, key string) pcommon.Value {
	t.Helper()
	value, ok := values.Get(key)
	require.True(t, ok, "missing map key %q", key)
	return value
}

func TestGetTraceOTLPNotFound(t *testing.T) {
	s, ctx := storetest.New(t)
	for _, traceID := range []string{"00000000000000000000000000000000", "not-a-trace-id"} {
		_, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return spans.GetTraceOTLP(ctx, db, traceID)
		})
		assert.ErrorIs(t, err, spans.ErrTraceIDNotFound)
	}
}

func TestGetTraceOTLPRejectsStoredSQLNull(t *testing.T) {
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, otlpTraceFixture(), s.FlushedIDs())
	}))

	err := s.WithDBRead(func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx, `update events set name = null`); err != nil {
			return err
		}
		_, err := spans.GetTraceOTLP(ctx, db, otlpTraceID)
		return err
	})
	assert.ErrorIs(t, err, spans.ErrSpansStoreInternal)
	assert.ErrorContains(t, err, "OTLP document contains SQL NULL")
}

func TestGetTraceOTLPRejectsDanglingAttribute(t *testing.T) {
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, otlpTraceFixture(), s.FlushedIDs())
	}))

	err := s.WithDBRead(func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx, `
			delete from attributes
			where id = (select unnest(attribute_ids) from spans limit 1)`); err != nil {
			return err
		}
		_, err := spans.GetTraceOTLP(ctx, db, otlpTraceID)
		return err
	})
	assert.ErrorIs(t, err, spans.ErrSpansStoreInternal)
	assert.ErrorContains(t, err, "stored OTel value is SQL NULL")
}

func otlpTraceFixture() ptrace.Traces {
	traces := ptrace.NewTraces()
	traceID := mustDecodeTraceID(otlpTraceID)

	rs := traces.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl("https://example.test/resource/v2")
	rs.Resource().SetDroppedAttributesCount(7)
	rs.Resource().Attributes().PutStr("service.name", "checkout")
	rs.Resource().Attributes().PutStr("MiXeD_snake\n\"é", "unchanged")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.SetSchemaUrl("https://example.test/scope/v2")
	ss.Scope().SetName("trace-fixture")
	ss.Scope().SetVersion("2.0.0")
	ss.Scope().SetDroppedAttributesCount(8)
	ss.Scope().Attributes().PutBool("scope.enabled", false)

	primary := ss.Spans().AppendEmpty()
	primary.SetTraceID(traceID)
	primary.SetSpanID(mustDecodeSpanID("ffffffffffffffff"))
	primary.SetParentSpanID(mustDecodeSpanID("0000000000000002"))
	primary.TraceState().FromRaw("vendor=opaque")
	primary.SetFlags(0x101)
	primary.SetName("primary")
	primary.SetKind(ptrace.SpanKind(99))
	primary.SetStartTimestamp(pcommon.Timestamp(math.MaxUint64 - 1))
	primary.SetEndTimestamp(pcommon.Timestamp(math.MaxUint64))
	primary.SetDroppedAttributesCount(9)
	primary.SetDroppedEventsCount(10)
	primary.SetDroppedLinksCount(11)
	primary.Status().SetCode(ptrace.StatusCode(99))
	primary.Status().SetMessage("unknown status retained")
	primary.Attributes().PutEmpty("empty")
	primary.Attributes().PutStr("empty.string", "")
	primary.Attributes().PutBool("false", false)
	primary.Attributes().PutInt("minimum", math.MinInt64)
	primary.Attributes().PutDouble("negative.zero", math.Float64frombits(0x8000000000000000))
	primary.Attributes().PutDouble("nan", math.Float64frombits(0x7ff8000000000001))
	primary.Attributes().PutEmptyBytes("bytes").FromRaw([]byte{0xfb, 0xff})
	nested := primary.Attributes().PutEmptyMap("nested")
	items := nested.PutEmptySlice("items")
	items.AppendEmpty()
	items.AppendEmpty().SetInt(math.MinInt64)
	items.AppendEmpty().SetDouble(math.Float64frombits(0x8000000000000000))
	primary.Attributes().PutEmptySlice("empty.array")
	primary.Attributes().PutEmptyMap("empty.map")
	tree := primary.Attributes().PutEmptyMap("tree")
	tree.PutStr("kind", "user-kind")
	tree.PutStr("value", "user-value")
	broad := tree.PutEmptySlice("broad")
	for i := 0; i < 12; i++ {
		broad.AppendEmpty().SetInt(int64(i))
	}
	deep := tree.PutEmptySlice("deep")
	for range 199 {
		deep = deep.AppendEmpty().SetEmptySlice()
	}
	deep.AppendEmpty().SetInt(42)

	for i := 0; i < 2; i++ {
		event := primary.Events().AppendEmpty()
		event.SetTimestamp(pcommon.Timestamp(math.MaxUint64 - uint64(3-i)))
		event.SetName(fmt.Sprintf("event-%d", i))
		event.SetDroppedAttributesCount(uint32(i + 1))
		event.Attributes().PutInt("event.index", int64(i))

		link := primary.Links().AppendEmpty()
		link.TraceState().FromRaw(fmt.Sprintf("link=%d", i))
		link.SetFlags(uint32(i + 1))
		link.SetDroppedAttributesCount(uint32(i + 2))
		link.Attributes().PutBool("link.enabled", i == 0)
		if i == 0 {
			link.SetTraceID(mustDecodeTraceID("11111111111111112222222222222222"))
			link.SetSpanID(mustDecodeSpanID("8000000000000000"))
		}
	}

	// A two-span cycle has no root and cannot be found by a parent-tree walk.
	cycle := ss.Spans().AppendEmpty()
	cycle.SetTraceID(traceID)
	cycle.SetSpanID(mustDecodeSpanID("0000000000000002"))
	cycle.SetParentSpanID(mustDecodeSpanID("ffffffffffffffff"))
	cycle.SetName("cycle-peer")
	cycle.SetStartTimestamp(20)
	cycle.SetEndTimestamp(30)

	// A separate resource and schema pair retains an orphan whose parent is absent.
	rs = traces.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl("https://example.test/resource/v1")
	rs.Resource().Attributes().PutStr("service.name", "worker")
	ss = rs.ScopeSpans().AppendEmpty()
	ss.SetSchemaUrl("https://example.test/scope/v1")
	ss.Scope().SetName("worker-scope")
	orphan := ss.Spans().AppendEmpty()
	orphan.SetTraceID(traceID)
	orphan.SetSpanID(mustDecodeSpanID("8000000000000001"))
	orphan.SetParentSpanID(mustDecodeSpanID("7777777777777777"))
	orphan.SetName("orphan")
	orphan.SetStartTimestamp(40)
	orphan.SetEndTimestamp(50)

	// A third disconnected component verifies that no root or reachability rule
	// controls membership, and that equal schema-free wrappers are not invented.
	rs = traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "detached")
	ss = rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("detached-scope")
	detached := ss.Spans().AppendEmpty()
	detached.SetTraceID(traceID)
	detached.SetSpanID(mustDecodeSpanID("0000000000000004"))
	detached.SetName("detached")
	detached.SetStartTimestamp(60)
	detached.SetEndTimestamp(70)

	return traces
}

func validateTraceOTLP(document []byte) error {
	if err := rejectDuplicateJSONKeys(document); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return validateTraceOTLPValue(value, "$")
}

func validateTraceOTLPValue(value any, path string) error {
	allowed := map[string]bool{
		"resourceSpans": true, "resource": true, "scopeSpans": true, "schemaUrl": true,
		"scope": true, "spans": true, "name": true, "version": true,
		"attributes": true, "droppedAttributesCount": true, "key": true, "value": true,
		"traceId": true, "spanId": true, "traceState": true, "parentSpanId": true,
		"flags": true, "kind": true, "startTimeUnixNano": true, "endTimeUnixNano": true,
		"events": true, "timeUnixNano": true, "droppedEventsCount": true,
		"links": true, "droppedLinksCount": true, "status": true, "message": true, "code": true,
		"stringValue": true, "boolValue": true, "intValue": true, "doubleValue": true,
		"arrayValue": true, "kvlistValue": true, "bytesValue": true, "values": true,
	}
	switch value := value.(type) {
	case nil:
		return fmt.Errorf("%s: serializer emitted null", path)
	case []any:
		for i, child := range value {
			if err := validateTraceOTLPValue(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
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
				if text, ok := child.(string); !ok || !otlpHex32.MatchString(text) {
					return fmt.Errorf("%s.%s: not fixed-width trace hex", path, key)
				}
			case "spanId", "parentSpanId":
				if text, ok := child.(string); !ok || !otlpHex16.MatchString(text) {
					return fmt.Errorf("%s.%s: not fixed-width span hex", path, key)
				}
			case "startTimeUnixNano", "endTimeUnixNano", "timeUnixNano", "intValue":
				if text, ok := child.(string); !ok || !otlpDecimal.MatchString(text) {
					return fmt.Errorf("%s.%s: 64-bit integer is not a decimal string", path, key)
				}
			case "kind", "code", "flags", "droppedAttributesCount", "droppedEventsCount", "droppedLinksCount":
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
			if err := validateTraceOTLPValue(child, path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectDuplicateJSONKeys(document []byte) error {
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

func BenchmarkGetTraceOTLP(b *testing.B) {
	ctx := context.Background()
	s, err := store.NewStore(ctx, "", zap.NewNop())
	require.NoError(b, err)
	b.Cleanup(func() { s.Close() })
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "benchmark")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("benchmark")
	traceID := mustDecodeTraceID(otlpTraceID)
	for i := 0; i < 1000; i++ {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID(mustDecodeSpanID(fmt.Sprintf("%016x", i+1)))
		span.SetName(fmt.Sprintf("span-%04d", i))
		span.SetStartTimestamp(pcommon.Timestamp(i * 100))
		span.SetEndTimestamp(pcommon.Timestamp(i*100 + 50))
		for j := 0; j < 16; j++ {
			span.Attributes().PutStr(fmt.Sprintf("attribute.%02d", j), strings.Repeat("v", 32))
		}
		nested := span.Attributes().PutEmptyMap("nested")
		values := nested.PutEmptySlice("values")
		for j := 0; j < 8; j++ {
			values.AppendEmpty().SetInt(int64(i*8 + j))
		}
	}
	require.NoError(b, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, traces, s.FlushedIDs())
	}))

	for _, threads := range []int{1, 4} {
		b.Run(fmt.Sprintf("threads-%d", threads), func(b *testing.B) {
			require.NoError(b, s.WithDBRead(func(db *sql.DB) error {
				_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
				return err
			}))
			for i := 0; i < b.N; i++ {
				err := s.WithDBRead(func(db *sql.DB) error {
					_, err := spans.GetTraceOTLP(ctx, db, otlpTraceID)
					return err
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
