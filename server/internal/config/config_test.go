package config

import (
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/idempotency"
	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

func setDefaultEnvironment(t *testing.T) {
	t.Helper()

	t.Setenv(sharedconfig.EnvServerAddress, "")
	t.Setenv(envShutdownTimeout, "")
	t.Setenv(envReadTimeout, "")
	t.Setenv(envWriteTimeout, "")
	t.Setenv(envIdempotencyRetention, "")
	t.Setenv(envIdempotencyCapacity, "")
	t.Setenv(sharedconfig.EnvServerMaxConnections, "")
}

func TestLoadAppliesDefaultConfig(t *testing.T) {
	setDefaultEnvironment(t)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Address != sharedconfig.DefaultServerAddress {
		t.Errorf(
			"Address = %q, want %q",
			got.Address,
			sharedconfig.DefaultServerAddress,
		)
	}

	if got.ShutdownTimeout != defaultShutdownTimeout {
		t.Errorf(
			"ShutdownTimeout = %v, want %v",
			got.ShutdownTimeout,
			defaultShutdownTimeout,
		)
	}

	if got.Idempotency.Retention != idempotency.DefaultRetention {
		t.Errorf(
			"Idempotency.Retention = %v, want %v",
			got.Idempotency.Retention,
			idempotency.DefaultRetention,
		)
	}

	if got.Idempotency.Capacity != idempotency.DefaultCapacity {
		t.Errorf(
			"Idempotency.Capacity = %d, want %d",
			got.Idempotency.Capacity,
			idempotency.DefaultCapacity,
		)
	}

	if got.ReadTimeout != sharedconfig.DefaultServerReadTimeout {
		t.Errorf(
			"ReadTimeout = %v, want %v",
			got.ReadTimeout,
			sharedconfig.DefaultServerReadTimeout,
		)
	}

	if got.WriteTimeout != sharedconfig.DefaultServerWriteTimeout {
		t.Errorf(
			"WriteTimeout = %v, want %v",
			got.WriteTimeout,
			sharedconfig.DefaultServerWriteTimeout,
		)
	}

	if got.MaxConnections != sharedconfig.DefaultServerMaxConnections {
		t.Errorf(
			"MaxConnections = %d, want %d",
			got.MaxConnections,
			sharedconfig.DefaultServerMaxConnections,
		)
	}
}

func TestLoadUsesEnvironmentConfig(t *testing.T) {
	setDefaultEnvironment(t)
	t.Setenv(sharedconfig.EnvServerAddress, "0.0.0.0:6381")
	t.Setenv(envShutdownTimeout, "15s")
	t.Setenv(envIdempotencyRetention, "48h")
	t.Setenv(envIdempotencyCapacity, "200000")
	t.Setenv(envReadTimeout, "45s")
	t.Setenv(envWriteTimeout, "20s")
	t.Setenv(sharedconfig.EnvServerMaxConnections, "2048")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Address != "0.0.0.0:6381" {
		t.Errorf("Address = %q, want %q", got.Address, "0.0.0.0:6381")
	}

	if got.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", got.ShutdownTimeout, 15*time.Second)
	}

	if got.Idempotency.Retention != 48*time.Hour {
		t.Errorf(
			"Idempotency.Retention = %v, want %v",
			got.Idempotency.Retention,
			48*time.Hour,
		)
	}

	if got.Idempotency.Capacity != 200000 {
		t.Errorf("Idempotency.Capacity = %d, want %d", got.Idempotency.Capacity, 200000)
	}

	if got.ReadTimeout != 45*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", got.ReadTimeout, 45*time.Second)
	}

	if got.WriteTimeout != 20*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", got.WriteTimeout, 20*time.Second)
	}

	if got.MaxConnections != 2048 {
		t.Errorf("MaxConnections = %d, want 2048", got.MaxConnections)
	}
}

func TestLoadRejectsInvalidDurations(t *testing.T) {
	setDefaultEnvironment(t)
	tests := []struct {
		name  string
		value string
	}{
		{name: "invalid format", value: "invalid"},
		{name: "zero", value: "0s"},
		{name: "negative", value: "-5s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envShutdownTimeout, test.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestLoadRejectsInvalidIdempotencyRetention(t *testing.T) {
	setDefaultEnvironment(t)
	tests := []struct {
		name  string
		value string
	}{
		{name: "invalid format", value: "invalid"},
		{name: "zero", value: "0s"},
		{name: "negative", value: "-1h"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envIdempotencyRetention, test.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestLoadRejectsInvalidIdempotencyCapacity(t *testing.T) {
	setDefaultEnvironment(t)
	tests := []struct {
		name  string
		value string
	}{
		{name: "invalid format", value: "invalid"},
		{name: "zero", value: "0"},
		{name: "negative", value: "-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envIdempotencyCapacity, test.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestLoadRejectsInvalidNetworkTimeouts(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		value string
	}{
		{"read invalid format", envReadTimeout, "invalid"},
		{"read zero", envReadTimeout, "0s"},
		{"read negative", envReadTimeout, "-1s"},
		{"write invalid format", envWriteTimeout, "invalid"},
		{"write zero", envWriteTimeout, "0s"},
		{"write negative", envWriteTimeout, "-1s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setDefaultEnvironment(t)
			t.Setenv(test.env, test.value)

			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestLoadRejectsInvalidMaxConnections(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"invalid format", "invalid"},
		{"zero", "0"},
		{"negative", "-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setDefaultEnvironment(t)
			t.Setenv(sharedconfig.EnvServerMaxConnections, test.value)

			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}
