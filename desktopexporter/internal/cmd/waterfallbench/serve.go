//go:build waterfallbench

package main

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/server"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/telemetry"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"go.uber.org/zap"
)

const shutdownTimeout = 10 * time.Second

func serve(ctx context.Context, listen string, stdout io.Writer) (err error) {
	benchmarkStore, err := store.NewStore(ctx, "", zap.NewNop())
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if closeErr := closeWithContext(closeCtx, benchmarkStore.Close); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close store: %w", closeErr))
		}
	}()

	if _, err := ingestFixtures(ctx, benchmarkStore); err != nil {
		return err
	}

	benchmarkServer, err := server.NewServer(listen, benchmarkStore, zap.NewNop(), telemetry.Disabled())
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	if err := benchmarkServer.Start(); err != nil {
		return fmt.Errorf("start server on %s: %w", listen, err)
	}
	// This cleanup is registered after store cleanup so LIFO order drains HTTP
	// requests before closing the database they use.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if shutdownErr := benchmarkServer.Shutdown(shutdownCtx); shutdownErr != nil {
			err = errors.Join(err, fmt.Errorf("shut down server: %w", shutdownErr))
		}
	}()

	if _, err := fmt.Fprintf(stdout, "%s listening on %s\n", benchmarkSentinel, listen); err != nil {
		return fmt.Errorf("print readiness: %w", err)
	}
	<-ctx.Done()
	return nil
}

func closeWithContext(ctx context.Context, closeFn func() error) error {
	closed := make(chan error, 1)
	go func() { closed <- closeFn() }()
	select {
	case err := <-closed:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func ingestFixtures(ctx context.Context, benchmarkStore *store.Store) (int, error) {
	manifest, err := loadEmbeddedFixtureManifest()
	if err != nil {
		return 0, err
	}

	totalRejected := 0
	for _, expected := range manifest.Fixtures {
		entry, data, err := loadFixture(expected.Name)
		if err != nil {
			return totalRejected, err
		}

		request := ptraceotlp.NewExportRequest()
		if err := request.UnmarshalProto(data); err != nil {
			return totalRejected, fmt.Errorf("decode fixture %q OTLP export request: %w", entry.Name, err)
		}
		traces := request.Traces()
		if got := traces.SpanCount(); got != entry.SpanCount {
			return totalRejected, fmt.Errorf(
				"fixture %q decoded to %d spans; manifest records %d", entry.Name, got, entry.SpanCount)
		}

		ingestCtx, cancel := context.WithTimeout(ctx, desktopexporter.IngestTimeout)
		err = benchmarkStore.WithConn(func(conn driver.Conn) error {
			rejected, ingestErr := spans.IngestReport(ingestCtx, conn, traces, benchmarkStore.FlushedIDs())
			totalRejected += rejected.Count()
			if ingestErr != nil {
				return ingestErr
			}
			if rejected.Count() != 0 {
				return fmt.Errorf("rejected %d spans: %v", rejected.Count(), rejected.Reasons())
			}
			return nil
		})
		cancel()
		if err != nil {
			return totalRejected, fmt.Errorf("ingest fixture %q: %w", entry.Name, err)
		}
	}
	return totalRejected, nil
}
