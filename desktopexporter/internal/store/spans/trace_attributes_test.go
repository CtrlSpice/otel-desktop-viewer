package spans_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestAttributeDefinitionsStayWithinRequestedTrace(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := ptrace.NewTraces()
	traceIDs := []pcommon.TraceID{{1}, {2}}
	type definition struct {
		Name  string `json:"name"`
		Scope string `json:"attributeScope"`
		Type  string `json:"type"`
	}
	expected := make([][]definition, len(traceIDs))
	var all []definition
	for i, traceID := range traceIDs {
		label := []string{"first", "second"}[i]
		rm := data.ResourceSpans().AppendEmpty()
		sm := rm.ScopeSpans().AppendEmpty()
		span := sm.Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID(pcommon.SpanID{1})
		event := span.Events().AppendEmpty()
		link := span.Links().AppendEmpty()
		link.SetTraceID(traceIDs[1-i])
		link.SetSpanID(pcommon.SpanID{1})
		for _, owner := range []struct {
			scope string
			attrs pcommon.Map
		}{
			{"resource", rm.Resource().Attributes()},
			{"scope", sm.Scope().Attributes()},
			{"span", span.Attributes()},
			{"event", event.Attributes()},
			{"link", link.Attributes()},
		} {
			key := owner.scope + "." + label
			owner.attrs.PutStr(key, "value")
			owner.attrs.PutStr("shared", "same")
			only := definition{key, owner.scope, "string"}
			shared := definition{"shared", owner.scope, "string"}
			expected[i] = append(expected[i], only, shared)
			all = append(all, only)
			if i == 0 {
				all = append(all, shared)
			}
		}
	}
	// A retained trace without attributes must also return an empty list.
	emptyTraceID := pcommon.TraceID{3}
	empty := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	empty.SetTraceID(emptyTraceID)
	empty.SetSpanID(pcommon.SpanID{1})
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, data, s.FlushedIDs())
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for i, id := range traceIDs {
			raw, err := spans.GetTraceAttributeDefinitionsByTraceID(ctx, db, id.String())
			require.NoError(t, err)
			var got []definition
			require.NoError(t, json.Unmarshal(raw, &got))
			assert.ElementsMatch(t, expected[i], got)
		}
		for _, id := range []pcommon.TraceID{emptyTraceID, {4}} {
			raw, err := spans.GetTraceAttributeDefinitionsByTraceID(ctx, db, id.String())
			require.NoError(t, err)
			assert.JSONEq(t, "[]", string(raw))
		}
		raw, err := spans.GetTraceAttributeDefinitions(ctx, db)
		require.NoError(t, err)
		var got []definition
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.ElementsMatch(t, all, got)
		return nil
	}))
}
