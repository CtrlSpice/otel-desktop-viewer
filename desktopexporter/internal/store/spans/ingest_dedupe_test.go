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

// Each span retains the complete shared payload with the same grouping identity.
func TestIngestRepeatsOwnedPayloadAcrossSpans(t *testing.T) {
	t.Parallel()
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
			sid[b] = byte((i + 1) >> (b * 8))
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
		return spans.Ingest(ctx, conn, td)
	}))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var spanCount, resourceCount, scopeCount, attrCount int
		require.NoError(t, db.QueryRow(`select count(*) from spans`).Scan(&spanCount))
		require.NoError(t, db.QueryRow(`select count(distinct resource.id) from spans`).Scan(&resourceCount))
		require.NoError(t, db.QueryRow(`select count(distinct scope.id) from spans`).Scan(&scopeCount))
		require.NoError(t, db.QueryRow(`select sum(len(variant_keys(attributes)) + len(variant_keys(resource.attributes))) from spans`).Scan(&attrCount))

		assert.Equal(t, n, spanCount)
		assert.Equal(t, 1, resourceCount, "200 spans, one resource")
		assert.Equal(t, 1, scopeCount)
		// 2 resource attrs + 2 span attrs; the scope has none.
		assert.Equal(t, 4*n, attrCount, "every span owns all received attributes")

		var dangling int
		require.NoError(t, db.QueryRow(`
			select count(*) from spans where resource.attributes['service.name']::varchar <> 'checkout'
			or resource.attributes['host.name']::varchar <> 'pod-a'`).Scan(&dangling))
		assert.Zero(t, dangling)

		var mismatched int
		require.NoError(t, db.QueryRow(
			`select count(*) from spans where attributes['http.request.method']::varchar <> 'GET'
			or attributes['http.response.status_code']::bigint <> 200`).Scan(&mismatched))
		assert.Zero(t, mismatched)
		return nil
	}))
}
