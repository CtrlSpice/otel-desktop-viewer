package desktopexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// "self" is the mode that differs: instrumented, but with ingest suppressed so
// exporting to yourself does not distort the ingest numbers.
func TestTelemetryMode(t *testing.T) {
	tests := []struct {
		telemetry        string
		enabled          bool
		instrumentIngest bool
	}{
		{telemetry: "", enabled: false, instrumentIngest: false},
		{telemetry: TelemetryDisabled, enabled: false, instrumentIngest: false},
		{telemetry: TelemetryEnabled, enabled: true, instrumentIngest: true},
		{telemetry: TelemetrySelf, enabled: true, instrumentIngest: false},
	}

	for _, tc := range tests {
		t.Run(tc.telemetry, func(t *testing.T) {
			cfg := Config{Telemetry: tc.telemetry}
			assert.Equal(t, tc.enabled, cfg.SelfTelemetry())
			assert.Equal(t, tc.instrumentIngest, cfg.InstrumentIngest())
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "defaults are valid",
			cfg:  Config{},
		},
		{
			name: "telemetry disabled",
			cfg:  Config{Telemetry: TelemetryDisabled},
		},
		{
			name: "telemetry on",
			cfg:  Config{Telemetry: TelemetryEnabled},
		},
		{
			name: "telemetry self",
			cfg:  Config{Telemetry: TelemetrySelf},
		},
		{
			name: "telemetry unset",
			cfg:  Config{Telemetry: ""},
		},
		{
			name:    "invalid telemetry",
			cfg:     Config{Telemetry: "yes please"},
			wantErr: `invalid telemetry "yes please": expected "disabled", "enabled", or "self"`,
		},
		{
			name:    "telemetry is case sensitive",
			cfg:     Config{Telemetry: "Enabled"},
			wantErr: `invalid telemetry "Enabled"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
