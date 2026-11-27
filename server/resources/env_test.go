package resources

import (
	"errors"
	"testing"
	"time"

	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

var resourceEnvironmentVariables = []string{
	envResourceProfile,
	envResourceCPU,
	envResourceMemory,
	envStorageMaxEntries,
	envStorageMemoryBudget,
	envStorageMaxKeySize,
	envStorageMaxValueSize,
	envIdempotencyMaxEntries,
	envIdempotencyCapacity,
	envIdempotencyMemoryBudget,
	envIdempotencyRetention,
	sharedconfig.EnvServerMaxConnections,
	envMaxInFlightRequests,
	envMaxFramePayloadSize,
	envProcessingMemoryBudget,
	envRuntimeMemoryBudget,
}

func clearResourceEnvironment(t *testing.T) {
	t.Helper()

	for _, name := range resourceEnvironmentVariables {
		t.Setenv(name, "")
	}
}

func TestParseMemorySize(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{"bytes without suffix", "1024", 1024},
		{"bytes suffix", "1024B", 1024},
		{"kilobytes decimal", "2KB", 2000},
		{"megabytes decimal", "2MB", 2_000_000},
		{"gigabytes decimal", "2GB", 2_000_000_000},
		{"kibibytes", "64KiB", 64 << 10},
		{"mebibytes", "512MiB", 512 << 20},
		{"gibibytes", "2GiB", 2 << 30},
		{"tebibytes", "1TiB", 1 << 40},
		{"fractional bytes resolving to integer", "1.5KiB", 1536},
		{"case insensitive suffix", "2gib", 2 << 30},
		{"surrounding spaces", " 64MiB ", 64 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemorySize(tt.value)
			if err != nil {
				t.Fatalf("parseMemorySize(%q): %v", tt.value, err)
			}

			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseMemorySizeRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"zero", "0"},
		{"negative", "-1GiB"},
		{"invalid number", "abcGiB"},
		{"unsupported unit", "2PiB"},
		{"fractional byte", "0.1B"},
		{"overflow", "9223372036854775808B"},
		{"malformed suffix", "2GiBB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseMemorySize(tt.value); err == nil {
				t.Fatalf("parseMemorySize(%q) expected error", tt.value)
			}
		})
	}
}

func TestParseCPUCapacity(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{"millicores", "2000m", 2000},
		{"one core", "1", 1000},
		{"two cores", "2", 2000},
		{"half core", "0.5", 500},
		{"fractional cores", "1.5", 1500},
		{"quarter core", "0.25", 250},
		{"surrounding spaces", " 1500m ", 1500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCPUCapacity(tt.value)
			if err != nil {
				t.Fatalf("parseCPUCapacity(%q): %v", tt.value, err)
			}

			if got != tt.want {
				t.Errorf("got %d millicores, want %d", got, tt.want)
			}
		})
	}
}

func TestParseCPUCapacityRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"zero cores", "0"},
		{"negative cores", "-1"},
		{"zero millicores", "0m"},
		{"negative millicores", "-100m"},
		{"invalid suffix", "2cores"},
		{"fractional millicores", "1.5m"},
		{"less than one millicore", "0.0001"},
		{"overflow", "9223372036854775808m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseCPUCapacity(tt.value); err == nil {
				t.Fatalf("parseCPUCapacity(%q) expected error", tt.value)
			}
		})
	}
}

func TestLoadResourceConfigFromEnvDefaultsToMid(t *testing.T) {
	clearResourceEnvironment(t)

	config, err := loadResourceConfigFromEnv(validDetectedResources())
	if err != nil {
		t.Fatalf("loadResourceConfigFromEnv: %v", err)
	}

	if config.Profile != ResourceProfileMid {
		t.Errorf("profile = %q, want %q", config.Profile, ResourceProfileMid)
	}

	if config.ProcessMemoryBudget != 1<<30 {
		t.Errorf(
			"process memory budget = %d, want %d",
			config.ProcessMemoryBudget,
			int64(1<<30),
		)
	}

	if config.Network.MaxConnections != 1024 {
		t.Errorf("max connections = %d, want 1024", config.Network.MaxConnections)
	}
}

func TestLoadResourceConfigFromEnvPreservesDefaultsForEmptyValues(t *testing.T) {
	clearResourceEnvironment(t)

	t.Setenv(envResourceProfile, " ")
	t.Setenv(envStorageMaxEntries, "")
	t.Setenv(envIdempotencyRetention, " ")

	config, err := loadResourceConfigFromEnv(validDetectedResources())
	if err != nil {
		t.Fatalf("loadResourceConfigFromEnv: %v", err)
	}

	if config.Profile != ResourceProfileMid {
		t.Errorf("profile = %q, want %q", config.Profile, ResourceProfileMid)
	}

	if config.Storage.MaxEntries != 500_000 {
		t.Errorf("storage max entries = %d, want 500000", config.Storage.MaxEntries)
	}

	if config.Idempotency.Retention != 24*time.Hour {
		t.Errorf(
			"idempotency retention = %s, want %s",
			config.Idempotency.Retention,
			24*time.Hour,
		)
	}
}

func TestLoadResourceConfigFromEnvAuto(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envResourceProfile, string(ResourceProfileAuto))

	detected := validDetectedResources()

	config, err := loadResourceConfigFromEnv(detected)
	if err != nil {
		t.Fatalf("loadResourceConfigFromEnv: %v", err)
	}

	if config.Profile != ResourceProfileAuto {
		t.Errorf("profile = %q, want %q", config.Profile, ResourceProfileAuto)
	}

	if config.CPUCapacity != detected.CPUCapacity {
		t.Errorf("CPU capacity = %d, want %d", config.CPUCapacity, detected.CPUCapacity)
	}

	if config.ProcessMemoryBudget != detected.MemoryCapacity {
		t.Errorf(
			"process memory budget = %d, want %d",
			config.ProcessMemoryBudget,
			detected.MemoryCapacity,
		)
	}
}

func TestLoadResourceConfigFromEnvCustom(t *testing.T) {
	clearResourceEnvironment(t)

	t.Setenv(envResourceProfile, string(ResourceProfileCustom))
	t.Setenv(envResourceCPU, "1.5")
	t.Setenv(envResourceMemory, "512MiB")

	config, err := loadResourceConfigFromEnv(validDetectedResources())
	if err != nil {
		t.Fatalf("loadResourceConfigFromEnv: %v", err)
	}

	if config.Profile != ResourceProfileCustom {
		t.Errorf("profile = %q, want %q", config.Profile, ResourceProfileCustom)
	}

	if config.CPUCapacity != 1500 {
		t.Errorf("CPU capacity = %d, want 1500", config.CPUCapacity)
	}

	if config.ProcessMemoryBudget != 512<<20 {
		t.Errorf(
			"process memory budget = %d, want %d",
			config.ProcessMemoryBudget,
			int64(512<<20),
		)
	}
}

func TestLoadResourceConfigFromEnvCustomRequiresCPUAndMemory(t *testing.T) {
	tests := []struct {
		name      string
		setCPU    bool
		setMemory bool
	}{
		{"missing CPU", false, true},
		{"missing memory", true, false},
		{"missing both", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearResourceEnvironment(t)
			t.Setenv(envResourceProfile, string(ResourceProfileCustom))

			if tt.setCPU {
				t.Setenv(envResourceCPU, "1000m")
			}
			if tt.setMemory {
				t.Setenv(envResourceMemory, "512MiB")
			}

			_, err := loadResourceConfigFromEnv(validDetectedResources())
			if !errors.Is(err, ErrInvalidResourceOptions) {
				t.Fatalf("expected ErrInvalidResourceOptions, got %v", err)
			}
		})
	}
}

func TestLoadResourceConfigFromEnvAppliesOverrides(t *testing.T) {
	clearResourceEnvironment(t)

	t.Setenv(envResourceProfile, string(ResourceProfileMid))
	t.Setenv(envStorageMaxEntries, "250000")
	t.Setenv(envStorageMemoryBudget, "550MiB")
	t.Setenv(envStorageMaxKeySize, "1KiB")
	t.Setenv(envStorageMaxValueSize, "32KiB")
	t.Setenv(envIdempotencyMaxEntries, "50000")
	t.Setenv(envIdempotencyMemoryBudget, "150MiB")
	t.Setenv(envIdempotencyRetention, "12h")
	t.Setenv(sharedconfig.EnvServerMaxConnections, "512")
	t.Setenv(envMaxInFlightRequests, "128")
	t.Setenv(envMaxFramePayloadSize, "128KiB")
	t.Setenv(envProcessingMemoryBudget, "150MiB")
	t.Setenv(envRuntimeMemoryBudget, "174MiB")

	config, err := loadResourceConfigFromEnv(validDetectedResources())
	if err != nil {
		t.Fatalf("loadResourceConfigFromEnv: %v", err)
	}

	if config.Storage.MaxEntries != 250_000 {
		t.Errorf("storage max entries = %d, want 250000", config.Storage.MaxEntries)
	}
	if config.Storage.MemoryBudget != 550<<20 {
		t.Errorf("storage memory budget = %d, want %d", config.Storage.MemoryBudget, int64(550<<20))
	}
	if config.Storage.MaxKeySize != 1<<10 {
		t.Errorf("storage max key size = %d, want 1024", config.Storage.MaxKeySize)
	}
	if config.Storage.MaxValueSize != 32<<10 {
		t.Errorf("storage max value size = %d, want %d", config.Storage.MaxValueSize, int64(32<<10))
	}
	if config.Idempotency.MaxEntries != 50_000 {
		t.Errorf("idempotency max entries = %d, want 50000", config.Idempotency.MaxEntries)
	}
	if config.Idempotency.MemoryBudget != 150<<20 {
		t.Errorf("idempotency memory budget = %d, want %d", config.Idempotency.MemoryBudget, int64(150<<20))
	}
	if config.Idempotency.Retention != 12*time.Hour {
		t.Errorf("idempotency retention = %s, want 12h", config.Idempotency.Retention)
	}
	if config.Network.MaxConnections != 512 {
		t.Errorf("max connections = %d, want 512", config.Network.MaxConnections)
	}
	if config.Network.MaxInFlightRequests != 128 {
		t.Errorf("max in-flight requests = %d, want 128", config.Network.MaxInFlightRequests)
	}
	if config.Network.MaxFramePayloadSize != 128<<10 {
		t.Errorf("max frame payload size = %d, want %d", config.Network.MaxFramePayloadSize, int64(128<<10))
	}
	if config.Processing.MemoryBudget != 150<<20 {
		t.Errorf("processing memory budget = %d, want %d", config.Processing.MemoryBudget, int64(150<<20))
	}
	if config.Runtime.MemoryBudget != 174<<20 {
		t.Errorf("runtime memory budget = %d, want %d", config.Runtime.MemoryBudget, int64(174<<20))
	}
}

func TestLoadResourceConfigFromEnvSupportsLegacyIdempotencyCapacity(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envIdempotencyCapacity, "75000")

	config, err := loadResourceConfigFromEnv(validDetectedResources())
	if err != nil {
		t.Fatalf("loadResourceConfigFromEnv: %v", err)
	}

	if config.Idempotency.MaxEntries != 75_000 {
		t.Errorf("idempotency max entries = %d, want 75000", config.Idempotency.MaxEntries)
	}
}

func TestLoadResourceConfigFromEnvAcceptsMatchingIdempotencyAliases(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envIdempotencyMaxEntries, "75000")
	t.Setenv(envIdempotencyCapacity, "75000")

	config, err := loadResourceConfigFromEnv(validDetectedResources())
	if err != nil {
		t.Fatalf("loadResourceConfigFromEnv: %v", err)
	}

	if config.Idempotency.MaxEntries != 75_000 {
		t.Errorf("idempotency max entries = %d, want 75000", config.Idempotency.MaxEntries)
	}
}

func TestLoadResourceConfigFromEnvRejectsConflictingIdempotencyAliases(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envIdempotencyMaxEntries, "75000")
	t.Setenv(envIdempotencyCapacity, "100000")

	_, err := loadResourceConfigFromEnv(validDetectedResources())
	if !errors.Is(err, ErrConflictingEnvironmentVariables) {
		t.Fatalf(
			"expected ErrConflictingEnvironmentVariables, got %v",
			err,
		)
	}
}

func TestLoadResourceConfigFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		value string
	}{
		{"invalid profile", envResourceProfile, "unknown"},
		{"invalid CPU", envResourceCPU, "fast"},
		{"zero CPU", envResourceCPU, "0"},
		{"invalid memory", envResourceMemory, "many"},
		{"zero memory", envResourceMemory, "0GiB"},
		{"invalid storage entries", envStorageMaxEntries, "many"},
		{"invalid storage budget", envStorageMemoryBudget, "5XB"},
		{"invalid key size", envStorageMaxKeySize, "invalid"},
		{"invalid value size", envStorageMaxValueSize, "invalid"},
		{"invalid idempotency entries", envIdempotencyMaxEntries, "many"},
		{"invalid idempotency retention", envIdempotencyRetention, "forever"},
		{"invalid max connections", sharedconfig.EnvServerMaxConnections, "many"},
		{"invalid in-flight requests", envMaxInFlightRequests, "many"},
		{"invalid frame size", envMaxFramePayloadSize, "invalid"},
		{"invalid processing budget", envProcessingMemoryBudget, "invalid"},
		{"invalid runtime budget", envRuntimeMemoryBudget, "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearResourceEnvironment(t)
			t.Setenv(tt.env, tt.value)

			_, err := loadResourceConfigFromEnv(validDetectedResources())
			if err == nil {
				t.Fatalf("expected error for %s=%q", tt.env, tt.value)
			}
		})
	}
}

func TestLoadResourceConfigFromEnvRejectsOutOfRangeInt(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(sharedconfig.EnvServerMaxConnections, "999999999999999999999999999")

	_, err := loadResourceConfigFromEnv(validDetectedResources())
	if err == nil {
		t.Fatal("expected error for max connections outside int64 range")
	}
}

func TestLoadResourceConfigFromEnvRejectsMemoryBudgetMismatch(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envStorageMemoryBudget, "600MiB")

	_, err := loadResourceConfigFromEnv(validDetectedResources())
	if !errors.Is(err, ErrInvalidResourceConfig) {
		t.Fatalf("expected ErrInvalidResourceConfig, got %v", err)
	}
}

func TestLoadResourceConfigFromEnvRejectsCPUExceedingDetectedCapacity(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envResourceProfile, string(ResourceProfileCustom))
	t.Setenv(envResourceCPU, "9000m")
	t.Setenv(envResourceMemory, "512MiB")

	_, err := loadResourceConfigFromEnv(validDetectedResources())
	if !errors.Is(err, ErrInsufficientResources) {
		t.Fatalf("expected ErrInsufficientResources, got %v", err)
	}
}

func TestLoadResourceConfigFromEnvRejectsMemoryExceedingDetectedCapacity(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envResourceProfile, string(ResourceProfileCustom))
	t.Setenv(envResourceCPU, "1000m")
	t.Setenv(envResourceMemory, "16GiB")

	_, err := loadResourceConfigFromEnv(validDetectedResources())
	if !errors.Is(err, ErrInsufficientResources) {
		t.Fatalf("expected ErrInsufficientResources, got %v", err)
	}
}

func TestParseMemorySizeRejectsMaximumOverflow(t *testing.T) {
	_, err := parseMemorySize("9223372036854775808B")
	if err == nil {
		t.Fatal("expected int64 overflow error")
	}

	_, err = parseMemorySize("1TiB")
	if err != nil {
		t.Fatalf("1TiB should be valid: %v", err)
	}
}
