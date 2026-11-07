package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

func TestNativeTypedSearchAgreesWithUntypedComparison(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, integrityTraces(1))
	}))
	count := func(typ, value string) int {
		query := map[string]any{"type": "condition", "query": map[string]any{
			"field": map[string]any{"name": "http.method", "type": typ,
				"searchScope": "attribute", "attributeScope": "span"},
			"fieldOperator": "=", "value": value,
		}}
		var out []map[string]any
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			raw, err := spans.SearchTraceSummaries(ctx, db, BoundedTimeRange(0, 1<<62), query)
			if err != nil {
				return err
			}
			return json.Unmarshal(raw, &out)
		}))
		return len(out)
	}
	assert.Equal(t, count("", "GET"), count("string", "GET"))
	assert.Positive(t, count("string", "GET"))
	assert.Zero(t, count("string", "POST"))
	assert.Zero(t, count("", "POST"))
}

func TestInlineSearchDistinguishesEmptyAndAbsent(t *testing.T) {
	ctx := t.Context()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	data := plog.NewLogs()
	records := data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	records.AppendEmpty().Attributes().PutEmpty("value")
	records.AppendEmpty().Attributes().PutStr("value", "null")
	records.AppendEmpty()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data)
	}))
	for _, tc := range []struct {
		kind, key, operator, value string
		want                       int
	}{
		{"empty", "value", "=", "null", 1},
		{"string", "value", "=", "null", 1},
		{"empty", "missing", "=", "null", 0},
		{"empty", "value", "IS NOT NULL", "", 1},
	} {
		t.Run(tc.kind+"/"+tc.key+"/"+tc.operator, func(t *testing.T) {
			criteria := map[string]any{"type": "condition", "query": map[string]any{
				"field":         map[string]any{"name": tc.key, "type": tc.kind, "searchScope": "attribute", "attributeScope": "log"},
				"fieldOperator": tc.operator, "value": tc.value,
			}}
			var result []map[string]any
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				raw, err := logs.Search(ctx, db, TimeRange{}, criteria)
				if err != nil {
					return err
				}
				return json.Unmarshal(raw, &result)
			}))
			require.Len(t, result, tc.want)
		})
	}
}

func TestMembershipExcludesSQLNull(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	for _, operator := range []string{"IN", "NOT IN"} {
		t.Run(operator, func(t *testing.T) {
			params := []search.NamedParam{}
			condition, err := search.BuildOperatorCondition(search.Text("candidate"), &search.Query{
				FieldOperator: operator, Value: `["blocked"]`,
			}, &params)
			require.NoError(t, err)
			require.Len(t, params, 1)
			var count int
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				return db.QueryRowContext(ctx, `select count(*) from (values (NULL::varchar), ('blocked'), ('allowed')) t(candidate)
				where `+condition, params[0].Value).Scan(&count)
			}))
			assert.Equal(t, 1, count)
		})
	}
}
