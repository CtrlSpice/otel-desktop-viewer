package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/exp/jsonrpc2"
)

func enabledEvalFlags(t *testing.T) *features.Flags {
	t.Helper()
	file := filepath.Join(t.TempDir(), "flags.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"flags":{"evals.enabled":{"state":"ENABLED","variants":{"on":true},"defaultVariant":"on"}}}`), 0600))
	flags, err := features.New(t.Context(), file)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, flags.Shutdown(context.Background())) })
	return flags
}

type evalRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func evalRPCCall(t *testing.T, s *Server, method string, params any) evalRPCResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(string(body))))
	require.Equal(t, http.StatusOK, response.Code)
	var out evalRPCResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &out))
	return out
}

func TestEvalSnapshotRPCAndExactDownload(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	original := " \r\n{\"duplicate\":1,\"duplicate\":2,\"x\":9007199254740993,\"number\":-0.00e+02,\"unknown\":\"雪😀\\u0061\",\"ref\":\"https://example.invalid/never-fetch\"}\t\n"
	response := evalRPCCall(t, s, "importEvalSnapshot", map[string]any{"source": "local-file", "format": "promptfoo-v3", "name": "original.json", "originalJSON": original})
	require.Nil(t, response.Error)
	var imported evals.ImportResult
	require.NoError(t, json.Unmarshal(response.Result, &imported))
	require.True(t, imported.Created)
	response = evalRPCCall(t, s, "importEvalSnapshot", []any{"different-source", "viewer-kit", "other-name.json", original})
	require.Nil(t, response.Error)
	var repeated evals.ImportResult
	require.NoError(t, json.Unmarshal(response.Result, &repeated))
	require.False(t, repeated.Created)
	require.Equal(t, imported.Snapshot, repeated.Snapshot)
	for _, params := range []any{map[string]any{"id": imported.Snapshot.ID}, []any{imported.Snapshot.ID}} {
		response = evalRPCCall(t, s, "getEvalSnapshot", params)
		require.Nil(t, response.Error)
		var got evals.Snapshot
		require.NoError(t, json.Unmarshal(response.Result, &got))
		require.Equal(t, original, got.OriginalJSON)
		require.Equal(t, imported.Snapshot, got.Metadata)
		// Validate the actual wire result against the advertised schema.
		var wire any
		require.NoError(t, decodeParams(response.Result, &wire))
		schemas := openRPCDocument()["components"].(map[string]any)["schemas"].(map[string]any)
		require.NoError(t, checkRPCWireSchema(rpcSchemaRef("EvalSnapshot"), wire, schemas))
	}
	for _, params := range []any{nil, []any{}, map[string]any{}, map[string]any{"limit": 1}, []any{1, 0}} {
		response = evalRPCCall(t, s, "searchEvalSnapshots", params)
		require.Nil(t, response.Error)
		require.NotContains(t, string(response.Result), "originalJSON")
		var page evals.Page
		require.NoError(t, json.Unmarshal(response.Result, &page))
		require.Equal(t, []evals.Metadata{imported.Snapshot}, page.Snapshots)
		require.False(t, page.HasMore)
	}
	response = evalRPCCall(t, s, "searchEvalSnapshots", map[string]any{"offset": 1})
	require.Nil(t, response.Error)
	require.JSONEq(t, `{"snapshots":[],"hasMore":false}`, string(response.Result))
	path := "/api/evals/snapshots/" + imported.Snapshot.ID + "/download"
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest(method, path, nil))
		require.Equal(t, http.StatusOK, r.Code)
		require.Equal(t, "no-store", r.Header().Get("Cache-Control"))
		require.Equal(t, "application/json", r.Header().Get("Content-Type"))
		require.Contains(t, r.Header().Get("Content-Disposition"), "attachment;")
		require.Equal(t, fmt.Sprint(len(original)), r.Header().Get("Content-Length"))
		if method == http.MethodGet {
			require.Equal(t, original, r.Body.String())
		} else {
			require.Empty(t, r.Body.String())
		}
	}
	r := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Range", "bytes=2-17")
	s.server.Handler.ServeHTTP(r, req)
	require.Equal(t, http.StatusPartialContent, r.Code)
	require.Equal(t, original[2:18], r.Body.String())
	for _, suffix := range []string{"?format=json", "?attachment=https://example.invalid"} {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path+suffix, nil))
		require.Equal(t, http.StatusBadRequest, r.Code)
	}
	// Changed content under the same name is independent.
	response = evalRPCCall(t, s, "importEvalSnapshot", []any{"local-file", "promptfoo-v3", "original.json", original + " "})
	require.Nil(t, response.Error)
	var changed evals.ImportResult
	require.NoError(t, json.Unmarshal(response.Result, &changed))
	require.True(t, changed.Created)
	require.NotEqual(t, imported.Snapshot.ID, changed.Snapshot.ID)
	response = evalRPCCall(t, s, "deleteEvalSnapshot", map[string]any{"id": imported.Snapshot.ID})
	require.Nil(t, response.Error)
	require.JSONEq(t, `{"id":"`+imported.Snapshot.ID+`","deleted":true}`, string(response.Result))
	for _, method := range []string{"getEvalSnapshot", "deleteEvalSnapshot"} {
		response = evalRPCCall(t, s, method, []any{imported.Snapshot.ID})
		require.NotNil(t, response.Error)
		require.Equal(t, ErrCodeEvalSnapshotNotFound, response.Error.Code)
	}
	r = httptest.NewRecorder()
	s.server.Handler.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusNotFound, r.Code)
	response = evalRPCCall(t, s, "getEvalSnapshot", []any{changed.Snapshot.ID})
	require.Nil(t, response.Error)
}

func TestEvalSnapshotFeatureGateBeforeStoreAccess(t *testing.T) {
	// No store: disabled methods must return before validation or database work.
	s, err := NewServer("localhost:0", 0, nil, zap.NewNop(), nil)
	require.NoError(t, err)
	for _, method := range []string{"importEvalSnapshot", "searchEvalSnapshots", "getEvalSnapshot", "deleteEvalSnapshot"} {
		for _, params := range []any{nil, map[string]any{"unknown": true}} {
			response := evalRPCCall(t, s, method, params)
			require.NotNil(t, response.Error)
			require.Equal(t, -32601, response.Error.Code)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest(method, "/api/evals/snapshots/bad/download", nil))
		require.Equal(t, http.StatusNotFound, r.Code)
	}
}

func TestEvalSnapshotInvalidParamsAndCancellation(t *testing.T) {
	h := setupHandler(t)
	h.features = enabledEvalFlags(t)
	for _, tc := range []struct {
		method string
		params any
	}{
		{"importEvalSnapshot", []any{"source", "unknown-json", "x"}},
		{"importEvalSnapshot", []any{"source", "unknown-json", "x", map[string]any{"not": "a string"}}},
		{"importEvalSnapshot", []any{"source", "unknown-json", "x", "{} {}"}},
		{"importEvalSnapshot", []any{"source", "auto", "x", "{}"}},
		{"importEvalSnapshot", []any{"source", "unknown-json", "x", `"\ud800"`}},
		{"getEvalSnapshot", []any{"bad"}}, {"deleteEvalSnapshot", []any{strings.Repeat("A", 64)}},
		{"getEvalSnapshot", map[string]any{"id": strings.Repeat("a", 64), "extra": true}},
		{"searchEvalSnapshots", []any{0}}, {"searchEvalSnapshots", []any{101}},
		{"searchEvalSnapshots", []any{1, -1}}, {"searchEvalSnapshots", []any{1, evals.MaxOffset + 1}},
		{"searchEvalSnapshots", []any{1.5}}, {"searchEvalSnapshots", []any{1, 0, 0}},
	} {
		_, err := h.Handle(t.Context(), createRequest(tc.method, tc.params))
		require.ErrorIs(t, err, jsonrpc2.ErrInvalidParams, "%s %v", tc.method, tc.params)
	}
	for _, method := range []string{"getEvalSnapshot", "deleteEvalSnapshot"} {
		_, err := h.Handle(t.Context(), createRequest(method, []any{strings.Repeat("a", 64)}))
		require.ErrorIs(t, err, ErrEvalSnapshotNotFound)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tc := range []struct {
		method string
		params any
	}{
		{"importEvalSnapshot", []any{"source", "unknown-json", "x", "{}"}},
		{"searchEvalSnapshots", []any{}}, {"getEvalSnapshot", []any{strings.Repeat("a", 64)}},
		{"deleteEvalSnapshot", []any{strings.Repeat("a", 64)}},
	} {
		_, err := h.Handle(ctx, createRequest(tc.method, tc.params))
		require.ErrorIs(t, err, ErrRequestCanceled)
	}
	// A cancellation while waiting for the store write lock cannot insert later.
	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { done <- h.store.WithDBWrite(func(db *sql.DB) error { close(locked); <-release; return nil }) }()
	<-locked
	ctx, cancel = context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := h.Handle(ctx, createRequest("importEvalSnapshot", []any{"source", "unknown-json", "x", "{}"}))
		result <- err
	}()
	cancel()
	close(release)
	require.NoError(t, <-done)
	require.ErrorIs(t, <-result, ErrRequestCanceled)
	page, err := h.Handle(t.Context(), createRequest("searchEvalSnapshots", []any{}))
	require.NoError(t, err)
	require.Empty(t, page.(evals.Page).Snapshots)
	require.NoError(t, h.store.Close())
	_, err = h.Handle(t.Context(), createRequest("searchEvalSnapshots", []any{}))
	require.ErrorIs(t, err, jsonrpc2.ErrInternal)
}

func TestEvalSnapshotRPCBodyBoundsAndUTF8(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil, WithFeatures(enabledEvalFlags(t)))
	require.NoError(t, err)
	// An archive exceeding the old RPC envelope limit is supported when enabled.
	original := `"` + strings.Repeat("x", maxRPCRequestBodyBytes) + `"`
	response := evalRPCCall(t, s, "importEvalSnapshot", []any{"file", "unknown-json", "large.json", original})
	require.Nil(t, response.Error)
	for _, tc := range []struct {
		body string
		code int
	}{
		{strings.Repeat(" ", maxEvalsRPCRequestBodyBytes+1), -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"importEvalSnapshot","params":["file","unknown-json","bad.json","` + "\xff" + `"]}`, -32700},
	} {
		r := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(tc.body)))
		var got evalRPCResponse
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &got))
		require.NotNil(t, got.Error)
		require.Equal(t, tc.code, got.Error.Code)
	}
}
