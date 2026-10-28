package spans_test

import (
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestNativeRootKeepsSpanIDsTraceLocal(t *testing.T) {
	s, ctx := storetest.New(t)
	data := ptrace.NewTraces()
	items := data.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for i, name := range []string{"alpha", "beta"} {
		span := items.AppendEmpty()
		span.SetTraceID(pcommon.TraceID{byte(i + 1)})
		span.SetSpanID(pcommon.SpanID{255, 255, 255, 255, 255, 255, 255, 255})
		span.SetName(name)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return spans.Ingest(ctx, conn, data) }))
	for i, name := range []string{"alpha", "beta"} {
		sqlText, args, err := spans.MatchingRowsSQL(&search.QueryNode{Type: "text", Query: &search.Query{FieldOperator: "CONTAINS", Value: name}}, timerange.TimeRange{})
		require.NoError(t, err)
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			rows, err := db.QueryContext(ctx, "select trace_id::varchar from ("+sqlText+")", args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			var found []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					return err
				}
				found = append(found, strings.ReplaceAll(id, "-", ""))
			}
			require.Equal(t, []string{pcommon.TraceID{byte(i + 1)}.String()}, found)
			return rows.Err()
		}))
	}
}
