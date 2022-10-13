package desktopexporter

import (
	"go.opentelemetry.io/collector/config"
	"go.uber.org/zap/zapcore"
)

// Config defines configuration for logging exporter.
type Config struct {
	config.ExporterSettings `mapstructure:",squash"` // squash ensures fields are correctly decoded in embedded struct

	// LogLevel defines log level of the logging exporter; options are debug, info, warn, error.
	LogLevel zapcore.Level `mapstructure:"loglevel"`

	// SamplingInitial defines how many samples are initially logged during each second.
	SamplingInitial int `mapstructure:"sampling_initial"`

	// SamplingThereafter defines the sampling rate after the initial samples are logged.
	SamplingThereafter int `mapstructure:"sampling_thereafter"`
}

var _ config.Exporter = (*Config)(nil)

// Validate checks if the exporter configuration is valid
func (cfg *Config) Validate() error {
	return nil
}
