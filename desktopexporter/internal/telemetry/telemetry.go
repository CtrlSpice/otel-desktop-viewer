// Package telemetry carries the exporter's self-observability: the spans and
// instruments it emits about its own ingest, queries, and retention.
//
// The tool is an OpenTelemetry viewer, so it measures itself the way it expects
// its users to measure their services -- with real telemetry rather than ad-hoc
// timing logs. Point it at its own OTLP endpoint and it renders its own query
// spans.
//
// Disabled is the default. A disabled Telemetry uses noop providers rather than
// nil checks, so call sites are unconditional and cost a virtual call when off.
// Two reasons for the default: a local tool should not spend cycles observing
// itself unless asked, and self-export creates a feedback loop -- ingesting your
// own spans emits spans about that ingest. The loop converges rather than
// explodes (a batch of self-spans produces one more span), but it is noise, and
// it distorts the very ingest numbers you would be measuring. Hence
// InstrumentIngest, which the exporter uses to suppress ingest spans while
// self-exporting.
package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// scopeName identifies this instrumentation scope in emitted telemetry.
const scopeName = "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter"

// Telemetry emits spans and instruments for the exporter's own operation.
// The zero value is not usable; use New or Disabled. Both return an instance on
// which every method is safe to call.
type Telemetry struct {
	tracer trace.Tracer

	ingestDuration    metric.Float64Histogram
	rpcDuration       metric.Float64Histogram
	rpcResponseBytes  metric.Int64Histogram
	retentionDuration metric.Float64Histogram

	// instrumentIngest is false when self-export is on, to keep the ingest
	// feedback loop out of the numbers. See the package comment.
	instrumentIngest bool
}

// Disabled returns a Telemetry backed by noop providers. Every method is safe
// and does nothing.
func Disabled() *Telemetry {
	t, _ := build(tracenoop.NewTracerProvider(), metricnoop.NewMeterProvider(), false)
	return t
}

// New returns a Telemetry wired to the collector's own configured providers,
// which honour whatever the user set under service::telemetry. Returns Disabled
// when enabled is false.
//
// instrumentIngest should be false when the exporter is exporting to itself.
func New(settings component.TelemetrySettings, enabled, instrumentIngest bool) (*Telemetry, error) {
	if !enabled {
		return Disabled(), nil
	}
	return build(settings.TracerProvider, settings.MeterProvider, instrumentIngest)
}

func build(tp trace.TracerProvider, mp metric.MeterProvider, instrumentIngest bool) (*Telemetry, error) {
	meter := mp.Meter(scopeName)
	t := &Telemetry{
		tracer:           tp.Tracer(scopeName),
		instrumentIngest: instrumentIngest,
	}

	var err error
	if t.ingestDuration, err = meter.Float64Histogram(
		"desktopexporter.ingest.duration",
		metric.WithUnit("ms"),
		metric.WithDescription("Time spent writing one OTLP batch to the store."),
	); err != nil {
		return nil, err
	}
	if t.rpcDuration, err = meter.Float64Histogram(
		"desktopexporter.rpc.duration",
		metric.WithUnit("ms"),
		metric.WithDescription("Time spent serving one JSON-RPC method call."),
	); err != nil {
		return nil, err
	}
	if t.rpcResponseBytes, err = meter.Int64Histogram(
		"desktopexporter.rpc.response_bytes",
		metric.WithUnit("By"),
		metric.WithDescription("Encoded size of one JSON-RPC response body."),
	); err != nil {
		return nil, err
	}
	if t.retentionDuration, err = meter.Float64Histogram(
		"desktopexporter.retention.duration",
		metric.WithUnit("ms"),
		metric.WithDescription("Time spent in one retention enforcement pass."),
	); err != nil {
		return nil, err
	}
	return t, nil
}

// Ingest starts a span around one OTLP batch write. signal is "traces",
// "metrics", or "logs"; items is the batch's span / datapoint / record count.
// The returned func ends the span and records the duration; pass it the ingest
// error, or nil.
//
// Returns a no-op when ingest instrumentation is suppressed (self-export).
func (t *Telemetry) Ingest(ctx context.Context, signal string, items int) (context.Context, func(error)) {
	if !t.instrumentIngest {
		return ctx, func(error) {}
	}
	attrs := attribute.NewSet(
		attribute.String("signal", signal),
		attribute.Int("items", items),
	)
	ctx, span := t.tracer.Start(ctx, "desktopexporter.ingest",
		trace.WithAttributes(attrs.ToSlice()...))
	start := time.Now()

	return ctx, func(err error) {
		t.ingestDuration.Record(ctx, millis(start),
			metric.WithAttributes(attribute.String("signal", signal)))
		endSpan(span, err)
	}
}

// RPC starts a span around one JSON-RPC method call. The returned func ends the
// span and records duration; call it with the response's encoded byte count
// (0 if the call failed before encoding) and the error, or nil.
func (t *Telemetry) RPC(ctx context.Context, method string) (context.Context, func(int, error)) {
	ctx, span := t.tracer.Start(ctx, "desktopexporter.rpc",
		trace.WithAttributes(attribute.String("method", method)))
	start := time.Now()

	return ctx, func(responseBytes int, err error) {
		methodAttr := metric.WithAttributes(attribute.String("method", method))
		t.rpcDuration.Record(ctx, millis(start), methodAttr)
		if responseBytes > 0 {
			span.SetAttributes(attribute.Int("response_bytes", responseBytes))
			t.rpcResponseBytes.Record(ctx, int64(responseBytes), methodAttr)
		}
		endSpan(span, err)
	}
}

// RPCEncode starts a child span around serialising one JSON-RPC response. The
// returned func ends the span; pass it the encoding error, or nil.
//
// This is the one part of serving a request that can be attributed on its own.
// DuckDB builds the response JSON inside the SQL query, so query time and
// JSON-construction time are not separable -- EncodeMessage is the clean split.
// Finding it trivial is the useful outcome: it says the latency lives entirely
// in the query and the client, not in our serialisation.
//
// No context is returned because nothing nests inside the span.
func (t *Telemetry) RPCEncode(ctx context.Context) func(error) {
	_, span := t.tracer.Start(ctx, "desktopexporter.rpc.encode")
	return func(err error) {
		endSpan(span, err)
	}
}

// Retention starts a span around one retention enforcement pass. The returned
// func ends the span and records duration; pass it the store size in bytes
// before and after the pass, and the error, or nil.
func (t *Telemetry) Retention(ctx context.Context) (context.Context, func(before, after int64, err error)) {
	ctx, span := t.tracer.Start(ctx, "desktopexporter.retention")
	start := time.Now()

	return ctx, func(before, after int64, err error) {
		span.SetAttributes(
			attribute.Int64("bytes_before", before),
			attribute.Int64("bytes_after", after),
			attribute.Int64("bytes_reclaimed", before-after),
		)
		t.retentionDuration.Record(ctx, millis(start))
		endSpan(span, err)
	}
}

func millis(start time.Time) float64 {
	return float64(time.Since(start).Nanoseconds()) / 1e6
}

// endSpan records err on the span, if any, and ends it. A nil err leaves the
// span status unset rather than explicitly Ok, matching the convention that Ok
// is reserved for an explicit assertion by the application.
func endSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
	}
	span.End()
}
