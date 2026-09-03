//go:build waterfallbench

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type armCFlatTrace struct {
	Format    string                  `json:"format"`
	TraceID   string                  `json:"traceID"`
	Resources map[string]armCResource `json:"resources"`
	Scopes    map[string]armCScope    `json:"scopes"`
	Rows      []armCFlatSpan          `json:"rows"`
}

type armCFlatSpan struct {
	SpanID                 string          `json:"spanID"`
	ParentSpanID           *string         `json:"parentSpanID"`
	TraceState             string          `json:"traceState"`
	Flags                  uint32          `json:"flags"`
	Name                   string          `json:"name"`
	Kind                   string          `json:"kind"`
	StartTime              string          `json:"startTime"`
	EndTime                string          `json:"endTime"`
	Attributes             []armCAttribute `json:"attributes"`
	Events                 []armCEvent     `json:"events"`
	Links                  []armCLink      `json:"links"`
	ResourceRef            string          `json:"resourceRef"`
	ScopeRef               string          `json:"scopeRef"`
	DroppedAttributesCount uint32          `json:"droppedAttributesCount"`
	DroppedEventsCount     uint32          `json:"droppedEventsCount"`
	DroppedLinksCount      uint32          `json:"droppedLinksCount"`
	StatusCode             string          `json:"statusCode"`
	StatusMessage          string          `json:"statusMessage"`
}

type armCAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

type armCEvent struct {
	Name                   string          `json:"name"`
	Timestamp              string          `json:"timestamp"`
	Attributes             []armCAttribute `json:"attributes"`
	DroppedAttributesCount uint32          `json:"droppedAttributesCount"`
}

type armCLink struct {
	TraceID                string          `json:"traceID"`
	SpanID                 string          `json:"spanID"`
	TraceState             string          `json:"traceState"`
	Flags                  uint32          `json:"flags"`
	Attributes             []armCAttribute `json:"attributes"`
	DroppedAttributesCount uint32          `json:"droppedAttributesCount"`
}

type armCResource struct {
	Attributes             []armCAttribute `json:"attributes"`
	DroppedAttributesCount uint32          `json:"droppedAttributesCount"`
}

type armCScope struct {
	Name                   string          `json:"name"`
	Version                string          `json:"version"`
	Attributes             []armCAttribute `json:"attributes"`
	DroppedAttributesCount uint32          `json:"droppedAttributesCount"`
}

func populatedArmCStore(t *testing.T) *store.Store {
	t.Helper()
	benchmarkStore, err := store.NewStore(context.Background(), "", zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, benchmarkStore.Close()) })
	rejected, err := ingestFixtures(context.Background(), benchmarkStore)
	require.NoError(t, err)
	require.Zero(t, rejected)
	return benchmarkStore
}

func TestArmCFlatQueryOwnsNoTreeShape(t *testing.T) {
	lower := strings.ToLower(armCFlatTraceSQL)
	require.Equal(t, 1, strings.Count(armCFlatTraceSQL, "?"))
	for _, forbidden := range []string{
		"with recursive", "sort_path", "cycle_point", "'depth'", "'matched'", "'salvaged'", "'cyclepoint'",
	} {
		require.NotContains(t, lower, forbidden)
	}
	require.NotContains(t, lower, "list(row_json order by")
	require.Contains(t, armCFlatTraceSQL, armCFlatFormat)
}

func TestArmCFlatQueryReturnsEveryCheckedFixture(t *testing.T) {
	benchmarkStore := populatedArmCStore(t)
	manifest, err := loadEmbeddedFixtureManifest()
	require.NoError(t, err)

	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			raw, err := queryArmCFlatTrace(context.Background(), benchmarkStore, fixture.TraceID)
			require.NoError(t, err)
			assertArmCWireShape(t, raw)

			var trace armCFlatTrace
			require.NoError(t, json.Unmarshal(raw, &trace))
			require.Equal(t, armCFlatFormat, trace.Format)
			require.Equal(t, fixture.TraceID, trace.TraceID)
			require.Len(t, trace.Rows, fixture.SpanCount)
			require.NotEmpty(t, trace.Resources)
			require.NotEmpty(t, trace.Scopes)

			seen := make(map[string]struct{}, len(trace.Rows))
			for _, row := range trace.Rows {
				require.NoError(t, validateHexID(row.SpanID, 8))
				if row.ParentSpanID != nil {
					require.NoError(t, validateHexID(*row.ParentSpanID, 8))
				}
				_, duplicate := seen[row.SpanID]
				require.Falsef(t, duplicate, "duplicate span %s", row.SpanID)
				seen[row.SpanID] = struct{}{}
				require.Contains(t, trace.Resources, row.ResourceRef)
				require.Contains(t, trace.Scopes, row.ScopeRef)
				requireDecimalTimestamp(t, row.StartTime)
				requireDecimalTimestamp(t, row.EndTime)
				start, ok := new(big.Int).SetString(row.StartTime, 10)
				require.True(t, ok)
				end, ok := new(big.Int).SetString(row.EndTime, 10)
				require.True(t, ok)
				require.NotEqual(t, 1, start.Cmp(end), "span %s ends before it starts", row.SpanID)
				for _, event := range row.Events {
					requireDecimalTimestamp(t, event.Timestamp)
				}
			}
		})
	}
}

func TestArmCFlatEndpoint(t *testing.T) {
	benchmarkStore := populatedArmCStore(t)
	manifest, err := loadEmbeddedFixtureManifest()
	require.NoError(t, err)
	fixture := manifest.Fixtures[0]
	handler := newArmCServer("127.0.0.1:0", benchmarkStore).httpServer.Handler

	t.Run("health", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, armCHealthPath, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, armCFlatFormat+"\n", response.Body.String())
	})

	t.Run("flat response", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, armCFlatPath,
			strings.NewReader(`{"traceID":"`+fixture.TraceID+`"}`))
		request.Header.Set("Content-Type", "application/json; charset=utf-8")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "application/json", response.Header().Get("Content-Type"))
		assertArmCWireShape(t, response.Body.Bytes())
		require.NotContains(t, response.Body.String(), `"jsonrpc"`)
		require.NotContains(t, response.Body.String(), `"result"`)
	})

	tests := []struct {
		name        string
		method      string
		contentType string
		body        string
		wantStatus  int
	}{
		{name: "wrong method", method: http.MethodGet, contentType: "application/json", body: `{}`, wantStatus: http.StatusMethodNotAllowed},
		{name: "wrong content type", method: http.MethodPost, contentType: "text/plain", body: `{}`, wantStatus: http.StatusUnsupportedMediaType},
		{name: "missing trace ID", method: http.MethodPost, contentType: "application/json", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, contentType: "application/json", body: `{"traceID":"` + fixture.TraceID + `","depth":0}`, wantStatus: http.StatusBadRequest},
		{name: "uppercase trace ID", method: http.MethodPost, contentType: "application/json", body: `{"traceID":"` + strings.ToUpper(fixture.TraceID) + `"}`, wantStatus: http.StatusBadRequest},
		{name: "trailing value", method: http.MethodPost, contentType: "application/json", body: `{"traceID":"` + fixture.TraceID + `"}{}`, wantStatus: http.StatusBadRequest},
		{name: "unknown trace", method: http.MethodPost, contentType: "application/json", body: `{"traceID":"ffffffffffffffffffffffffffffffff"}`, wantStatus: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, armCFlatPath, strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, test.wantStatus, response.Code)
		})
	}
}

func TestArmCFlatQueryReportsMissingTrace(t *testing.T) {
	_, err := queryArmCFlatTrace(context.Background(), populatedArmCStore(t), "ffffffffffffffffffffffffffffffff")
	require.ErrorIs(t, err, errArmCTraceNotFound)
}

func assertArmCWireShape(t *testing.T, raw []byte) {
	t.Helper()
	var trace map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &trace))
	requireExactJSONKeys(t, trace, "format", "resources", "rows", "scopes", "traceID")

	var resources map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(trace["resources"], &resources))
	for _, resource := range resources {
		requireExactJSONKeys(t, resource, "attributes", "droppedAttributesCount")
		assertArmCAttributes(t, resource["attributes"])
	}

	var scopes map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(trace["scopes"], &scopes))
	for _, scope := range scopes {
		requireExactJSONKeys(t, scope, "attributes", "droppedAttributesCount", "name", "version")
		assertArmCAttributes(t, scope["attributes"])
	}

	var rows []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(trace["rows"], &rows))
	for _, row := range rows {
		requireExactJSONKeys(t, row,
			"attributes", "droppedAttributesCount", "droppedEventsCount", "droppedLinksCount",
			"endTime", "events", "flags", "kind", "links", "name", "parentSpanID",
			"resourceRef", "scopeRef", "spanID", "startTime", "statusCode", "statusMessage", "traceState")
		assertArmCAttributes(t, row["attributes"])

		var events []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(row["events"], &events))
		for _, event := range events {
			requireExactJSONKeys(t, event, "attributes", "droppedAttributesCount", "name", "timestamp")
			assertArmCAttributes(t, event["attributes"])
		}

		var links []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(row["links"], &links))
		for _, link := range links {
			requireExactJSONKeys(t, link, "attributes", "droppedAttributesCount", "flags", "spanID", "traceID", "traceState")
			assertArmCAttributes(t, link["attributes"])
		}
	}
}

func assertArmCAttributes(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var attributes []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &attributes))
	for _, attribute := range attributes {
		requireExactJSONKeys(t, attribute, "key", "type", "value")
	}
}

func requireExactJSONKeys(t *testing.T, object map[string]json.RawMessage, want ...string) {
	t.Helper()
	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	slices.Sort(got)
	slices.Sort(want)
	require.Equal(t, want, got)
}

func requireDecimalTimestamp(t *testing.T, value string) {
	t.Helper()
	require.NotEmpty(t, value)
	require.NotContains(t, value, ".")
	_, ok := new(big.Int).SetString(value, 10)
	require.Truef(t, ok, "%q is not a decimal integer", value)
	require.False(t, strings.HasPrefix(value, "-"))
}

func TestDecodeArmCRequestRejectsOversizedBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, armCFlatPath,
		bytes.NewReader(bytes.Repeat([]byte(" "), maxArmCRequestBytes+1)))
	response := httptest.NewRecorder()
	_, err := decodeArmCRequest(response, request)
	require.Error(t, err)
}
