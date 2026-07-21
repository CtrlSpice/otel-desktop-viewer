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
			name:    "reserved port",
			cfg:     Config{Endpoint: "localhost:8888"},
			wantErr: "port 8888",
		},
		{
			name:    "invalid max size",
			cfg:     Config{Endpoint: "localhost:8000", DbMaxSize: "lots"},
			wantErr: "invalid db_max_size",
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
