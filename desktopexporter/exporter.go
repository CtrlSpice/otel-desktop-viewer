package desktopexporter

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
)

const (
	MAX_QUEUE_LENGTH = 10000
)

type desktopExporter struct {
	traceStore *store.Store
	server     *server.Server
}

func (exporter *desktopExporter) pushTraces(ctx context.Context, traces ptrace.Traces) error {
	spans := extractSpans(ctx, traces)
	for _, span := range spans {
		exporter.traceStore.Add(ctx, span)
	}
	return nil
}

func newDesktopExporter(cfg *Config) *desktopExporter {
	traceStore := store.NewStore(MAX_QUEUE_LENGTH)
	server := server.NewServer(traceStore, cfg.Endpoint)
	return &desktopExporter{
		traceStore: traceStore,
		server:     server,
	}
}

func (exporter *desktopExporter) Start(ctx context.Context, host component.Host) error {
	go func() {
		err := exporter.server.Start()

		if errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server closed\n")
		} else if err != nil {
			fmt.Printf("error listening for server: %s\n", err)
		}

	}()
	return nil
}

func (exporter *desktopExporter) Shutdown(ctx context.Context) error {
	return exporter.server.Close()
}
