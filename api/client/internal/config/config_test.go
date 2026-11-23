package config

import (
	"testing"
	"time"

	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

func TestLoadAppliesDefaultConfig(t *testing.T) {
	t.Setenv(sharedconfig.EnvClientAddress, "")
	t.Setenv(sharedconfig.EnvDialTimeout, "")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load retornou erro: %v", err)
	}

	if config.Address != sharedconfig.DefaultClientAddress {
		t.Errorf(
			"endereço inesperado: obtido %q, esperado %q",
			config.Address,
			sharedconfig.DefaultClientAddress,
		)
	}

	if config.DialTimeout != defaultDialTimeout {
		t.Errorf(
			"timeout inesperado: obtido %s, esperado %s",
			config.DialTimeout,
			defaultDialTimeout,
		)
	}
}

func TestLoadUsesEnvironmentConfig(t *testing.T) {
	const (
		expectedAddress = "192.168.1.100:6381"
		expectedTimeout = 8 * time.Second
	)

	t.Setenv(sharedconfig.EnvClientAddress, expectedAddress)
	t.Setenv(sharedconfig.EnvDialTimeout, expectedTimeout.String())

	config, err := Load()
	if err != nil {
		t.Fatalf("Load retornou erro: %v", err)
	}

	if config.Address != expectedAddress {
		t.Errorf(
			"endereço inesperado: obtido %q, esperado %q",
			config.Address,
			expectedAddress,
		)
	}

	if config.DialTimeout != expectedTimeout {
		t.Errorf(
			"timeout inesperado: obtido %s, esperado %s",
			config.DialTimeout,
			expectedTimeout,
		)
	}
}

func TestLoadRejectsInvalidDialTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
	}{
		{
			name:    "invalid duration",
			timeout: "invalid",
		},
		{
			name:    "zero duration",
			timeout: "0s",
		},
		{
			name:    "negative duration",
			timeout: "-5s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(sharedconfig.EnvDialTimeout, tt.timeout)

			_, err := Load()
			if err == nil {
				t.Fatal("esperava erro para timeout inválido")
			}
		})
	}
}
