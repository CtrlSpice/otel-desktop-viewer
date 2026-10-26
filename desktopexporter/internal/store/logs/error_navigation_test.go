package logs_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/require"
)

func TestLogErrorNavigation(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	data := createTestLogsPdata(1700000000123456789)
	records := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	records.At(2).SetSeverityNumber(21) // Fatal is also an error-navigation target.
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data, s.FlushedIDs()) }))
	type target struct {
		LogRef string `json:"logRef"`
		Offset int    `json:"offset"`
	}
	type navigation struct {
		Count    int     `json:"count"`
		Index    *int    `json:"index"`
		Previous *target `json:"previous"`
		Next     *target `json:"next"`
	}
	options := search.ResultOptions{Sort: &search.Sort{Field: "severity", Direction: "asc"}}
	fetch := func(selected string, opts search.ResultOptions, query any) navigation {
		t.Helper()
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.GetErrorNavigation(ctx, db, store.BoundedTimeRange(0, ^uint64(0)), query, opts, selected)
		})
		require.NoError(t, err)
		var result navigation
		require.NoError(t, json.Unmarshal(raw, &result))
		return result
	}
	start := fetch("", options, nil)
	require.Equal(t, 2, start.Count)
	require.Nil(t, start.Index)
	require.Nil(t, start.Previous)
	require.NotNil(t, start.Next)
	require.Equal(t, 1, start.Next.Offset)
	first := fetch(start.Next.LogRef, options, nil)
	require.Equal(t, 1, *first.Index)
	require.Nil(t, first.Previous)
	require.NotNil(t, first.Next)
	require.Equal(t, 2, first.Next.Offset)
	last := fetch(first.Next.LogRef, options, nil)
	require.Equal(t, 2, *last.Index)
	require.Nil(t, last.Next)
	require.Equal(t, start.Next, last.Previous)
	limit := int64(1)
	options.Limit = &limit
	require.Equal(t, 0, fetch("", options, nil).Count)
	options.Limit = nil
	query := map[string]any{"id": "severity", "type": "condition", "query": map[string]any{"field": map[string]any{"name": "severityNumber", "searchScope": "field", "type": "int64"}, "fieldOperator": ">=", "value": "21"}}
	filtered := fetch("", options, query)
	require.Equal(t, 1, filtered.Count)
	require.Equal(t, 0, filtered.Next.Offset)
}
