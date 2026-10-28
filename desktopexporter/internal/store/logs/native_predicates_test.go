package logs_test

import (
	"database/sql"
	"database/sql/driver"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/timerange"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestNativeArrayAndBodyPredicates(t *testing.T) {
	s, ctx := storetest.New(t)
	data := plog.NewLogs()
	records := data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	for _, value := range []any{int64(math.MaxInt64), "9223372036854775807", int64(math.MaxInt64 - 1), true, "true", 42.5} {
		r := records.AppendEmpty()
		require.NoError(t, r.Attributes().PutEmptySlice("values").AppendEmpty().FromRaw(value))
	}
	records.AppendEmpty().Attributes().PutEmptySlice("values")
	records.AppendEmpty().Body().SetEmptyMap().PutStr("status", "ready")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	for _, tc := range []struct {
		kind, operator, value string
		want                  int
	}{
		{"int64[]", "CONTAINS", "9223372036854775807", 1},
		{"int64[]", "CONTAINS", "9223372036854775806", 1},
		{"string[]", "CONTAINS", "9223372036854775807", 1},
		{"boolean[]", "CONTAINS", "true", 1},
		{"string[]", "CONTAINS", "true", 1},
		{"float64[]", "CONTAINS", "42.5", 1},
		{"array", "CONTAINS", "9223372036854775807", 2},
		{"array", "NOT CONTAINS", "9223372036854775807", 5},
		{"body", "CONTAINS", "ready", 1},
		{"body", "CONTAINS", "kind", 0},
		{"body", "CONTAINS", "value", 0},
	} {
		t.Run(tc.kind+"/"+tc.operator+"/"+tc.value, func(t *testing.T) {
			field := &search.FieldDefinition{Owner: "log", Path: []any{"attributes", "values"}, Type: tc.kind}
			if tc.kind == "body" {
				field = &search.FieldDefinition{Name: "body", SearchScope: "field"}
			}
			query, args, err := logs.MatchingRowsSQL(&search.QueryNode{Type: "condition", Query: &search.Query{Field: field, FieldOperator: tc.operator, Value: tc.value}}, timerange.TimeRange{})
			require.NoError(t, err)
			require.NotContains(t, query, "otel_value_json")
			require.NotContains(t, query, "json_each")
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				var count int
				if err := db.QueryRowContext(ctx, "select count(*) from ("+query+")", args...).Scan(&count); err != nil {
					return err
				}
				require.Equal(t, tc.want, count)
				return nil
			}))
		})
	}
}
