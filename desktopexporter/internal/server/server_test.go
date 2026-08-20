package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func setupServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	str, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	s, err := NewServer("localhost:8000", str, zap.NewNop(), telemetry.Disabled())
	require.NoError(t, err)
	testServer := httptest.NewServer(s.server.Handler)

	return testServer, func() {
		testServer.Close()
		_ = s.Shutdown(context.Background())
		str.Close()
	}
}

func TestIndexHandler(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	res, err := http.Get(fmt.Sprintf("%s/", testServer.URL))
	assert.Nilf(t, err, "could not send GET request: %v", err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
}

// Client-side routes have no matching file on disk; the server must fall back to
// index.html so a hard load, refresh, or shared deep link boots the SPA (which then
// owns the route). Without this, a refreshed /traces or a shared /traces/{id} 404s.
func TestSPAFallback(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	for _, path := range []string{"/traces", "/traces/abc123def456", "/metrics", "/logs"} {
		res, err := http.Get(testServer.URL + path)
		require.NoErrorf(t, err, "GET %s", path)
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		require.NoErrorf(t, err, "read body for %s", path)

		assert.Equalf(t, http.StatusOK, res.StatusCode, "GET %s status", path)
		assert.Containsf(t, res.Header.Get("Content-Type"), "text/html", "GET %s content-type", path)
		assert.Containsf(t, strings.ToLower(string(body)), "<!doctype html",
			"GET %s should serve index.html", path)
	}
}

func TestMissingAsset404(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	res, err := http.Get(testServer.URL + "/assets/stale-hash.js")
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestRPCHandlerInvalidJSON(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	// Send invalid JSON
	invalidJSON := `{"jsonrpc": "2.0", "method": "test", "id": 1, "params": [}`
	res, err := http.Post(fmt.Sprintf("%s/rpc", testServer.URL), "application/json", strings.NewReader(invalidJSON))
	assert.Nilf(t, err, "could not send POST request: %v", err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode) // JSON-RPC always returns 200

	body, err := io.ReadAll(res.Body)
	assert.Nilf(t, err, "could not read response body: %v", err)

	var response map[string]any
	err = json.Unmarshal(body, &response)
	assert.Nilf(t, err, "could not unmarshal response: %v", err)

	// Should be a JSON-RPC parse error
	assert.Equal(t, "2.0", response["jsonrpc"])
	assert.NotNil(t, response["error"])
	errorObj := response["error"].(map[string]any)
	assert.Equal(t, float64(-32700), errorObj["code"]) // Parse error code
}

func TestRPCHandlerOversizedBody(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	oversized := make([]byte, maxRPCRequestBodyBytes+1)
	res, err := http.Post(fmt.Sprintf("%s/rpc", testServer.URL), "application/json", bytes.NewReader(oversized))
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode) // JSON-RPC always returns 200

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	var response map[string]any
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "2.0", response["jsonrpc"])
	assert.NotNil(t, response["error"])
	errorObj := response["error"].(map[string]any)
	assert.Equal(t, float64(-32600), errorObj["code"]) // Invalid Request code
}

func TestRPCHandlerInvalidRequest(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	// Send valid JSON but invalid JSON-RPC request
	invalidRequest := `{"jsonrpc": "2.0", "method": "invalidMethod", "id": 1}`
	res, err := http.Post(fmt.Sprintf("%s/rpc", testServer.URL), "application/json", strings.NewReader(invalidRequest))
	assert.Nilf(t, err, "could not send POST request: %v", err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)

	body, err := io.ReadAll(res.Body)
	assert.Nilf(t, err, "could not read response body: %v", err)

	var response map[string]any
	err = json.Unmarshal(body, &response)
	assert.Nilf(t, err, "could not unmarshal response: %v", err)

	assert.Equal(t, "2.0", response["jsonrpc"])
	assert.NotNil(t, response["error"])
	errorObj := response["error"].(map[string]any)
	assert.Equal(t, float64(-32601), errorObj["code"]) // Method not found
}

func TestCORSHeaders(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	// Test preflight request
	req, err := http.NewRequest("OPTIONS", fmt.Sprintf("%s/rpc", testServer.URL), nil)
	assert.Nilf(t, err, "could not create OPTIONS request: %v", err)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	client := &http.Client{}
	res, err := client.Do(req)
	assert.Nilf(t, err, "could not send OPTIONS request: %v", err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusNoContent, res.StatusCode) // CORS preflight returns 204
	assert.Equal(t, "http://localhost:5173", res.Header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, res.Header.Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, res.Header.Get("Access-Control-Allow-Headers"), "Content-Type")
}

func TestStartBindConflict(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	defer ln.Close()

	str, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	defer str.Close()

	s, err := NewServer(addr, str, zap.NewNop(), telemetry.Disabled())
	require.NoError(t, err)

	err = s.Start()
	require.Error(t, err)
}

// TestStaticCacheHeaders covers the two populations the embedded frontend
// splits into, and the one mistake that would be expensive.
//
// Nothing was sent before this: assets are served from an embed.FS, whose files
// report a zero mod time, and Go omits Last-Modified when the time is zero and
// never synthesises an ETag. No freshness directive, no validator, so the
// browser re-fetched the whole 2MB frontend on every page load.
func TestStaticCacheHeaders(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":              &fstest.MapFile{Data: []byte("<!doctype html><div id=app>")},
		"assets/index-abc123.js":  &fstest.MapFile{Data: []byte("console.log(1)")},
		"assets/style-def456.css": &fstest.MapFile{Data: []byte("body{}")},
	}
	handler := spaHandler(fsys, zap.NewNop())

	get := func(target string) *http.Response {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec.Result()
	}

	t.Run("hashed assets are immutable", func(t *testing.T) {
		for _, target := range []string{"/assets/index-abc123.js", "/assets/style-def456.css"} {
			resp := get(target)
			assert.Equal(t, immutableCacheControl, resp.Header.Get("Cache-Control"),
				"%s is content-hashed, so its URL changes when its bytes do", target)
		}
	})

	// The dangerous one. index.html has a stable name and its contents name the
	// hashed files, so caching it hands an upgraded binary's user stale HTML
	// pointing at chunks that no longer exist.
	t.Run("index.html is never immutable", func(t *testing.T) {
		for _, target := range []string{"/", "/traces", "/index.html"} {
			resp := get(target)
			cc := resp.Header.Get("Cache-Control")
			assert.Equal(t, revalidateCacheControl, cc, "%s must revalidate", target)
			assert.NotContains(t, cc, "immutable",
				"%s serves the document that names every hashed asset", target)
		}
	})

	// Relied on rather than implemented: since Go 1.23, serveError deletes
	// Cache-Control, Etag, Last-Modified and Content-Encoding on the error
	// path, because a caller may have set them for the success case. Without
	// that, a request for an asset that does not exist would be answered with
	// a year-long immutable 404 -- and a later build that adds the file could
	// not dislodge it. Asserted because the whole prefix rule leans on it.
	t.Run("a missing asset is not cached", func(t *testing.T) {
		resp := get("/assets/never-existed-000000.js")
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Empty(t, resp.Header.Get("Cache-Control"),
			"a 404 must not inherit the immutable header meant for real assets")
	})

	t.Run("index carries an ETag and honours If-None-Match", func(t *testing.T) {
		resp := get("/")
		etag := resp.Header.Get("ETag")
		require.NotEmpty(t, etag, "no validator means revalidation re-sends the whole document")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("If-None-Match", etag)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotModified, rec.Code,
			"a matching validator should cost a 304, not another copy")
		assert.Empty(t, rec.Body.String())
	})
}
