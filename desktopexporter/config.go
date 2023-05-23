package desktopexporter

import (
	"fmt"

	"go.opentelemetry.io/collector/component"
)

// Config defines configuration for logging exporter.
type Config struct {
	// Endpoint defines where we serve our frontend app
	Endpoint string `mapstructure:"endpoint"`
	UIFlag   bool   `mapstructure:"ui-flag"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the exporter configuration is valid
func (cfg *Config) Validate() error {
	if cfg.Endpoint == "localhost:8888" {
		return fmt.Errorf("port 8888 is not supported as it is used internally")
	}

	return nil
}
