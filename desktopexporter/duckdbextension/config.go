package duckdbextension

import (
	"fmt"
	"strconv"
	"strings"
)

// Config holds the settings for the shared DuckDB store and the viewer it
// serves. These lived on the exporter before the store moved here; the exporter
// now only writes.
type Config struct {
	// Endpoint defines the host and port where we serve our frontend app.
	Endpoint string `mapstructure:"endpoint"`

	// Db defines the path of your database file. Setting an empty string opens
	// DuckDB in in-memory mode.
	Db string `mapstructure:"db"`

	// DbMaxSize caps the size of the telemetry store as a human-readable byte
	// size (e.g. "512MB", "2GB"). "0" disables retention enforcement. An empty
	// string picks a default based on the storage mode: 512MB in-memory, 2GB
	// on disk.
	DbMaxSize string `mapstructure:"db_max_size"`

	// Telemetry controls whether the extension instruments its own operation
	// (RPC serving and retention). Any value the desktop exporter accepts is
	// accepted here, so one flag can set both components; the extension only
	// distinguishes "disabled"/"" from the rest -- the ingest-suppressing
	// "self" mode is exporter behavior.
	Telemetry string `mapstructure:"telemetry"`
}

// Telemetry modes. Shared vocabulary with the desktop exporter's config.
const (
	TelemetryDisabled = "disabled"
	TelemetryEnabled  = "enabled"
	TelemetrySelf     = "self"
)

// SelfTelemetry reports whether the extension should instrument itself.
func (cfg *Config) SelfTelemetry() bool {
	return cfg.Telemetry == TelemetryEnabled || cfg.Telemetry == TelemetrySelf
}

// Validate checks if the extension configuration is valid.
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
