//go:build evals

package server

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	querysearch "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEvalExperimentSummarySearchAndScopedDetail(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	startEvalReceiver(t, s)
	input := `{"evalId":"experiment","config":{"description":"Experiment 2"},"results":{"version":3,"timestamp":"2026-10-21T10:00:00Z","results":[
{"provider":{"id":"a"},"promptId":"p","success":true,"future":9007199254740993,"id":"firstonly","score":0.95,"response":{"output":"exact phrase"}},
{"provider":{"id":"b"},"promptId":"p","success":true,"answer":"OTHER RUN"},
{"provider":{"id":"a"},"promptId":"p","gradingResult":{"pass":false,"reason":"Missing idempotency key"},"id":"secondonly","score":0.25,"success":false},
{"provider":{"id":"a"},"promptId":"q","failureReason":2},
{"provider":{"id":"a"},"promptId":"p","success":true},
{"id":"missing-provider-and-prompt"}
]}}`
	r := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
	require.Equal(t, 200, r.Code, r.Body.String())
	search := func(path string) evals.ExperimentSearchPage {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 200, r.Code, r.Body.String())
		var page evals.ExperimentSearchPage
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &page))
		return page
	}
	condition := func(field, op, value string) querysearch.QueryNode {
		scope := "field"
		if field == "" {
			scope = "global"
		}
		return querysearch.QueryNode{Type: "condition", Query: &querysearch.Query{Field: &querysearch.FieldDefinition{Name: field, SearchScope: scope}, FieldOperator: op, Value: value}}
	}
	group := func(op string, children ...querysearch.QueryNode) querysearch.QueryNode {
		return querysearch.QueryNode{Type: "group", Group: &querysearch.QueryGroup{LogicalOperator: op, Children: children}}
	}
	criteriaURL := func(node querysearch.QueryNode) string {
		raw, err := json.Marshal(node)
		require.NoError(t, err)
		return "/api/evals/experiments?criteria=" + url.QueryEscape(string(raw))
	}
	metadataURL := criteriaURL(condition("", "CONTAINS", "Experiment 2"))
	page := search("/api/evals/experiments?sort=name&limit=1")
	require.Equal(t, int64(4), page.Total)
	require.True(t, page.HasMore)
	require.Len(t, page.Experiments, 1)
	require.Zero(t, page.Indexing.Remaining)
	filtered := search(metadataURL + "&outcome=error%3AFail")
	require.Equal(t, int64(1), filtered.Total)
	require.Len(t, filtered.Experiments, 1)
	experiment := filtered.Experiments[0]
	require.Equal(t, int64(3), experiment.ResultCount)
	require.Nil(t, experiment.PlannedCount)
	require.Equal(t, "Unknown", experiment.State)
	require.Equal(t, []evals.OutcomeCount{{Key: "success:Pass", Label: "Pass", Tone: "success", Count: 2}, {Key: "error:Fail", Label: "Fail", Tone: "error", Count: 1}}, experiment.Outcomes)
	// Facets count whole experiments, before the outcome filter, not rows in this page.
	require.Equal(t, int64(2), filtered.Facets[0].Count)
	// Counts and facets survive an empty page, whether caused by the cursor or
	// by an outcome filter. No placeholder experiment escapes the left join.
	beyond := search(metadataURL + "&outcome=error%3AFail&offset=100")
	require.Equal(t, filtered.Total, beyond.Total)
	require.Equal(t, filtered.Facets, beyond.Facets)
	require.Empty(t, beyond.Experiments)
	require.False(t, beyond.HasMore)
	noOutcome := search(metadataURL + "&outcome=warning%3APartial")
	require.Zero(t, noOutcome.Total)
	require.Equal(t, filtered.Facets, noOutcome.Facets)
	require.Empty(t, noOutcome.Experiments)
	var ordinals []int64
	after := int64(-1)
	for {
		var detail evals.ExperimentSourcePage
		require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
			var err error
			detail, err = evals.GetExperimentPage(t.Context(), db, experiment.ID, after, 1)
			return err
		}))
		require.Equal(t, experiment, detail.Summary)
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
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("GET", "/api/evals/experiments/"+experiment.ID+"/rows?limit=1", nil))
	require.Equal(t, 200, r.Code)
	require.Contains(t, r.Body.String(), "9007199254740993")
	// Selection remains retrievable when it is absent from a different list filter.
	require.Zero(t, search(criteriaURL(condition("", "CONTAINS", "not-present"))).Total)
	require.Equal(t, int64(1), search(criteriaURL(condition("", "CONTAINS", "idempotency key"))).Total)
	for _, tc := range []struct {
		name string
		node querysearch.QueryNode
		want int64
	}{
		{"different results cannot satisfy AND", group("AND", condition("", "CONTAINS", "firstonly"), condition("", "CONTAINS", "secondonly")), 0},
		{"same result can satisfy AND", group("AND", condition("", "CONTAINS", "secondonly"), condition("", "CONTAINS", "idempotency")), 1},
		{"parent metadata and result", group("AND", condition("evaluation", "=", "Experiment 2"), condition("result.id", "=", "secondonly")), 1},
		{"nested Boolean groups", group("AND", group("OR", condition("result.id", "=", "firstonly"), condition("result.id", "=", "secondonly")), condition("result.score", "<", "0.5")), 1},
		{"typed values on different results", group("AND", condition("result.id", "=", "firstonly"), condition("result.score", "<", "0.5")), 0},
		{"numeric list", condition("result.score", "IN", `["0.25", "0.5"]`), 1},
		{"boolean", condition("result.success", "=", "false"), 1},
		{"boolean list", condition("result.success", "IN", `["false"]`), 1},
		{"ordinal numeric comparison", condition("result.ordinal", ">", "4"), 1},
		{"missing result field", condition("result.id", "IS NULL", ""), 3},
		{"phrase", condition("result.output", "CONTAINS", "exact phrase"), 1},
		{"case follows shared search", condition("result.output", "CONTAINS", "EXACT PHRASE"), 0},
		{"regex", condition("result.provider", "REGEXP", "[ab]"), 3},
		{"set", condition("result.prompt", "IN", `["p"]`), 2},
		{"negative predicate stays on row", group("AND", condition("result.id", "=", "firstonly"), condition("result.text", "NOT CONTAINS", "secondonly")), 1},
		{"SQL-looking value is literal", condition("name", "=", "' OR true --"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := search(criteriaURL(tc.node))
			require.Equal(t, tc.want, got.Total)
			if tc.name == "nested Boolean groups" {
				require.Equal(t, experiment, got.Experiments[0])
			}
		})
	}
	for _, invalid := range []string{
		"/api/evals/experiments?q=old-matcher",
		"/api/evals/experiments?criteria=%7B",
		"/api/evals/experiments?criteria=null",
		criteriaURL(condition("unknown", "=", "x")),
		criteriaURL(condition("result.ordinal", "=", "9223372036854775808")),
		criteriaURL(condition("result.ordinal", ">", "1.5")),
		criteriaURL(condition("result.score", "CONTAINS", "1")),
		criteriaURL(condition("result.success", "=", "yes")),
		criteriaURL(group("XOR", condition("name", "=", "x"))),
	} {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest("GET", invalid, nil))
		require.Equal(t, 400, r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest("GET", "/api/evals/experiments/"+experiment.ID+"/rows", nil))
	require.Equal(t, 200, r.Code)
}
