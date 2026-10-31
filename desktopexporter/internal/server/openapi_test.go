package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/features"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestOpenAPIDiscoveryAndAgentWorkflow(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:0", 0, h.store, zap.NewNop(), nil)
	require.NoError(t, err)
	request := func(method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, r)
		return w
	}
	w := request("GET", "/openapi.json", nil, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	require.Empty(t, w.Header().Get("Location"))
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)
	var doc struct {
		OpenAPI string `json:"openapi"`
		Paths   map[string]struct {
			Post struct {
				RequestBody struct {
					Content map[string]struct {
						Example json.RawMessage `json:"example"`
					} `json:"content"`
				} `json:"requestBody"`
				Responses map[string]struct {
					Content map[string]json.RawMessage `json:"content"`
				} `json:"responses"`
			} `json:"post"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	require.Equal(t, "3.1.0", doc.OpenAPI)
	for _, path := range []string{"/openapi.json", "/openrpc.json", "/llms.txt", "/rpc", "/api/features", "/api/data/{method}", "/export/{signal}/{id}"} {
		require.Contains(t, doc.Paths, path)
	}
	for _, path := range []string{"/api/evals/ingest", "/api/evals/experiments", "/api/evals/experiments/{id}/rows"} {
		_, exists := doc.Paths[path]
		require.Equal(t, features.EvalsBuilt, exists, path)
	}
	require.Contains(t, doc.Paths["/api/data/{method}"].Post.Responses["200"].Content, arrowStreamContentType)
	require.Contains(t, w.Body.String(), "browser-oriented")
	for _, path := range []string{"/openapi.json", "/openrpc.json"} {
		head := request("HEAD", path, nil, nil)
		require.Equal(t, http.StatusOK, head.Code)
		require.Empty(t, head.Body.Bytes())
	}
	conditional := request("GET", "/openapi.json", nil, map[string]string{"If-None-Match": etag})
	require.Equal(t, http.StatusNotModified, conditional.Code)
	require.Empty(t, conditional.Body.Bytes())
	index := request("GET", "/", nil, nil)
	for _, path := range []string{"/openapi.json", "/openrpc.json", "/llms.txt"} {
		require.Contains(t, index.Header().Get("Link"), path)
		require.Contains(t, index.Body.String(), `href="`+path+`"`)
	}
	// Execute the documented agent entry point, not an Arrow/browser request.
	example := doc.Paths["/rpc"].Post.RequestBody.Content["application/json"].Example
	require.NotEmpty(t, example)
	reply := request("POST", "/rpc", example, map[string]string{"Content-Type": "application/json"})
	require.Equal(t, http.StatusOK, reply.Code)
	require.Equal(t, "application/json", reply.Header().Get("Content-Type"))
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	require.NoError(t, json.Unmarshal(reply.Body.Bytes(), &envelope))
	require.Empty(t, envelope.Error)
	require.JSONEq(t, `[]`, string(envelope.Result))
}
