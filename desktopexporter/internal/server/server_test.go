package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/stretchr/testify/assert"
)

func setupServer() (*httptest.Server, func()) {
	store := store.NewStore(context.Background(), "")
	s := NewServer("localhost:8000", store)
	testServer := httptest.NewServer(s.server.Handler)

	return testServer, func() {
		testServer.Close()
		s.Close()
		store.Close()
	}
}

func TestIndexHandler(t *testing.T) {
	testServer, teardown := setupServer()
	defer teardown()

	res, err := http.Get(fmt.Sprintf("%s/", testServer.URL))
	assert.Nilf(t, err, "could not send GET request: %v", err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
}

func TestRPCHandlerInvalidJSON(t *testing.T) {
	testServer, teardown := setupServer()
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

func TestRPCHandlerInvalidRequest(t *testing.T) {
	testServer, teardown := setupServer()
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
	testServer, teardown := setupServer()
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
