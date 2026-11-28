package server

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/CtrlSpice/otel-desktop-viewer/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func setupServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	testServer, _, teardown := setupServerWithStore(t)
	return testServer, teardown
}

func setupServerWithStore(t *testing.T) (*httptest.Server, *store.Store, func()) {
	t.Helper()
	str, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	s, err := NewServer("localhost:8000", 0, str, zap.NewNop(), telemetry.Disabled())
	require.NoError(t, err)
	testServer := httptest.NewServer(s.server.Handler)

	return testServer, str, func() {
		testServer.Close()
		_ = s.Shutdown(context.Background())
		str.Close()
	}
}

func TestSearchTraceSummariesPublicRPCReturnsWholeTraceAndMatchedSpans(t *testing.T) {
	testServer, str, teardown := setupServerWithStore(t)
	defer teardown()

	traceID := [16]byte{15: 1}
	data := ptrace.NewTraces()
	addSpan := func(service string, spanID, parentID byte, start, end uint64, status ptrace.StatusCode) {
		rs := data.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", service)
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID([8]byte{7: spanID})
		if parentID != 0 {
			span.SetParentSpanID([8]byte{7: parentID})
		}
		span.SetName(service)
		span.SetStartTimestamp(pcommon.Timestamp(start))
		span.SetEndTimestamp(pcommon.Timestamp(end))
		span.Status().SetCode(status)
	}
	addSpan("service-a", 1, 0, 100, 500, ptrace.StatusCodeOk)
	addSpan("service-b", 2, 1, 200, 300, ptrace.StatusCodeOk)
	addSpan("service-a", 3, 1, 50, 80, ptrace.StatusCodeError)
	require.NoError(t, str.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(context.Background(), conn, data)
	}))

	requestBody := `{"jsonrpc":"2.0","id":1,"method":"searchTraceSummaries","params":{` +
		`"startTime":"150","endTime":"250","query":{"id":"service-b","type":"condition","query":{` +
		`"field":{"name":"serviceName","searchScope":"field","type":"string"},` +
		`"fieldOperator":"=","value":"service-b"}},"limit":2}}`
	response, err := http.Post(testServer.URL+"/rpc", "application/json", strings.NewReader(requestBody))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)

	var rpcResponse struct {
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&rpcResponse))
	var summaries []struct {
		TraceID      string `json:"traceID"`
		HasRootSpan  bool   `json:"hasRootSpan"`
		StartTime    string `json:"startTime"`
		DurationNs   string `json:"durationNs"`
		SpanCount    int    `json:"spanCount"`
		ErrorCount   int    `json:"errorCount"`
		MatchedSpans []struct {
			TraceID string `json:"traceID"`
			SpanID  string `json:"spanID"`
		} `json:"matchedSpans"`
	}
	require.NoError(t, json.Unmarshal(rpcResponse.Result, &summaries))
	require.Len(t, summaries, 1)
	assert.Equal(t, "00000000000000000000000000000001", summaries[0].TraceID)
	assert.True(t, summaries[0].HasRootSpan)
	assert.Equal(t, "50", summaries[0].StartTime)
	assert.Equal(t, "450", summaries[0].DurationNs)
	assert.Equal(t, 3, summaries[0].SpanCount)
	assert.Equal(t, 1, summaries[0].ErrorCount)
	require.Len(t, summaries[0].MatchedSpans, 1)
	assert.Equal(t, "00000000000000000000000000000001", summaries[0].MatchedSpans[0].TraceID)
	assert.Equal(t, "0000000000000002", summaries[0].MatchedSpans[0].SpanID)
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

func TestLLMsTextHandler(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	response, err := http.Get(testServer.URL + "/llms.txt")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "text/plain; charset=utf-8", response.Header.Get("Content-Type"))
	assert.Equal(t, revalidateCacheControl, response.Header.Get("Cache-Control"))
	assert.Equal(t, skills.Guide(), string(body))
	etag := response.Header.Get("ETag")
	require.NotEmpty(t, etag)

	headRequest, err := http.NewRequest(http.MethodHead, testServer.URL+"/llms.txt", nil)
	require.NoError(t, err)
	headResponse, err := http.DefaultClient.Do(headRequest)
	require.NoError(t, err)
	headBody, err := io.ReadAll(headResponse.Body)
	headResponse.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, headResponse.StatusCode)
	assert.Empty(t, headBody)
	assert.Equal(t, int64(len(skills.Guide())), headResponse.ContentLength)

	conditionalRequest, err := http.NewRequest(http.MethodGet, testServer.URL+"/llms.txt", nil)
	require.NoError(t, err)
	conditionalRequest.Header.Set("If-None-Match", etag)
	conditionalResponse, err := http.DefaultClient.Do(conditionalRequest)
	require.NoError(t, err)
	conditionalBody, err := io.ReadAll(conditionalResponse.Body)
	conditionalResponse.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotModified, conditionalResponse.StatusCode)
	assert.Empty(t, conditionalBody)
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

// The icon index.html links to must be embedded, or the browser tab has none.
func TestFaviconServed(t *testing.T) {
	testServer, teardown := setupServer(t)
	defer teardown()

	res, err := http.Get(testServer.URL + "/")
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	require.NoError(t, err)

	match := regexp.MustCompile(`<link rel="icon" type="image/svg\+xml" href="([^"]+)"`).FindSubmatch(body)
	require.NotNil(t, match, "index.html should declare an SVG icon")

	iconRes, err := http.Get(testServer.URL + string(match[1]))
	require.NoError(t, err)
	defer iconRes.Body.Close()

	assert.Equal(t, http.StatusOK, iconRes.StatusCode)
	assert.Equal(t, "image/svg+xml", iconRes.Header.Get("Content-Type"))
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

	client := &http.Client{}
	for _, allowed := range []string{
		"http://localhost",
		"http://localhost:5173",
		"http://127.0.0.1:3001",
		"http://[::1]:3001",
		"http://[::1]",
	} {
		t.Run("allows loopback "+allowed, func(t *testing.T) {
			req, err := http.NewRequest("OPTIONS", fmt.Sprintf("%s/rpc", testServer.URL), nil)
			require.NoError(t, err)
			req.Header.Set("Origin", allowed)
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "Content-Type")

			res, err := client.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, allowed, res.Header.Get("Access-Control-Allow-Origin"))
			assert.Contains(t, res.Header.Get("Access-Control-Allow-Methods"), "POST")
			assert.Contains(t, res.Header.Get("Access-Control-Allow-Headers"), "Content-Type")

			postReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/rpc", testServer.URL), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"listDatabases","params":[]}`))
			require.NoError(t, err)
			postReq.Header.Set("Content-Type", "application/json")
			postReq.Header.Set("Origin", allowed)
			postRes, err := client.Do(postReq)
			require.NoError(t, err)
			defer postRes.Body.Close()
			assert.Equal(t, http.StatusOK, postRes.StatusCode)
			assert.Equal(t, allowed, postRes.Header.Get("Access-Control-Allow-Origin"))
		})
	}

	for _, blocked := range []string{
		"https://evil.example",
		"http://evil.example",
		"http://localhost.evil.example",
		"http://127.0.0.1.evil.example",
		"https://localhost:5173",
		"null",
	} {
		t.Run("blocks non-loopback "+blocked, func(t *testing.T) {
			req, err := http.NewRequest("OPTIONS", fmt.Sprintf("%s/rpc", testServer.URL), nil)
			require.NoError(t, err)
			req.Header.Set("Origin", blocked)
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "Content-Type")

			res, err := client.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			assert.Empty(t, res.Header.Get("Access-Control-Allow-Origin"))

			// Even a simple text/plain POST without preflight must be blocked on /rpc.
			postReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/rpc", testServer.URL), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"newDatabase","params":["pwned"]}`))
			require.NoError(t, err)
			postReq.Header.Set("Content-Type", "text/plain")
			postReq.Header.Set("Origin", blocked)
			postRes, err := client.Do(postReq)
			require.NoError(t, err)
			defer postRes.Body.Close()
			assert.Equal(t, http.StatusForbidden, postRes.StatusCode)
		})
	}

	t.Run("blocks Sec-Fetch-Site cross-site without Origin", func(t *testing.T) {
		postReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/rpc", testServer.URL), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"listDatabases","params":[]}`))
		require.NoError(t, err)
		postReq.Header.Set("Content-Type", "text/plain")
		postReq.Header.Set("Sec-Fetch-Site", "cross-site")
		postRes, err := client.Do(postReq)
		require.NoError(t, err)
		defer postRes.Body.Close()
		assert.Equal(t, http.StatusForbidden, postRes.StatusCode)
	})
}

func TestStartBindConflict(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	defer ln.Close()

	str, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	defer str.Close()

	s, err := NewServer(addr, 0, str, zap.NewNop(), telemetry.Disabled())
	require.NoError(t, err)

	err = s.Start()
	require.Error(t, err)
}

// TestStaticCacheHeaders verifies immutable caching for hashed assets and
// revalidation for index.html.
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

	// index.html must not remain stale across an application upgrade.
	t.Run("index.html is never immutable", func(t *testing.T) {
		for _, target := range []string{"/", "/traces", "/index.html"} {
			resp := get(target)
			cc := resp.Header.Get("Cache-Control")
			assert.Equal(t, revalidateCacheControl, cc, "%s must revalidate", target)
			assert.NotContains(t, cc, "immutable",
				"%s serves the document that names every hashed asset", target)
		}
	})

	// Go's file server must remove success cache headers from 404 responses.
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

func TestChangesSnapshotsReconnectAndShutdown(t *testing.T) {
	db, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer db.Close()
	s, err := NewServer("localhost:0", 0, db, zap.NewNop(), nil)
	require.NoError(t, err)
	httpServer := httptest.NewServer(s.server.Handler)
	defer httpServer.Close()
	defer s.Shutdown(context.Background())
	client := &http.Client{Timeout: 5 * time.Second}
	connect := func() (*http.Response, *bufio.Scanner) {
		t.Helper()
		response, err := client.Get(httpServer.URL + "/api/changes")
		require.NoError(t, err)
		require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
		return response, bufio.NewScanner(response.Body)
	}
	read := func(scanner *bufio.Scanner) string {
		t.Helper()
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				return scanner.Text()
			}
		}
		t.Fatalf("stream ended: %v", scanner.Err())
		return ""
	}
	response, scanner := connect()
	first := read(scanner)
	require.NoError(t, db.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec("create table notification_test (id integer)")
		return err
	}))
	second := read(scanner)
	require.NotEqual(t, first, second)
	require.NoError(t, response.Body.Close())
	response, scanner = connect()
	defer response.Body.Close()
	require.Equal(t, second, read(scanner), "reconnect must send a snapshot even without another write")
	require.NoError(t, s.Shutdown(t.Context()))
	for scanner.Scan() {
	}
	require.NoError(t, scanner.Err(), "shutdown must close the SSE handler")
}

func TestRPCTraceContextAndFailureStatus(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		failure    bool
	}{
		{"success", `{"jsonrpc":"2.0","id":1,"method":"query","params":{"sql":"SELECT 1"}}`, false},
		{"query error", `{"jsonrpc":"2.0","id":1,"method":"query","params":{"sql":"SELECT * FROM missing_table"}}`, true},
		{"decode error", `{`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			defer provider.Shutdown(context.Background())
			settings := componenttest.NewNopTelemetrySettings()
			settings.TracerProvider = provider
			tel, err := telemetry.New(settings, true, false)
			require.NoError(t, err)
			db, err := store.NewStore(context.Background(), "", zap.NewNop())
			require.NoError(t, err)
			defer db.Close()
			server, err := NewServer("localhost:0", 0, db, zap.NewNop(), tel)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(tc.body))
			request.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
			server.rpcHandler(httptest.NewRecorder(), request)
			spans := recorder.Ended()
			require.Len(t, spans, 2)
			encode, rpc := spans[0], spans[1]
			require.Equal(t, "11111111111111111111111111111111", rpc.SpanContext().TraceID().String())
			require.Equal(t, "2222222222222222", rpc.Parent().SpanID().String())
			require.True(t, rpc.Parent().IsRemote())
			require.Equal(t, trace.SpanKindServer, rpc.SpanKind())
			require.Equal(t, rpc.SpanContext().SpanID(), encode.Parent().SpanID())
			if tc.failure {
				require.Equal(t, codes.Error, rpc.Status().Code)
				require.NotEmpty(t, rpc.Events())
			} else {
				require.Equal(t, codes.Unset, rpc.Status().Code)
				require.Equal(t, "viewer.rpc query", rpc.Name())
			}
		})
	}
}
