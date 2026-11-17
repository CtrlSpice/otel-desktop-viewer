package evals_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/require"
)

func TestCatalogueRequiresOneResultForEntireExperimentPredicate(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	input := `{"evalId":"experiment","config":{},"results":{"version":3,"results":[
	{"provider":{"id":"providerword"},"promptId":"prompt","success":true,"response":{"output":"alpha"}},
	{"provider":{"id":"providerword"},"promptId":"prompt","success":false,"response":{"output":"beta"}}]}}`
	var parts []evals.IngestPart
	require.NoError(t, evaldecode.ParseIngest(t.Context(), strings.NewReader(input), func(p evals.IngestPart) error { parts = append(parts, p); return nil }))
	importID := strings.Repeat("f", 64)
	datasetID := evals.DatasetID(importID, 0)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		if _, err := db.Exec(`insert into eval_imports(id) values(?)`, importID); err != nil {
			return err
		}
		if err := evals.AppendParts(t.Context(), db, importID, "catalogue.json", parts); err != nil {
			return err
		}
		return evals.Publish(t.Context(), db, importID)
	}))
	require.NoError(t, evals.EnsureExperimentIndex(t.Context(), s, datasetID, "dataset"))
	text := func(v string) search.QueryNode {
		return search.QueryNode{Type: "text", Query: &search.Query{FieldOperator: "CONTAINS", Value: v}}
	}
	condition := func(v string) search.QueryNode {
		return search.QueryNode{Type: "condition", Query: &search.Query{Field: &search.FieldDefinition{Owner: "result", Path: []any{"success"}}, FieldOperator: "=", Value: v}}
	}
	count := func(want int64, children ...search.QueryNode) {
		t.Helper()
		criteria := &search.QueryNode{Type: "group", Group: &search.QueryGroup{LogicalOperator: "AND", Children: children}}
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			page, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Criteria: criteria, Limit: 10, Sort: "name", Direction: "asc"})
			if err != nil {
				return err
			}
			require.Equal(t, want, page.Total)
			return nil
		}))
	}
	count(1, text("providerword"), text("alpha"))
	count(0, text("alpha"), text("beta"))
	count(0, text("alpha"), condition("false"))
	count(1, text("alpha"), condition("true"))
	count(0, text("alph"))
	// The rebuild contract permits discarding disposable tracking tables while
	// retaining the source. Old derived owners must be removed transactionally,
	// even when no eval_experiment_search row survives to identify them.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`delete from eval_experiment_search; delete from eval_experiment_rows; delete from eval_experiments; delete from eval_experiment_indexes`)
		return err
	}))
	require.NoError(t, evals.EnsureExperimentIndex(t.Context(), s, datasetID, "dataset"))
	count(1, text("alpha"), condition("true"))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var n int
		require.NoError(t, db.QueryRow(`select count(*) from search_owners where signal='experiment' and owner='result'`).Scan(&n))
		require.Equal(t, 2, n)
		return nil
	}))
	require.NoError(t, evals.PurgeExperimentIndex(t.Context(), s, datasetID))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var n int
		require.NoError(t, db.QueryRow(`select count(*) from search_owners where signal='experiment'`).Scan(&n))
		require.Zero(t, n)
		require.NoError(t, db.QueryRow(`select count(*) from search_postings l where not exists(select 1 from search_owners o where o.id=l.owner_id)`).Scan(&n))
		require.Zero(t, n)
		return nil
	}))
	require.NoError(t, evals.EnsureExperimentIndex(t.Context(), s, datasetID, "dataset"))
	count(1, text("alpha"), condition("true"))
}
