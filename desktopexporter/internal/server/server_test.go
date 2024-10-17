package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"github.com/stretchr/testify/assert"
)

func setupWithTrace(t *testing.T) (*Server, func(*testing.T)) {
	s := NewServer("localhost:8000")
	testSpanData := telemetry.SpanData{
		TraceID:      "1234567890",
		TraceState:   "",
		SpanID:       "12345",
		ParentSpanID: "",
		Name:         "test",
		Kind:         "",
		StartTime:    time.Now(),
		EndTime:      time.Now().Add(time.Second),
		Attributes:   map[string]interface{}{},
		Events:       []telemetry.EventData{},
		Links:        []telemetry.LinkData{},
		Resource:     &telemetry.ResourceData{Attributes: map[string]any{"service.name": "pumpkin.pie"}, DroppedAttributesCount: 0},
		Scope: &telemetry.ScopeData{
			Name:                   "test.scope",
			Version:                "1",
			Attributes:             map[string]any{},
			DroppedAttributesCount: 0,
		},
		DroppedAttributesCount: 0,
		DroppedEventsCount:     0,
		DroppedLinksCount:      0,
		StatusCode:             "",
		StatusMessage:          "",
	}

	if err := s.Store.AddSpans(context.Background(), []telemetry.SpanData{testSpanData}); err != nil {
		t.Fatalf("could not create test span: %v", err)
	}
	return s, func(t *testing.T) {
		s.Store.Close()
	}
}
func TestTracesHandler(t *testing.T) {
	t.Run("Traces Handler (Empty)", func(t *testing.T) {
		s := NewServer("localhost:8000")
		defer s.Store.Close()

		testSummaries := telemetry.TraceSummaries{
			TraceSummaries: []telemetry.TraceSummary{
				{
					HasRootSpan:     true,
					RootServiceName: "groot",
					RootName:        "i.am.groot",
					RootStartTime:   time.Now(),
					RootEndTime:     time.Now().Add(time.Minute),
					SpanCount:       2,
					TraceID:         "12345",
				},
			},
		}

		req, err := http.NewRequest("GET", "/api/traces", nil)
		if err != nil {
			t.Fatalf("could not create request: %v", err)
		}

		rec := httptest.NewRecorder()
		s.tracesHandler(rec, req)
		res := rec.Result()
		defer res.Body.Close()

		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status OK; got %v", res.Status)
		}

		err = json.Unmarshal(b, &testSummaries)
		if err != nil {
			t.Fatalf("could not unmarshal bytes to trace summaries: %v", err.Error())
		}

		assert.Len(t, testSummaries.TraceSummaries, 0)
	})

	t.Run("Traces Handler (Not Empty)", func(t *testing.T) {
		testSummaries := telemetry.TraceSummaries{}
		s, teardown := setupWithTrace(t)
		defer teardown(t)

		req, err := http.NewRequest("GET", "/api/traces", nil)
		if err != nil {
			t.Fatalf("could not create request: %v", err)
		}

		rec := httptest.NewRecorder()
		s.tracesHandler(rec, req)
		res := rec.Result()
		defer res.Body.Close()

		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status OK; got %v", res.Status)
		}

		err = json.Unmarshal(b, &testSummaries)
		if err != nil {
			t.Fatalf("could not unmarshal bytes to trace summaries: %v", err.Error())
		}

		assert.Equal(t, "1234567890", testSummaries.TraceSummaries[0].TraceID)
		assert.Equal(t, true, testSummaries.TraceSummaries[0].HasRootSpan)
		assert.Equal(t, "test", testSummaries.TraceSummaries[0].RootName)
		assert.Equal(t, "pumpkin.pie", testSummaries.TraceSummaries[0].RootServiceName)
		assert.Equal(t, uint32(1), testSummaries.TraceSummaries[0].SpanCount)
	})
}

func TestTraceIDHandler(t *testing.T) {
	s, teardown := setupWithTrace(t)
	defer teardown(t)

	srv := httptest.NewServer(s.Handler(false))
	defer srv.Close()

	t.Run("Trace ID Handler (Not Found)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", srv.URL, "/api/traces/987654321"))
		if err != nil {
			t.Fatalf("could not send GET request: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400 Bad Request; got %v", res.Status)
		}
	})

	t.Run("Traces ID Handler (ID Found)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", srv.URL, "/api/traces/1234567890"))
		if err != nil {
			t.Fatalf("could not send GET request: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status OK; got %v", res.Status)
		}

		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		testTrace := telemetry.TraceData{}
		err = json.Unmarshal(b, &testTrace)
		if err != nil {
			t.Fatalf("could not unmarshal bytes to trace summaries: %v", err.Error())
		}

		assert.Equal(t, "1234567890", testTrace.TraceID)
		assert.Equal(t, "12345", testTrace.Spans[0].SpanID)
		assert.Equal(t, "test", testTrace.Spans[0].Name)
		assert.Equal(t, "pumpkin.pie", testTrace.Spans[0].Resource.Attributes["service.name"])
		assert.Equal(t, 1, len(testTrace.Spans))
	})
}

func TestClearTracesHandler(t *testing.T) {
	s, teardown := setupWithTrace(t)
	defer teardown(t)

	testSummaries := telemetry.TraceSummaries{}

	// Clear traces
	req, err := http.NewRequest("GET", "/api/clearData", nil)
	if err != nil {
		t.Fatalf("could not create request: %v", err)
	}

	rec := httptest.NewRecorder()
	s.clearTracesHandler(rec, req)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status OK; got %v", res.Status)
	}

	// Get trace summaries
	req, err = http.NewRequest("GET", "/api/traces", nil)
	if err != nil {
		t.Fatalf("could not create request: %v", err)
	}

	rec = httptest.NewRecorder()
	s.tracesHandler(rec, req)
	res = rec.Result()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("could not read response: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status OK; got %v", res.Status)
	}

	err = json.Unmarshal(b, &testSummaries)
	if err != nil {
		t.Fatalf("could not unmarshal bytes to trace summaries: %v", err.Error())
	}

	// Check that there are no traces in store
	assert.Len(t, testSummaries.TraceSummaries, 0)
}

func TestSampleHandler(t *testing.T) {
	s := NewServer("localhost:8000")
	defer s.Store.Close()

	srv := httptest.NewServer(s.Handler(false))
	defer srv.Close()

	t.Run("Sample Data Handler (Traces)", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s%s", srv.URL, "/api/sampleData"))
		if err != nil {
			t.Fatalf("could not send GET request: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status OK; got %v", res.Status)
		}

		res, err = http.Get(fmt.Sprintf("%s%s", srv.URL, "/api/traces/42957c7c2fca940a0d32a0cdd38c06a4"))
		if err != nil {
			t.Fatalf("could not send GET request: %v", err)
		}

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected status OK; got %v", res.Status)
		}

		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("could not read response: %v", err)
		}

		testTrace := telemetry.TraceData{}
		err = json.Unmarshal(b, &testTrace)
		if err != nil {
			t.Fatalf("could not unmarshal bytes to trace summaries: %v", err.Error())
		}

		assert.Equal(t, "42957c7c2fca940a0d32a0cdd38c06a4", testTrace.TraceID)
		assert.Equal(t, "37fd1349bf83d330", testTrace.Spans[0].SpanID)
		assert.Equal(t, "SAMPLE HTTP POST", testTrace.Spans[0].Name)
		assert.Equal(t, "sample-loadgenerator", testTrace.Spans[0].Resource.Attributes["service.name"])
		assert.Equal(t, 3, len(testTrace.Spans))
	})
}

func TestRouting(t *testing.T) {
	// No need to start s, as we only need the Handler method
	s := NewServer("localhost:8000")
	defer s.Store.Close()

	testTable := []struct {
		name     string
		route    string
		expected string
	}{
		{"Traces Handler", "/api/traces", `{"traceSummaries":[]}`},
		{"Sample Data Handler", "/api/sampleData", ``},
		{"Clear Traces Handler", "/api/clearData", ``},
	}

	srv := httptest.NewServer(s.Handler(false))
	defer srv.Close()

	for _, tc := range testTable {
		t.Run(tc.name, func(t *testing.T) {
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
