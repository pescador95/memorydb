package config

import (
	"errors"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/resources"
	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

func setDefaultEnvironment(t *testing.T) {
	t.Helper()

	t.Setenv(sharedconfig.EnvServerAddress, "")
	t.Setenv(envShutdownTimeout, "")
	t.Setenv(envReadTimeout, "")
	t.Setenv(envWriteTimeout, "")
	t.Setenv(envHealthAddress, "")
}

func validResourceConfig() resources.ResourceConfig {
	const (
		mib = int64(1024 * 1024)
		gib = 1024 * mib
	)

	return resources.ResourceConfig{
		Profile:             resources.ResourceProfile("mid"),
		ProcessMemoryBudget: gib,
		CPUCapacity:         2000,
		Storage: resources.StorageResourceConfig{
			MaxEntries:   500_000,
			MemoryBudget: 590_558_003,
			MaxKeySize:   512,
			MaxValueSize: 64 * 1024,
		},
		Idempotency: resources.IdempotencyResourceConfig{
			MaxEntries:   100_000,
			MemoryBudget: 161_061_273,
			Retention:    24 * time.Hour,
		},
		Network: resources.NetworkResourceConfig{
			MaxConnections:      1024,
			MaxInFlightRequests: 256,
			MaxFramePayloadSize: 256 * 1024,
		},
		Processing: resources.ProcessingResourceConfig{
			MemoryBudget: 161_061_273,
		},
		Runtime: resources.RuntimeResourceConfig{
			MemoryBudget: 161_061_275,
		},
	}
}

func resourceLoader(config resources.ResourceConfig) func() (resources.ResourceConfig, error) {
	return func() (resources.ResourceConfig, error) {
		return config, nil
	}
}

func TestLoadAppliesDefaultConfig(t *testing.T) {
	setDefaultEnvironment(t)

	expectedResources := validResourceConfig()

	got, err := loadWithResourceConfig(resourceLoader(expectedResources))
	if err != nil {
		t.Fatalf("loadWithResourceConfig() error = %v", err)
	}

	if got.Address != sharedconfig.DefaultServerAddress {
		t.Errorf(
			"Address = %q, want %q",
			got.Address,
			sharedconfig.DefaultServerAddress,
		)
	}

	if got.HealthAddress != defaultHealthAddress {
		t.Errorf(
			"HealthAddress = %q, want %q",
			got.HealthAddress,
			defaultHealthAddress,
		)
	}

	if got.ShutdownTimeout != defaultShutdownTimeout {
		t.Errorf(
			"ShutdownTimeout = %v, want %v",
			got.ShutdownTimeout,
			defaultShutdownTimeout,
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

	if got.Resources.ProcessMemoryBudget != expectedResources.ProcessMemoryBudget {
		t.Errorf(
			"Resources.ProcessMemoryBudget = %d, want %d",
			got.Resources.ProcessMemoryBudget,
			expectedResources.ProcessMemoryBudget,
		)
	}

	if got.Resources.CPUCapacity != expectedResources.CPUCapacity {
		t.Errorf(
			"Resources.CPUCapacity = %d, want %d",
			got.Resources.CPUCapacity,
			expectedResources.CPUCapacity,
		)
	}

	if got.Resources.Storage != expectedResources.Storage {
		t.Errorf(
			"Resources.Storage = %+v, want %+v",
			got.Resources.Storage,
			expectedResources.Storage,
		)
	}

	if got.Resources.Idempotency != expectedResources.Idempotency {
		t.Errorf(
			"Resources.Idempotency = %+v, want %+v",
			got.Resources.Idempotency,
			expectedResources.Idempotency,
		)
	}

	if got.Resources.Network != expectedResources.Network {
		t.Errorf(
			"Resources.Network = %+v, want %+v",
			got.Resources.Network,
			expectedResources.Network,
		)
	}
}

func TestLoadUsesEnvironmentConfig(t *testing.T) {
	setDefaultEnvironment(t)

	t.Setenv(sharedconfig.EnvServerAddress, "0.0.0.0:6381")
	t.Setenv(envHealthAddress, "127.0.0.1:8082")
	t.Setenv(envShutdownTimeout, "15s")
	t.Setenv(envReadTimeout, "45s")
	t.Setenv(envWriteTimeout, "20s")

	got, err := loadWithResourceConfig(resourceLoader(validResourceConfig()))
	if err != nil {
		t.Fatalf("loadWithResourceConfig() error = %v", err)
	}

	if got.Address != "0.0.0.0:6381" {
		t.Errorf("Address = %q, want %q", got.Address, "0.0.0.0:6381")
	}

	if got.HealthAddress != "127.0.0.1:8082" {
		t.Errorf(
			"HealthAddress = %q, want %q",
			got.HealthAddress,
			"127.0.0.1:8082",
		)
	}

	if got.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", got.ShutdownTimeout, 15*time.Second)
	}

	if got.ReadTimeout != 45*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", got.ReadTimeout, 45*time.Second)
	}

	if got.WriteTimeout != 20*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", got.WriteTimeout, 20*time.Second)
	}
}

func TestLoadRejectsInvalidDurations(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		value string
	}{
		{"shutdown invalid format", envShutdownTimeout, "invalid"},
		{"shutdown zero", envShutdownTimeout, "0s"},
		{"shutdown negative", envShutdownTimeout, "-5s"},
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

			_, err := loadWithResourceConfig(resourceLoader(validResourceConfig()))
			if err == nil {
				t.Fatal("loadWithResourceConfig() error = nil, want error")
			}
		})
	}
}

func TestLoadPropagatesResourceLoaderError(t *testing.T) {
	sentinel := errors.New("resource detection failure")

	_, err := loadWithResourceConfig(func() (resources.ResourceConfig, error) {
		return resources.ResourceConfig{}, sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want wrapped error %v", err, sentinel)
	}
}

func TestLoadRejectsInvalidResourceConfig(t *testing.T) {
	invalid := validResourceConfig()
	invalid.Storage.MaxEntries = 0

	_, err := loadWithResourceConfig(resourceLoader(invalid))
	if err == nil {
		t.Fatal("loadWithResourceConfig() error = nil, want validation error")
	}
}

func TestLoadRejectsNilResourceLoader(t *testing.T) {
	_, err := loadWithResourceConfig(nil)
	if err == nil {
		t.Fatal("loadWithResourceConfig(nil) error = nil, want error")
	}
}
