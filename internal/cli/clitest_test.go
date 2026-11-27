package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/duckdbextension"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/extension/extensiontest"
	"go.opentelemetry.io/collector/otelcol"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden CLI files")

var defaultTestClock = time.Unix(10000, 0)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func offlineHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP request to %s", r.URL)
		return nil, nil
	})}
}

func canceledHTTPClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
}

func staticRPCClient(result json.RawMessage) *http.Client {
	body := `{"jsonrpc":"2.0","id":1,"result":` + string(result) + `}`
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
}

func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

func testCollectorSettings() otelcol.CollectorSettings {
	return otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{Command: "otel-desktop-viewer", Version: "test"},
	}
}

type cliOptions struct {
	client    *http.Client
	now       func() time.Time
	ctx       context.Context
	runViewer runCollectorFunc
	openURL   openURLFunc
	mergeErr  bool
}

func execCLI(t *testing.T, opts cliOptions, args ...string) (string, string, error) {
	t.Helper()
	client := opts.client
	if client == nil {
		client = http.DefaultClient
	}
	now := opts.now
	if now == nil {
		now = fixedClock(defaultTestClock)
	}
	runViewer := opts.runViewer
	if runViewer == nil {
		runViewer = func(context.Context, otelcol.CollectorSettings) error {
			t.Fatal("CLI command unexpectedly started a viewer")
			return nil
		}
	}
	openURL := opts.openURL
	if openURL == nil {
		openURL = func(string) error {
			t.Fatal("CLI command unexpectedly opened a browser")
			return nil
		}
	}
	cmd := newRootCommand(testCollectorSettings(), client, now, runViewer, openURL)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	if opts.mergeErr {
		cmd.SetErr(&out)
	} else {
		cmd.SetErr(&errBuf)
	}
	if opts.ctx != nil {
		cmd.SetContext(opts.ctx)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errBuf.String(), err
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, _, err := execCLI(t, cliOptions{}, args...)
	return out, err
}

func runCLIAt(t *testing.T, at time.Time, args ...string) (string, error) {
	t.Helper()
	out, _, err := execCLI(t, cliOptions{now: fixedClock(at)}, args...)
	return out, err
}

func mustRunCLIAt(t *testing.T, at time.Time, args ...string) string {
	t.Helper()
	out, err := runCLIAt(t, at, args...)
	require.NoError(t, err)
	return out
}

func runExportCLI(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()
	out, _, err := execCLI(t, cliOptions{}, append([]string{"export"}, args...)...)
	return []byte(out), err
}

// assertGolden compares got against testdata/<name>.golden.
//
// Regenerate with: go test ./internal/cli -run Golden -update-golden
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file %s; run with -update-golden", path)
	require.Equal(t, string(want), got,
		"golden output %s changed. If deliberate, re-run with -update-golden and read the diff carefully", path)
}

func staticRPCServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server
}

func rpcResultServer(t *testing.T, expectedMethod string, expectedParams map[string]any, result json.RawMessage) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req queryRPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, expectedMethod, req.Method)
		if expectedParams != nil {
			require.Equal(t, expectedParams, req.Params)
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(append(append([]byte(`{"jsonrpc":"2.0","id":1,"result":`), result...), '}'))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server
}

type attributeIntegrationHost struct {
	ext component.Component
}

func (h attributeIntegrationHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{component.NewID(duckdbextension.Type): h.ext}
}

// Exercise ingestion, canonical value encoding, the production HTTP query
// handler/executor, and the actual commands together, without a browser or
// a collector subprocess. Only this test's extension owns the store.
func startAttributeIntegration(t *testing.T) (string, exporter.Traces, exporter.Logs, exporter.Metrics) {
	t.Helper()
	return startAttributeIntegrationWithImportPort(t, 0)
}

func startAttributeIntegrationWithImportPort(t *testing.T, port int) (string, exporter.Traces, exporter.Logs, exporter.Metrics) {
	t.Helper()
	ctx := context.Background()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	factory := duckdbextension.NewFactory()
	cfg := factory.CreateDefaultConfig().(*duckdbextension.Config)
	cfg.Endpoint = address
	cfg.Db = filepath.Join(t.TempDir(), "test.duckdb")
	cfg.DbMaxSize = "0"
	cfg.OTLPHTTPPort = port
	ext, err := factory.Create(ctx, extensiontest.NewNopSettings(duckdbextension.Type), cfg)
	require.NoError(t, err)
	require.NoError(t, ext.Start(ctx, componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, ext.Shutdown(context.Background())) })

	exporterFactory := desktopexporter.NewFactory()
	exporterCfg := exporterFactory.CreateDefaultConfig().(*desktopexporter.Config)
	exporterCfg.SendingQueue = configoptional.None[exporterhelper.QueueBatchConfig]()
	settings := exporter.Settings{
		ID:                component.NewID(exporterFactory.Type()),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}
	tracesExporter, err := exporterFactory.CreateTraces(ctx, settings, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, tracesExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, tracesExporter.Shutdown(context.Background())) })

	logsExporter, err := exporterFactory.CreateLogs(ctx, settings, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, logsExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, logsExporter.Shutdown(context.Background())) })

	metricsExporter, err := exporterFactory.CreateMetrics(ctx, settings, exporterCfg)
	require.NoError(t, err)
	require.NoError(t, metricsExporter.Start(ctx, attributeIntegrationHost{ext: ext}))
	t.Cleanup(func() { require.NoError(t, metricsExporter.Shutdown(context.Background())) })

	return "http://" + address, tracesExporter, logsExporter, metricsExporter
}
