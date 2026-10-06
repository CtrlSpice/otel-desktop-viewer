package main

import (
	"net"
	"strconv"

	"go.opentelemetry.io/collector/otelcol"
)

type configOptions struct {
	host        string
	httpPort    int
	grpcPort    int
	browserPort int
	db          string
	dbMaxSize   string

	// selfTelemetryEndpoint turns on the viewer's own instrumentation and sends
	// its traces and metrics to an external OTLP gRPC endpoint. An empty value
	// keeps viewer telemetry off.
	selfTelemetryEndpoint string
}

// collectorURIs builds the yaml config fragments that stand in for a config
// file. Keys use confmap's "::" delimiter for flat values; the telemetry block
// is a nested document because the declarative-config reader and processor
// schemas are lists of maps.
func collectorURIs(o configOptions) []string {
	endpoint := formatEndpoint(o.host)
	URIs := []string{
		`yaml:receivers::otlp::protocols::http::cors::allowed_origins: [https://*,http://*]`,
		`yaml:receivers::otlp::protocols::http::endpoint: "` + endpoint(o.httpPort) + `"`,
		`yaml:receivers::otlp::protocols::grpc::endpoint: "` + endpoint(o.grpcPort) + `"`,
		// The duckdb extension owns the store, the viewer server, and
		// retention; the desktop exporter only writes and finds the store
		// through the extension at startup.
		`yaml:extensions::duckdb::endpoint: "` + endpoint(o.browserPort) + `"`,
		`yaml:extensions::duckdb::db: ` + o.db,
		`yaml:extensions::duckdb::db_max_size: "` + o.dbMaxSize + `"`,
		`yaml:service::extensions: [duckdb]`,
		// Batching happens here rather than in the exporter's sending queue.
		// send_batch_max_size bounds each write; timeout limits display latency
		// when traffic is too light to fill a batch.
		`yaml:processors::batch::send_batch_size: 8192`,
		`yaml:processors::batch::send_batch_max_size: 20000`,
		`yaml:processors::batch::timeout: 1s`,
		// Pipelines require the desktop exporter declaration. An empty map uses
		// its defaults.
		`yaml:exporters::desktop: {}`,
		`yaml:service::pipelines::traces::receivers: [otlp]`,
		`yaml:service::pipelines::traces::processors: [batch]`,
		`yaml:service::pipelines::traces::exporters: [desktop]`,
		`yaml:service::pipelines::metrics::receivers: [otlp]`,
		`yaml:service::pipelines::metrics::processors: [batch]`,
		`yaml:service::pipelines::metrics::exporters: [desktop]`,
		`yaml:service::pipelines::logs::receivers: [otlp]`,
		`yaml:service::pipelines::logs::processors: [batch]`,
		`yaml:service::pipelines::logs::exporters: [desktop]`,
	}
	return append(URIs, telemetryURIs(o)...)
}

// telemetryURIs composes the service::telemetry block and enables the viewer's
// own instrumentation when an external endpoint is configured.
//
// Off is the default and keeps metrics at level "none", which is what stops the
// collector standing up a metrics pipeline for a tool nobody asked to observe.
//
// An endpoint points both the metric readers and the span processors at the
// same external OTLP gRPC receiver.
// The explicit readers list replaces the default Prometheus reader so metrics
// are sent to the configured OTLP endpoint.
func telemetryURIs(o configOptions) []string {
	if o.selfTelemetryEndpoint == "" {
		return []string{`yaml:service::telemetry::metrics::level: none`}
	}
	target := strconv.Quote(o.selfTelemetryEndpoint)
	return []string{
		`yaml:exporters::desktop::telemetry: enabled`,
		`yaml:extensions::duckdb::telemetry: enabled`,
		"yaml:" + `
service:
  telemetry:
    metrics:
      level: detailed
      readers:
        - periodic:
            exporter:
              otlp:
                protocol: grpc
                endpoint: ` + target + `
    traces:
      processors:
        - batch:
            exporter:
              otlp:
                protocol: grpc
                endpoint: ` + target + `
`,
	}
}

func prepareCollectorSettings(set otelcol.CollectorSettings, options configOptions) otelcol.CollectorSettings {
	set.ConfigProviderSettings.ResolverSettings.URIs = collectorURIs(options)
	set.ConfigProviderSettings.ResolverSettings.DefaultScheme = "env"
	return set
}

// formatEndpoint returns a function that produces a properly formatted
// host:port string for the given host. IPv6 addresses are wrapped in
// brackets so that net.Dial / net.Listen can parse them correctly.
func formatEndpoint(host string) func(port int) string {
	return func(port int) string {
		return net.JoinHostPort(host, strconv.Itoa(port))
	}
}

// browserHostFor returns the host string to use when opening the browser.
// Wildcard addresses (0.0.0.0, ::) are replaced with localhost because
// browsers cannot connect to wildcard addresses directly.
func browserHostFor(host string) string {
	if host == "0.0.0.0" || host == "::" || host == "[::]" {
		return "localhost"
	}
	return host
}
