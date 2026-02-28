package desktopexporter

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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

type desktopExporter struct {
	server *server.Server
	store  *store.Store
}

func newDesktopExporter(cfg *Config) *desktopExporter {
	store := store.NewStore(context.Background(), cfg.Db)
	server := server.NewServer(cfg.Endpoint, store)
	return &desktopExporter{
		server: server,
		store:  store,
	}
}

func (e *desktopExporter) pushTraces(ctx context.Context, source ptrace.Traces) error {
	if err := e.store.CheckConnection(); err != nil {
		return fmt.Errorf("failed to add spans: %w", err)
	}
	e.store.Lock()
	defer e.store.Unlock()
	return spans.Ingest(ctx, e.store.Conn(), source)
}

func (e *desktopExporter) pushMetrics(ctx context.Context, source pmetric.Metrics) error {
	if err := e.store.CheckConnection(); err != nil {
		return fmt.Errorf("failed to add metrics: %w", err)
	}
	e.store.Lock()
	defer e.store.Unlock()
	return metrics.Ingest(ctx, e.store.Conn(), source)
}

func (e *desktopExporter) pushLogs(ctx context.Context, source plog.Logs) error {
	if err := e.store.CheckConnection(); err != nil {
		return fmt.Errorf("failed to add logs: %w", err)
	}
	e.store.Lock()
	defer e.store.Unlock()
	return logs.Ingest(ctx, e.store.Conn(), source)
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
	return nil
}

func (e *desktopExporter) Shutdown(ctx context.Context) error {
	// Close server first (stops accepting new requests)
	if err := e.server.Close(); err != nil {
		return err
	}

	// Then close the store
	return e.store.Close()
}
