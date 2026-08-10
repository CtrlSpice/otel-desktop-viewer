package desktopexporter

import (
	"fmt"
	"strconv"
	"strings"
)

// Config represents the exporter config settings (provided to the collector via command line on launch)
type Config struct {
	// Endpoint defines the host and port where we serve our frontend app
	Endpoint string `mapstructure:"endpoint"`

	// DBPath defines the path of your database file. Setting an empty string opens DuckDB in in-memory mode
	Db string `mapstructure:"db"`

	// DbMaxSize caps the size of the telemetry store as a human-readable byte
	// size (e.g. "512MB", "2GB"). "0" disables retention enforcement. An empty
	// string picks a default based on the storage mode: 512MB in-memory, 2GB
	// on disk.
	DbMaxSize string `mapstructure:"db_max_size"`

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
}

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
	if _, err := parseByteSize(cfg.DbMaxSize); err != nil {
		return fmt.Errorf("invalid db_max_size %q: %w", cfg.DbMaxSize, err)
	}

	switch cfg.Telemetry {
	case "", TelemetryDisabled, TelemetryEnabled, TelemetrySelf:
	default:
		return fmt.Errorf("invalid telemetry %q: expected %q, %q, or %q",
			cfg.Telemetry, TelemetryDisabled, TelemetryEnabled, TelemetrySelf)
	}

	return nil
}

// parseByteSize converts a human-readable size string ("512MB", "2GB", "0")
// into a byte count. Units are binary (KB = 1024 bytes) and case-insensitive;
// a bare number is taken as bytes. An empty string returns -1, meaning
// "unset: apply the mode-dependent default".
func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1, nil
	}

	units := []struct {
		suffix     string
		multiplier int64
	}{
		{"TB", 1 << 40},
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
		{"B", 1},
	}

	upper := strings.ToUpper(s)
	multiplier := int64(1)
	digits := upper
	for _, u := range units {
		if strings.HasSuffix(upper, u.suffix) {
			multiplier = u.multiplier
			digits = strings.TrimSpace(strings.TrimSuffix(upper, u.suffix))
			break
		}
	}

	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("expected a size like 512MB or 2GB: %w", err)
	}
	if value < 0 {
		return 0, fmt.Errorf("size must not be negative")
	}
	return value * multiplier, nil
}
