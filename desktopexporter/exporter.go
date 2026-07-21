package desktopexporter

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
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

	retentionCancel context.CancelFunc
	retentionDone   chan struct{}
}

func newDesktopExporter(cfg *Config) (*desktopExporter, error) {
	str, err := store.NewStore(context.Background(), cfg.Db)
	if err != nil {
		return nil, err
	}

	srv, err := server.NewServer(cfg.Endpoint, str)
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
			if err := e.store.EnforceRetention(ctx, e.store.RetentionCap()); err != nil {
				log.Printf("retention enforcement failed: %v", err)
			}
		}
	}
}

func (e *desktopExporter) pushTraces(ctx context.Context, source ptrace.Traces) error {
	return e.store.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, source)
	})
}

func (e *desktopExporter) pushMetrics(ctx context.Context, source pmetric.Metrics) error {
	return e.store.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, source)
	})
}

func (e *desktopExporter) pushLogs(ctx context.Context, source plog.Logs) error {
	return e.store.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, source)
	})
}

func (e *desktopExporter) Start(ctx context.Context, host component.Host) error {
	go func() {
		err := e.server.Start()

		if errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server closed\n")
		} else if err != nil {
			fmt.Printf("error listening for server: %s\n", err)
		}

	}()

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

	// Close server first (stops accepting new requests)
	if err := e.server.Close(); err != nil {
		return err
	}

	// Then close the store
	return e.store.Close()
}
