package desktopexporter

import (
	"go.opentelemetry.io/collector/component"
)

// Config defines configuration for logging exporter.
type Config struct {
}

var _ component.Config = (*Config)(nil)

// Validate checks if the exporter configuration is valid
func (cfg *Config) Validate() error {
	return nil
}
