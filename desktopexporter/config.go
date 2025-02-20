package desktopexporter

import (
	"fmt"
)

// Config represents the exporter config settings (provided to the collector via command line on launch)
type Config struct {
	// Endpoint defines the host and port where we serve our frontend app
	Endpoint string `mapstructure:"endpoint"`

	// DbPath defines the path of your database file. Setting an empty string opens DuckDB in in-memory mode
	DbPath string `mapstructure:"dbpath"`
}

// Validate checks if the exporter configuration is valid
func (cfg *Config) Validate() error {
	if cfg.Endpoint == "localhost:8888" {
		return fmt.Errorf("port 8888 is not supported as it is used internally")
	}

	return nil
}
