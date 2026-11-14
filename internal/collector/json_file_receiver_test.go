package collector

import (
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpjsonfilereceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver"
)

func TestJSONFileReceiverDefaults(t *testing.T) {
	factory := otlpjsonfilereceiver.NewFactory()
	cfg := factory.CreateDefaultConfig().(*otlpjsonfilereceiver.Config)
	assert.Equal(t, "end", cfg.Config.StartAt)
	assert.True(t, cfg.Config.IncludeFileName)
	assert.Equal(t, 1024*1024, int(cfg.Config.MaxLogSize))
	assert.Equal(t, "split", cfg.Config.MaxLogSizeBehavior)
	assert.False(t, cfg.ReplayFile)
	assert.False(t, cfg.Config.DeleteAfterRead)
	_, err := factory.CreateTraces(t.Context(), receiver.Settings{
		ID: component.NewID(factory.Type()), TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, cfg, new(consumertest.TracesSink))
	require.Error(t, err, "registration alone has no usable file paths")
}
