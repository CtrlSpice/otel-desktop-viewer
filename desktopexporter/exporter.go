package desktopexporter

import (
	"context"
	"database/sql/driver"
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

	return &desktopExporter{
		server: srv,
		store:  str,
	}, nil
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
