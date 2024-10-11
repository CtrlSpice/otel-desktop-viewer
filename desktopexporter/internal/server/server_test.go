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

func TestTracesHandler(t *testing.T) {
	s := NewServer("localhost:8000")
	defer s.Store.Close()

	t.Run("Traces Handler (Empty)", func(t *testing.T) {
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

		testSummaries := telemetry.TraceSummaries{}
		err := s.Store.AddSpans(context.Background(), []telemetry.SpanData{testSpanData})
		if err != nil {
			t.Fatalf("could not create test span: %v", err)
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

		assert.Equal(t, testSpanData.TraceID, testSummaries.TraceSummaries[0].TraceID)
		assert.Equal(t, true, testSummaries.TraceSummaries[0].HasRootSpan)
		assert.Equal(t, testSpanData.Name, testSummaries.TraceSummaries[0].RootName)
		assert.Equal(t, testSpanData.Resource.Attributes["service.name"], testSummaries.TraceSummaries[0].RootServiceName)
		assert.Equal(t, uint32(1), testSummaries.TraceSummaries[0].SpanCount)
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
