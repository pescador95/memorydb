package resources

import (
	"errors"
	"testing"
)

func TestPublicLoadResourceConfigFromEnv(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envResourceProfile, string(ResourceProfileAuto))

	config, err := LoadResourceConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadResourceConfigFromEnv() error = %v", err)
	}

	if config.Profile != ResourceProfileAuto {
		t.Errorf(
			"Profile = %q, want %q",
			config.Profile,
			ResourceProfileAuto,
		)
	}

	if config.CPUCapacity <= 0 {
		t.Errorf("CPUCapacity = %d, want positive value", config.CPUCapacity)
	}

	if config.ProcessMemoryBudget <= 0 {
		t.Errorf(
			"ProcessMemoryBudget = %d, want positive value",
			config.ProcessMemoryBudget,
		)
	}

	if err := config.Validate(); err != nil {
		t.Errorf("config.Validate() error = %v", err)
	}
}

func TestPublicLoadResourceConfigFromEnvRejectsInvalidProfile(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envResourceProfile, "invalid")

	_, err := LoadResourceConfigFromEnv()
	if !errors.Is(err, ErrInvalidResourceOptions) {
		t.Fatalf(
			"LoadResourceConfigFromEnv() error = %v, want ErrInvalidResourceOptions",
			err,
		)
	}
}
