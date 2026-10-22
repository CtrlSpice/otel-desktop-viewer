package evals_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/stretchr/testify/require"
)

func TestIngestDatasetReopenAndPendingVisibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "received.duckdb")
	s, err := store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	importID := strings.Repeat("a", 64)
	var parts []evals.IngestPart
	input := `{"evalId":"source-run","config":{},"results":{"version":3,"results":[{"id":"sample","exact":18446744073709551615,"unknown":null,"zero":-0.00,"unicode":"雪😀"}]}}`
	require.NoError(t, evals.ParseIngest(t.Context(), strings.NewReader(input), func(p evals.IngestPart) error { parts = append(parts, p); return nil }))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`insert into eval_imports(id) values (?)`, importID)
		if err != nil {
			return err
		}
		return evals.AppendParts(t.Context(), db, importID, "source.json", parts[:1])
	}))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var count int
		if err := db.QueryRow(`select count(*) from eval_datasets d join eval_imports i on i.id=d.import_id where i.complete`).Scan(&count); err != nil {
			return err
		}
		require.Zero(t, count)
		return nil
	}))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		if err := evals.AppendParts(t.Context(), db, importID, "source.json", parts[1:]); err != nil {
			return err
		}
		return evals.Publish(t.Context(), db, importID)
	}))
	require.NoError(t, evals.EnsureRunIndex(t.Context(), s, evals.DatasetID(importID, 0), "dataset"))
	require.NoError(t, s.Close())
	s, err = store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		page, err := evals.Records(t.Context(), db, evals.DatasetID(importID, 0), 0, 1)
		require.NoError(t, err)
		require.Equal(t, string(parts[0].Payload), string(page.Rows[0]))
		var typ string
		require.NoError(t, db.QueryRow(`select typeof(payload) from eval_records limit 1`).Scan(&typ))
		require.Equal(t, "JSON", typ)
		var byteSize int64
		require.NoError(t, db.QueryRow(`select byte_size from eval_datasets where id=?`, evals.DatasetID(importID, 0)).Scan(&byteSize))
		require.Equal(t, fmt.Sprint(len(input)), fmt.Sprint(byteSize))
		runs, err := evals.SearchRuns(t.Context(), db, evals.RunSearch{Limit: 10, Sort: "name", Direction: "asc"})
		require.NoError(t, err)
		require.Len(t, runs.Runs, 1)
		detail, err := evals.GetRunPage(t.Context(), db, runs.Runs[0].ID, -1, 100)
		require.NoError(t, err)
		require.Equal(t, string(parts[0].Payload), detail.Rows[0].JSON)
		return nil
	}))
}
