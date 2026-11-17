package desktopexporter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Observe the context that reaches ingestion without sleeping through a timeout
// or replacing the production exporter/helper/ingest path.
type ingestDeadlineRecorder struct {
	*tracetest.SpanRecorder
	deadlines []time.Time
}

func (r *ingestDeadlineRecorder) OnStart(ctx context.Context, span sdktrace.ReadWriteSpan) {
	if span.Name() == "desktopexporter.ingest" {
		deadline, _ := ctx.Deadline()
		r.deadlines = append(r.deadlines, deadline)
	}
	r.SpanRecorder.OnStart(ctx, span)
}

func TestSignalExporterIngestDeadlines(t *testing.T) {
	host, _ := startTestExtension(t)
	factory := NewFactory()
	for _, signal := range []struct {
		name   string
		create func(context.Context, exporter.Settings, component.Config) (component.Component, func(context.Context) error, error)
	}{
		{"traces", func(ctx context.Context, set exporter.Settings, cfg component.Config) (component.Component, func(context.Context) error, error) {
			exp, err := factory.CreateTraces(ctx, set, cfg)
			return exp, func(ctx context.Context) error { return exp.ConsumeTraces(ctx, traceBatch("deadline", 0, 1)) }, err
		}},
		{"logs", func(ctx context.Context, set exporter.Settings, cfg component.Config) (component.Component, func(context.Context) error, error) {
			exp, err := factory.CreateLogs(ctx, set, cfg)
			return exp, func(ctx context.Context) error { return exp.ConsumeLogs(ctx, logBatch("deadline", 1)) }, err
		}},
		{"metrics", func(ctx context.Context, set exporter.Settings, cfg component.Config) (component.Component, func(context.Context) error, error) {
			exp, err := factory.CreateMetrics(ctx, set, cfg)
			return exp, func(ctx context.Context) error { return exp.ConsumeMetrics(ctx, metricBatch("deadline", 1)) }, err
		}},
	} {
		t.Run(signal.name, func(t *testing.T) {
			recorder := &ingestDeadlineRecorder{SpanRecorder: tracetest.NewSpanRecorder()}
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			settings := testExporterSettings(t)
			settings.TracerProvider = provider
			cfg := factory.CreateDefaultConfig().(*Config)
			cfg.Telemetry = TelemetryEnabled
			cfg.SendingQueue = configoptional.None[exporterhelper.QueueBatchConfig]()
			exp, consume, err := signal.create(context.Background(), settings, cfg)
			require.NoError(t, err)
			require.NoError(t, exp.Start(context.Background(), host))
			t.Cleanup(func() { require.NoError(t, exp.Shutdown(context.Background())) })

			t.Run("ingestion limit", func(t *testing.T) {
				recorder.deadlines = nil
				before := time.Now()
				require.NoError(t, consume(context.Background()))
				after := time.Now()
				require.Len(t, recorder.deadlines, 1)
				require.WithinRange(t, recorder.deadlines[0], before.Add(IngestTimeout), after.Add(IngestTimeout))
			})

			t.Run("earlier caller deadline", func(t *testing.T) {
				recorder.deadlines = nil
				deadline := time.Now().Add(20 * time.Second)
				ctx, cancel := context.WithDeadline(context.Background(), deadline)
				defer cancel()
				require.NoError(t, consume(ctx))
				require.Len(t, recorder.deadlines, 1)
				require.Equal(t, deadline, recorder.deadlines[0])
			})

			t.Run("caller cancellation", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				require.ErrorIs(t, consume(ctx), context.Canceled)
			})
		})
	}
}
