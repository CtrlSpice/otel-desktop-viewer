package desktopexporter

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/duckdbextension"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/metadata"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensiontest"
)

func testExporterSettings(t *testing.T) exporter.Settings {
	t.Helper()
	return exporter.Settings{
		ID:                component.NewID(metadata.Type),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}
}

func freeLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// extensionHost is the minimal component.Host the exporter needs: an
// extensions map to scan for the store owner.
type extensionHost struct {
	exts map[component.ID]component.Component
}

func (h *extensionHost) GetExtensions() map[component.ID]component.Component {
	return h.exts
}

// startTestExtension builds and starts a duckdb extension on a free port and
// returns a host exposing it, mirroring how the collector presents extensions
// to pipeline components. Cleanup shuts the extension down.
func startTestExtension(t *testing.T) (*extensionHost, string) {
	t.Helper()
	ctx := context.Background()

	factory := duckdbextension.NewFactory()
	cfg := factory.CreateDefaultConfig().(*duckdbextension.Config)
	cfg.Endpoint = freeLocalAddr(t)
	cfg.DbMaxSize = "0"

	ext, err := factory.Create(ctx, extensiontest.NewNopSettings(duckdbextension.Type), cfg)
	require.NoError(t, err)
	require.NoError(t, ext.Start(ctx, componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, ext.Shutdown(context.Background())) })

	host := &extensionHost{exts: map[component.ID]component.Component{
		component.NewID(duckdbextension.Type): ext,
	}}
	return host, cfg.Endpoint
}

func TestCreateExporterNilConfig(t *testing.T) {
	ctx := context.Background()
	set := testExporterSettings(t)

	_, err := createMetricsExporter(ctx, set, nil)
	require.EqualError(t, err, "nil config")

	_, err = createLogsExporter(ctx, set, nil)
	require.EqualError(t, err, "nil config")

	_, err = createTracesExporter(ctx, set, nil)
	require.EqualError(t, err, "nil config")
}

// The exporter is write-only: without a store-owning extension in the host it
// must refuse to start, with an error that says what to add.
func TestExporterStartRequiresExtension(t *testing.T) {
	ctx := context.Background()

	exp, err := createTracesExporter(ctx, testExporterSettings(t), createDefaultConfig())
	require.NoError(t, err)

	err = exp.Start(ctx, componenttest.NewNopHost())
	require.ErrorContains(t, err, "no store extension configured")
	require.NoError(t, exp.Shutdown(ctx))
}

// Two store owners would make the write target fall to map iteration order, so
// it is rejected rather than silently picking one.
func TestExporterStartRejectsMultipleExtensions(t *testing.T) {
	ctx := context.Background()
	hostA, _ := startTestExtension(t)
	hostB, _ := startTestExtension(t)

	both := &extensionHost{exts: map[component.ID]component.Component{}}
	for id, ext := range hostA.exts {
		both.exts[id] = ext
	}
	for _, ext := range hostB.exts {
		both.exts[component.NewIDWithName(duckdbextension.Type, "second")] = ext
	}

	exp, err := createTracesExporter(ctx, testExporterSettings(t), createDefaultConfig())
	require.NoError(t, err)

	err = exp.Start(ctx, both)
	require.ErrorContains(t, err, "exactly one")
	require.NoError(t, exp.Shutdown(ctx))
}

// All three signal exporters start independently against one extension, and the
// extension's viewer serves while they run. This replaces the old
// sharedcomponent singleton tests: sharing is now by lookup, so there is no
// construction-order coupling left to protect.
func TestExportersShareExtensionStore(t *testing.T) {
	ctx := context.Background()
	set := testExporterSettings(t)
	host, endpoint := startTestExtension(t)
	cfg := createDefaultConfig()

	var exps []component.Component
	mExp, err := createMetricsExporter(ctx, set, cfg)
	require.NoError(t, err)
	lExp, err := createLogsExporter(ctx, set, cfg)
	require.NoError(t, err)
	tExp, err := createTracesExporter(ctx, set, cfg)
	require.NoError(t, err)
	exps = append(exps, mExp, lExp, tExp)

	for _, exp := range exps {
		require.NoError(t, exp.Start(ctx, host))
	}

	resp, err := http.Get("http://" + endpoint + "/")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	for _, exp := range exps {
		require.NoError(t, exp.Shutdown(ctx))
	}
}

// The exporter finds the store through the storeHost interface, so the
// extension type must keep satisfying it -- and must remain a real extension.
var (
	_ storeHost           = (*duckdbextension.DuckDBExtension)(nil)
	_ extension.Extension = (*duckdbextension.DuckDBExtension)(nil)
)
