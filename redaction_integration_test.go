package main

import (
	"bytes"
	"context"
	"io"
	"math"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func redactionTestPort(t *testing.T) int {
	t.Helper()
	return redactionTestPorts(t, 1)[0]
}

func redactionTestPorts(t *testing.T, count int) []int {
	t.Helper()
	ports := make([]int, count)
	for i := range ports {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		// Hold every reservation until all ports are chosen, so sibling
		// listeners cannot receive the same ephemeral port.
		defer func() { require.NoError(t, l.Close()) }()
		ports[i] = l.Addr().(*net.TCPAddr).Port
	}
	return ports
}

// Both wire protocols feed the real upstream processor and desktop exporter.
// HTTP JSON is also the browser file-import path. Query the actual stored values
// and download reconstructed OTLP, so UI-only masking cannot satisfy this test.
func TestRedactionReceiveStoreExport(t *testing.T) {
	for _, transport := range []string{"http-file-import", "grpc"} {
		t.Run(transport, func(t *testing.T) {
			viewer, te, le, me := startAttributeIntegration(t)
			tp, lp, mp := startRedaction(t, te, le, me)
			o := testOptions()
			o.host = "127.0.0.1"
			ports := redactionTestPorts(t, 2)
			o.httpPort, o.grpcPort = ports[0], ports[1]
			cfg, err := resolveConfig(t, o)
			require.NoError(t, err)
			factories, err := components()
			require.NoError(t, err)
			id := component.MustNewID("otlp")
			factory := factories.Receivers[id.Type()]
			set := receivertest.NewNopSettings(id.Type())
			tr, err := factory.CreateTraces(t.Context(), set, cfg.Receivers[id], tp)
			require.NoError(t, err)
			lr, err := factory.CreateLogs(t.Context(), set, cfg.Receivers[id], lp)
			require.NoError(t, err)
			mr, err := factory.CreateMetrics(t.Context(), set, cfg.Receivers[id], mp)
			require.NoError(t, err)
			for _, receiver := range []component.Component{tr, lr, mr} {
				require.NoError(t, receiver.Start(t.Context(), componenttest.NewNopHost()))
				t.Cleanup(func() { require.NoError(t, receiver.Shutdown(context.Background())) })
			}
			traces, logs, metrics := redactionFixture()
			if transport == "grpc" {
				conn, err := grpc.NewClient(formatEndpoint(o.host)(o.grpcPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, conn.Close()) })
				_, err = ptraceotlp.NewGRPCClient(conn).Export(t.Context(), ptraceotlp.NewExportRequestFromTraces(traces))
				require.NoError(t, err)
				_, err = plogotlp.NewGRPCClient(conn).Export(t.Context(), plogotlp.NewExportRequestFromLogs(logs))
				require.NoError(t, err)
				_, err = pmetricotlp.NewGRPCClient(conn).Export(t.Context(), pmetricotlp.NewExportRequestFromMetrics(metrics))
				require.NoError(t, err)
			} else {
				traceJSON, err := (&ptrace.JSONMarshaler{}).MarshalTraces(traces)
				require.NoError(t, err)
				logJSON, err := (&plog.JSONMarshaler{}).MarshalLogs(logs)
				require.NoError(t, err)
				metricJSON, err := (&pmetric.JSONMarshaler{}).MarshalMetrics(metrics)
				require.NoError(t, err)
				for signal, payload := range map[string][]byte{"traces": traceJSON, "logs": logJSON, "metrics": metricJSON} {
					response, err := http.Post("http://"+formatEndpoint(o.host)(o.httpPort)+"/v1/"+signal, "application/json", bytes.NewReader(payload))
					require.NoError(t, err)
					body, err := io.ReadAll(response.Body)
					require.NoError(t, response.Body.Close())
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, response.StatusCode, string(body))
				}
			}
			_, result, err := requestQuery(t.Context(), http.DefaultClient, viewer, `select count(*)::varchar from (
				select to_json(s)::varchar as v from spans s
				union all select to_json(l)::varchar from logs l
				union all select to_json(m)::varchar from metrics m
				union all select to_json(d)::varchar from metric_datapoints d
				union all select to_json(e)::varchar from events e
				union all select to_json(l)::varchar from links l
				union all select to_json(e)::varchar from exemplars e
			) where contains(v, 'synthetic-secret')`, 1)
			require.NoError(t, err)
			require.Equal(t, "0", result.Rows[0][0], "secrets must never reach stored attributes or log bodies")
			for signal, table := range map[string]string{"traces": "spans", "logs": "logs", "metrics": "metrics"} {
				idColumn := "id::varchar"
				if signal == "traces" {
					idColumn = "trace_id"
				}
				_, result, err := requestQuery(t.Context(), http.DefaultClient, viewer, "select "+idColumn+" from "+table, 1)
				require.NoError(t, err)
				require.Len(t, result.Rows, 1, signal)
				ref, ok := result.Rows[0][0].(string)
				require.True(t, ok)
				response, err := http.Get(viewer + "/export/" + signal + "/" + ref)
				require.NoError(t, err)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, response.Body.Close())
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, response.StatusCode, string(body))
				assert.NotContains(t, string(body), "synthetic-secret", signal)
				assert.Contains(t, string(body), "****", signal)
				assert.Contains(t, string(body), "9223372036854775807", signal)
				assert.Contains(t, string(body), "keep-id", signal)
				if signal == "logs" {
					got, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(body)
					require.NoError(t, err)
					log := got.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
					assert.Equal(t, "credential created", log.Body().Slice().At(0).Str())
					assert.Contains(t, log.Body().Slice().At(1).Str(), `"integrationID":"keep-integration"`)
					assert.Contains(t, log.Body().Slice().At(1).Str(), `"type":"oauth"`)
					assert.Contains(t, log.Body().Slice().At(1).Str(), `"active":true`)
					exact, ok := log.Body().Slice().At(2).Map().Get("exact")
					require.True(t, ok)
					assert.Equal(t, int64(math.MaxInt64), exact.Int())
				}
			}
		})
	}
}
