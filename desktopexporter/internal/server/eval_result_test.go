//go:build evals

package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

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
	request("PUT", root+"r3", `{"id":"r3","provider":{"id":"other"},"success":true}`, 200)
	require.Len(t, groups(), 2)
}
