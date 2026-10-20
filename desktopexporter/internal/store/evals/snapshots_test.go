package evals_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/schema"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/require"
)

func importSnapshot(ctx context.Context, s *store.Store, name, original string) (evals.ImportResult, error) {
	var result evals.ImportResult
	err := s.WithDBWrite(func(db *sql.DB) error {
		var err error
		result, err = evals.Import(ctx, db, "local-file", "unknown-json", name, original)
		return err
	})
	return result, err
}

func TestNativeJSONExactBytesReopenAndAdditiveCompatibility(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "snapshots.duckdb")
	s, err := store.NewStore(ctx, path, nil)
	require.NoError(t, err)
	// Simulate an existing current-version file from before this additive table.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `drop table eval_snapshots`)
		return err
	}))
	require.NoError(t, s.Close())
	s, err = store.NewStore(ctx, path, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, s.Close()) }()
	corpus := []string{
		" \r\n{\n \"unknown\": [9007199254740993,18446744073709551615,-0,1.2300,1E+009,1e9999],\n \"duplicate\":1,\"duplicate\":2,\"unicode\":\"雪😀\",\"escape\":\"\\u0061\\/\\ud83d\\ude00\"\n}\t ",
		`{"attachments":["file:///not-read","https://example.invalid/not-fetched"],"future":{"opaque":true}}`,
		" null \n", " [1, 2, 3] ", `"a scalar"`, " true ", " -0.00e-01 ",
	}
	results := make([]evals.ImportResult, len(corpus))
	for i, original := range corpus {
		results[i], err = importSnapshot(ctx, s, fmt.Sprintf("result-%d.json", i), original)
		require.NoError(t, err)
		require.True(t, results[i].Created)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(original))), results[i].Snapshot.ID)
		require.Equal(t, int64(len(original)), results[i].Snapshot.ByteSize)
	}
	// Test both native JSON typing and exact text retrieval, not a parsed equality.
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var typ string
		require.NoError(t, db.QueryRowContext(ctx, `select typeof(payload) from eval_snapshots limit 1`).Scan(&typ))
		require.Equal(t, "JSON", typ)
		var version int
		require.NoError(t, db.QueryRowContext(ctx, schema.ReadVersionQuery).Scan(&version))
		require.Equal(t, schema.Version, version)
		return nil
	}))
	require.NoError(t, s.Close())
	s, err = store.NewStore(ctx, path, nil)
	require.NoError(t, err)
	for i, original := range corpus {
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			got, err := evals.Get(ctx, db, results[i].Snapshot.ID)
			require.NoError(t, err)
			require.Equal(t, original, got.OriginalJSON)
			require.Equal(t, results[i].Snapshot, got.Metadata)
			return nil
		}))
		again, err := importSnapshot(ctx, s, "different-name.json", original)
		require.NoError(t, err)
		require.False(t, again.Created)
		require.Equal(t, results[i].Snapshot, again.Snapshot)
	}
	changed, err := importSnapshot(ctx, s, "result-0.json", corpus[0]+" ")
	require.NoError(t, err)
	require.True(t, changed.Created)
	require.NotEqual(t, results[0].Snapshot.ID, changed.Snapshot.ID)

	// Telemetry clear and retention cannot silently remove archived files.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		require.NoError(t, spans.Clear(ctx, db))
		require.NoError(t, logs.Clear(ctx, db))
		require.NoError(t, metrics.Clear(ctx, db))
		return nil
	}))
	require.NoError(t, s.EnforceRetention(ctx, 1))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		page, err := evals.Search(ctx, db, 2, 0)
		require.NoError(t, err)
		require.Len(t, page.Snapshots, 2)
		require.True(t, page.HasMore)
		require.Equal(t, changed.Snapshot.ID, page.Snapshots[0].ID)
		page, err = evals.Search(ctx, db, 2, int64(len(corpus)))
		require.NoError(t, err)
		require.Len(t, page.Snapshots, 1)
		require.False(t, page.HasMore)
		page, err = evals.Search(ctx, db, 2, 100)
		require.NoError(t, err)
		require.NotNil(t, page.Snapshots)
		require.Empty(t, page.Snapshots)
		return nil
	}))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		deleted, err := evals.Delete(ctx, db, changed.Snapshot.ID)
		require.NoError(t, err)
		require.True(t, deleted.Deleted)
		_, err = evals.Delete(ctx, db, changed.Snapshot.ID)
		require.ErrorIs(t, err, evals.ErrSnapshotNotFound)
		return nil
	}))
	require.NoError(t, s.Close())
	s, err = store.NewStore(ctx, path, nil)
	require.NoError(t, err)
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		_, err := evals.Get(ctx, db, changed.Snapshot.ID)
		require.ErrorIs(t, err, evals.ErrSnapshotNotFound)
		return nil
	}))
}

func TestConcurrentExactImport(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	const n = 12
	results := make([]evals.ImportResult, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { results[i], errs[i] = importSnapshot(t.Context(), s, fmt.Sprintf("%d.json", i), "{\n\"x\":1}") })
	}
	wg.Wait()
	created := 0
	for i := range n {
		require.NoError(t, errs[i])
		require.Equal(t, results[0].Snapshot, results[i].Snapshot)
		if results[i].Created {
			created++
		}
	}
	require.Equal(t, 1, created)
}

func TestValidationAndCancellation(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	for _, original := range []string{"", "{} {}", "{", "\xef\xbb\xbf{}", "\"\xff\"", `{"a":NaN}`, `"\ud800"`, strings.Repeat(" ", evals.MaxPayloadBytes) + "{}"} {
		_, err := importSnapshot(t.Context(), s, "bad.json", original)
		require.ErrorIs(t, err, evals.ErrInvalidSnapshot)
	}
	// The exact upper bound succeeds, without allowing an extra byte.
	original := `"` + strings.Repeat("x", evals.MaxPayloadBytes-2) + `"`
	result, err := importSnapshot(t.Context(), s, "max.json", original)
	require.NoError(t, err)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		for _, args := range [][3]string{
			{"", "unknown-json", "x"}, {"file", "auto", "x"}, {"file", "unknown-json", ""},
			{strings.Repeat("x", evals.MaxSourceBytes+1), "unknown-json", "x"},
			{"file", "unknown-json", strings.Repeat("x", evals.MaxNameBytes+1)}, {"file", "unknown-json", "\xff"},
		} {
			_, err := evals.Import(t.Context(), db, args[0], args[1], args[2], "{}")
			require.ErrorIs(t, err, evals.ErrInvalidSnapshot)
		}
		for _, id := range []string{"", "../bad", strings.Repeat("A", 64), strings.Repeat("g", 64)} {
			_, err := evals.Get(t.Context(), db, id)
			require.ErrorIs(t, err, evals.ErrInvalidSnapshot)
			_, err = evals.Delete(t.Context(), db, id)
			require.ErrorIs(t, err, evals.ErrInvalidSnapshot)
		}
		for _, bounds := range [][2]int64{{0, 0}, {101, 0}, {1, -1}, {1, evals.MaxOffset + 1}} {
			_, err := evals.Search(t.Context(), db, bounds[0], bounds[1])
			require.ErrorIs(t, err, evals.ErrInvalidSnapshot)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := evals.Import(ctx, db, "file", "unknown-json", "x", "{}")
		require.ErrorIs(t, err, context.Canceled)
		_, err = evals.Get(ctx, db, result.Snapshot.ID)
		require.ErrorIs(t, err, context.Canceled)
		_, err = evals.Search(ctx, db, 1, 0)
		require.ErrorIs(t, err, context.Canceled)
		_, err = evals.Delete(ctx, db, result.Snapshot.ID)
		require.ErrorIs(t, err, context.Canceled)
		return nil
	}))
}
