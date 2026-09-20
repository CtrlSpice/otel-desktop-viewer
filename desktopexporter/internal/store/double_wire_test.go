package store

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDoubleWireBitsSurvivePersistentStorage(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "double-wire.db")
	want := []struct {
		value float64
		wire  string
	}{
		{math.Copysign(0, -1), `"0x8000000000000000"`},
		{math.Float64frombits(0x7ff8000000000001), `"0x7ff8000000000001"`},
		{math.Float64frombits(0xfff8000000000002), `"0xfff8000000000002"`},
		{math.Inf(1), `"0x7ff0000000000000"`},
		{math.Inf(-1), `"0xfff0000000000000"`},
		{1.25, `1.25`},
	}

	s, err := NewStore(ctx, path, zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		if _, err := db.ExecContext(ctx, `create table double_wire_fixture (position integer, value double)`); err != nil {
			return err
		}
		for i, fixture := range want {
			if _, err := db.ExecContext(ctx, `insert into double_wire_fixture values (?, ?)`, i, fixture.value); err != nil {
				return err
			}
		}
		return nil
	}))
	require.NoError(t, s.Close())

	s, err = NewStore(ctx, path, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		connections := make([]*sql.Conn, maxPoolConns)
		for i := range connections {
			conn, err := db.Conn(ctx)
			if err != nil {
				return err
			}
			connections[i] = conn
			defer conn.Close()
		}

		rows, err := connections[len(connections)-1].QueryContext(ctx, `select double_wire_json(value)::varchar from double_wire_fixture order by position`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for _, fixture := range want {
			require.True(t, rows.Next())
			var wire string
			require.NoError(t, rows.Scan(&wire))
			require.Equal(t, fixture.wire, wire)
		}
		require.False(t, rows.Next())
		return rows.Err()
	}))
}
