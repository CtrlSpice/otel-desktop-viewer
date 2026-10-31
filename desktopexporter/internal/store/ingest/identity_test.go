package ingest_test

import (
	"context"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestHashFramingIsUnambiguous(t *testing.T) {
	assert.NotEqual(t, ingest.AttributeID("ab", "c"), ingest.AttributeID("a", "bc"))
	assert.NotEqual(t, ingest.AttributeID("k", "a|b=c,d"), ingest.AttributeID("k", "a|b=c"))
	assert.Equal(t, ingest.AttributeID("k", "v"), ingest.AttributeID("k", "v"))
}

func TestOwnerIdentityPreservesCompleteTypedPayload(t *testing.T) {
	base := pcommon.NewResource()
	base.Attributes().PutStr("service.name", "checkout")
	base.Attributes().PutInt("process.pid", 42)
	first, err := ingest.ResourceInput(base, "schema-a")
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		change func(pcommon.Resource)
	}{
		{"type", func(r pcommon.Resource) { r.Attributes().PutStr("process.pid", "42") }},
		{"other attribute", func(r pcommon.Resource) { r.Attributes().PutStr("region", "eu") }},
		{"dropped count", func(r pcommon.Resource) { r.SetDroppedAttributesCount(1) }},
		{"present empty", func(r pcommon.Resource) { r.Attributes().PutEmpty("missing") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := pcommon.NewResource()
			base.CopyTo(r)
			test.change(r)
			got, err := ingest.ResourceInput(r, "schema-a")
			require.NoError(t, err)
			assert.NotEqual(t, first["payload_id"], got["payload_id"])
		})
	}
	other, err := ingest.ResourceInput(base, "schema-b")
	require.NoError(t, err)
	assert.Equal(t, first["payload_id"], other["payload_id"])
	assert.NotEqual(t, first["id"], other["id"])
	reordered := pcommon.NewResource()
	reordered.Attributes().PutInt("process.pid", 42)
	reordered.Attributes().PutStr("service.name", "checkout")
	got, err := ingest.ResourceInput(reordered, "schema-a")
	require.NoError(t, err)
	assert.Equal(t, first, got)
	var empty []duckdb.UUID
	assert.NotEqual(t, ingest.ScopeID("a", "1", "x", empty, 0), ingest.ScopeID("b", "1", "x", empty, 0))
	assert.NotEqual(t, ingest.ScopeID("a", "1", "x", empty, 0), ingest.ScopeID("a", "2", "x", empty, 0))
	assert.NotEqual(t, ingest.ScopeID("a", "1", "x", empty, 0), ingest.ScopeID("a", "1", "y", empty, 0))
	assert.NotEqual(t, ingest.ScopeID("a", "1", "x", empty, 0), ingest.ScopeID("a", "1", "x", empty, 1))
}

func TestAttributeIdentityAndTransportSelectLastKey(t *testing.T) {
	for _, pair := range [][2]string{
		{`{"stringValue":"old"}`, `{"intValue":"9223372036854775807"}`},
		{`{"intValue":"9223372036854775807"}`, `{"stringValue":"old"}`},
		{`{"doubleValue":-0.0}`, `{"doubleValue":-0.0}`},
	} {
		data, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs([]byte(`{"resourceLogs":[{"resource":{"attributes":[{"key":"key","value":` + pair[0] + `},{"key":"key","value":` + pair[1] + `}]}}]}`))
		require.NoError(t, err)
		winner, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs([]byte(`{"resourceLogs":[{"resource":{"attributes":[{"key":"key","value":` + pair[1] + `}]}}]}`))
		require.NoError(t, err)
		attrs := data.ResourceLogs().At(0).Resource().Attributes()
		wantAttrs := winner.ResourceLogs().At(0).Resource().Attributes()
		ids, err := ingest.AttributeIdentities(attrs)
		require.NoError(t, err)
		wantIDs, err := ingest.AttributeIdentities(wantAttrs)
		require.NoError(t, err)
		assert.Equal(t, wantIDs, ids)
		payload, err := ingest.AttributeBytes(attrs)
		require.NoError(t, err)
		wantPayload, err := ingest.AttributeBytes(wantAttrs)
		require.NoError(t, err)
		assert.Equal(t, wantPayload, payload)
		assert.Equal(t, 2, attrs.Len())
	}
}
