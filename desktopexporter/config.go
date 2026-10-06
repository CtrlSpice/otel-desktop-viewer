package desktopexporter

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// Config holds write-side exporter settings.
type Config struct {
	// Telemetry controls the exporter's own instrumentation -- the spans and
	// metrics it emits about its own ingest, queries and retention. Emission
	// goes wherever the collector's service::telemetry config points.
	//
	//	"disabled" (default) no self-instrumentation
	//	"enabled"  full self-instrumentation
	//	"self"     as "enabled", minus ingest spans
	//
	// Values avoid "on"/"off" deliberately: YAML 1.1 resolves those as
	// booleans, so a hand-written `telemetry: on` would arrive as `true` and
	// fail validation with a confusing message.
	//
	// "self" is for pointing the exporter at its own OTLP endpoint so it
	// renders its own telemetry. Ingest instrumentation is suppressed there
	// because writing your own spans emits spans about that write: the loop
	// converges rather than explodes, but it is noise, and it distorts the
	// ingest numbers you would be measuring.
	Telemetry string `mapstructure:"telemetry"`

	// SendingQueue decouples OTLP receipt from DuckDB writes. Disable it with
	// `sending_queue: {enabled: false}` to make clients wait for each write.
	SendingQueue configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`
}

// defaultSendingQueue configures one ordered local DuckDB writer:
//
//   - NumConsumers 1 matches the store's single appender connection.
//   - WaitForResult false releases clients after enqueue; write failures are
//     reported through exporter logs and telemetry.
//   - BlockOnOverflow true: a full queue applies backpressure to the client
//     instead of dropping data; the client export timeout bounds the wait.
//   - Capacity is measured in telemetry items, not request count.
//   - The batch processor owns batching, leaving one buffer before the store.
//
// Local DuckDB write failures are not retried because a partial write may
// already have committed rows with the same primary keys.
func defaultSendingQueue() configoptional.Optional[exporterhelper.QueueBatchConfig] {
	return configoptional.Some(exporterhelper.QueueBatchConfig{
		NumConsumers:    1,
		WaitForResult:   false,
		BlockOnOverflow: true,
		Sizer:           exporterhelper.RequestSizerTypeItems,
		QueueSize:       50_000,
		Batch:           configoptional.None[exporterhelper.BatchConfig](),
	})
}

// IngestTimeout bounds a single batch write.
//
// This is a backstop for a hung write, not a latency target. It is deliberately
// generous because a timeout can leave a batch partially applied and the queue
// does not retry.
const IngestTimeout = 30 * time.Second

// Telemetry modes.
const (
	TelemetryDisabled = "disabled"
	TelemetryEnabled  = "enabled"
	TelemetrySelf     = "self"
)

// TelemetryEnabled reports whether self-instrumentation should be active.
func (cfg *Config) SelfTelemetry() bool {
	return cfg.Telemetry == TelemetryEnabled || cfg.Telemetry == TelemetrySelf
}

// InstrumentIngest reports whether ingest itself should be instrumented. False
// in "self" mode, to keep the feedback loop out of the measurements.
func (cfg *Config) InstrumentIngest() bool {
	return cfg.Telemetry == TelemetryEnabled
}

// Validate checks whether the exporter configuration is valid.
func (cfg *Config) Validate() error {
	switch cfg.Telemetry {
	case "", TelemetryDisabled, TelemetryEnabled, TelemetrySelf:
	default:
		return fmt.Errorf("invalid telemetry %q: expected %q, %q, or %q",
			cfg.Telemetry, TelemetryDisabled, TelemetryEnabled, TelemetrySelf)
	}

	// Validated explicitly rather than trusting recursive config validation to
	// reach inside the Optional wrapper.
	if cfg.SendingQueue.HasValue() {
		if err := cfg.SendingQueue.Get().Validate(); err != nil {
			return fmt.Errorf("invalid sending_queue: %w", err)
		}
	}

	return nil
}
