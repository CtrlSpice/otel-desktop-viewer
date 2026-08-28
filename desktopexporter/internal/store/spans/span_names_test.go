package spans_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// namedSpans builds one trace holding each name count-many times, so
// frequency ordering has something to order.
func namedSpans(names map[string]int) ptrace.Traces {
	tr := ptrace.NewTraces()
	rs := tr.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "names")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("sc")
	traceID := mustDecodeTraceID("000000000000000000000000000000dd")
	i := 0
	for name, count := range names {
		for range count {
			i++
			sp := ss.Spans().AppendEmpty()
			sp.SetTraceID(traceID)
			sp.SetSpanID(mustDecodeSpanID(fmt.Sprintf("%016x", i)))
			sp.SetName(name)
			sp.SetStartTimestamp(pcommon.Timestamp(1000))
			sp.SetEndTimestamp(pcommon.Timestamp(2000))
		}
	}
	return tr
}

func spanNames(t *testing.T, s *store.Store, term string, limit int64) []string {
	t.Helper()
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return spans.GetSpanNames(t.Context(), db, term, limit)
	})
	require.NoError(t, err)
	var out []string
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestGetSpanNames(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, namedSpans(map[string]int{
			"checkout/pay":    5,
			"checkout/verify": 3,
			"db/select 100%":  2,
			"auth/login":      1,
		}), s.FlushedIDs())
	}))

	// Most frequent first; ties broken by name.
	assert.Equal(t,
		[]string{"checkout/pay", "checkout/verify", "db/select 100%", "auth/login"},
		spanNames(t, s, "", 10))

	// Substring match, case-insensitive.
	assert.Equal(t,
		[]string{"checkout/pay", "checkout/verify"},
		spanNames(t, s, "CHECK", 10))

	// A literal % in the term matches itself, not everything.
	assert.Equal(t, []string{"db/select 100%"}, spanNames(t, s, "100%", 10))
	assert.Empty(t, spanNames(t, s, "%zzz%", 10))

	// The limit is a limit.
	assert.Len(t, spanNames(t, s, "", 2), 2)

	// No match is an empty array, not null.
	assert.NotNil(t, spanNames(t, s, "no-such-name", 10))
}
