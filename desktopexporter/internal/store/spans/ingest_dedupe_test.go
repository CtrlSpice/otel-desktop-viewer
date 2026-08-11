package spans_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// The dedupe claim, end to end: many spans sharing a resource must produce one
// resource row and one set of dictionary entries, not one copy per span.
func TestIngestDedupesAcrossSpans(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "checkout")
	rs.Resource().Attributes().PutStr("host.name", "pod-a")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("otelhttp")
	ss.Scope().SetVersion("1.2.0")

	const n = 200
	for i := 0; i < n; i++ {
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID(pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		var sid [8]byte
		for b := 0; b < 8; b++ {
			sid[b] = byte(i >> (b * 8))
		}
		sp.SetSpanID(sid)
		sp.SetName("GET /cart")
		now := time.Now()
		sp.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
		sp.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Millisecond)))
		sp.Attributes().PutStr("http.request.method", "GET")
		sp.Attributes().PutInt("http.response.status_code", 200)
	}

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, td, s.FlushedIDs())
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var spanCount, resourceCount, scopeCount, attrCount int
		require.NoError(t, db.QueryRow(`select count(*) from spans`).Scan(&spanCount))
		require.NoError(t, db.QueryRow(`select count(*) from resources`).Scan(&resourceCount))
		require.NoError(t, db.QueryRow(`select count(*) from scopes`).Scan(&scopeCount))
		require.NoError(t, db.QueryRow(`select count(*) from attributes`).Scan(&attrCount))

		assert.Equal(t, n, spanCount)
		assert.Equal(t, 1, resourceCount, "200 spans, one resource")
		assert.Equal(t, 1, scopeCount)
		// 2 resource attrs + 2 span attrs; the scope has none.
		assert.Equal(t, 4, attrCount, "the dictionary holds distinct attributes, not one row per span")

		var dangling int
		require.NoError(t, db.QueryRow(`
			select count(*) from (select unnest(attribute_ids) id from spans) x
			where not exists (select 1 from attributes a where a.id = x.id)`).Scan(&dangling))
		assert.Zero(t, dangling)

		var mismatched int
		require.NoError(t, db.QueryRow(
			`select count(*) from attributes where id <> attr_id(key, value, type::varchar, scope)`).Scan(&mismatched))
		assert.Zero(t, mismatched)
		return nil
	}))
}
