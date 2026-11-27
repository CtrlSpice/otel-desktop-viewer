package logs_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"testing"

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
		return logs.Ingest(ctx, conn, data)
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
	searchLogSummaries := func(query *search.QueryNode) []logSummaryJSON {
		raw, err := storetest.Read(s, func(db *sql.DB) (json.RawMessage, error) {
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
	require.Len(t, searchLogSummaries(query), 1)
	query = &search.QueryNode{Type: "condition", Query: &search.Query{
		Field:         &search.FieldDefinition{Name: "timestamp", SearchScope: "field"},
		FieldOperator: "IN",
		Value:         `["9223372036854775808","18446744073709551615"]`,
	}}
	require.Len(t, searchLogSummaries(query), 2)

	all := searchLogSummaries(nil)
	require.Equal(t, "18446744073709551615", all[0].Timestamp)
	require.Equal(t, "18446744073709551615", all[1].Timestamp)
}

func TestArrowPreservesReceivedFloatBitsAcrossSharedContexts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "float-bits.duckdb")
	s, err := store.NewStore(ctx, path, nil)
	require.NoError(t, err)
	defer func() { _ = s.Close() }()
	data := plog.NewLogs()
	bits := []uint64{0x7ff8000000000042, 0xfff8000000000013, 0x8000000000000000}
	for i, bit := range bits {
		r := data.ResourceLogs().AppendEmpty()
		r.Resource().Attributes().PutDouble("bits", math.Float64frombits(bit))
		sc := r.ScopeLogs().AppendEmpty()
		sc.Scope().Attributes().PutDouble("bits", math.Float64frombits(bit))
		log := sc.LogRecords().AppendEmpty()
		log.SetTimestamp(pcommon.Timestamp(i + 1))
		log.Body().SetDouble(math.Float64frombits(bit))
		log.Attributes().PutDouble("bits", math.Float64frombits(bit))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	verify := func() {
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			for i, bit := range bits {
				var body, attr, resource, scope float64
				err := db.QueryRow(`select body::double,attributes['bits']::double,resource.attributes['bits']::double,scope.attributes['bits']::double from logs where timestamp=?`, i+1).Scan(&body, &attr, &resource, &scope)
				if err != nil {
					return err
				}
				for _, value := range []float64{body, attr, resource, scope} {
					require.Equal(t, bit, math.Float64bits(value))
				}
			}
			return nil
		}))
	}
	verify()
	require.NoError(t, s.Close())
	s, err = store.NewStore(ctx, path, nil)
	require.NoError(t, err)
	verify()
}
