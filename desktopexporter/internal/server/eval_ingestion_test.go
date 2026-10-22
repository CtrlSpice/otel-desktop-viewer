package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type evalUploadReader struct {
	io.Reader
	check func()
}

func (r *evalUploadReader) Read(p []byte) (int, error) {
	if r.check != nil {
		check := r.check
		r.check = nil
		check()
	}
	return r.Reader.Read(p)
}

func TestEvalIngestionHandlesLargeNativePayload(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	row := `{"answer":"` + strings.Repeat("x", 3<<20) + `","exact":9007199254740993}`
	input := `{"evalId":"large","config":{},"results":{"version":3,"results":[` + strings.TrimSuffix(strings.Repeat(row+",", 6), ",") + `]}}`
	reader := &evalUploadReader{Reader: strings.NewReader(input), check: func() {
		// A producer can be slow: neither readers nor writes wait on upload.
		require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error {
			var count int
			require.NoError(t, db.QueryRow(`select count(*) from eval_imports`).Scan(&count))
			require.Zero(t, count)
			return nil
		}))
	}}
	response := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/evals/ingest", reader))
	require.Equal(t, 200, response.Code, response.Body.String())
	var imported struct {
		IDs []string `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &imported))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		page, err := evals.Records(t.Context(), db, imported.IDs[0], 0, 100)
		require.NoError(t, err)
		require.Len(t, page.Rows, 1)
		require.Equal(t, int64(6), page.Total)
		require.True(t, page.HasMore)
		var count int
		require.NoError(t, db.QueryRow(`select count(*) from eval_records`).Scan(&count))
		require.Equal(t, 6, count)
		return nil
	}))
}

func TestEvalIngestionGateBeforeBodyOrStore(t *testing.T) {
	s, err := NewServer("localhost:0", 0, nil, zap.NewNop(), nil)
	require.NoError(t, err)
	r := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader("bad")))
	require.Equal(t, 403, r.Code)
	require.Contains(t, r.Body.String(), "evals.enabled")
}

func TestEvalIngestionAtomicBatchesRetryAndQuery(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	row := `{"id":"source-id","future":9007199254740993,"value":-0,"missingIsNotNull":null,"unicode":"雪😀"}`
	input := `{"evalId":"source-run","config":{},"results":{"version":3,"results":[` + strings.TrimSuffix(strings.Repeat(row+",", 130), ",") + `]}}`
	post := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/evals/ingest?name=received.jsonl", strings.NewReader(body)))
		return response
	}
	bad := post(input + "\n{\"late\":")
	require.Equal(t, 400, bad.Code)
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var count int
		require.NoError(t, db.QueryRow(`select count(*) from eval_imports`).Scan(&count))
		require.Zero(t, count)
		return nil
	}))
	response := post(input)
	require.Equal(t, 200, response.Code, response.Body.String())
	var imported struct {
		IDs      []string `json:"ids"`
		Created  bool     `json:"created"`
		Complete bool     `json:"complete"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &imported))
	require.True(t, imported.Created)
	require.True(t, imported.Complete)
	require.Len(t, imported.IDs, 1)
	id := imported.IDs[0]
	var importID string
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select import_id from eval_datasets where id = ?`, id).Scan(&importID)
	}))
	// A cancelled publish may have committed before its caller observes an error.
	// Cleanup must never erase an already-completed dataset.
	s.discardEvalImport(importID)
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		page, err := evals.Records(t.Context(), db, id, 64, 2)
		require.NoError(t, err)
		require.Equal(t, int64(130), page.Total)
		require.Len(t, page.Rows, 2)
		require.Equal(t, row, string(page.Rows[0]))
		require.True(t, page.HasMore)
		list, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Sort: "started", Direction: "desc", Limit: 10})
		require.NoError(t, err)
		require.Equal(t, int64(130), list.Total)
		return nil
	}))
	response = post(input)
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"created":false`)
	// Simulate a prior process interrupted before publication; retry replaces hidden rows.
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error { _, err := db.Exec(`update eval_imports set complete = false`); return err }))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		list, err := evals.SearchExperiments(t.Context(), db, evals.ExperimentSearch{Sort: "started", Direction: "desc", Limit: 10})
		require.NoError(t, err)
		require.Empty(t, list.Experiments)
		return nil
	}))
	response = post(input)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"created":true`)
}
