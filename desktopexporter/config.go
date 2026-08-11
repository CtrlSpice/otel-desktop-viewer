package desktopexporter

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// Config represents the exporter config settings. The store, viewer endpoint,
// and retention cap moved to the duckdb extension's config -- this exporter
// only writes, so only write-side settings remain.
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

	// SendingQueue decouples OTLP receipt from the DuckDB write: batches are
	// enqueued and a consumer goroutine drains them into the store. Enabled by
	// default (see defaultSendingQueue for the tuning and its rationale);
	// disable with `sending_queue: {enabled: false}` to restore the synchronous
	// write path, where the client blocks on -- and sees the error from -- the
	// store write.
	SendingQueue configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`
}

// defaultSendingQueue tunes the exporter queue for a single local DuckDB
// writer rather than a remote backend:
//
//   - NumConsumers 1: the store serializes writers behind one appender
//     connection anyway, so extra consumers would only contend for its write
//     lock. One consumer makes the serialization explicit and keeps batches
//     arriving in order.
//   - WaitForResult false: the OTLP client is released as soon as the batch is
//     enqueued instead of waiting out the store write -- and, during ingest
//     bursts, the wait for the store's write lock behind other batches. The
//     trade: a failed write is no longer the client's problem; it surfaces in
//     the exporter's own logs and telemetry instead.
//   - BlockOnOverflow true: a full queue applies backpressure to the client
//     instead of rejecting data. For a tool whose whole job is showing you
//     your telemetry, quietly shedding it under load is the worst failure
//     mode; the client SDK's own export timeout bounds the blocking.
//   - Sized in items (spans / datapoints / records), because request count
//     says nothing about how much work or memory a batch represents.
//   - No batching here. Batching is the batch processor's job (configured in
//     main.go's composed pipeline), so there is exactly one buffer in front of
//     the store rather than two with independent flush timers and different
//     overflow behaviour. Merged batches still mean fewer, larger appender
//     transactions, and let ingest resolve each distinct attribute set once
//     per merged batch rather than once per client request.
//
// Deliberately no retry (exporterhelper.WithRetry): a local DuckDB write
// failure is not transient the way a network export failure is, and replaying
// a partially applied batch would collide with already-written primary keys
// and fail forever.
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
// This is a backstop against a hung write, not a latency control. A write that
// stalls holds the store's write lock, and every reader takes that lock -- so a
// wedged ingest freezes the UI permanently rather than degrading it. The
// deadline guarantees the lock is always released.
//
// Sizing, and the caveat that matters: the measurements behind it were taken on
// an Apple M4 Pro, which is fast. They are an *upper bound* on performance, so
// they cannot be used directly to justify a lower bound like this one.
//
// Measured there: ~25us/span, so the batch processor's 20k send_batch_max_size
// is ~500ms of work. Budgeting an order of magnitude for slower hardware -- an
// older laptop, a loaded machine, a cold disk-backed store, a VM with one core
// -- puts a worst-case legitimate batch at ~5s. 30s is ~6x that, which is the
// margin to reason about; the ~60x implied by the M4 figure is not real.
//
// It is deliberately far above the working range because tripping it is
// harmful: appenders flush every flushIntervalSpans (500) records, so a batch
// cut short is *partially* applied, and the queue runs without retry. A
// deadline that fires means silent partial data loss, which is worse than a
// slow write. Note that raising the flush interval widened that partial-write
// window tenfold, which argues for keeping this deadline generous rather than
// tightening it.
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

// Validate checks if the exporter configuration is valid
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
