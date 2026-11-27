package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func writeImportTestFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	return path
}

func importTestViewer(t *testing.T, receiver *httptest.Server) *httptest.Server {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(receiver.URL, "http://"))
	require.NoError(t, err)
	viewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/rpc", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"method":"getImportConfig"`)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"otlpHttpPort":%s}}`, port)
	}))
	t.Cleanup(viewer.Close)
	return viewer
}

func importTestBatches(t *testing.T, input string, maximum int64) ([3][]string, []string) {
	t.Helper()
	source := strings.NewReader(input)
	plan, err := scanImportFile(t.Context(), source, int64(len(input)))
	require.NoError(t, err)
	issues := plan.issues
	var batches [3][]string
	for kind, resources := range plan.resources {
		err := batchImportFile(t.Context(), source, kind, resources, maximum, func(reader io.Reader, size int64) error {
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, int64(len(body)), size)
			require.LessOrEqual(t, size, maximum)
			require.True(t, json.Valid(body), string(body))
			batches[kind] = append(batches[kind], string(body))
			return nil
		}, func(issue string) { issues = append(issues, issue) })
		require.NoError(t, err)
	}
	return batches, issues
}

func TestImportScanPreservesBytesAndMixedJSONL(t *testing.T) {
	resource := `{"resource":{"attributes":[{"key":"precise","value":{"intValue":18446744073709551615}}]},"scopeSpans":[{"scope":{"name":"x"},"spans":[{"startTimeUnixNano":18446744073709551615,"attributes":[{"key":"n","value":{"doubleValue":-0}},{"key":"i","value":{"intValue":-9223372036854775808}},{"key":"text","value":{"stringValue":"\\u1234"}}]}]}]}`
	input := "\xef\xbb\xbf" + `{"resourceSpans":[` + resource + `],"resourceLogs":[{}],"resourceMetrics":[{}]}` + "\n" + `{"resourceSpans":[{}]}`
	batches, issues := importTestBatches(t, input, importRequestBytes)
	require.Empty(t, issues)
	require.Equal(t, []string{`{"resourceSpans":[` + resource + `,{}]}`}, batches[0])
	require.Equal(t, []string{`{"resourceLogs":[{}]}`}, batches[1])
	require.Equal(t, []string{`{"resourceMetrics":[{}]}`}, batches[2])
}

func TestImportSplitMetadataAndMetricAssociation(t *testing.T) {
	point := `{"asInt":"9223372036854775807","timeUnixNano":"18446744073709551615"}`
	input := `{"resourceMetrics":[{"resource":{"attributes":[]},"scopeMetrics":[{"scope":{"name":"scope"},"metrics":[{"name":"metric","unit":"s","gauge":{"dataPoints":[` + point + `,` + point + `,` + point + `]},"metadata":[{"key":"z","value":{"doubleValue":-0}}]}],"schemaUrl":"scope-schema"}],"schemaUrl":"resource-schema"}]}`
	batches, issues := importTestBatches(t, input, int64(len(input)-len(point)-1))
	require.Empty(t, issues)
	require.Len(t, batches[2], 2)
	count := 0
	for _, batch := range batches[2] {
		for _, metadata := range []string{`"resource":{"attributes":[]}`, `"name":"scope"`, `"name":"metric","unit":"s"`, `"doubleValue":-0`, `"schemaUrl":"scope-schema"`, `"schemaUrl":"resource-schema"`} {
			require.Contains(t, batch, metadata)
		}
		count += strings.Count(batch, point)
	}
	require.Equal(t, 3, count)
}

func TestImportTwentyMiBBatchingAndOversizedRecord(t *testing.T) {
	record := `{"body":{"stringValue":"` + strings.Repeat("a", 11*1024*1024) + `"}}`
	input := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[` + record + `,` + record + `]}]}]}`
	batches, issues := importTestBatches(t, input, importRequestBytes)
	require.Empty(t, issues)
	require.Len(t, batches[1], 2)
	tooBig := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"` + strings.Repeat("b", int(importRequestBytes)) + `"}},{}]}]}]}`
	batches, issues = importTestBatches(t, tooBig, importRequestBytes)
	require.Len(t, batches[1], 1)
	require.Len(t, issues, 1)
	require.Contains(t, issues[0], "individual OTLP record")
}

func TestImportMalformedCollectionsNeverOmittedWhenSplitting(t *testing.T) {
	input := `{"resourceSpans":[{"scopeSpans":[{"spans":[null,{"name":"` + strings.Repeat("a", 100) + `"}]}]}]}`
	batches, issues := importTestBatches(t, input, int64(len(input)-1))
	require.Empty(t, batches[0])
	require.Len(t, issues, 1)
	require.Contains(t, issues[0], "Invalid OTLP collection")
}

func TestImportScanSyntaxAndCancellation(t *testing.T) {
	for _, input := range []string{`{"resourceSpans":[{}]}` + "\n" + `{"resourceLogs":[`, `{"resourceSpans":[{}]}{"resourceSpans":[{}]}`, `[]`, `{"resourceSpans":[{}],}`} {
		_, err := scanImportFile(t.Context(), strings.NewReader(input), int64(len(input)))
		require.Error(t, err, input)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := scanImportFile(ctx, bytes.NewReader(nil), 0)
	require.ErrorIs(t, err, context.Canceled)
}

func TestImportWrapperIssuesAndEmptyFiles(t *testing.T) {
	batches, issues := importTestBatches(t, `{"resourceProfiles":[],"wrong":[],"resourceSpans":null,"resourceLogs":[true,{}]}`, importRequestBytes)
	require.Len(t, issues, 4)
	require.Contains(t, issues[0], "coming soon")
	require.Equal(t, []string{`{"resourceLogs":[{}]}`}, batches[1])
	_, issues = importTestBatches(t, "", importRequestBytes)
	require.Equal(t, []string{"No OTLP resource wrappers found (byte 1)"}, issues)
}

func TestImportSplitsResourcesScopesAndRecordsAtExactBoundaries(t *testing.T) {
	for _, key := range []string{"gauge", "sum", "histogram", "exponentialHistogram", "summary"} {
		t.Run(key, func(t *testing.T) {
			point := `{"asDouble":-0.000e+10,"count":18446744073709551615}`
			metric := `{"name":"` + key + `","` + key + `":{"dataPoints":[` + point + `,` + point + `]}}`
			input := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[` + metric + `]}]}]}`
			batches, issues := importTestBatches(t, input, int64(len(input)-len(point)-1))
			require.Empty(t, issues)
			require.Len(t, batches[2], 2)
			for _, batch := range batches[2] {
				require.Contains(t, batch, point)
				require.Contains(t, batch, `"name":"`+key+`"`)
			}
		})
	}
	resource := `{"resource":{"attributes":[]},"scopeSpans":[{"scope":{"name":"first"},"spans":[{"name":"one"}]},{"scope":{"name":"second"},"spans":[{"name":"two"}]}]}`
	input := `{"resourceSpans":[` + resource + `,` + resource + `]}`
	maximum := int64(len(`{"resourceSpans":[` + resource + `]}`))
	batches, issues := importTestBatches(t, input, maximum)
	require.Empty(t, issues)
	require.Equal(t, []string{`{"resourceSpans":[` + resource + `]}`, `{"resourceSpans":[` + resource + `]}`}, batches[0])
	batches, issues = importTestBatches(t, input, maximum-1)
	require.Empty(t, issues)
	require.Len(t, batches[0], 4)
	for i, batch := range batches[0] {
		require.Contains(t, batch, `"resource":{"attributes":[]}`)
		if i%2 == 0 {
			require.Contains(t, batch, `"name":"first"`)
			require.NotContains(t, batch, `"name":"second"`)
		} else {
			require.Contains(t, batch, `"name":"second"`)
			require.NotContains(t, batch, `"name":"first"`)
		}
	}
}

func TestImportCommandPortDiscoveryTablesAndFilePreservation(t *testing.T) {
	input := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"precise","startTimeUnixNano":18446744073709551615}]}]}]}`
	path := writeImportTestFile(t, input)
	var received string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/traces", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		received = string(body)
		fmt.Fprint(w, `{}`)
	}))
	defer receiver.Close()
	viewer := importTestViewer(t, receiver)

	output, _, err := execCLI(t, cliOptions{client: viewer.Client(), ctx: t.Context()}, "import", path, "--endpoint", viewer.URL)
	require.NoError(t, err)
	require.Equal(t, input, received)
	require.Contains(t, output, "ACCEPTED REQUESTS")
	require.Contains(t, output, "Received")
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, input, string(unchanged))

	for _, tc := range []struct {
		args   []string
		golden string
	}{
		{[]string{"--help"}, "help_root"},
		{[]string{"import", "--help"}, "help_import"},
	} {
		help, _, err := execCLI(t, cliOptions{client: offlineHTTPClient(t)}, tc.args...)
		require.NoError(t, err)
		assertGolden(t, tc.golden, help)
	}
}

func TestImportCommandLateInvalidSyntaxAndLaterFilesContinue(t *testing.T) {
	bad := writeImportTestFile(t, `{"resourceLogs":[{}]}`+"\n"+`{"resourceSpans":[`)
	good := writeImportTestFile(t, `{"resourceLogs":[{}]}`)
	requests := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; fmt.Fprint(w, `{}`) }))
	defer receiver.Close()
	viewer := importTestViewer(t, receiver)

	output, _, err := execCLI(t, cliOptions{client: viewer.Client(), ctx: t.Context()}, "import", bad, good, "--endpoint", viewer.URL)
	require.Error(t, err)
	require.Equal(t, 1, requests)
	require.Contains(t, output, "invalid JSON")
	require.Contains(t, output, "Rejected:")
	require.Contains(t, output, "Received")
}

func TestImportResponseIssues(t *testing.T) {
	for _, test := range []struct {
		body    string
		status  int
		issue   string
		invalid bool
	}{
		{`{}`, 200, "", false},
		{`{"partialSuccess":{}}`, 200, "", false},
		{`{"partialSuccess":{"rejectedSpans":"000"}}`, 200, "", false},
		{`{"partialSuccess":{"rejectedLogRecords":"18446744073709551615"}}`, 200, "18446744073709551615", false},
		{`{"partialSuccess":{"rejectedDataPoints":2,"errorMessage":"bad points"}}`, 200, "bad points", false},
		{`{"partialSuccess":{"errorMessage":"warning"}}`, 200, "warning", false},
		{`{"partialSuccess":{"rejectedSpans":-1}}`, 200, "", true},
		{`{"partialSuccess":{"rejectedSpans":9007199254740992}}`, 200, "", true},
		{`{"partialSuccess":{"rejectedSpans":"1.5"}}`, 200, "", true},
		{`{"partialSuccess":{"errorMessage":null}}`, 200, "", true},
		{`{"partialSuccess":null}`, 200, "", true},
		{`[]`, 200, "", true},
		{``, 200, "", true},
		{`{"message":" receiver error "}`, 400, "receiver error", false},
		{`proxy error`, 502, "proxy error", false},
	} {
		t.Run(test.body, func(t *testing.T) {
			response := &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}
			issue, err := importResponseIssue(response)
			if test.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Contains(t, issue, test.issue)
			}
		})
	}
}

func TestImportReceiverFailureContinuesSignalsAndCancellation(t *testing.T) {
	requests := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/v1/traces" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"message":"invalid trace"}`)
		} else {
			fmt.Fprint(w, `{"partialSuccess":{"rejectedLogRecords":"1"}}`)
		}
	}))
	defer receiver.Close()
	viewer := importTestViewer(t, receiver)
	file := writeImportTestFile(t, `{"resourceSpans":[{}],"resourceLogs":[{}]}`)
	accepted, issues, err := importOTLPFile(t.Context(), viewer.Client(), viewer.URL, file)
	require.NoError(t, err)
	require.Zero(t, accepted)
	require.Equal(t, 2, requests)
	require.Len(t, issues, 2)
	require.Contains(t, issues[0], "invalid trace")
	require.Contains(t, issues[1], "rejected 1")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = importOTLPFile(ctx, viewer.Client(), viewer.URL, file)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 2, requests)
}

func TestImportConfigValidation(t *testing.T) {
	for _, result := range []string{`null`, `{}`, `{"otlpHttpPort":0}`, `{"otlpHttpPort":65536}`, `{"otlpHttpPort":"4318"}`, `{"otlpHttpPort":1.5}`} {
		viewer := staticRPCServer(t, `{"jsonrpc":"2.0","id":1,"result":`+result+`}`)
		_, _, err := importOTLPFile(t.Context(), viewer.Client(), viewer.URL, writeImportTestFile(t, `{"resourceLogs":[{}]}`))
		require.ErrorContains(t, err, "invalid OTLP HTTP port")
	}
}

func TestImportCancellationStopsInFlightRequestAndLaterSignals(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	requests := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests++
		_, err := io.Copy(io.Discard, r.Body)
		require.NoError(t, err)
		cancel()
		<-r.Context().Done()
	}))
	defer receiver.Close()
	viewer := importTestViewer(t, receiver)
	path := writeImportTestFile(t, `{"resourceSpans":[{}],"resourceLogs":[{}]}`)
	accepted, _, err := importOTLPFile(ctx, viewer.Client(), viewer.URL, path)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, accepted)
	require.Equal(t, 1, requests)
}

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
	output, _, err := execCLI(t, cliOptions{ctx: t.Context()}, args...)
	require.NoError(t, err)
	return output
}

func TestCLIImportExportedTelemetryThroughReceiverAndStore(t *testing.T) {
	t.Parallel()
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
		body, err := runExportCLI(t, signal, id, "--endpoint", source)
		require.NoError(t, err)
		file := writeImportTestFile(t, string(body))
		output := runImportIntegrationCLI(t, target, file)
		require.Contains(t, output, "Received")
		_, result, err = requestQuery(t.Context(), http.DefaultClient, target, query, 1)
		require.NoError(t, err)
		retained, err := runExportCLI(t, signal, result.Rows[0][0].(string), "--endpoint", target)
		require.NoError(t, err)
		require.JSONEq(t, string(body), string(retained))
		// JSON equality alone would miss signed zero and large-number rounding.
		switch signal {
		case "trace":
			value, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(retained)
			require.NoError(t, err)
			attrs := value.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
			maxVal, _ := attrs.Get("maximum")
			minVal, _ := attrs.Get("minimum")
			zero, _ := attrs.Get("negative.zero")
			require.Equal(t, int64(math.MaxInt64), maxVal.Int())
			require.Equal(t, int64(math.MinInt64), minVal.Int())
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
	t.Parallel()
	dataset := os.Getenv("OTLP_DATASET")
	if dataset == "" {
		dataset = filepath.Join("..", "..", "testdata", "otlp", "small")
	}
	manifestBytes, err := os.ReadFile(filepath.Join(dataset, "manifest.json"))
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
