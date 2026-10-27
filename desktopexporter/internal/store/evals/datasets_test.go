package evals_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
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
	require.NoError(t, evaldecode.ParseIngest(t.Context(), strings.NewReader(input), func(p evals.IngestPart) error { parts = append(parts, p); return nil }))
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
	require.NoError(t, evals.EnsureExperimentIndex(t.Context(), s, evals.DatasetID(importID, 0), "dataset"))
	require.NoError(t, s.Close())
	s, err = store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		page, err := evals.Records(t.Context(), db, evals.DatasetID(importID, 0), 0, 1)
		require.NoError(t, err)
		require.JSONEq(t, string(parts[0].Payload), string(page.Rows[0]))
		require.Contains(t, string(page.Rows[0]), `"exact":18446744073709551615`)
		var typ string
		require.NoError(t, db.QueryRow(`select typeof(payload) from eval_records limit 1`).Scan(&typ))
		require.Equal(t, "VARIANT", typ)
		var exact string
		var negativeZero, hasNull bool
		require.NoError(t, db.QueryRow(`select payload.exact::ubigint::varchar,
		 signbit(payload.zero::double), variant_exists(payload,'unknown')
		 from eval_records where payload.id::varchar='sample'`).Scan(&exact, &negativeZero, &hasNull))
		require.Equal(t, "18446744073709551615", exact)
		require.True(t, negativeZero)
		require.True(t, hasNull)
		var byteSize int64
		require.NoError(t, db.QueryRow(`select byte_size from eval_datasets where id=?`, evals.DatasetID(importID, 0)).Scan(&byteSize))
		require.Equal(t, fmt.Sprint(len(input)), fmt.Sprint(byteSize))
		experiments, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Limit: 10, Sort: "name", Direction: "asc"})
		require.NoError(t, err)
		require.Len(t, experiments.Experiments, 1)
		detail, err := evals.GetExperimentPage(t.Context(), db, experiments.Experiments[0].ID, -1, 100)
		require.NoError(t, err)
		require.JSONEq(t, string(parts[0].Payload), detail.Rows[0].JSON)
		require.Contains(t, detail.Rows[0].JSON, `"exact":18446744073709551615`)
		return nil
	}))
}

func TestNativeEvalNumberRanges(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return evals.AppendParts(t.Context(), db, "numbers", "numbers.json", []evals.IngestPart{{
			Payload: json.RawMessage(`{"signed":-9223372036854775808,"unsigned":18446744073709551615,"overflow":18446744073709551616,"huge":1e9999,"decimal":1.2300e+02,"zero":-0,"text":"-0 1e9999 \"quoted\""}`),
		}})
	}))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var raw, signed, unsigned, overflowKind, hugeKind string
		var negativeZero bool
		err := db.QueryRow(`select to_json(payload)::varchar,
		 payload.signed::bigint::varchar, payload.unsigned::ubigint::varchar,
		 variant_typeof(payload.overflow), variant_typeof(payload.huge),
		 signbit(payload.zero::double) from eval_records`).Scan(&raw, &signed, &unsigned, &overflowKind, &hugeKind, &negativeZero)
		require.NoError(t, err)
		require.True(t, json.Valid([]byte(raw)))
		require.Equal(t, "-9223372036854775808", signed)
		require.Equal(t, "18446744073709551615", unsigned)
		require.Equal(t, "VARCHAR", overflowKind)
		require.Equal(t, "VARCHAR", hugeKind)
		require.True(t, negativeZero)
		require.Contains(t, raw, `"overflow":"18446744073709551616"`)
		require.Contains(t, raw, `"huge":"1e9999"`)
		require.Contains(t, raw, `"decimal":123.0`)
		require.Contains(t, raw, `"text":"-0 1e9999 \"quoted\""`)
		return nil
	}))
}
