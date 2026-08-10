package desktopexporter

import (
	"context"
	"database/sql/driver"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
)

const (
	// retentionInterval is how often the retention loop checks the store size.
	retentionInterval = 30 * time.Second

	// Default store size caps applied when db_max_size is unset. In-memory
	// mode gets a tighter default because the data competes with everything
	// else for RAM; a database file can afford more room.
	defaultMaxSizeInMemory = 512 << 20 // 512 MB
	defaultMaxSizeOnDisk   = 2 << 30   // 2 GB
)

type desktopExporter struct {
	server *server.Server
	store  *store.Store
	logger *zap.Logger
	tel    *telemetry.Telemetry

	retentionCancel context.CancelFunc
	retentionDone   chan struct{}
}

func newDesktopExporter(ctx context.Context, cfg *Config, settings component.TelemetrySettings) (*desktopExporter, error) {
	logger := settings.Logger

	tel, err := telemetry.New(settings, cfg.SelfTelemetry(), cfg.InstrumentIngest())
	if err != nil {
		return nil, err
	}

	str, err := store.NewStore(ctx, cfg.Db)
	if err != nil {
		return nil, err
	}

	srv, err := server.NewServer(cfg.Endpoint, str, logger, tel)
	if err != nil {
		str.Close()
		return nil, err
	}

	// Config is already validated, so the only parse outcomes are a size,
	// 0 (disabled), or -1 (unset: apply the mode-dependent default).
	maxBytes, err := parseByteSize(cfg.DbMaxSize)
	if err != nil {
		str.Close()
		return nil, err
	}
	if maxBytes < 0 {
		if cfg.Db == "" {
			maxBytes = defaultMaxSizeInMemory
		} else {
			maxBytes = defaultMaxSizeOnDisk
		}
	}
	// The cap lives on the store so getStats can report it alongside usage.
	str.SetRetentionCap(maxBytes)

	return &desktopExporter{
		server: srv,
		store:  str,
		logger: logger,
		tel:    tel,
	}, nil
}

// runRetentionLoop enforces the store size cap every retentionInterval until
// ctx is cancelled. It closes done on exit so Shutdown can wait for the last
// enforcement pass to finish before closing the store underneath it.
func (e *desktopExporter) runRetentionLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	ticker := time.NewTicker(retentionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.enforceRetention(ctx)
		}
	}
}

// enforceRetention runs one retention pass inside a span that carries the store
// size on either side of it, so how much a pass actually reclaims is visible.
//
// The measurements are best-effort: a failed SizeBytes reports 0 for that side
// and the pass runs (or is reported) anyway. Instrumentation must never break
// the thing it measures, and a pass that pruned successfully is not a failure
// just because we could not size the result.
func (e *desktopExporter) enforceRetention(ctx context.Context) {
	spanCtx, endRetention := e.tel.Retention(ctx)

	before := e.storeSizeBytes(spanCtx)
	err := e.store.EnforceRetention(spanCtx, e.store.RetentionCap())
	after := e.storeSizeBytes(spanCtx)

	endRetention(before, after, err)

	if err != nil {
		e.logger.Error("retention enforcement failed", zap.Error(err))
	}
}

// storeSizeBytes measures the store for the retention span, reporting 0 when
// the measurement itself fails.
func (e *desktopExporter) storeSizeBytes(ctx context.Context) int64 {
	size, err := e.store.SizeBytes(ctx)
	if err != nil {
		e.logger.Debug("could not measure store size for retention span", zap.Error(err))
		return 0
	}
	return size
}

// The three push paths each wrap ingest in a span carrying the batch's item
// count, so throughput is measurable per signal. The span covers WithConn, not
// just the write, because acquiring the store's write lock is part of what
// makes ingest slow when it contends with queries.

func (e *desktopExporter) pushTraces(ctx context.Context, source ptrace.Traces) error {
	ctx, end := e.tel.Ingest(ctx, "traces", source.SpanCount())
	err := e.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, source)
	})
	end(err)
	return err
}

func (e *desktopExporter) pushMetrics(ctx context.Context, source pmetric.Metrics) error {
	ctx, end := e.tel.Ingest(ctx, "metrics", source.DataPointCount())
	err := e.store.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, source)
	})
	end(err)
	return err
}

func (e *desktopExporter) pushLogs(ctx context.Context, source plog.Logs) error {
	ctx, end := e.tel.Ingest(ctx, "logs", source.LogRecordCount())
	err := e.store.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, source)
	})
	end(err)
	return err
}

func (e *desktopExporter) Start(ctx context.Context, host component.Host) error {
	if err := e.server.Start(); err != nil {
		return err
	}

	if e.store.RetentionCap() > 0 {
		// The loop gets its own context rather than the startup ctx, which
		// the collector cancels once Start returns.
		retentionCtx, cancel := context.WithCancel(context.Background())
		e.retentionCancel = cancel
		e.retentionDone = make(chan struct{})
		go e.runRetentionLoop(retentionCtx, e.retentionDone)
	}
	return nil
}

func (e *desktopExporter) Shutdown(ctx context.Context) error {
	// Stop the retention loop and wait for any in-flight enforcement pass,
	// so the store isn't closed out from under it.
	if e.retentionCancel != nil {
		e.retentionCancel()
		<-e.retentionDone
	}

	// Shut down the HTTP server and wait for the serve goroutine to exit.
	if err := e.server.Shutdown(ctx); err != nil {
		return err
	}

	// Then close the store
	return e.store.Close()
}
