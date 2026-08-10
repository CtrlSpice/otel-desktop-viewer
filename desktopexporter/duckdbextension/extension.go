// Package duckdbextension owns the shared telemetry store and the viewer that
// serves it: the DuckDB database, the frontend HTTP server, and the retention
// loop that keeps the store under its size cap.
//
// It is an extension rather than part of the exporter because the collector
// starts extensions before any pipeline component and shuts them down after
// (service.Start / service.Shutdown document the order), which is exactly the
// lifetime the store needs: up before the first ingest, alive until the last
// queued write has drained. It also gives the three signal exporters one
// instance to share by lookup through host.GetExtensions(), replacing the
// hand-rolled sharedcomponent workaround.
//
// The exporter finds this extension by interface, not by type name: anything in
// the extensions map exposing `Store() *store.Store` qualifies.
package duckdbextension

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
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

// DuckDBExtension is the running component. The store and server exist only
// between Start and Shutdown; construction is deliberately IO-free so factory
// creation cannot fail on a bad db path -- that surfaces at Start, where the
// collector reports component startup errors coherently.
type DuckDBExtension struct {
	cfg    *Config
	logger *zap.Logger
	tel    *telemetry.Telemetry

	store  *store.Store
	server *server.Server

	retentionCancel context.CancelFunc
	retentionDone   chan struct{}
}

func newDuckDBExtension(cfg *Config, set component.TelemetrySettings) (*DuckDBExtension, error) {
	tel, err := telemetry.New(set, cfg.SelfTelemetry(), false)
	if err != nil {
		return nil, err
	}
	return &DuckDBExtension{
		cfg:    cfg,
		logger: set.Logger,
		tel:    tel,
	}, nil
}

// Store exposes the shared store to pipeline components. It is the lookup
// surface the desktop exporter asserts against; only valid between Start and
// Shutdown, which the collector's component ordering guarantees for pipeline
// callers.
func (e *DuckDBExtension) Store() *store.Store {
	return e.store
}

func (e *DuckDBExtension) Start(ctx context.Context, _ component.Host) error {
	str, err := store.NewStore(ctx, e.cfg.Db)
	if err != nil {
		return err
	}

	srv, err := server.NewServer(e.cfg.Endpoint, str, e.logger, e.tel)
	if err != nil {
		str.Close()
		return err
	}

	// Config is already validated, so the only parse outcomes are a size,
	// 0 (disabled), or -1 (unset: apply the mode-dependent default).
	maxBytes, err := parseByteSize(e.cfg.DbMaxSize)
	if err != nil {
		str.Close()
		return err
	}
	if maxBytes < 0 {
		if e.cfg.Db == "" {
			maxBytes = defaultMaxSizeInMemory
		} else {
			maxBytes = defaultMaxSizeOnDisk
		}
	}
	// The cap lives on the store so getStats can report it alongside usage.
	str.SetRetentionCap(maxBytes)

	if err := srv.Start(); err != nil {
		str.Close()
		return err
	}

	e.store = str
	e.server = srv

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

func (e *DuckDBExtension) Shutdown(ctx context.Context) error {
	// Stop the retention loop and wait for any in-flight enforcement pass,
	// so the store isn't closed out from under it.
	if e.retentionCancel != nil {
		e.retentionCancel()
		<-e.retentionDone
		e.retentionCancel = nil
	}

	// Shut down the HTTP server and wait for the serve goroutine to exit.
	if e.server != nil {
		if err := e.server.Shutdown(ctx); err != nil {
			return err
		}
		e.server = nil
	}

	// Then close the store. The collector shuts extensions down after every
	// pipeline component, so the exporters' queues have already drained.
	//
	// Close takes the store's write lock, which waits on every in-flight
	// reader -- so a slow query would otherwise stall shutdown indefinitely,
	// past whatever deadline the collector gave us. Bound it by ctx and report
	// rather than hang: the process is going away, and a store left unclosed
	// at exit loses at most the WAL, which DuckDB replays on next open.
	if e.store != nil {
		store := e.store
		e.store = nil

		closed := make(chan error, 1)
		go func() { closed <- store.Close() }()

		select {
		case err := <-closed:
			return err
		case <-ctx.Done():
			e.logger.Warn("store close did not finish before shutdown deadline; "+
				"a long-running query is still holding the lock",
				zap.Error(ctx.Err()))
			return nil
		}
	}
	return nil
}

// runRetentionLoop enforces the store size cap every retentionInterval until
// ctx is cancelled. It closes done on exit so Shutdown can wait for the last
// enforcement pass to finish before closing the store underneath it.
func (e *DuckDBExtension) runRetentionLoop(ctx context.Context, done chan<- struct{}) {
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
func (e *DuckDBExtension) enforceRetention(ctx context.Context) {
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
func (e *DuckDBExtension) storeSizeBytes(ctx context.Context) int64 {
	size, err := e.store.SizeBytes(ctx)
	if err != nil {
		e.logger.Debug("could not measure store size for retention span", zap.Error(err))
		return 0
	}
	return size
}
