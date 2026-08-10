package desktopexporter

import (
	"context"
	"database/sql/driver"
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

// storeHost is what the exporter needs from the extensions map: something that
// owns the shared store. Declared consumer-side so the exporter is coupled to
// the capability, not to the duckdb extension's package or type name --
// anything in host.GetExtensions() exposing Store() qualifies.
type storeHost interface {
	Store() *store.Store
}

// desktopExporter writes one signal's batches into the shared store. It owns
// nothing else: the store, the viewer server, and retention live in the duckdb
// extension, which the collector starts before -- and shuts down after -- any
// pipeline component. Each signal's exporter is an independent instance;
// sharing happens through the extension lookup, not through shared construction.
type desktopExporter struct {
	tel *telemetry.Telemetry

	// store is resolved in Start and never changes afterwards. The collector
	// guarantees extensions are started first, so by the time the pipeline
	// calls the push functions this is non-nil.
	store *store.Store
}

func newDesktopExporter(cfg *Config, settings component.TelemetrySettings) (*desktopExporter, error) {
	tel, err := telemetry.New(settings, cfg.SelfTelemetry(), cfg.InstrumentIngest())
	if err != nil {
		return nil, err
	}
	return &desktopExporter{tel: tel}, nil
}

// Start resolves the shared store from the collector's extensions. Exactly one
// store-owning extension must be configured: none is a configuration error
// (add `duckdb:` under extensions and service::extensions), and more than one
// would make the choice fall to map iteration order, so it is rejected rather
// than silently picking a writer target at random.
func (e *desktopExporter) Start(_ context.Context, host component.Host) error {
	var found *store.Store
	for _, ext := range host.GetExtensions() {
		sh, ok := ext.(storeHost)
		if !ok {
			continue
		}
		if found != nil {
			return errors.New("multiple store extensions configured; the desktop exporter needs exactly one duckdb extension")
		}
		found = sh.Store()
	}
	if found == nil {
		return errors.New("no store extension configured: add `duckdb` to extensions and service::extensions")
	}
	e.store = found
	return nil
}

// The three push paths each wrap ingest in a span carrying the batch's item
// count, so throughput is measurable per signal. The span covers WithConn, not
// just the write, because acquiring the store's write lock is part of what
// makes ingest slow when it contends with queries.
//
// Each also imposes IngestTimeout. The incoming context is not a useful
// deadline here: with the sending queue enabled the batcher starts a fresh
// context.Background() per merged batch (it must -- the client's request has
// already completed), so nothing upstream bounds the write. See IngestTimeout
// for why the bound exists and why it is set so far above the working range.
func withIngestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, IngestTimeout)
}

func (e *desktopExporter) pushTraces(ctx context.Context, source ptrace.Traces) error {
	ctx, cancel := withIngestTimeout(ctx)
	defer cancel()

	ctx, end := e.tel.Ingest(ctx, "traces", source.SpanCount())
	err := e.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, source)
	})
	end(err)
	return err
}

func (e *desktopExporter) pushMetrics(ctx context.Context, source pmetric.Metrics) error {
	ctx, cancel := withIngestTimeout(ctx)
	defer cancel()

	ctx, end := e.tel.Ingest(ctx, "metrics", source.DataPointCount())
	err := e.store.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, source)
	})
	end(err)
	return err
}

func (e *desktopExporter) pushLogs(ctx context.Context, source plog.Logs) error {
	ctx, cancel := withIngestTimeout(ctx)
	defer cancel()

	ctx, end := e.tel.Ingest(ctx, "logs", source.LogRecordCount())
	err := e.store.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, source)
	})
	end(err)
	return err
}
