package evals_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/stretchr/testify/require"
)

type afterIndexedBatch struct {
	*store.Store
	after func()
}

func (s afterIndexedBatch) WithDBWrite(fn func(*sql.DB) error) error {
	err := s.Store.WithDBWrite(fn)
	if err != nil {
		return err
	}
	var count int
	err = s.Store.WithDBRead(func(db *sql.DB) error {
		if err := db.QueryRow(`select count(*) from eval_experiment_rows`).Scan(&count); err != nil {
			return err
		}
		return nil
	})
	if err == nil && count >= 64 {
		s.after()
	}
	return err
}

func TestExperimentIndexCancellationAndRetryCountsWholeExperiment(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	row := `{"provider":{"id":"p"},"promptId":"prompt","success":true,"vars":{"question":"Why did checkout fail?"},"response":{"output":"Retry budget exhausted"},"gradingResult":{"reason":"Missing idempotency key"}}`
	input := `{"evalId":"experiment","config":{},"results":{"version":3,"results":[` + strings.TrimSuffix(strings.Repeat(row+",", 130), ",") + `]}}`
	var parts []evals.IngestPart
	require.NoError(t, evals.ParseIngest(t.Context(), strings.NewReader(input), func(part evals.IngestPart) error {
		parts = append(parts, part)
		return nil
	}))
	importID := strings.Repeat("a", 64)
	datasetID := evals.DatasetID(importID, 0)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		if _, err := db.Exec(`insert into eval_imports(id) values (?)`, importID); err != nil {
			return err
		}
		if err := evals.AppendParts(t.Context(), db, importID, "experiment.json", parts); err != nil {
			return err
		}
		return evals.Publish(t.Context(), db, importID)
	}))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err = evals.EnsureExperimentIndex(ctx, afterIndexedBatch{s, cancel}, datasetID, "dataset")
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		page, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Limit: 10, Sort: "name", Direction: "asc"})
		require.NoError(t, err)
		require.Zero(t, page.Total)
		require.Equal(t, int64(1), page.Indexing.Remaining)
		return nil
	}))
	require.NoError(t, evals.EnsureExperimentIndex(t.Context(), s, datasetID, "dataset"))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		page, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Limit: 10, Sort: "name", Direction: "asc"})
		require.NoError(t, err)
		require.Equal(t, int64(1), page.Total)
		require.Equal(t, int64(130), page.Experiments[0].ResultCount)
		require.Equal(t, int64(130), page.Experiments[0].Outcomes[0].Count)
		for _, text := range []string{"checkout fail", "RETRY budget", "idempotency key"} {
			found, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Query: text, Limit: 10, Sort: "name", Direction: "asc"})
			require.NoError(t, err)
			require.Equal(t, int64(1), found.Total)
		}
		// JSON member names are schema, not received result text.
		keys, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Query: "gradingResult", Limit: 10, Sort: "name", Direction: "asc"})
		require.NoError(t, err)
		require.Zero(t, keys.Total)
		return nil
	}))
	// Rebuild old concatenated titles without changing source rows or local IDs.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`update eval_experiments set name='legacy description plus prompt body' where dataset_id=?`, datasetID)
		return err
	}))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		candidate, err := evals.NextExperimentIndex(t.Context(), db)
		require.NoError(t, err)
		require.Equal(t, datasetID, candidate.ID)
		return nil
	}))
	require.NoError(t, evals.EnsureExperimentIndex(t.Context(), s, datasetID, "dataset"))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		_, err := evals.NextExperimentIndex(t.Context(), db)
		require.ErrorIs(t, err, sql.ErrNoRows)
		page, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Limit: 10, Sort: "name", Direction: "asc"})
		require.NoError(t, err)
		require.Equal(t, "p", page.Experiments[0].Name)
		require.Equal(t, int64(130), page.Experiments[0].ResultCount)
		return nil
	}))
	// Deletion between projection batches must not recreate a hidden source copy.
	require.NoError(t, evals.PurgeExperimentIndex(t.Context(), s, datasetID))
	deleted := false
	require.NoError(t, evals.EnsureExperimentIndex(t.Context(), afterIndexedBatch{s, func() {
		if deleted {
			return
		}
		deleted = true
		if err := s.WithDBWrite(func(db *sql.DB) error {
			_, err := db.Exec(`update eval_datasets set deleted=true where id=?`, datasetID)
			return err
		}); err != nil {
			t.Error(err)
		}
	}}, datasetID, "dataset"))
	require.True(t, deleted)
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var count int
		require.NoError(t, db.QueryRow(`select count(*) from eval_experiments`).Scan(&count))
		require.Zero(t, count)
		require.NoError(t, db.QueryRow(`select count(*) from eval_experiment_search`).Scan(&count))
		require.Zero(t, count)
		return nil
	}))
}
