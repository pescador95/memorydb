package resources

import (
	"errors"
	"testing"
	"time"
)

func int64Pointer(value int64) *int64 {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func durationPointer(value time.Duration) *time.Duration {
	return &value
}

func validDetectedResources() DetectedResources {
	return DetectedResources{
		CPUCapacity:    8000,
		MemoryCapacity: 8 << 30,
		CPUSource:      ResourceSourceHost,
		MemorySource:   ResourceSourceHost,
	}
}

func TestLoadResourceConfigDefaultsToMid(t *testing.T) {
	config, err := loadResourceConfig(
		ResourceOptions{},
		validDetectedResources(),
	)
	if err != nil {
		t.Fatalf("load resource config: %v", err)
	}

	if config.Profile != ResourceProfileMid {
		t.Fatalf("expected mid profile, got %q", config.Profile)
	}

	if err := config.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestLoadResourceConfigPresets(t *testing.T) {
	tests := []struct {
		name    string
		profile ResourceProfile
		cpu     int64
		memory  int64
	}{
		{
			name:    "low",
			profile: ResourceProfileLow,
			cpu:     1000,
			memory:  256 << 20,
		},
		{
			name:    "mid",
			profile: ResourceProfileMid,
			cpu:     2000,
			memory:  1 << 30,
		},
		{
			name:    "high",
			profile: ResourceProfileHigh,
			cpu:     4000,
			memory:  4 << 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := loadResourceConfig(
				ResourceOptions{Profile: tt.profile},
				validDetectedResources(),
			)
			if err != nil {
				t.Fatalf("load resource config: %v", err)
			}

			if config.Profile != tt.profile {
				t.Fatalf(
					"expected profile %q, got %q",
					tt.profile,
					config.Profile,
				)
			}

			if config.CPUCapacity != tt.cpu {
				t.Errorf(
					"expected CPU capacity %d, got %d",
					tt.cpu,
					config.CPUCapacity,
				)
			}

			if config.ProcessMemoryBudget != tt.memory {
				t.Errorf(
					"expected memory budget %d, got %d",
					tt.memory,
					config.ProcessMemoryBudget,
				)
			}
		})
	}
}

func TestLoadResourceConfigAuto(t *testing.T) {
	detected := validDetectedResources()

	config, err := loadResourceConfig(
		ResourceOptions{Profile: ResourceProfileAuto},
		detected,
	)
	if err != nil {
		t.Fatalf("load auto resource config: %v", err)
	}

	if config.Profile != ResourceProfileAuto {
		t.Fatalf("expected auto profile, got %q", config.Profile)
	}

	if config.CPUCapacity != detected.CPUCapacity {
		t.Errorf(
			"expected detected CPU capacity %d, got %d",
			detected.CPUCapacity,
			config.CPUCapacity,
		)
	}

	if config.ProcessMemoryBudget != detected.MemoryCapacity {
		t.Errorf(
			"expected detected memory capacity %d, got %d",
			detected.MemoryCapacity,
			config.ProcessMemoryBudget,
		)
	}
}

func TestLoadResourceConfigCustom(t *testing.T) {
	options := ResourceOptions{
		Profile:             ResourceProfileCustom,
		CPUCapacity:         1500,
		ProcessMemoryBudget: 512 << 20,
	}

	config, err := loadResourceConfig(options, validDetectedResources())
	if err != nil {
		t.Fatalf("load custom resource config: %v", err)
	}

	if config.Profile != ResourceProfileCustom {
		t.Fatalf("expected custom profile, got %q", config.Profile)
	}

	if config.CPUCapacity != options.CPUCapacity {
		t.Errorf(
			"expected CPU capacity %d, got %d",
			options.CPUCapacity,
			config.CPUCapacity,
		)
	}

	if config.ProcessMemoryBudget != options.ProcessMemoryBudget {
		t.Errorf(
			"expected memory budget %d, got %d",
			options.ProcessMemoryBudget,
			config.ProcessMemoryBudget,
		)
	}
}

func TestLoadResourceConfigRejectsInsufficientPresetResources(t *testing.T) {
	tests := []struct {
		name     string
		profile  ResourceProfile
		detected DetectedResources
	}{
		{
			name:    "insufficient CPU",
			profile: ResourceProfileMid,
			detected: DetectedResources{
				CPUCapacity:    1000,
				MemoryCapacity: 8 << 30,
				CPUSource:      ResourceSourceContainer,
				MemorySource:   ResourceSourceHost,
			},
		},
		{
			name:    "insufficient memory",
			profile: ResourceProfileMid,
			detected: DetectedResources{
				CPUCapacity:    8000,
				MemoryCapacity: 512 << 20,
				CPUSource:      ResourceSourceHost,
				MemorySource:   ResourceSourceContainer,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadResourceConfig(
				ResourceOptions{Profile: tt.profile},
				tt.detected,
			)

			if !errors.Is(err, ErrInsufficientResources) {
				t.Fatalf(
					"expected ErrInsufficientResources, got %v",
					err,
				)
			}
		})
	}
}

func TestLoadResourceConfigRejectsInsufficientCustomResources(t *testing.T) {
	tests := []struct {
		name    string
		options ResourceOptions
	}{
		{
			name: "CPU exceeds detected capacity",
			options: ResourceOptions{
				Profile:             ResourceProfileCustom,
				CPUCapacity:         9000,
				ProcessMemoryBudget: 512 << 20,
			},
		},
		{
			name: "memory exceeds detected capacity",
			options: ResourceOptions{
				Profile:             ResourceProfileCustom,
				CPUCapacity:         1000,
				ProcessMemoryBudget: 16 << 30,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadResourceConfig(
				tt.options,
				validDetectedResources(),
			)

			if !errors.Is(err, ErrInsufficientResources) {
				t.Fatalf(
					"expected ErrInsufficientResources, got %v",
					err,
				)
			}
		})
	}
}

func TestLoadResourceConfigCustomRequiresExplicitResources(t *testing.T) {
	tests := []struct {
		name    string
		options ResourceOptions
	}{
		{
			name: "missing CPU",
			options: ResourceOptions{
				Profile:             ResourceProfileCustom,
				ProcessMemoryBudget: 512 << 20,
			},
		},
		{
			name: "missing memory",
			options: ResourceOptions{
				Profile:     ResourceProfileCustom,
				CPUCapacity: 1000,
			},
		},
		{
			name: "both missing",
			options: ResourceOptions{
				Profile: ResourceProfileCustom,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadResourceConfig(
				tt.options,
				validDetectedResources(),
			)

			if !errors.Is(err, ErrInvalidResourceOptions) {
				t.Fatalf(
					"expected ErrInvalidResourceOptions, got %v",
					err,
				)
			}
		})
	}
}

func TestLoadResourceConfigRejectsExplicitResourcesForNonCustomProfiles(t *testing.T) {
	tests := []ResourceProfile{
		ResourceProfileLow,
		ResourceProfileMid,
		ResourceProfileHigh,
		ResourceProfileAuto,
	}

	for _, profile := range tests {
		t.Run(string(profile), func(t *testing.T) {
			_, err := loadResourceConfig(
				ResourceOptions{
					Profile:             profile,
					CPUCapacity:         1000,
					ProcessMemoryBudget: 256 << 20,
				},
				validDetectedResources(),
			)

			if !errors.Is(err, ErrInvalidResourceOptions) {
				t.Fatalf(
					"expected ErrInvalidResourceOptions, got %v",
					err,
				)
			}
		})
	}
}

func TestLoadResourceConfigAppliesOverrides(t *testing.T) {
	retention := 12 * time.Hour

	options := ResourceOptions{
		Profile: ResourceProfileMid,
		Overrides: ResourceOverrides{
			StorageMaxEntries:     int64Pointer(250_000),
			StorageMaxKeySize:     int64Pointer(1024),
			StorageMaxValueSize:   int64Pointer(32 << 10),
			IdempotencyMaxEntries: int64Pointer(50_000),
			IdempotencyRetention:  durationPointer(retention),
			MaxConnections:        intPointer(512),
			MaxInFlightRequests:   intPointer(128),
			MaxFramePayloadSize:   int64Pointer(128 << 10),
		},
	}

	config, err := loadResourceConfig(options, validDetectedResources())
	if err != nil {
		t.Fatalf("load resource config with overrides: %v", err)
	}

	if config.Storage.MaxEntries != 250_000 {
		t.Errorf("unexpected storage max entries: %d", config.Storage.MaxEntries)
	}

	if config.Storage.MaxKeySize != 1024 {
		t.Errorf("unexpected storage max key size: %d", config.Storage.MaxKeySize)
	}

	if config.Storage.MaxValueSize != 32<<10 {
		t.Errorf("unexpected storage max value size: %d", config.Storage.MaxValueSize)
	}

	if config.Idempotency.MaxEntries != 50_000 {
		t.Errorf(
			"unexpected idempotency max entries: %d",
			config.Idempotency.MaxEntries,
		)
	}

	if config.Idempotency.Retention != retention {
		t.Errorf(
			"unexpected idempotency retention: %s",
			config.Idempotency.Retention,
		)
	}

	if config.Network.MaxConnections != 512 {
		t.Errorf("unexpected max connections: %d", config.Network.MaxConnections)
	}

	if config.Network.MaxInFlightRequests != 128 {
		t.Errorf(
			"unexpected max in-flight requests: %d",
			config.Network.MaxInFlightRequests,
		)
	}

	if config.Network.MaxFramePayloadSize != 128<<10 {
		t.Errorf(
			"unexpected max frame payload size: %d",
			config.Network.MaxFramePayloadSize,
		)
	}
}

func TestLoadResourceConfigRejectsInconsistentMemoryOverrides(t *testing.T) {
	options := ResourceOptions{
		Profile: ResourceProfileMid,
		Overrides: ResourceOverrides{
			StorageMemoryBudget: int64Pointer(600 << 20),
		},
	}

	_, err := loadResourceConfig(options, validDetectedResources())
	if !errors.Is(err, ErrInvalidResourceConfig) {
		t.Fatalf("expected ErrInvalidResourceConfig, got %v", err)
	}
}

func TestLoadResourceConfigRejectsOverrideExceedingDetectedCapacity(t *testing.T) {
	options := ResourceOptions{
		Profile: ResourceProfileLow,
		Overrides: ResourceOverrides{
			CPUCapacity: int64Pointer(9000),
		},
	}

	_, err := loadResourceConfig(options, validDetectedResources())
	if !errors.Is(err, ErrInsufficientResources) {
		t.Fatalf("expected ErrInsufficientResources, got %v", err)
	}
}

func TestLoadResourceConfigRejectsInvalidOverrides(t *testing.T) {
	tests := []struct {
		name      string
		overrides ResourceOverrides
	}{
		{
			name: "zero storage entries",
			overrides: ResourceOverrides{
				StorageMaxEntries: int64Pointer(0),
			},
		},
		{
			name: "negative retention",
			overrides: ResourceOverrides{
				IdempotencyRetention: durationPointer(-time.Second),
			},
		},
		{
			name: "zero connections",
			overrides: ResourceOverrides{
				MaxConnections: intPointer(0),
			},
		},
		{
			name: "frame payload smaller than value size",
			overrides: ResourceOverrides{
				MaxFramePayloadSize: int64Pointer(1024),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadResourceConfig(
				ResourceOptions{
					Profile:   ResourceProfileMid,
					Overrides: tt.overrides,
				},
				validDetectedResources(),
			)

			if !errors.Is(err, ErrInvalidResourceConfig) {
				t.Fatalf(
					"expected ErrInvalidResourceConfig, got %v",
					err,
				)
			}
		})
	}
}

func TestLoadResourceConfigRejectsInvalidDetectedResources(t *testing.T) {
	detected := validDetectedResources()
	detected.CPUSource = "unknown"

	_, err := loadResourceConfig(
		ResourceOptions{Profile: ResourceProfileMid},
		detected,
	)

	if !errors.Is(err, ErrInvalidResources) {
		t.Fatalf("expected ErrInvalidResources, got %v", err)
	}
}

func TestLoadResourceConfigRejectsUnknownProfile(t *testing.T) {
	_, err := loadResourceConfig(
		ResourceOptions{Profile: ResourceProfile("unknown")},
		validDetectedResources(),
	)

	if !errors.Is(err, ErrInvalidResourceOptions) {
		t.Fatalf("expected ErrInvalidResourceOptions, got %v", err)
	}
}
