package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func startCLIImportIntegration(t *testing.T) (viewer, receiver string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	viewer, traces, logs, metrics := startAttributeIntegrationWithImportPort(t, port)
	factory := otlpreceiver.NewFactory()
	cfg := factory.CreateDefaultConfig().(*otlpreceiver.Config)
	cfg.Protocols.GRPC = configoptional.None[configgrpc.ServerConfig]()
	cfg.Protocols.HTTP.GetOrInsertDefault().ServerConfig.NetAddr.Endpoint = address
	set := receivertest.NewNopSettings(factory.Type())
	tr, err := factory.CreateTraces(t.Context(), set, cfg, traces)
	require.NoError(t, err)
	lr, err := factory.CreateLogs(t.Context(), set, cfg, logs)
	require.NoError(t, err)
	mr, err := factory.CreateMetrics(t.Context(), set, cfg, metrics)
	require.NoError(t, err)
	for _, receiver := range []component.Component{tr, lr, mr} {
		require.NoError(t, receiver.Start(t.Context(), componenttest.NewNopHost()))
		t.Cleanup(func() { require.NoError(t, receiver.Shutdown(context.Background())) })
	}
	return viewer, "http://" + address
}

func runImportIntegrationCLI(t *testing.T, endpoint string, files ...string) string {
	t.Helper()
	args := append(append([]string{"import"}, files...), "--endpoint", endpoint)
	if binary := os.Getenv("OTEL_IMPORT_TEST_BINARY"); binary != "" {
		output, err := exec.CommandContext(t.Context(), binary, args...).CombinedOutput()
		require.NoError(t, err, string(output))
		return string(output)
	}
	cmd := newCommand(otelcol.CollectorSettings{BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer"}})
	cmd.SetArgs(args)
	var output bytes.Buffer
	cmd.SetOut(&output)
	require.NoError(t, cmd.ExecuteContext(t.Context()))
	return output.String()
}

func TestCLIImportExportedTelemetryThroughReceiverAndStore(t *testing.T) {
	source, tracesExporter, logsExporter, metricsExporter := startAttributeIntegration(t)
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("owner", "resource")
	rs.SetSchemaUrl("resource-schema")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("scope")
	span := ss.Spans().AppendEmpty()
	span.SetTraceID([16]byte{15: 1})
	span.SetSpanID([8]byte{7: 1})
	span.SetStartTimestamp(1)
	span.SetEndTimestamp(2)
	span.Attributes().PutInt("maximum", math.MaxInt64)
	span.Attributes().PutInt("minimum", math.MinInt64)
	span.Attributes().PutDouble("negative.zero", math.Copysign(0, -1))
	require.NoError(t, tracesExporter.ConsumeTraces(t.Context(), traces))
	logs := plog.NewLogs()
	log := logs.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	log.SetTimestamp(1)
	log.SetObservedTimestamp(math.MaxUint64)
	log.Body().SetDouble(math.Copysign(0, -1))
	require.NoError(t, logsExporter.ConsumeLogs(t.Context(), logs))
	metrics := pmetric.NewMetrics()
	metric := metrics.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("exact-count")
	dp := metric.SetEmptyHistogram().DataPoints().AppendEmpty()
	dp.SetTimestamp(1)
	dp.SetCount(math.MaxUint64)
	dp.SetSum(math.Copysign(0, -1))
	require.NoError(t, metricsExporter.ConsumeMetrics(t.Context(), metrics))
	target, _ := startCLIImportIntegration(t)
	for _, signal := range []string{"trace", "log", "metric"} {
		query := "select id::varchar from " + signal + "s limit 1"
		if signal == "trace" {
			query = "select replace(trace_id::varchar, '-', '') from spans limit 1"
		}
		_, result, err := requestQuery(t.Context(), http.DefaultClient, source, query, 1)
		require.NoError(t, err)
		id := result.Rows[0][0].(string)
		body, err := runExportCLI(signal, id, "--endpoint", source)
		require.NoError(t, err)
		file := writeImportTestFile(t, string(body))
		output := runImportIntegrationCLI(t, target, file)
		require.Contains(t, output, "ingestion not confirmed")
		_, result, err = requestQuery(t.Context(), http.DefaultClient, target, query, 1)
		require.NoError(t, err)
		retained, err := runExportCLI(signal, result.Rows[0][0].(string), "--endpoint", target)
		require.NoError(t, err)
		require.JSONEq(t, string(body), string(retained))
		// JSON equality alone would miss signed zero and large-number rounding.
		switch signal {
		case "trace":
			value, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(retained)
			require.NoError(t, err)
			attrs := value.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
			max, _ := attrs.Get("maximum")
			min, _ := attrs.Get("minimum")
			zero, _ := attrs.Get("negative.zero")
			require.Equal(t, int64(math.MaxInt64), max.Int())
			require.Equal(t, int64(math.MinInt64), min.Int())
			require.Equal(t, uint64(1<<63), math.Float64bits(zero.Double()))
		case "log":
			value, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(retained)
			require.NoError(t, err)
			log := value.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
			require.Equal(t, uint64(math.MaxUint64), uint64(log.ObservedTimestamp()))
			require.Equal(t, uint64(1<<63), math.Float64bits(log.Body().Double()))
		case "metric":
			value, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(retained)
			require.NoError(t, err)
			point := value.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Histogram().DataPoints().At(0)
			require.Equal(t, uint64(math.MaxUint64), point.Count())
			require.Equal(t, uint64(1<<63), math.Float64bits(point.Sum()))
		}
		unchanged, err := os.ReadFile(file)
		require.NoError(t, err)
		require.Equal(t, body, unchanged)
	}
}

func TestCLIImportSharedDatasetThroughReceiverAndStore(t *testing.T) {
	dataset := os.Getenv("OTLP_DATASET")
	if dataset == "" {
		dataset = filepath.Join("testdata", "otlp", "small")
	}
	manifestBytes, err := os.ReadFile(filepath.Join(dataset, "manifest.json"))
	if os.IsNotExist(err) && os.Getenv("OTLP_DATASET") == "" {
		t.Skip("shared small dataset is not present on this base; set OTLP_DATASET to its existing checkout")
	}
	require.NoError(t, err)
	var manifest struct {
		Start    string                                `json:"startTimeUnixNano"`
		End      string                                `json:"endTimeUnixNano"`
		Requests []struct{ Signal, File string }       `json:"requests"`
		Counts   struct{ Spans, Logs, Datapoints int } `json:"storedCounts"`
	}
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	viewer, receiver := startCLIImportIntegration(t)
	// Seed with the manifest's exact request bytes and compare a full CLI import.
	for _, request := range manifest.Requests {
		body, err := os.ReadFile(filepath.Join(dataset, request.File))
		require.NoError(t, err)
		response, err := http.Post(receiver+"/v1/"+request.Signal, "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		issue, err := importResponseIssue(response)
		require.NoError(t, err)
		require.Empty(t, issue)
	}
	target, _ := startCLIImportIntegration(t)
	var files []string
	for _, request := range manifest.Requests {
		files = append(files, filepath.Join(dataset, request.File))
	}
	output := runImportIntegrationCLI(t, target, files...)
	t.Log(output)
	require.Contains(t, output, "ACCEPTED REQUESTS")
	for _, endpoint := range []string{viewer, target} {
		for _, check := range []struct {
			table, timestamp string
			count            int
		}{
			{"spans", "start_time", manifest.Counts.Spans}, {"logs", "timestamp", manifest.Counts.Logs}, {"metric_datapoints", "timestamp", manifest.Counts.Datapoints},
		} {
			sql := "select count(*)::varchar from " + check.table + " where " + check.timestamp + " between " + manifest.Start + " and " + manifest.End
			_, result, err := requestQuery(t.Context(), http.DefaultClient, endpoint, sql, 1)
			require.NoError(t, err)
			require.Equal(t, strconv.Itoa(check.count), result.Rows[0][0])
		}
	}
}
