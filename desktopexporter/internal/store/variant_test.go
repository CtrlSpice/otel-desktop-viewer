package store

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestNativeVariantsRoundTripAndShred(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "variants.duckdb")
	s, err := NewStore(ctx, path, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if s != nil {
			_ = s.Close()
		}
	})
	data := plog.NewLogs()
	records := data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords()
	want := map[string]string{}
	values := []any{
		map[string]any{"user": map[string]any{"id": int64(9007199254740993), "tier": "pro"},
			"mixed": []any{int64(1), float64(1), "1", nil, []byte{0, 255}},
			"zero":  math.Copysign(0, -1), "nan": math.Float64frombits(0x7ff8000000000001),
			"'); DROP TABLE logs; --": "a.b['x']\x00<>&\u2028"},
		int64(math.MinInt64), int64(math.MaxInt64), float64(1), true, false,
		[]any{}, map[string]any{}, []byte{}, "", nil,
		map[string]any{"deep": []any{map[string]any{"a": []any{nil, "tail"}}}, "sibling": int64(9)},
		map[string]any{"": "empty key", "a.b": "literal", "key\x00🦆": "value\x00",
			"large": strings.Repeat("abcdef", 12000), "infinity": math.Inf(1), "negative_infinity": math.Inf(-1)},
		map[string]any{"siblings": []any{map[string]any{"zero": math.Copysign(0, -1)}, map[string]any{"zero": float64(0)}}},
		map[string]any{"siblings": []any{map[string]any{"zero": float64(0)}, map[string]any{"zero": math.Copysign(0, -1)}}},
	}
	// Multi-byte metadata offset tables must not wrap their cursor at byte 256.
	wide := map[string]any{"": ""}
	for i := range 300 {
		wide[fmt.Sprintf("field.%03d", i)] = map[string]any{"value": int64(9007199254740993), "empty": ""}
	}
	values = append(values, wide)
	// Repeated payloads must stay attached to every owner, including sibling
	// rows whose only difference is the sign of a nested zero.
	values = append(values, values...)
	for i, raw := range values {
		log := records.AppendEmpty()
		name := fmt.Sprintf("fixture-%d", i)
		log.SetEventName(name)
		require.NoError(t, log.Body().FromRaw(raw))
		log.Body().CopyTo(log.Attributes().PutEmpty(name))
		encoded, err := util.EncodeValue(log.Body())
		require.NoError(t, err)
		want[name] = string(encoded)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, data)
	}))
	// The input can be reused immediately after ingestion.
	records.At(0).Body().SetStr("mutated")
	verify := func() {
		t.Helper()
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			for name, expected := range want {
				var body, attr string
				err := db.QueryRow(`select otel_value_json(l.body)::varchar,
				 otel_value_json(l.attributes[$1])::varchar from logs l
				 where l.event_name=$1 limit 1`, name).Scan(&body, &attr)
				require.NoError(t, err, name)
				require.Equal(t, expected, body, name)
				require.Equal(t, expected, attr, name)
			}
			var id int64
			require.NoError(t, db.QueryRow(`select body.user.id::bigint from logs
			 where body.user.tier::varchar='pro' limit 1`).Scan(&id))
			require.Equal(t, int64(9007199254740993), id)
			var count int
			require.NoError(t, db.QueryRow(`select count(*) from logs
			 where otel_value_kind(attributes) <> 'map'`).Scan(&count))
			require.Zero(t, count)
			return nil
		}))
	}
	verify()
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`insert into logs select uuid(), l.* exclude(id)
		 from logs l, range(32000) where l.event_name='fixture-0'; checkpoint`)
		return err
	}))
	require.NoError(t, s.Close())
	s, err = NewStore(ctx, path, nil)
	require.NoError(t, err)
	verify()
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var segments int
		err := db.QueryRow(`select count(*) from pragma_storage_info('logs')
		 where column_name='body' and segment_type in ('BIGINT','DOUBLE')`).Scan(&segments)
		require.NoError(t, err)
		require.Positive(t, segments, "nested native values should shred at checkpoint")
		return nil
	}))
}
