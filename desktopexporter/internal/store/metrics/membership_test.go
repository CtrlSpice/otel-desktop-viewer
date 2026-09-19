package metrics_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricTypeMembershipExecution(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, createTestMetricsPdata(), s.FlushedIDs())
	}))

	count := func(operator, value string) int {
		t.Helper()
		query := &search.QueryNode{Type: "condition", Query: &search.Query{
			Field:         &search.FieldDefinition{Name: "type", SearchScope: "field"},
			FieldOperator: operator,
			Value:         value,
		}}
		var raw json.RawMessage
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			var err error
			raw, err = metrics.SearchSummaries(ctx, db, store.BoundedTimeRange(0, time.Now().Add(time.Hour).UnixNano()), query)
			return err
		}))
		var summaries []map[string]any
		require.NoError(t, json.Unmarshal(raw, &summaries))
		return len(summaries)
	}

	assert.Equal(t, 2, count("IN", `["Gauge"]`))
	assert.Equal(t, 3, count("NOT IN", `["Gauge"]`))
}
