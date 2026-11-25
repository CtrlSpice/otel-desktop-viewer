//go:build evals

package server

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

func TestLiveResultChecksGroupingLimitBeforeClaim(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil)
	require.NoError(t, err)
	startEvalReceiver(t, s)
	put := func(run string) int {
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, httptest.NewRequest("PUT", "/api/evals/runs/"+run+"/results/r", strings.NewReader(`{"id":"r","provider":{"id":"m"}}`)))
		return w.Code
	}
	require.Equal(t, 400, put(strings.Repeat("x", 8193)))
	var claims int
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select count(*) from eval_result_claims`).Scan(&claims)
	}))
	require.Zero(t, claims)
	require.Equal(t, 200, put(strings.Repeat("x", 8192)))
}

func TestLiveResultObservationWithoutLocalPUT(t *testing.T) {
	const route = "/api/evals/runs/run/results/r1"
	request := func(t *testing.T, s *Server, method, path, body string) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		require.Equal(t, 200, w.Code)
		return w.Body.Bytes()
	}

	// Use a real processed observation from a different store. Its claim is
	// local to that store and must not travel with the log.
	src, err := NewServer("localhost:0", 0, setupHandler(t).store, zap.NewNop(), nil)
	require.NoError(t, err)
	observation := plog.NewLogs()
	startEvalReceiverWithSink(t, src, func(data plog.Logs) { data.CopyTo(observation) })
	request(t, src, "PUT", route, `{"id":"r1","provider":{"id":"m"},"success":false}`)

	for _, tc := range []struct {
		name string
		read bool
	}{{"already reconciled", true}, {"not yet reconciled", false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewServer("localhost:0", 0, setupHandler(t).store, zap.NewNop(), nil)
			require.NoError(t, err)
			startEvalReceiver(t, s)
			data := plog.NewLogs()
			observation.CopyTo(data)
			require.NoError(t, s.store.WithConn(func(conn driver.Conn) error { return logs.Ingest(t.Context(), conn, data) }))
			groups := func() []evals.ExperimentSummary {
				var page evals.ExperimentSearchPage
				require.NoError(t, json.Unmarshal(request(t, s, "GET", "/api/evals/experiments", ""), &page))
				return page.Experiments
			}
			if tc.read {
				require.Empty(t, groups())
				var claims int
				require.NoError(t, s.store.WithDBRead(func(db *sql.DB) error {
					return db.QueryRow(`select count(*) from eval_result_claims`).Scan(&claims)
				}))
				require.Zero(t, claims)
			}
			var receipt struct{ Ordinal int64 }
			require.NoError(t, json.Unmarshal(request(t, s, "PUT", route, `{"id":"r1","provider":{"id":"m"},"success":true}`), &receipt))
			require.Zero(t, receipt.Ordinal)
			g := groups()
			require.Len(t, g, 1)
			var detail evals.ExperimentSourcePage
			require.NoError(t, json.Unmarshal(request(t, s, "GET", "/api/evals/experiments/"+g[0].ID+"/rows", ""), &detail))
			require.Len(t, detail.Rows, 1)
			require.Contains(t, detail.Rows[0].JSON, `"success":true`)
			// The same foreign observation arriving after publication must
			// neither replace the local row nor prevent an idempotent PUT.
			observation.CopyTo(data)
			require.NoError(t, s.store.WithConn(func(conn driver.Conn) error { return logs.Ingest(t.Context(), conn, data) }))
			request(t, s, "PUT", route, `{"id":"r1","provider":{"id":"m"},"success":true}`)
			require.NoError(t, json.Unmarshal(request(t, s, "GET", "/api/evals/experiments/"+g[0].ID+"/rows", ""), &detail))
			require.Len(t, detail.Rows, 1)
			require.Contains(t, detail.Rows[0].JSON, `"success":true`)
		})
	}
}

func TestLiveEvaluationRetainsEachResultOnceInItsModelGroup(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil)
	require.NoError(t, err)
	startEvalReceiver(t, s)
	request := func(method, path, body string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		require.Equal(t, status, w.Code, w.Body.String())
		return w.Body.Bytes()
	}
	first := `{"id":"r1","provider":{"id":"m"},"promptId":"p1","success":true,"sequence":9007199254740993,"password":"synthetic-password"}`
	const root = "/api/evals/runs/run/results/"
	request("PUT", root+"r1", strings.Replace(first, `"provider":{"id"`, `"provider":{"ID"`, 1), 400)
	request("PUT", root+"r1", strings.Replace(first, `"id":"r1"`, `"id":"r1","evaluationId":"elsewhere"`, 1), 400)
	request("PUT", root+"r1", first, 200)
	groups := func() []evals.ExperimentSummary {
		var p evals.ExperimentSearchPage
		require.NoError(t, json.Unmarshal(request("GET", "/api/evals/experiments", "", 200), &p))
		return p.Experiments
	}
	g := groups()
	require.Len(t, g, 1)
	require.EqualValues(t, 1, g[0].ResultCount)
	groupID := g[0].ID
	request("PUT", root+"r1", first, 200)
	request("PUT", root+"r1", strings.Replace(first, "9007199254740993", "3", 1), 409)
	request("PUT", root+"wrong", first, 400)
	request("PUT", root+"bad", `{"id":"bad","provider":{"id":"m"},"failureReason":99}`, 400)
	receiver := s.evalReceiverURL
	s.evalReceiverURL = "http://127.0.0.1:0/v1/logs"
	request("PUT", root+"lost", `{"id":"lost","provider":{"id":"m"},"success":true}`, 502)
	s.evalReceiverURL = receiver
	require.EqualValues(t, 1, groups()[0].ResultCount)
	request("PUT", root+"r2", `{"id":"r2","provider":{"id":"m"},"promptId":"p2","failureReason":2,"success":false}`, 200)
	g = groups()
	require.Len(t, g, 1)
	require.Equal(t, groupID, g[0].ID)
	require.EqualValues(t, 2, g[0].ResultCount)
	var detail evals.ExperimentSourcePage
	require.NoError(t, json.Unmarshal(request("GET", "/api/evals/experiments/"+groupID+"/rows", "", 200), &detail))
	require.Len(t, detail.Rows, 2)
	require.Contains(t, detail.Rows[0].JSON, "9007199254740993")
	require.NotContains(t, detail.Rows[0].JSON, "synthetic-password")
	require.EqualValues(t, 2, detail.Rows[1].Ordinal, "failed publication leaves a gap, not a missing prerequisite")
	var oldest string
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select e.log_id::varchar from eval_evidence e join eval_datasets d on d.import_id=e.import_id join eval_live_members m on m.dataset_id=d.id where m.ordinal=0 and m.experiment_id=?`, groupID).Scan(&oldest)
	}))
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error {
		return ingest.DeleteLogs(t.Context(), db, `delete from logs where id=?::uuid`, oldest)
	}))
	g = groups()
	require.Len(t, g, 1)
	require.Equal(t, groupID, g[0].ID)
	require.EqualValues(t, 1, g[0].ResultCount)
	require.NoError(t, json.Unmarshal(request("GET", "/api/evals/experiments/"+groupID+"/rows", "", 200), &detail))
	require.Len(t, detail.Rows, 1)
	require.Contains(t, detail.Rows[0].JSON, `"id":"r2"`)
	request("PUT", root+"later", `{"id":"later","provider":{"id":"m"},"success":true}`, 200)
	require.EqualValues(t, 2, groups()[0].ResultCount)
	request("PUT", root+"r3", `{"id":"r3","provider":{"id":"other"},"success":true}`, 200)
	require.Len(t, groups(), 2)
}

func TestUnverifiedLiveResultNeverBecomesVisible(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil)
	require.NoError(t, err)
	startEvalReceiverWithSink(t, s, func(logs plog.Logs) {
		log := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
		source, _ := log.Body().Map().Get("source")
		if _, has := source.Map().Get("password"); has {
			source.Map().PutStr("password", "synthetic-unmasked")
		}
		if id, _ := source.Map().Get("id"); id.Str() == "wrong-ordinal" {
			log.Attributes().PutInt("eval.result.ordinal", 99)
		}
	})
	req := func(method, url, body string, code int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, httptest.NewRequest(method, url, strings.NewReader(body)))
		require.Equal(t, code, w.Code)
		require.NotContains(t, w.Body.String(), "synthetic-unmasked")
		return w.Body.Bytes()
	}
	req("PUT", "/api/evals/runs/check/results/ok", `{"id":"ok","provider":{"id":"m"},"success":true}`, 200)
	req("PUT", "/api/evals/runs/check/results/bad", `{"id":"bad","provider":{"id":"m"},"password":"synthetic-unmasked","success":true}`, 500)
	req("PUT", "/api/evals/runs/check/results/wrong-ordinal", `{"id":"wrong-ordinal","provider":{"id":"m"},"success":true}`, 500)
	var cached int
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select count(*) from eval_records`).Scan(&cached)
	}))
	require.Equal(t, 1, cached, "only the verified result is cached")
	var page evals.ExperimentSearchPage
	require.NoError(t, json.Unmarshal(req("GET", "/api/evals/experiments", "", 200), &page))
	require.Len(t, page.Experiments, 1)
	require.EqualValues(t, 1, page.Experiments[0].ResultCount)
	var detail evals.ExperimentSourcePage
	require.NoError(t, json.Unmarshal(req("GET", "/api/evals/experiments/"+page.Experiments[0].ID+"/rows", "", 200), &detail))
	require.Len(t, detail.Rows, 1)
	require.Contains(t, detail.Rows[0].JSON, `"id":"ok"`)
}
