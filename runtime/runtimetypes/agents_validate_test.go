package runtimetypes_test

import (
	"testing"

	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_ExternalACPConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     runtimetypes.ExternalACPConfig
		wantErr bool
	}{
		{
			name: "stdio with command is valid",
			cfg: runtimetypes.ExternalACPConfig{
				Transport: runtimetypes.ExternalACPTransportStdio,
				Command:   "my-acp-agent",
			},
			wantErr: false,
		},
		{
			name: "stdio without command is invalid",
			cfg: runtimetypes.ExternalACPConfig{
				Transport: runtimetypes.ExternalACPTransportStdio,
			},
			wantErr: true,
		},
		{
			name: "endpoint with url is valid",
			cfg: runtimetypes.ExternalACPConfig{
				Transport: runtimetypes.ExternalACPTransportEndpoint,
				URL:       "https://agent.example.com/acp",
			},
			wantErr: false,
		},
		{
			name: "endpoint without url is invalid",
			cfg: runtimetypes.ExternalACPConfig{
				Transport: runtimetypes.ExternalACPTransportEndpoint,
			},
			wantErr: true,
		},
		{
			name:    "empty transport is invalid",
			cfg:     runtimetypes.ExternalACPConfig{},
			wantErr: true,
		},
		{
			name: "unknown transport is invalid",
			cfg: runtimetypes.ExternalACPConfig{
				Transport: "carrier-pigeon",
				Command:   "irrelevant",
				URL:       "irrelevant",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
