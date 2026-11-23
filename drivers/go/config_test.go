package memorydb

import (
	"testing"
	"time"
)

const testDialAddress = "127.0.0.1:6380"

func TestDialConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  DialConfig
		wantErr string
	}{
		{
			name: "valid configuration",
			config: DialConfig{
				Address: testDialAddress,
				Timeout: 5 * time.Second,
			},
		},
		{
			name: "valid configuration without timeout",
			config: DialConfig{
				Address: testDialAddress,
			},
		},
		{
			name: "empty address",
			config: DialConfig{
				Timeout: 5 * time.Second,
			},
			wantErr: "address cannot be empty",
		},
		{
			name: "negative timeout",
			config: DialConfig{
				Address: testDialAddress,
				Timeout: -1 * time.Second,
			},
			wantErr: "timeout cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() error = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("validate() error = nil, want %q", tt.wantErr)
			}

			if err.Error() != tt.wantErr {
				t.Errorf("validate() error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}
