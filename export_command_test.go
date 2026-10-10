package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func runExportCLI(args ...string) ([]byte, error) {
	cmd := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
	var output, diagnostics bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&diagnostics)
	cmd.SetArgs(append([]string{"export"}, args...))
	err := cmd.Execute()
	return output.Bytes(), err
}

func TestExportThroughProductionServer(t *testing.T) {
	endpoint, tracesExporter, logsExporter, metricsExporter := startAttributeIntegration(t)
	ctx := context.Background()
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl("https://example.test/resource")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.SetSchemaUrl("https://example.test/scope")
	for i := byte(1); i <= 2; i++ {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID([16]byte{15: 1})
		span.SetSpanID([8]byte{7: i})
		span.SetParentSpanID([8]byte{7: 3 - i}) // A cycle must still export completely.
		span.SetStartTimestamp(1)
		span.SetEndTimestamp(2)
		span.Attributes().PutDouble("negative.zero", math.Copysign(0, -1))
		span.Attributes().PutInt("exact", math.MaxInt64)
		span.Attributes().PutInt("minimum", math.MinInt64)
		if i == 1 {
			span.Attributes().PutStr("large", strings.Repeat("x", 1024*1024))
		}
	}
	require.NoError(t, tracesExporter.ConsumeTraces(ctx, traces))
	logs := plog.NewLogs()
	log := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	log.SetTimestamp(1)
	log.SetObservedTimestamp(math.MaxUint64)
	log.Body().SetDouble(math.Copysign(0, -1))
	require.NoError(t, logsExporter.ConsumeLogs(ctx, logs))
	metrics := pmetric.NewMetrics()
	metric := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("negative.zero")
	dp := metric.SetEmptyExponentialHistogram().DataPoints().AppendEmpty()
	dp.SetTimestamp(1)
	dp.SetCount(math.MaxUint64)
	dp.SetZeroThreshold(math.Copysign(0, -1))
	require.NoError(t, metricsExporter.ConsumeMetrics(ctx, metrics))
	refs := map[string]string{"trace": "00000000000000000000000000000001"}
	for _, signal := range []string{"log", "metric"} {
		raw, err := requestViewerRPC(ctx, http.DefaultClient, endpoint, "query", map[string]any{"sql": "select id::varchar from " + signal + "s", "limit": 1})
		require.NoError(t, err)
		var result struct{ Rows [][]string }
		require.NoError(t, json.Unmarshal(raw, &result))
		require.Len(t, result.Rows, 1)
		refs[signal] = result.Rows[0][0]
	}
	for signal, id := range refs {
		for _, query := range []string{"", "?format=json"} {
			t.Run(signal+"/"+query, func(t *testing.T) {
				url := endpoint + "/export/" + signal + "s/" + id + query
				response, err := http.Get(url)
				require.NoError(t, err)
				defer response.Body.Close()
				require.Equal(t, http.StatusOK, response.StatusCode)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				assert.Equal(t, "no-store", response.Header.Get("Cache-Control"))
				assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
				assert.Contains(t, response.Header.Get("Content-Disposition"), signal+"-"+id+".json")
				head, err := http.Head(url)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, head.StatusCode)
				assert.Equal(t, response.Header.Get("Content-Disposition"), head.Header.Get("Content-Disposition"))
				headBody, err := io.ReadAll(head.Body)
				require.NoError(t, err)
				require.NoError(t, head.Body.Close())
				assert.Empty(t, headBody)
				output, err := runExportCLI(signal, strings.ToUpper(id), "--endpoint", endpoint)
				require.NoError(t, err)
				assert.Equal(t, body, output, "CLI must preserve bytes, including negative zero and trailing whitespace")
				if query == "" {
					assertJSONFileReplay(t, signal, body)
				}
				switch signal {
				case "trace":
					data, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(body)
					require.NoError(t, err)
					assert.Equal(t, 2, data.SpanCount())
					assert.Equal(t, rs.SchemaUrl(), data.ResourceSpans().At(0).SchemaUrl())
					assert.Equal(t, ss.SchemaUrl(), data.ResourceSpans().At(0).ScopeSpans().At(0).SchemaUrl())
					attrs := data.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
					value, ok := attrs.Get("negative.zero")
					require.True(t, ok)
					assert.Equal(t, uint64(1<<63), math.Float64bits(value.Double()))
					integer, ok := attrs.Get("exact")
					require.True(t, ok)
					assert.Equal(t, int64(math.MaxInt64), integer.Int())
				case "log":
					data, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(body)
					require.NoError(t, err)
					assert.Equal(t, uint64(1<<63), math.Float64bits(data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().Double()))
				case "metric":
					data, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(body)
					require.NoError(t, err)
					assert.Equal(t, uint64(1<<63), math.Float64bits(data.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).ExponentialHistogram().DataPoints().At(0).ZeroThreshold()))
				}
			})
		}
	}
	for _, path := range []string{
		"/export/traces/" + refs["trace"] + "?format=protobuf",
		"/export/traces/" + refs["trace"] + "?format=",
		"/export/traces/" + refs["trace"] + "?format=json&format=json",
		"/export/traces/" + refs["trace"] + "?format=xml",
		"/export/traces/" + refs["trace"] + "?format=json&format=protobuf",
		"/export/traces/not-an-id?format=json",
	} {
		response, err := http.Get(endpoint + path)
		require.NoError(t, err)
		response.Body.Close()
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Empty(t, response.Header.Get("Content-Disposition"))
	}
	for _, signal := range []string{"trace", "log", "metric"} {
		output, err := runExportCLI(signal, "000000000000000000000000000000ff", "--endpoint", endpoint)
		require.ErrorContains(t, err, "404")
		assert.Empty(t, output)
	}
}

func TestExportCLIRejectsArgumentsAndNonExportResponses(t *testing.T) {
	for _, args := range [][]string{
		{"trace"},
		{"trace", "00000000000000000000000000000001", "--format", "xml"},
		{"trace", "00000000000000000000000000000001", "--format", "json"},
		{"trace", "invalid"},
		{"trace", "00000000000000000000000000000001", "--endpoint", "invalid"},
	} {
		output, err := runExportCLI(args...)
		require.Error(t, err)
		assert.Empty(t, output)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>viewer</html>")
	}))
	defer server.Close()
	output, err := runExportCLI("trace", "00000000000000000000000000000001", "--endpoint", server.URL)
	require.ErrorContains(t, err, "content type")
	assert.Empty(t, output)
}

func TestExportHelpIsOffline(t *testing.T) {
	for _, signal := range []string{"trace", "log", "metric"} {
		output, err := runExportCLI(signal, "--help", "--endpoint", "http://127.0.0.1:1")
		require.NoError(t, err)
		assert.Contains(t, string(output), "📤")
		assert.NotContains(t, string(output), "--format")
		assert.Contains(t, string(output), "--endpoint")
	}
}

func TestExportCLICancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.RawQuery)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		cancel()
		<-r.Context().Done()
	}))
	defer server.Close()
	cmd := newExportCommand(server.Client())
	cmd.SetContext(ctx)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"trace", "00000000000000000000000000000001", "--endpoint", server.URL})
	err := cmd.Execute()
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, output.Bytes())
}
