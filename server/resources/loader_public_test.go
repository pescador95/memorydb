package resources

import (
	"errors"
	"testing"
)

func TestPublicLoadResourceConfig(t *testing.T) {
	config, err := LoadResourceConfig(ResourceOptions{
		Profile: ResourceProfileAuto,
	})
	if err != nil {
		t.Fatalf("LoadResourceConfig() error = %v", err)
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

func TestPublicLoadResourceConfigRejectsInvalidOptions(t *testing.T) {
	_, err := LoadResourceConfig(ResourceOptions{
		Profile: ResourceProfileCustom,
	})
	if !errors.Is(err, ErrInvalidResourceOptions) {
		t.Fatalf(
			"LoadResourceConfig() error = %v, want ErrInvalidResourceOptions",
			err,
		)
	}
}
