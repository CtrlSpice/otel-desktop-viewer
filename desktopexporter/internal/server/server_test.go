package server_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
)

func TestRouting(t *testing.T) {
	s := server.NewServer("localhost:8000")
	defer s.Store.Close()

	testTable := []struct {
		name     string
		route    string
		expected string
	}{
		{"Traces Handler", "/api/traces", `{"traceSummaries":[]}`},
		{"Trace ID Handler", "/api/traces/12345", `{"traceID":"12345","spans":[]}`},
		{"Sample Data Handler", "/api/sampleData", ``},
		{"Clear Traces Handler", "/api/clearData", ``},
	}

	srv := httptest.NewServer(s.Handler(true))
	defer srv.Close()

	for _, tc := range testTable {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(s.Handler(false))
			defer srv.Close()

			res, err := http.Get(fmt.Sprintf("%s%s", srv.URL, tc.route))
			if err != nil {
				t.Fatalf("could not send GET request: %v", err)
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				t.Errorf("expected status OK; got %v", res.Status)
			}

			b, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("could not read response: %v", err)
			}

			if val := string(bytes.TrimSpace(b)); val != tc.expected {
				t.Fatalf("expected %s; got %v", tc.expected, val)
			}
		})
	}
}
