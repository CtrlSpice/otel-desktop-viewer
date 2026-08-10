package desktopexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{input: "", want: -1},
		{input: "  ", want: -1},
		{input: "0", want: 0},
		{input: "1024", want: 1024},
		{input: "512MB", want: 512 << 20},
		{input: "2GB", want: 2 << 30},
		{input: "1TB", want: 1 << 40},
		{input: "10kb", want: 10 << 10},
		{input: "1gB", want: 1 << 30},
		{input: "100 KB", want: 100 << 10},
		{input: "7B", want: 7},
		{input: "banana", wantErr: true},
		{input: "12XB", wantErr: true},
		{input: "-5MB", wantErr: true},
		{input: "1.5GB", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseByteSize(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

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
			cfg := Config{Endpoint: "localhost:8000", Telemetry: tc.telemetry}
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
			cfg:  Config{Endpoint: "localhost:8000"},
		},
		{
			name: "valid max size",
			cfg:  Config{Endpoint: "localhost:8000", DbMaxSize: "2GB"},
		},
		{
			name: "zero disables retention",
			cfg:  Config{Endpoint: "localhost:8000", DbMaxSize: "0"},
		},
		{
			name:    "invalid max size",
			cfg:     Config{Endpoint: "localhost:8000", DbMaxSize: "lots"},
			wantErr: "invalid db_max_size",
		},
		{
			name: "telemetry disabled",
			cfg:  Config{Endpoint: "localhost:8000", Telemetry: TelemetryDisabled},
		},
		{
			name: "telemetry on",
			cfg:  Config{Endpoint: "localhost:8000", Telemetry: TelemetryEnabled},
		},
		{
			name: "telemetry self",
			cfg:  Config{Endpoint: "localhost:8000", Telemetry: TelemetrySelf},
		},
		{
			name: "telemetry unset",
			cfg:  Config{Endpoint: "localhost:8000", Telemetry: ""},
		},
		{
			name:    "invalid telemetry",
			cfg:     Config{Endpoint: "localhost:8000", Telemetry: "yes please"},
			wantErr: `invalid telemetry "yes please": expected "disabled", "enabled", or "self"`,
		},
		{
			name:    "telemetry is case sensitive",
			cfg:     Config{Endpoint: "localhost:8000", Telemetry: "Enabled"},
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
