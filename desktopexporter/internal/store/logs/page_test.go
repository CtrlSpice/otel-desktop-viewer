package logs_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestSearchLogPageSelectedColumns(t *testing.T) {
	s, ctx := storetest.New(t)
	data := plog.NewLogs()
	rl := data.ResourceLogs().AppendEmpty()
	rl.SetSchemaUrl("https://example.test/resource")
	rl.Resource().Attributes().PutStr("same", "resource")
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("instrumentation")
	sl.Scope().Attributes().PutBool("same", false)
	l := sl.LogRecords().AppendEmpty()
	l.SetTimestamp(pcommon.Timestamp(math.MaxUint64))
	l.SetFlags(128)
	l.Attributes().PutInt("same", math.MaxInt64)
	l.Attributes().PutInt("flags", 0)
	l.Attributes().PutStr("x' OR 1=1 --", "literal key")
	l.Attributes().PutEmpty("")
	l.Attributes().PutDouble("negative-zero", math.Copysign(0, -1))
	l.Body().SetStr("body")
	other := sl.LogRecords().AppendEmpty()
	other.SetTimestamp(1)
	other.Body().SetStr("outside page")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	columns := []logs.Column{
		{Source: "log", Kind: "field", Key: "timeUnixNano"},
		{Source: "log", Kind: "field", Key: "flags"},
		{Source: "log", Kind: "attribute", Key: "flags"},
		{Source: "log", Kind: "attribute", Key: "same"},
		{Source: "resource", Kind: "attribute", Key: "same"},
		{Source: "scope", Kind: "attribute", Key: "same"},
		{Source: "log", Kind: "attribute", Key: "missing"},
		{Source: "log", Kind: "attribute", Key: ""},
		{Source: "log", Kind: "attribute", Key: "x' OR 1=1 --"},
		{Source: "log", Kind: "attribute", Key: "negative-zero"},
		{Source: "log", Kind: "field", Key: "traceId"},
		{Source: "resource", Kind: "field", Key: "schemaUrl"},
		{Source: "scope", Kind: "field", Key: "name"},
	}
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return logs.SearchPage(ctx, db, store.BoundedTimeRange(uint64(0), uint64(math.MaxUint64)), nil, search.ResultOptions{}, logs.PageOptions{Size: 1, Columns: columns})
	})
	require.NoError(t, err)
	var page struct {
		Total int `json:"total"`
		Logs  []struct {
			ColumnValues []struct {
				logs.Column
				Values json.RawMessage `json:"values"`
			} `json:"columnValues"`
		} `json:"logs"`
	}
	require.NoError(t, json.Unmarshal(raw, &page))
	require.Equal(t, 2, page.Total)
	require.Len(t, page.Logs, 1)
	want := []string{
		`[{"kind":"string","value":"18446744073709551615"}]`,
		`[{"kind":"int64","value":"128"}]`,
		`[{"kind":"int64","value":"0"}]`,
		`[{"kind":"int64","value":"9223372036854775807"}]`,
		`[{"kind":"string","value":"resource"}]`,
		`[{"kind":"bool","value":false}]`,
		`[]`, `[{"kind":"empty","value":null}]`,
		`[{"kind":"string","value":"literal key"}]`,
		`[{"kind":"double","value":"0x8000000000000000"}]`,
		`[]`, `[{"kind":"string","value":"https://example.test/resource"}]`,
		`[{"kind":"string","value":"instrumentation"}]`,
	}
	require.Len(t, page.Logs[0].ColumnValues, len(columns))
	for i, cell := range page.Logs[0].ColumnValues {
		require.Equal(t, columns[i], cell.Column)
		require.JSONEq(t, want[i], string(cell.Values), "%+v", columns[i])
	}
}

func TestSearchLogPage(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := createTestLogsPdata(1700000000123456789)
	// Tie all effective timestamps, including the observed-time fallback.
	records := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	for i := 0; i < records.Len(); i++ {
		record := records.At(i)
		record.SetTimestamp(0)
		record.SetObservedTimestamp(pcommon.Timestamp(18446744073709551614))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	type result struct {
		Logs  []logSummaryJSON `json:"logs"`
		Total int              `json:"total"`
	}
	fetch := func(offset, size int64, options search.ResultOptions, query any, window store.TimeRange) result {
		t.Helper()
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.SearchPage(ctx, db, window, query, options, logs.PageOptions{Size: size, Offset: offset})
		})
		require.NoError(t, err)
		var page result
		require.NoError(t, json.Unmarshal(raw, &page))
		require.NotNil(t, page.Logs)
		return page
	}
	window := store.BoundedTimeRange(0, ^uint64(0))
	all := fetch(0, 100, search.ResultOptions{}, nil, window)
	require.Equal(t, 3, all.Total)
	for i, log := range all.Logs {
		page := fetch(int64(i), 1, search.ResultOptions{}, nil, window)
		require.Equal(t, 3, page.Total)
		require.Equal(t, []logSummaryJSON{log}, page.Logs)
		require.Equal(t, "18446744073709551614", log.Timestamp)
		if i > 0 {
			require.Less(t, all.Logs[i-1].LogRef, log.LogRef)
		}
	}
	empty := fetch(3, 100, search.ResultOptions{}, nil, window)
	require.Empty(t, empty.Logs)
	require.Equal(t, 3, empty.Total)
	limit := int64(2)
	options := search.ResultOptions{Limit: &limit, Sort: &search.Sort{Field: "severity", Direction: "asc"}}
	limited := fetch(1, 100, options, nil, window)
	require.Equal(t, 2, limited.Total)
	require.Len(t, limited.Logs, 1)
	require.Equal(t, "WARN", limited.Logs[0].SeverityText)
	require.Empty(t, fetch(2, 100, options, nil, window).Logs)

	query := map[string]any{"id": "severity", "type": "condition", "query": map[string]any{"field": map[string]any{"name": "severityNumber", "searchScope": "field", "type": "int64"}, "fieldOperator": ">=", "value": "13"}}
	filtered := fetch(0, 1, search.ResultOptions{}, query, window)
	require.Equal(t, 2, filtered.Total)
	require.Len(t, filtered.Logs, 1)
	require.GreaterOrEqual(t, filtered.Logs[0].SeverityNumber, int32(13))
	outside := fetch(0, 100, search.ResultOptions{}, query, store.BoundedTimeRange(0, 1700000000000000000))
	require.Zero(t, outside.Total)
	require.Empty(t, outside.Logs)
}

func TestSearchLogPageSortsSeverityNumerically(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := createTestLogsPdata(1700000000123456789)
	records := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	// Labels deliberately disagree with numeric and lexicographic-number order.
	for i, severity := range []plog.SeverityNumber{2, 10, 17} {
		records.At(i).SetSeverityNumber(severity)
		records.At(i).SetSeverityText([]string{"ZZZ", "AAA", "MMM"}[i])
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	for _, tc := range []struct {
		direction string
		want      []int32
	}{{"asc", []int32{2, 10, 17}}, {"desc", []int32{17, 10, 2}}} {
		t.Run(tc.direction, func(t *testing.T) {
			for offset, want := range tc.want {
				raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
					return logs.SearchPage(ctx, db, store.BoundedTimeRange(0, ^uint64(0)), nil,
						search.ResultOptions{Sort: &search.Sort{Field: "severity", Direction: tc.direction}},
						logs.PageOptions{Size: 1, Offset: int64(offset)})
				})
				require.NoError(t, err)
				var page struct {
					Total int              `json:"total"`
					Logs  []logSummaryJSON `json:"logs"`
				}
				require.NoError(t, json.Unmarshal(raw, &page))
				require.Equal(t, 3, page.Total)
				require.Len(t, page.Logs, 1)
				require.Equal(t, want, page.Logs[0].SeverityNumber)
			}
		})
	}
}
