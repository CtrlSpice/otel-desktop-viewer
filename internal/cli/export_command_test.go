package cli

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/internal/collector"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator/helper"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpjsonfilereceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

type exportReplayTarget struct {
	endpoint string
	traces   exporter.Traces
	logs     exporter.Logs
	metrics  exporter.Metrics
}

func newExportReplayTarget(t *testing.T) exportReplayTarget {
	t.Helper()
	endpoint, te, le, me := startAttributeIntegration(t)
	return exportReplayTarget{endpoint: endpoint, traces: te, logs: le, metrics: me}
}

func TestExportThroughProductionServer(t *testing.T) {
	t.Parallel()
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

	// Share one replay target per record-count mode across disjoint signal tables.
	replayTargets := map[int]exportReplayTarget{
		1: newExportReplayTarget(t),
		2: newExportReplayTarget(t),
	}
	factories, err := collector.Components()
	require.NoError(t, err)
	replayFactory := factories.Receivers[otlpjsonfilereceiver.NewFactory().Type()]
	require.NotNil(t, replayFactory)

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

				output, err := runExportCLI(t, signal, strings.ToUpper(id), "--endpoint", endpoint)
				require.NoError(t, err)
				assert.Equal(t, body, output, "CLI must preserve bytes, including negative zero and trailing whitespace")
				if query == "" {
					assertJSONFileReplay(t, replayFactory, signal, body, replayTargets)
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
		output, err := runExportCLI(t, signal, "000000000000000000000000000000ff", "--endpoint", endpoint)
		require.ErrorContains(t, err, "404")
		assert.Empty(t, output)
	}
}

// Test-owned paths and settings do not select a production import policy.
func assertJSONFileReplay(t *testing.T, factory receiver.Factory, signal string, body []byte, targets map[int]exportReplayTarget) {
	t.Helper()
	for _, records := range []int{1, 2} {
		name := "renamed-json"
		if records == 2 {
			name = "renamed-jsonl"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "unrelated.data")
			contents := append([]byte(nil), body...)
			if records == 2 {
				contents = append(contents, '\n')
				contents = append(contents, body...)
				contents = append(contents, '\n')
			}
			require.NoError(t, os.WriteFile(path, contents, 0o600))
			cfg := factory.CreateDefaultConfig().(*otlpjsonfilereceiver.Config)
			cfg.Config.Include = []string{path}
			cfg.Config.StartAt = "beginning"
			cfg.Config.IncludeFileName = false
			// Bound the test record to its actual bytes, not an arbitrary import limit.
			cfg.Config.MaxLogSize = helper.ByteSize(len(body) + 1)
			cfg.Config.PollInterval = 10 * time.Millisecond
			cfg.Config.FlushPeriod = 10 * time.Millisecond
			settings := receivertest.NewNopSettings(factory.Type())
			traces, logs, metrics := new(consumertest.TracesSink), new(consumertest.LogsSink), new(consumertest.MetricsSink)
			tr, err := factory.CreateTraces(t.Context(), settings, cfg, traces)
			require.NoError(t, err)
			lr, err := factory.CreateLogs(t.Context(), settings, cfg, logs)
			require.NoError(t, err)
			mr, err := factory.CreateMetrics(t.Context(), settings, cfg, metrics)
			require.NoError(t, err)
			stopped := false
			for _, r := range []component.Component{tr, lr, mr} {
				require.NoError(t, r.Start(t.Context(), componenttest.NewNopHost()))
				t.Cleanup(func() {
					if !stopped {
						assert.NoError(t, r.Shutdown(context.Background()))
					}
				})
			}
			require.Eventually(t, func() bool {
				switch signal {
				case "trace":
					return len(traces.AllTraces()) == records
				case "log":
					return len(logs.AllLogs()) == records
				default:
					return len(metrics.AllMetrics()) == records
				}
			}, 5*time.Second, 10*time.Millisecond)
			// Stop all three readers before checking that unrelated consumers stayed empty.
			for _, r := range []component.Component{tr, lr, mr} {
				require.NoError(t, r.Shutdown(context.Background()))
			}
			stopped = true

			target := targets[records]
			var id string
			switch signal {
			case "trace":
				assert.Empty(t, logs.AllLogs())
				assert.Empty(t, metrics.AllMetrics())
				expected, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(body)
				require.NoError(t, err)
				for _, data := range traces.AllTraces() {
					assert.Equal(t, expected, data)
				}
				data := traces.AllTraces()[0]
				id = data.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID().String()
				require.NoError(t, target.traces.ConsumeTraces(t.Context(), data))
			case "log":
				assert.Empty(t, traces.AllTraces())
				assert.Empty(t, metrics.AllMetrics())
				expected, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(body)
				require.NoError(t, err)
				for _, data := range logs.AllLogs() {
					assert.Equal(t, expected, data)
				}
				require.NoError(t, target.logs.ConsumeLogs(t.Context(), logs.AllLogs()[0]))
			case "metric":
				assert.Empty(t, traces.AllTraces())
				assert.Empty(t, logs.AllLogs())
				expected, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(body)
				require.NoError(t, err)
				for _, data := range metrics.AllMetrics() {
					assert.Equal(t, expected, data)
				}
				require.NoError(t, target.metrics.ConsumeMetrics(t.Context(), metrics.AllMetrics()[0]))
			}
			if signal != "trace" {
				_, refs, err := requestQuery(t.Context(), http.DefaultClient, target.endpoint, "select id::varchar from "+signal+"s", 1)
				require.NoError(t, err)
				require.Len(t, refs.Rows, 1)
				id = refs.Rows[0][0].(string)
			}
			response, err := http.Get(target.endpoint + "/export/" + signal + "s/" + id)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			replayed, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, string(body), string(replayed), "receiver and second store must retain exact values, including negative-zero bits")
		})
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
		out, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t)}, append([]string{"export"}, args...)...)
		require.Error(t, err)
		assert.Empty(t, out)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>viewer</html>")
	}))
	defer server.Close()
	output, err := runExportCLI(t, "trace", "00000000000000000000000000000001", "--endpoint", server.URL)
	require.ErrorContains(t, err, "content type")
	assert.Empty(t, output)
}

func TestExportHelpIsOffline(t *testing.T) {
	for _, signal := range []string{"trace", "log", "metric"} {
		out, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t)}, "export", signal, "--help", "--endpoint", "http://127.0.0.1:1")
		require.NoError(t, err)
		assert.Contains(t, out, "📤")
		assert.NotContains(t, out, "--format")
		assert.Contains(t, out, "--endpoint")
	}
}

func TestExportCLICancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.RawQuery)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		cancel()
		<-r.Context().Done()
	}))
	defer server.Close()
	out, _, err := execCLI(t, cliOptions{client: server.Client(), ctx: ctx},
		"export", "trace", "00000000000000000000000000000001", "--endpoint", server.URL)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, out)
}
