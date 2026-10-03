package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestStoreStartupHardening(t *testing.T) {
	for _, tc := range []struct {
		name string
		path func(*testing.T) string
	}{
		{"in-memory", func(*testing.T) string { return "" }},
		{"file-backed", func(t *testing.T) string { return filepath.Join(t.TempDir(), "viewer.db") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dbPath := tc.path(t)
			s, err := NewStore(ctx, dbPath, zap.NewNop())
			require.NoError(t, err)
			verifyStoreHardening(t, s)

			err = s.WithConn(func(conn driver.Conn) error {
				return spans.Ingest(ctx, conn, buildStoreTestTraces(), s.FlushedIDs())
			})
			require.NoError(t, err)
			require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
				_, err := db.ExecContext(ctx, "CHECKPOINT")
				return err
			}))
			assertStoredSpanCount(t, s, 2)
			require.NoError(t, s.Close())

			if dbPath != "" {
				s, err = NewStore(ctx, dbPath, zap.NewNop())
				require.NoError(t, err)
				verifyStoreHardening(t, s)
				assertStoredSpanCount(t, s, 2)
				require.NoError(t, s.Close())
			}
		})
	}
}

func TestStoreRejectsDuckDBDSNDelimitersInPath(t *testing.T) {
	for _, delimiter := range []string{"?", "#"} {
		_, err := NewStore(context.Background(), filepath.Join(t.TempDir(), "viewer"+delimiter+"unsafe.db"), zap.NewNop())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrStoreInitFailed)
		assert.Contains(t, err.Error(), "database path cannot contain '?' or '#'")
	}
}

func verifyStoreHardening(t *testing.T, s *Store) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "host.csv")
	require.NoError(t, os.WriteFile(fixture, []byte("value\nsecret\n"), 0o600))

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		connections := make([]*sql.Conn, maxPoolConns)
		for i := range connections {
			conn, err := db.Conn(context.Background())
			if err != nil {
				return err
			}
			connections[i] = conn
			defer conn.Close()
		}

		for _, conn := range connections {
			for name, expected := range map[string]string{
				"autoload_known_extensions":    "false",
				"autoinstall_known_extensions": "false",
				"allow_community_extensions":   "false",
				"enable_external_access":       "false",
				"lock_configuration":           "true",
			} {
				var actual string
				if err := conn.QueryRowContext(context.Background(), "SELECT current_setting(?)::varchar", name).Scan(&actual); err != nil {
					return err
				}
				if actual != expected {
					return fmt.Errorf("setting %s = %s, want %s", name, actual, expected)
				}
			}
		}

		for _, conn := range connections {
			var kind string
			if err := conn.QueryRowContext(context.Background(), `
          select json_extract_string('{"kind":"int64","value":"7"}'::json, '$.kind')`).Scan(&kind); err != nil {
				return err
			}
			assert.Equal(t, "int64", kind)

			var winterOffset, summerOffset int64
			if err := conn.QueryRowContext(context.Background(), `
          select tz_offset_ns_at(1710052200000000000::ubigint, 'America/New_York'),
                 tz_offset_ns_at(1710055800000000000::ubigint, 'America/New_York')`).Scan(&winterOffset, &summerOffset); err != nil {
				return err
			}
			assert.Equal(t, int64(-18000000000000), winterOffset)
			assert.Equal(t, int64(-14400000000000), summerOffset)
		}

		assertQueryFails(t, connections[0], "SELECT * FROM read_csv(?)", fixture)
		assertQueryFails(t, connections[0], "SELECT read_text(?)", fixture)
		assertQueryFails(t, connections[0], "SELECT * FROM read_csv('https://example.com/private.csv')")
		assertExecFails(t, connections[0], "ATTACH ? AS external_db", filepath.Join(t.TempDir(), "other.db"))
		assertExecFails(t, connections[0], "COPY spans TO ?", filepath.Join(t.TempDir(), "spans.csv"))
		assertExecFails(t, connections[0], "INSTALL httpfs")
		assertExecFails(t, connections[0], "LOAD httpfs")
		assertExecFails(t, connections[0], "SET enable_external_access = true")
		assertExecFails(t, connections[0], "SET autoload_known_extensions = true")
		assertExecFails(t, connections[0], "SET autoinstall_known_extensions = true")
		assertExecFails(t, connections[0], "SET allow_community_extensions = true")
		assertExecFails(t, connections[0], "SET threads = 1")
		return nil
	}))
}

func assertQueryFails(t *testing.T, conn *sql.Conn, statement string, args ...any) {
	t.Helper()
	rows, err := conn.QueryContext(context.Background(), statement, args...)
	if rows != nil {
		rows.Close()
	}
	require.Error(t, err, statement)
}

func assertExecFails(t *testing.T, conn *sql.Conn, statement string, args ...any) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), statement, args...)
	require.Error(t, err, statement)
}

func assertStoredSpanCount(t *testing.T, s *Store, expected int) {
	t.Helper()
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM spans").Scan(&count); err != nil {
			return err
		}
		assert.Equal(t, expected, count)
		return nil
	}))
}
