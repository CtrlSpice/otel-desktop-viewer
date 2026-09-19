package logs_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogScalarMembershipExecution(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	baseTime := time.Now().UnixNano()
	data := createTestLogsPdata(baseTime)
	records := data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords()
	records.At(0).Attributes().PutStr("same.key", "42")
	records.At(1).Attributes().PutInt("same.key", 42)
	records.At(0).Attributes().PutEmptySlice("received.array").AppendEmpty().SetStr("value")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data, s.FlushedIDs())
	}))

	searchCount := func(field *search.FieldDefinition, operator, value string) (int, error) {
		t.Helper()
		query := &search.QueryNode{Type: "condition", Query: &search.Query{
			Field: field, FieldOperator: operator, Value: value,
		}}
		var raw json.RawMessage
		err := s.WithDBRead(func(db *sql.DB) error {
			var err error
			raw, err = logs.Search(ctx, db, store.BoundedTimeRange(baseTime-int64(time.Hour), baseTime+int64(time.Hour)), query)
			return err
		})
		if err != nil {
			return 0, err
		}
		var summaries []logSummaryJSON
		if err := json.Unmarshal(raw, &summaries); err != nil {
			return 0, err
		}
		return len(summaries), nil
	}

	for _, tc := range []struct {
		name     string
		field    *search.FieldDefinition
		operator string
		value    string
		want     int
	}{
		{"text IN", &search.FieldDefinition{Name: "severityText", SearchScope: "field"}, "IN", `["ERROR"]`, 1},
		{"native integer NOT IN", &search.FieldDefinition{Name: "severityNumber", SearchScope: "field"}, "NOT IN", `["17"]`, 2},
		{"wire ID IN", &search.FieldDefinition{Name: "traceID", SearchScope: "field"}, "IN", `["00000000-0000-0000-0000-000000000099"]`, 3},
		{"malformed wire ID", &search.FieldDefinition{Name: "traceID", SearchScope: "field"}, "IN", `["not-a-trace"]`, 0},
		{"hostile text remains data", &search.FieldDefinition{Name: "severityText", SearchScope: "field"}, "IN", `["ERROR') OR TRUE --"]`, 0},
		{"string kind", &search.FieldDefinition{Name: "same.key", SearchScope: "attribute", AttributeScope: "log", Type: "string"}, "IN", `["42"]`, 1},
		{"integer kind remains text-bound", &search.FieldDefinition{Name: "same.key", SearchScope: "attribute", AttributeScope: "log", Type: "int64"}, "IN", `["42"]`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := searchCount(tc.field, tc.operator, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := searchCount(&search.FieldDefinition{
		Name: "received.array", SearchScope: "attribute", AttributeScope: "log", Type: "array",
	}, "IN", `["value"]`)
	assert.ErrorIs(t, err, search.ErrInvalidQuery)
}
