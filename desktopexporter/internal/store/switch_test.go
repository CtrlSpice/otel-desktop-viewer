package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

func insertNamedLog(t *testing.T, s *Store, body string) {
	t.Helper()
	batch := plog.NewLogs()
	rec := batch.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.Body().SetStr(body)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(context.Background(), conn, batch)
	}))
}

func logBodies(t *testing.T, s *Store) []string {
	t.Helper()
	var bodies []string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(context.Background(), `select body::varchar from logs order by body::varchar`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				return err
			}
			bodies = append(bodies, b)
		}
		return rows.Err()
	}))
	return bodies
}

func TestStoreCreateAndOpenDatabaseSwitchesCleanly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "current.duckdb")
	secondPath := filepath.Join(dir, "incident.duckdb")

	s, err := NewStore(ctx, firstPath, zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	insertNamedLog(t, s, "first-session-log")
	revBefore, chBefore, closed := s.CommitSnapshot()
	require.False(t, closed)

	var remembered string
	require.NoError(t, s.CreateDatabase(ctx, secondPath, func() error {
		remembered = secondPath
		return nil
	}))
	assert.Equal(t, secondPath, remembered)
	assert.Equal(t, secondPath, s.DBPath())
	assert.Empty(t, logBodies(t, s), "new database starts empty")

	// Switching away checkpointed the first file so no stale WAL remains.
	_, walErr := os.Stat(firstPath + ".wal")
	assert.True(t, errors.Is(walErr, os.ErrNotExist), "switched-away database should be checkpointed")

	// Live-update observer woke up and advanced monotonically.
	select {
	case <-chBefore:
	default:
		t.Fatal("expected CommitSnapshot channel to wake on database switch")
	}
	revAfterCreate, _, closed := s.CommitSnapshot()
	require.False(t, closed)
	assert.Greater(t, revAfterCreate, revBefore)

	insertNamedLog(t, s, "second-session-log")
	assert.Equal(t, []string{"second-session-log"}, logBodies(t, s))

	// Switch back to the first session and confirm its telemetry survived.
	require.NoError(t, s.OpenDatabase(ctx, firstPath, func() error {
		remembered = firstPath
		return nil
	}))
	assert.Equal(t, firstPath, remembered)
	assert.Equal(t, []string{"first-session-log"}, logBodies(t, s))

	// Switch back to the second session and confirm its telemetry survived too.
	require.NoError(t, s.OpenDatabase(ctx, secondPath, nil))
	assert.Equal(t, []string{"second-session-log"}, logBodies(t, s))
}

func TestStoreSwitchRollsBackOnErrors(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "current.duckdb")

	s, err := NewStore(ctx, firstPath, zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	insertNamedLog(t, s, "keep-me")

	t.Run("create refuses existing file", func(t *testing.T) {
		err := s.CreateDatabase(ctx, firstPath, nil)
		require.ErrorIs(t, err, ErrDatabaseAlreadyExists)
		assert.Equal(t, []string{"keep-me"}, logBodies(t, s))
	})

	t.Run("open refuses missing file", func(t *testing.T) {
		err := s.OpenDatabase(ctx, filepath.Join(dir, "missing.duckdb"), nil)
		require.ErrorIs(t, err, ErrDatabaseNotFound)
		assert.Equal(t, []string{"keep-me"}, logBodies(t, s))
	})

	t.Run("open rolls back on corrupt file", func(t *testing.T) {
		corrupt := filepath.Join(dir, "corrupt.duckdb")
		require.NoError(t, os.WriteFile(corrupt, []byte("not a duckdb file"), 0600))

		err := s.OpenDatabase(ctx, corrupt, nil)
		require.Error(t, err)
		assert.Equal(t, firstPath, s.DBPath())
		assert.Equal(t, []string{"keep-me"}, logBodies(t, s))
	})

	t.Run("create rolls back and deletes destination when callback fails", func(t *testing.T) {
		target := filepath.Join(dir, "aborted.duckdb")
		sentinel := errors.New("persist selection failed")

		err := s.CreateDatabase(ctx, target, func() error { return sentinel })
		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, firstPath, s.DBPath())
		assert.Equal(t, []string{"keep-me"}, logBodies(t, s))
		_, statErr := os.Stat(target)
		assert.True(t, errors.Is(statErr, os.ErrNotExist), "aborted destination should be cleaned up")
	})

	t.Run("create rejects pre-existing wal or tmp without removing them", func(t *testing.T) {
		for _, suffix := range []string{".wal", ".tmp"} {
			target := filepath.Join(dir, "orphan-"+suffix[1:]+".duckdb")
			sidecar := target + suffix
			require.NoError(t, os.WriteFile(sidecar, []byte("preexisting"), 0600))

			err := s.CreateDatabase(ctx, target, nil)
			require.ErrorIs(t, err, ErrDatabaseAlreadyExists)
			assert.Equal(t, firstPath, s.DBPath())
			data, readErr := os.ReadFile(sidecar)
			require.NoError(t, readErr, "pre-existing %s must not be deleted", suffix)
			assert.Equal(t, "preexisting", string(data))
		}
	})

	t.Run("create does not unlink file created during checkpoint", func(t *testing.T) {
		target := filepath.Join(dir, "raced-checkpoint.duckdb")
		beforeCreateOpenHook = func() {
			_ = os.WriteFile(target, []byte("external-owner"), 0600)
		}
		t.Cleanup(func() { beforeCreateOpenHook = nil })

		err := s.CreateDatabase(ctx, target, nil)
		require.ErrorIs(t, err, ErrDatabaseAlreadyExists)
		assert.Equal(t, firstPath, s.DBPath())
		data, readErr := os.ReadFile(target)
		require.NoError(t, readErr, "externally created file must not be unlinked")
		assert.Equal(t, "external-owner", string(data))
	})

	t.Run("create does not unlink corrupt or locked file created right before connector", func(t *testing.T) {
		target := filepath.Join(dir, "raced-connector.duckdb")
		beforeCreateConnectorHook = func() {
			_ = os.WriteFile(target, []byte("not-our-duckdb-file"), 0600)
		}
		t.Cleanup(func() { beforeCreateConnectorHook = nil })

		err := s.CreateDatabase(ctx, target, nil)
		require.Error(t, err)
		assert.Equal(t, firstPath, s.DBPath())
		data, readErr := os.ReadFile(target)
		require.NoError(t, readErr, "file not created by openDatabaseHandles must not be unlinked")
		assert.Equal(t, "not-our-duckdb-file", string(data))
	})
}

func TestStoreSchemaCompatibilityConcurrentWithSwitches(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.duckdb")
	secondPath := filepath.Join(dir, "second.duckdb")

	s, err := NewStore(ctx, firstPath, zap.NewNop())
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.CreateDatabase(ctx, secondPath, nil))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.SchemaCompatibility()
			}
		}
	}()

	for i := 0; i < 6; i++ {
		target := firstPath
		if i%2 == 1 {
			target = secondPath
		}
		require.NoError(t, s.OpenDatabase(ctx, target, nil))
	}
	close(stop)
	wg.Wait()
	assert.Equal(t, SchemaOK, s.SchemaCompatibility())
}

func TestStoreSwitchUnsupportedInMemory(t *testing.T) {
	ctx := context.Background()
	s, err := NewStore(ctx, "", zap.NewNop())
	require.NoError(t, err)
	defer s.Close()

	err = s.CreateDatabase(ctx, filepath.Join(t.TempDir(), "x.duckdb"), nil)
	require.ErrorIs(t, err, ErrDatabaseSwitchUnsupported)
}
