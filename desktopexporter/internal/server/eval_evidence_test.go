package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.uber.org/zap"
)

func TestCollectorEvalRedactionFidelityAndCacheRebuild(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	startEvalReceiver(t, s)
	input, err := os.ReadFile("../evaldecode/testdata/promptfoo-v3.json")
	require.NoError(t, err)
	r := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest?name=synthetic.json", bytes.NewReader(input)))
	require.Equal(t, 200, r.Code, r.Body.String())
	var reply struct {
		IDs []string `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &reply))
	id := reply.IDs[0]
	read := func() evals.RecordPage {
		var page evals.RecordPage
		require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
			var err error
			page, err = evals.Records(t.Context(), db, id, 0, 100)
			return err
		}))
		return page
	}
	page := read()
	require.Len(t, page.Rows, 1)
	for _, text := range []string{`"large":9007199254740993`, `"negativeZero":-0`, `"decimal":1.2300e+02`, `"stringNumber":"9007199254740993"`, `"null":null`, `"success":false`, `"secret":"****"`, `"password":"****"`} {
		require.Contains(t, string(page.Rows[0]), text)
	}
	require.Contains(t, string(page.Context), `1e9999`)
	require.NotContains(t, string(page.Context), "synthetic-secret")
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		for _, q := range []string{`select count(*) from logs where body::varchar like '%synthetic-secret%' or body::varchar like '%synthetic-nested-secret%'`, `select count(*) from eval_records where payload::varchar like '%synthetic-nested-secret%'`, `select count(*) from eval_datasets where context::varchar like '%synthetic-secret%'`} {
			var n int
			require.NoError(t, db.QueryRow(q).Scan(&n))
			require.Zero(t, n)
		}
		var n int
		require.NoError(t, db.QueryRow(`select count(*) from spans`).Scan(&n))
		require.Zero(t, n)
		require.NoError(t, db.QueryRow(`select count(*) from logs where trace_id is not null or span_id is not null or timestamp<>0`).Scan(&n))
		require.Zero(t, n)
		return nil
	}))
	// Exercise the existing single-log OTLP serializer, then decode the retained
	// body again. This is an internal fidelity contract, not an Evals archive API.
	var refs []string
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		rows, err := db.Query(`select id::varchar from logs`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref string
			if err := rows.Scan(&ref); err != nil {
				return err
			}
			refs = append(refs, ref)
		}
		return rows.Err()
	}))
	for _, ref := range refs {
		var raw json.RawMessage
		require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error { var err error; raw, err = logs.GetLogOTLP(t.Context(), db, ref); return err }))
		data, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(raw)
		require.NoError(t, err)
		part, err := evaldecode.Decode(data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0))
		require.NoError(t, err)
		if part.Row == 0 {
			require.Equal(t, string(page.Rows[0]), string(part.Payload))
		}
	}
	// Destroy every disposable projection/tracking table; retained logs alone
	// recreate the same source identities, values and experiment indexes.
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error {
		for _, table := range []string{"eval_experiment_search", "eval_experiment_rows", "eval_experiments", "eval_experiment_indexes", "eval_records", "eval_datasets", "eval_parts", "eval_imports", "eval_transfers", "eval_evidence"} {
			if _, err := db.Exec(`delete from ` + table); err != nil {
				return err
			}
		}
		return nil
	}))
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	require.Equal(t, page, read())
	// The size-based retention path also invalidates all derived copies, rather
	// than leaving an apparently complete Evals dataset after its logs expire.
	require.NoError(t, h.store.EnforceRetention(t.Context(), 1))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		_, err := evals.Records(t.Context(), db, id, 0, 100)
		require.ErrorIs(t, err, evals.ErrEvalNotFound)
		return nil
	}))
	// Retention/ordinary log deletion cannot leave source or search copies alive.
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error { return logs.Clear(t.Context(), db) }))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		for _, table := range []string{"eval_records", "eval_experiment_search", "eval_experiment_rows", "eval_experiments"} {
			var n int
			require.NoError(t, db.QueryRow(`select count(*) from `+table).Scan(&n))
			require.Zero(t, n)
		}
		_, err := evals.Records(t.Context(), db, id, 0, 100)
		require.ErrorIs(t, err, evals.ErrEvalNotFound)
		return nil
	}))
}

func TestEvalManifestCoverageDuplicatesConflictsAndDeletionGeneration(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	startEvalReceiver(t, s)
	input := `{"evalId":"synthetic","config":{},"results":{"version":3,"results":[{"value":-0}]}}`
	var parts []evaldecode.IngestPart
	require.NoError(t, evaldecode.ParseIngest(t.Context(), strings.NewReader(input), func(p evaldecode.IngestPart) error { parts = append(parts, p); return nil }))
	id := strings.Repeat("d", 64)
	manifest := evaldecode.IngestPart{Envelope: -1, Row: -2, Count: 1, Payload: json.RawMessage(`{"name":"synthetic"}`)}
	require.NoError(t, s.forwardEvalPart(t.Context(), id, manifest))
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	complete, _, err := s.evalImportState(t.Context(), id)
	require.NoError(t, err)
	require.False(t, complete)
	for range 2 {
		for _, part := range parts {
			require.NoError(t, s.forwardEvalPart(t.Context(), id, part))
		}
	}
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	complete, _, err = s.evalImportState(t.Context(), id)
	require.NoError(t, err)
	require.True(t, complete)
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var n int
		require.NoError(t, db.QueryRow(`select count(*) from eval_records`).Scan(&n))
		require.Equal(t, 1, n)
		return nil
	}))
	conflict := parts[0]
	conflict.Payload = json.RawMessage(`{"value":"conflicting-value"}`)
	require.NoError(t, s.forwardEvalPart(t.Context(), id, conflict))
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	complete, reason, err := s.evalImportState(t.Context(), id)
	require.NoError(t, err)
	require.False(t, complete)
	require.Contains(t, reason, "Conflicting")
	var ref string
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select id::varchar from logs where body::varchar like '%conflicting-value%'`).Scan(&ref)
	}))
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error { return logs.DeleteLogsByRefs(t.Context(), db, []any{ref}) }))
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	complete, _, err = s.evalImportState(t.Context(), id)
	require.NoError(t, err)
	require.True(t, complete)
	// The existing deliberate dataset tombstone produces a fresh generation on
	// explicit reimport. Rebuilding an index never removes the tombstone.
	post := func() *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
		return r
	}
	r := post()
	require.Equal(t, 200, r.Code, r.Body.String())
	var first, second struct {
		IDs []string `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &first))
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`update eval_datasets set deleted=true where id=?`, first.IDs[0])
		return err
	}))
	r = post()
	require.Equal(t, 200, r.Code, r.Body.String())
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &second))
	require.NotEqual(t, first.IDs, second.IDs)
}

func TestEvalAcceptedIsNotPersistedCancellationConcurrentRetryAndLateArrival(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	startEvalReceiver(t, s)
	realURL := s.evalReceiverURL
	var mu sync.Mutex
	var accepted [][]byte
	manifestAccepted := make(chan struct{})
	ack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", 500)
			return
		}
		mu.Lock()
		accepted = append(accepted, body)
		count := len(accepted)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
		if count == 3 {
			close(manifestAccepted)
		}
	}))
	defer ack.Close()
	s.evalReceiverURL = ack.URL
	input := `{"evalId":"late","config":{},"results":{"version":3,"results":[{"value":9007199254740993}]}}`
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)).WithContext(ctx))
		done <- r
	}()
	select {
	case <-manifestAccepted:
	case <-time.After(10 * time.Second):
		t.Fatal("submission did not reach manifest")
	}
	r := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
	require.Equal(t, 409, r.Code, r.Body.String())
	select {
	case r := <-done:
		t.Fatalf("receiver acceptance completed import: %d %s", r.Code, r.Body.String())
	default:
	}
	cancel()
	select {
	case r := <-done:
		require.NotEqual(t, 200, r.Code)
	case <-time.After(10 * time.Second):
		t.Fatal("cancellation did not stop wait")
	}
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		for _, table := range []string{"logs", "eval_records", "eval_datasets"} {
			var n int
			require.NoError(t, db.QueryRow(`select count(*) from `+table).Scan(&n))
			require.Zero(t, n)
		}
		return nil
	}))
	mu.Lock()
	queued := append([][]byte(nil), accepted...)
	mu.Unlock()
	for _, body := range queued {
		response, err := http.Post(realURL, "application/x-protobuf", bytes.NewReader(body))
		require.NoError(t, err)
		require.Equal(t, 200, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	s.evalReceiverURL = realURL
	r = httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
	require.Equal(t, 200, r.Code, r.Body.String())
	require.Contains(t, r.Body.String(), `"created":false`)
}

func TestEvalForwardingRejectsPartialAndMalformedAcknowledgements(t *testing.T) {
	partial := plogotlp.NewExportResponse()
	partial.PartialSuccess().SetRejectedLogRecords(1)
	partial.PartialSuccess().SetErrorMessage("synthetic rejection")
	partialBytes, err := partial.MarshalProto()
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		status int
		body   []byte
	}{
		{"partial", 200, partialBytes},
		{"malformed", 200, []byte("invalid protobuf")},
		{"unavailable", 503, nil},
		{"redirect", 307, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-protobuf")
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(tc.status)
				_, _ = w.Write(tc.body)
			}))
			defer receiver.Close()
			s := &Server{evalReceiverURL: receiver.URL}
			err := s.forwardEvalPart(t.Context(), strings.Repeat("a", 64), evaldecode.IngestPart{Payload: json.RawMessage(`{"value":1}`)})
			require.Error(t, err)
		})
	}
}
