package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/redaction"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/redactionprocessor"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.yaml.in/yaml/v3"
)

// These stateless processors use the same policy and implementation as ingestion.
// They only normalize a lookup's copy; their sinks never write telemetry.
type importNormalizer struct {
	logs    processor.Logs
	metrics processor.Metrics
}

var importRedactors = sync.OnceValues(func() (*importNormalizer, error) {
	var policy struct {
		Processors struct {
			Redaction map[string]any `yaml:"redaction"`
		} `yaml:"processors"`
	}
	if err := yaml.Unmarshal([]byte(redaction.Policy), &policy); err != nil {
		return nil, err
	}
	factory := redactionprocessor.NewFactory()
	config := factory.CreateDefaultConfig()
	if err := confmap.NewFromStringMap(policy.Processors.Redaction).Unmarshal(config); err != nil {
		return nil, err
	}
	settings := processor.Settings{
		ID: component.MustNewID("redaction"),
		TelemetrySettings: component.TelemetrySettings{
			Logger: zap.NewNop(), MeterProvider: metricnoop.NewMeterProvider(), TracerProvider: tracenoop.NewTracerProvider(),
		},
	}
	logsSink, err := consumer.NewLogs(func(context.Context, plog.Logs) error { return nil })
	if err != nil {
		return nil, err
	}
	metricsSink, err := consumer.NewMetrics(func(context.Context, pmetric.Metrics) error { return nil })
	if err != nil {
		return nil, err
	}
	logs, err := factory.CreateLogs(context.Background(), settings, config, logsSink)
	if err != nil {
		return nil, err
	}
	metrics, err := factory.CreateMetrics(context.Background(), settings, config, metricsSink)
	return &importNormalizer{logs: logs, metrics: metrics}, err
})

func (s *Server) importTargetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 20<<20))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "Import target exceeds 20 MiB", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Could not read import target", http.StatusBadRequest)
		}
		return
	}
	normalizer, err := importRedactors()
	if err != nil {
		http.Error(w, "Could not initialize import lookup", http.StatusInternalServerError)
		return
	}
	target, err := s.store.FindImportTarget(r.Context(), r.PathValue("signal"), body, normalizer.logs.ConsumeLogs, normalizer.metrics.ConsumeMetrics)
	if err != nil {
		if errors.Is(err, store.ErrInvalidImportTarget) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			s.logger.Error("finding import target", zap.Error(err))
			http.Error(w, "Could not find import target", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(target)
}
