package server

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// Exercise the HTTP-to-generated-SQL paths reported by go/sql-injection.
// Hostile-looking received strings must remain searchable data; identifiers,
// directions and timestamp bounds must never become arbitrary SQL fragments.
func TestDiscoverySQLInputBoundaries(t *testing.T) {
	h := setupHandler(t)
	s, err := NewServer("localhost:8000", 0, h.store, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	data := buildTestTraces()
	resource := data.ResourceSpans().At(0)
	span := resource.ScopeSpans().At(0).Spans().At(0)
	const hostile = "' OR true; DROP TABLE spans; --\\雪"
	resource.Resource().Attributes().PutStr("service.name", hostile)
	resource.Resource().Attributes().PutStr("service.namespace", hostile)
	span.Attributes().PutStr(hostile, hostile)
	require.NoError(t, h.store.WithConn(func(conn driver.Conn) error { return spans.Ingest(t.Context(), conn, data) }))
	for _, kind := range []string{"services", "keys", "values", "home-values"} {
		options := map[string]any{"kind": kind, "signal": "traces", "ownerType": "span", "key": hostile, "serviceName": hostile, "namespace": hostile, "queries": map[string]any{}}
		// JSON value search sees escaped backslashes; this prefix is common to
		// both the native string and its exact JSON identity.
		options["term"] = "' OR true; DROP TABLE spans; --"
		if kind == "home-values" {
			page := homePage(t, h, options)
			require.Len(t, page.Values, 1)
			require.Equal(t, hostile, page.Values[0].Preview)
		} else {
			wire, err := json.Marshal(map[string]any{"options": options})
			require.NoError(t, err)
			native := readArrowDataset(t, s, "discoverTelemetry", string(wire))
			require.Equal(t, 1, native.Column(0).(*array.List).ListValues().Len(), kind)
			result, err := h.Handle(t.Context(), createRequest("discoverTelemetry", map[string]any{"options": options}))
			require.NoError(t, err)
			var page struct {
				Rows [][]json.RawMessage `json:"rows"`
			}
			require.NoError(t, json.Unmarshal(result.(json.RawMessage), &page))
			require.Len(t, page.Rows, 1, kind)
		}
	}
	traceID := span.TraceID().String()
	_, err = h.Handle(t.Context(), createRequest("discoverTelemetry", map[string]any{"options": map[string]any{
		"kind": "home-values", "ownerType": "span", "key": hostile, "endTime": "0; DROP TABLE spans",
	}}))
	require.Error(t, err)
	for _, method := range []string{"getTraceView", "getTraceColumnPreviews"} {
		wire, err := json.Marshal(map[string]any{"traceID": traceID, "columns": []map[string]string{{"source": "span", "key": hostile}}})
		require.NoError(t, err)
		readArrowDataset(t, s, method, string(wire))
	}
	for _, request := range []struct {
		method string
		params map[string]any
	}{
		{"discoverTelemetry", map[string]any{"options": map[string]any{"kind": "keys", "signal": "traces", "ownerType": "event", "startTime": "0' OR true --"}}},
		{"getTraceView", map[string]any{"traceID": traceID, "columns": []map[string]string{{"source": hostile, "key": hostile}}}},
		{"getTraceColumnPreviews", map[string]any{"traceID": traceID, "columns": []map[string]string{{"source": hostile, "key": hostile}}}},
		{"searchTraceSummaries", map[string]any{"startTime": nil, "endTime": nil, "sort": map[string]string{"field": "duration", "direction": "desc; DROP TABLE spans"}}},
	} {
		wire, err := json.Marshal(request.params)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/data/"+request.method, strings.NewReader(string(wire))))
		require.Equal(t, http.StatusBadRequest, response.Code, "%s: %s", request.method, response.Body.String())
	}
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var count int
		err := db.QueryRowContext(t.Context(), "select count(*) from spans").Scan(&count)
		require.Positive(t, count)
		return err
	}))
}
