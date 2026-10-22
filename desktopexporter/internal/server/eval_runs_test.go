package server

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEvalRunSummarySearchAndScopedDetail(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	input := `{"evalId":"run","config":{"description":"Run 2"},"results":{"version":3,"timestamp":"2026-10-21T10:00:00Z","results":[
{"provider":{"id":"a"},"promptId":"p","success":true,"future":9007199254740993},
{"provider":{"id":"b"},"promptId":"p","success":true,"answer":"OTHER RUN"},
{"provider":{"id":"a"},"promptId":"p","gradingResult":{"pass":false,"reason":"Missing idempotency key"}},
{"provider":{"id":"a"},"promptId":"q","failureReason":2},
{"provider":{"id":"a"},"promptId":"p","success":true},
{"id":"missing-provider-and-prompt"}
]}}`
	r := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
	require.Equal(t, 200, r.Code, r.Body.String())
	search := func(path string) evals.RunSearchPage {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 200, r.Code, r.Body.String())
		var page evals.RunSearchPage
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &page))
		return page
	}
	page := search("/api/evals/runs?sort=name&limit=1")
	require.Equal(t, int64(4), page.Total)
	require.True(t, page.HasMore)
	require.Len(t, page.Runs, 1)
	require.Zero(t, page.Indexing.Remaining)
	filtered := search("/api/evals/runs?outcome=error%3AFail&q=run%202")
	require.Equal(t, int64(1), filtered.Total)
	require.Len(t, filtered.Runs, 1)
	run := filtered.Runs[0]
	require.Equal(t, int64(3), run.RepCount)
	require.Nil(t, run.PlannedCount)
	require.Equal(t, "Unknown", run.State)
	require.Equal(t, []evals.OutcomeCount{{Key: "success:Pass", Label: "Pass", Tone: "success", Count: 2}, {Key: "error:Fail", Label: "Fail", Tone: "error", Count: 1}}, run.Outcomes)
	// Facets count whole runs, before the outcome filter, not rows in this page.
	require.Equal(t, int64(2), filtered.Facets[0].Count)
	var ordinals []int64
	after := int64(-1)
	for {
		var detail evals.RunSourcePage
		require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
			var err error
			detail, err = evals.GetRunPage(t.Context(), db, run.ID, after, 1)
			return err
		}))
		require.Equal(t, run, detail.Summary)
		require.NotContains(t, detail.Rows[0].JSON, "OTHER RUN")
		ordinals = append(ordinals, detail.Rows[0].Ordinal)
		if after == -1 {
			require.Contains(t, detail.Rows[0].JSON, "9007199254740993")
		}
		if !detail.HasMore {
			break
		}
		after = detail.NextAfter
	}
	require.Equal(t, []int64{0, 2, 4}, ordinals)
	r = httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("GET", "/api/evals/runs/"+run.ID+"/rows?limit=1", nil))
	require.Equal(t, 200, r.Code)
	require.Contains(t, r.Body.String(), "9007199254740993")
	// Selection remains retrievable when it is absent from a different list filter.
	require.Zero(t, search("/api/evals/runs?q=not-present").Total)
	require.Equal(t, int64(1), search("/api/evals/runs?q=idempotency+key").Total)
	r = httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("GET", "/api/evals/runs/"+run.ID+"/rows", nil))
	require.Equal(t, 200, r.Code)
}
