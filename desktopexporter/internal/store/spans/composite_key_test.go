package spans_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// traceReusingSpanIDs builds one trace whose spans use ids 1 and 2, with an
// event and a link on each, named after the trace so they can be told apart.
func traceReusingSpanIDs(traceHex, label string) ptrace.Traces {
	base := time.Now().UnixNano()
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", label)
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("sc")
	traceID := mustDecodeTraceID(traceHex)

	for _, n := range []string{"0000000000000001", "0000000000000002"} {
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(traceID)
		sp.SetSpanID(mustDecodeSpanID(n))
		sp.SetName(label + "-" + n)
		sp.SetStartTimestamp(pcommon.Timestamp(base))
		sp.SetEndTimestamp(pcommon.Timestamp(base + int64(time.Second)))

		e := sp.Events().AppendEmpty()
		e.SetName(label + "-event")
		e.SetTimestamp(pcommon.Timestamp(base))

		l := sp.Links().AppendEmpty()
		l.SetTraceID(mustDecodeTraceID("000000000000000000000000000000ff"))
		l.SetSpanID(mustDecodeSpanID("00000000000000ff"))
	}
	return tr
}

// Two traces may legitimately use the same span ids.
//
// Span ids are only required to be unique within a trace -- global uniqueness
// is a property of random generation, not a guarantee. Keying spans on span_id
// alone rejected the second trace outright, which made a conformant sender look
// like it was reusing ids.
func TestCompositeKey_TwoTracesMayShareSpanIDs(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	repA, err := ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000a1", "alpha"))
	require.NoError(t, err)
	assert.Zero(t, repA.Count())

	repB, err := ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000b2", "bravo"))
	require.NoError(t, err)
	assert.Zero(t, repB.Count(), "a second trace reusing span ids is not a duplicate")

	assert.Equal(t, 4, countRows(t, s, ctx, "select count(*) from spans"))
	assert.Equal(t, 4, countRows(t, s, ctx, "select count(*) from events"))
	assert.Equal(t, 4, countRows(t, s, ctx, "select count(*) from links"))
}

// Fetching one of those traces must not pull in the other's children, which is
// the contamination the composite key makes possible if a join forgets the
// trace.
func TestCompositeKey_TraceFetchDoesNotBorrowAnotherTracesChildren(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)

	_, err := ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000a1", "alpha"))
	require.NoError(t, err)
	_, err = ingestReport(t, s, traceReusingSpanIDs("000000000000000000000000000000b2", "bravo"))
	require.NoError(t, err)

	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.SearchSpans(ctx, db, "00000000-0000-0000-0000-0000000000a1", nil)
	})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	body := string(raw)
	assert.Contains(t, body, "alpha-event")
	assert.NotContains(t, body, "bravo-event",
		"events from the other trace leaked in through a span id match")
	assert.Equal(t, 2, len(got["spans"].([]any)))
}
