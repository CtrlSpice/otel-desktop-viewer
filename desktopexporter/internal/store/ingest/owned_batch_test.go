package ingest_test

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
)

// Each row retains its own payload, including values SQL equality considers
// equal, such as positive and negative floating zero.
func TestOwnedBatchPreservesEveryTypedValue(t *testing.T) {
	s, ctx := storetest.New(t)
	data := plog.NewLogs()
	records := data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	values := []any{
		float64(0), math.Copysign(0, -1), int64(0), nil,
		map[string]any{"zero": float64(0)}, map[string]any{"zero": math.Copysign(0, -1)},
		map[string]any{"nested": []any{map[string]any{"zero": float64(0)}}},
		map[string]any{"nested": []any{map[string]any{"zero": math.Copysign(0, -1)}}},
		map[string]any{"a": []any{1.1, 2.2, 3.3}, "b": []any{true, false}, "c": []any{int64(42), "42"}},
		map[string]any{"a": []any{map[string]any{"b": []any{int64(9007199254740993), []byte{0, 255}}}}},
		[]any{[]any{}, map[string]any{}, nil}, math.NaN(), math.Inf(1), math.Inf(-1),
	}
	want := map[string]string{}
	for repeat := range 3 {
		for i, value := range values {
			record := records.AppendEmpty()
			name := fmt.Sprintf("%d/%d", repeat, i)
			record.SetEventName(name)
			require.NoError(t, record.Body().FromRaw(value))
			record.Body().CopyTo(record.Attributes().PutEmpty("copy"))
			encoded, err := util.EncodeValue(record.Body())
			require.NoError(t, err)
			want[name] = string(encoded)
		}
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data)
	}))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.Query(`select event_name, otel_value_json(body)::varchar,
			otel_value_json(attributes.copy)::varchar from logs`)
		if err != nil {
			return err
		}
		defer rows.Close()
		seen := map[string]bool{}
		for rows.Next() {
			var name, body, attr string
			require.NoError(t, rows.Scan(&name, &body, &attr))
			require.False(t, seen[name], "a received record must not multiply")
			require.JSONEq(t, want[name], body, name)
			require.JSONEq(t, want[name], attr, name)
			seen[name] = true
		}
		require.Len(t, seen, len(want), "no received record may disappear during construction")
		return rows.Err()
	}))
}
