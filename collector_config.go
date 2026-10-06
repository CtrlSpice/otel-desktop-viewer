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

	// selfTelemetry turns on the exporter's own instrumentation and points the
	// collector's service telemetry back at this process's own OTLP receiver,
	// so the viewer renders its own spans and metrics.
	selfTelemetry bool
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
		// send_batch_max_size bounds a merged batch, which is what makes the
		// exporter's IngestTimeout a meaningful deadline: without a ceiling no
		// timeout can tell "large" from "stuck".
		//
		// timeout is a maximum wait, not a per-batch delay: whichever of size or
		// time fires first wins. It therefore only binds when traffic is too
		// light to reach send_batch_size, which is exactly interactive
		// debugging -- there it is the lag between a request happening and its
		// trace appearing.
		//
		// 1s is the compromise. At the reference capture's ~3,100 spans/sec,
		// reaching 8192 takes ~2.6s, so during replay the timeout still governs
		// and produces ~3,100-span batches -- 5x larger than 200ms did, which
		// means 5x fewer appender transactions and, after the dedupe rewrite,
		// each distinct resource resolved 5x fewer times. The cost is a 1s
		// worst-case lag when debugging a single request at a time.
		`yaml:processors::batch::send_batch_size: 8192`,
		`yaml:processors::batch::send_batch_max_size: 20000`,
		`yaml:processors::batch::timeout: 1s`,
		// The exporter must still be declared even though all of its store
		// config moved to the extension: pipelines reference `desktop`, and a
		// config with no exporters section is rejected outright. The empty map
		// means "with defaults", exactly like writing `desktop:` in a config
		// file.
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
	return append(URIs, telemetryURIs(o, endpoint(o.grpcPort))...)
}

// telemetryURIs composes the service::telemetry block and the exporter's own
// telemetry mode.
//
// Off is the default and keeps metrics at level "none", which is what stops the
// collector standing up a metrics pipeline for a tool nobody asked to observe.
//
// The self mode points both the metric readers and the span processors at this
// process's own OTLP gRPC receiver, so the viewer renders its own telemetry.
// The explicit readers list matters even though the config would validate
// without it: otelconftelemetry's default config already carries a Pull
// (Prometheus) reader, so raising the level without replacing that reader would
// publish metrics on a Prometheus endpoint rather than sending them to us.
//
// The exporter is put in "self" rather than "enabled" so ingest spans are
// suppressed -- writing our own telemetry would otherwise emit spans describing
// that write.
func telemetryURIs(o configOptions, otlpEndpoint string) []string {
	if !o.selfTelemetry {
		return []string{`yaml:service::telemetry::metrics::level: none`}
	}
	target := "http://" + otlpEndpoint
	return []string{
		`yaml:exporters::desktop::telemetry: self`,
		`yaml:extensions::duckdb::telemetry: self`,
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
