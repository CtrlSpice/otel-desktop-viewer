package logs_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"database/sql/driver"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestLogTimestampsRoundTripAndSearchAcrossUint64Range(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	values := []uint64{0, 1<<63 - 1, 1 << 63, ^uint64(0)}

	data := plog.NewLogs()
	logsSlice := data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	for i, timestamp := range values {
		record := logsSlice.AppendEmpty()
		record.SetTimestamp(pcommon.Timestamp(timestamp))
		record.SetObservedTimestamp(pcommon.Timestamp(timestamp))
		record.Body().SetStr(fmt.Sprintf("boundary-%d", i))
	}
	fallback := logsSlice.AppendEmpty()
	fallback.SetTimestamp(0)
	fallback.SetObservedTimestamp(pcommon.Timestamp(^uint64(0)))
	fallback.Body().SetStr("fallback")

	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data, s.FlushedIDs())
	}))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select timestamp, observed_timestamp from logs order by timestamp, observed_timestamp`)
		if err != nil {
			return err
		}
		defer rows.Close()
		got := make(map[[2]uint64]bool)
		for rows.Next() {
			var timestamp, observed uint64
			require.NoError(t, rows.Scan(&timestamp, &observed))
			got[[2]uint64{timestamp, observed}] = true
		}
		require.NoError(t, rows.Err())
		for _, want := range values {
			require.True(t, got[[2]uint64{want, want}])
		}
		require.True(t, got[[2]uint64{0, ^uint64(0)}])
		return nil
	}))

	query := &search.QueryNode{Type: "condition", Query: &search.Query{
		Field:         &search.FieldDefinition{Name: "timestamp", SearchScope: "field"},
		FieldOperator: "=",
		Value:         "18446744073709551615",
	}}
	searchLogs := func(query *search.QueryNode) []logSummaryJSON {
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			if query == nil {
				return logs.Search(ctx, db, store.BoundedTimeRange(uint64(0), ^uint64(0)), nil)
			}
			return logs.Search(ctx, db, store.BoundedTimeRange(uint64(0), ^uint64(0)), query)
		})
		require.NoError(t, err)
		var got []logSummaryJSON
		require.NoError(t, json.Unmarshal(raw, &got))
		return got
	}
	require.Len(t, searchLogs(query), 1)
	query = &search.QueryNode{Type: "condition", Query: &search.Query{
		Field:         &search.FieldDefinition{Name: "timestamp", SearchScope: "field"},
		FieldOperator: "IN",
		Value:         `["9223372036854775808","18446744073709551615"]`,
	}}
	require.Len(t, searchLogs(query), 2)

	all := searchLogs(nil)
	require.Equal(t, "18446744073709551615", all[0].Timestamp)
	require.Equal(t, "18446744073709551615", all[1].Timestamp)
}
