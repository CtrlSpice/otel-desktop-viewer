package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator/helper"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpjsonfilereceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

// Test-owned paths and settings do not select a production import policy.
func assertJSONFileReplay(t *testing.T, signal string, body []byte) {
	t.Helper()
	for _, records := range []int{1, 2} {
		name := "renamed-json"
		if records == 2 {
			name = "renamed-jsonl"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unrelated.data")
			contents := append([]byte(nil), body...)
			if records == 2 {
				contents = append(contents, '\n')
				contents = append(contents, body...)
				contents = append(contents, '\n')
			}
			require.NoError(t, os.WriteFile(path, contents, 0o600))
			factories, err := components()
			require.NoError(t, err)
			factory := factories.Receivers[otlpjsonfilereceiver.NewFactory().Type()]
			require.NotNil(t, factory)
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
			endpoint, te, le, me := startAttributeIntegration(t)
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
				require.NoError(t, te.ConsumeTraces(t.Context(), data))
			case "log":
				assert.Empty(t, traces.AllTraces())
				assert.Empty(t, metrics.AllMetrics())
				expected, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(body)
				require.NoError(t, err)
				for _, data := range logs.AllLogs() {
					assert.Equal(t, expected, data)
				}
				require.NoError(t, le.ConsumeLogs(t.Context(), logs.AllLogs()[0]))
			case "metric":
				assert.Empty(t, traces.AllTraces())
				assert.Empty(t, logs.AllLogs())
				expected, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(body)
				require.NoError(t, err)
				for _, data := range metrics.AllMetrics() {
					assert.Equal(t, expected, data)
				}
				require.NoError(t, me.ConsumeMetrics(t.Context(), metrics.AllMetrics()[0]))
			}
			if signal != "trace" {
				_, refs, err := requestQuery(t.Context(), http.DefaultClient, endpoint, "select id::varchar from "+signal+"s", 1)
				require.NoError(t, err)
				require.Len(t, refs.Rows, 1)
				id = refs.Rows[0][0].(string)
			}
			response, err := http.Get(endpoint + "/export/" + signal + "s/" + id)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			replayed, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, string(body), string(replayed), "receiver and second store must retain exact values, including negative-zero bits")
		})
	}
}

func TestJSONFileReceiverDefaults(t *testing.T) {
	factory := otlpjsonfilereceiver.NewFactory()
	cfg := factory.CreateDefaultConfig().(*otlpjsonfilereceiver.Config)
	assert.Equal(t, "end", cfg.Config.StartAt)
	assert.True(t, cfg.Config.IncludeFileName)
	assert.Equal(t, 1024*1024, int(cfg.Config.MaxLogSize))
	assert.Equal(t, "split", cfg.Config.MaxLogSizeBehavior)
	assert.False(t, cfg.ReplayFile)
	assert.False(t, cfg.Config.DeleteAfterRead)
	_, err := factory.CreateTraces(t.Context(), receiver.Settings{
		ID: component.NewID(factory.Type()), TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg, new(consumertest.TracesSink))
	require.Error(t, err, "registration alone has no usable file paths")
}
