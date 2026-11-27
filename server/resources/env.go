package resources

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

const (
	envResourceProfile = "MEMORYDB_RESOURCE_PROFILE"
	envResourceCPU     = "MEMORYDB_RESOURCE_CPU"
	envResourceMemory  = "MEMORYDB_RESOURCE_MEMORY"

	envStorageMaxEntries   = "MEMORYDB_STORAGE_MAX_ENTRIES"
	envStorageMemoryBudget = "MEMORYDB_STORAGE_MEMORY_BUDGET"
	envStorageMaxKeySize   = "MEMORYDB_STORAGE_MAX_KEY_SIZE"
	envStorageMaxValueSize = "MEMORYDB_STORAGE_MAX_VALUE_SIZE"

	envIdempotencyMaxEntries   = "MEMORYDB_IDEMPOTENCY_MAX_ENTRIES"
	envIdempotencyCapacity     = "MEMORYDB_IDEMPOTENCY_CAPACITY"
	envIdempotencyMemoryBudget = "MEMORYDB_IDEMPOTENCY_MEMORY_BUDGET"
	envIdempotencyRetention    = "MEMORYDB_IDEMPOTENCY_RETENTION"

	envMaxInFlightRequests = "MEMORYDB_MAX_IN_FLIGHT_REQUESTS"
	envMaxFramePayloadSize = "MEMORYDB_MAX_FRAME_PAYLOAD_SIZE"

	envProcessingMemoryBudget = "MEMORYDB_PROCESSING_MEMORY_BUDGET"
	envRuntimeMemoryBudget    = "MEMORYDB_RUNTIME_MEMORY_BUDGET"
)

var ErrConflictingEnvironmentVariables = errors.New(
	"conflicting environment variables",
)

// LoadResourceConfigFromEnv loads resource options from environment
// variables, detects available resources, and returns a validated config.
func LoadResourceConfigFromEnv() (ResourceConfig, error) {
	detected, err := DetectResources()
	if err != nil {
		return ResourceConfig{}, fmt.Errorf(
			"%w: %w",
			ErrResourceDetection,
			err,
		)
	}

	return loadResourceConfigFromEnv(detected)
}

func loadResourceConfigFromEnv(
	detected DetectedResources,
) (ResourceConfig, error) {
	options, err := resourceOptionsFromEnv()
	if err != nil {
		return ResourceConfig{}, err
	}

	return loadResourceConfig(options, detected)
}

func resourceOptionsFromEnv() (ResourceOptions, error) {
	var options ResourceOptions

	profileValue, profileExists, err := optionalEnvironmentValue(envResourceProfile)
	if err != nil {
		return ResourceOptions{}, err
	}
	if profileExists {
		options.Profile = ResourceProfile(profileValue)
	}

	cpuValue, cpuExists, err := optionalEnvironmentValue(envResourceCPU)
	if err != nil {
		return ResourceOptions{}, err
	}
	if cpuExists {
		options.CPUCapacity, err = parseCPUCapacity(cpuValue)
		if err != nil {
			return ResourceOptions{}, fmt.Errorf(
				"invalid %s: %w",
				envResourceCPU,
				err,
			)
		}
	}

	memoryValue, memoryExists, err := optionalEnvironmentValue(envResourceMemory)
	if err != nil {
		return ResourceOptions{}, err
	}
	if memoryExists {
		options.ProcessMemoryBudget, err = parseMemorySize(memoryValue)
		if err != nil {
			return ResourceOptions{}, fmt.Errorf(
				"invalid %s: %w",
				envResourceMemory,
				err,
			)
		}
	}

	profile := options.Profile
	if profile == "" {
		profile = ResourceProfileMid
	}

	if profile == ResourceProfileCustom {
		if !cpuExists {
			return ResourceOptions{}, fmt.Errorf(
				"%w: %s is required for the custom profile",
				ErrInvalidResourceOptions,
				envResourceCPU,
			)
		}
		if !memoryExists {
			return ResourceOptions{}, fmt.Errorf(
				"%w: %s is required for the custom profile",
				ErrInvalidResourceOptions,
				envResourceMemory,
			)
		}
	}

	if options.Overrides.StorageMaxEntries, err = optionalInt64Override(envStorageMaxEntries); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.StorageMemoryBudget, err = optionalMemoryOverride(envStorageMemoryBudget); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.StorageMaxKeySize, err = optionalMemoryOverride(envStorageMaxKeySize); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.StorageMaxValueSize, err = optionalMemoryOverride(envStorageMaxValueSize); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.IdempotencyMaxEntries, err = optionalIdempotencyEntriesOverride(); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.IdempotencyMemoryBudget, err = optionalMemoryOverride(envIdempotencyMemoryBudget); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.IdempotencyRetention, err = optionalDurationOverride(envIdempotencyRetention); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.MaxConnections, err = optionalIntOverride(sharedconfig.EnvServerMaxConnections); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.MaxInFlightRequests, err = optionalIntOverride(envMaxInFlightRequests); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.MaxFramePayloadSize, err = optionalMemoryOverride(envMaxFramePayloadSize); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.ProcessingMemoryBudget, err = optionalMemoryOverride(envProcessingMemoryBudget); err != nil {
		return ResourceOptions{}, err
	}
	if options.Overrides.RuntimeMemoryBudget, err = optionalMemoryOverride(envRuntimeMemoryBudget); err != nil {
		return ResourceOptions{}, err
	}

	return options, nil
}

func optionalEnvironmentValue(name string) (string, bool, error) {
	value, exists := os.LookupEnv(name)
	if !exists {
		return "", false, nil
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, nil
	}

	return value, true, nil
}

func optionalInt64Override(name string) (*int64, error) {
	value, exists, err := optionalEnvironmentValue(name)
	if err != nil || !exists {
		return nil, err
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}

	return &parsed, nil
}

func optionalMemoryOverride(name string) (*int64, error) {
	value, exists, err := optionalEnvironmentValue(name)
	if err != nil || !exists {
		return nil, err
	}

	parsed, err := parseMemorySize(value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}

	return &parsed, nil
}

func optionalIntOverride(name string) (*int, error) {
	value, exists, err := optionalEnvironmentValue(name)
	if err != nil || !exists {
		return nil, err
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}

	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	if parsed > maxInt || parsed < minInt {
		return nil, fmt.Errorf("%s exceeds the platform int range", name)
	}

	result := int(parsed)
	return &result, nil
}

func optionalDurationOverride(name string) (*time.Duration, error) {
	value, exists, err := optionalEnvironmentValue(name)
	if err != nil || !exists {
		return nil, err
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}

	return &parsed, nil
}

// optionalIdempotencyEntriesOverride accepts the canonical MAX_ENTRIES name
// and the previous CAPACITY name for backward compatibility.
func optionalIdempotencyEntriesOverride() (*int64, error) {
	canonicalValue, canonicalExists, err := optionalEnvironmentValue(
		envIdempotencyMaxEntries,
	)
	if err != nil {
		return nil, err
	}

	legacyValue, legacyExists, err := optionalEnvironmentValue(
		envIdempotencyCapacity,
	)
	if err != nil {
		return nil, err
	}

	var canonical *int64
	if canonicalExists {
		parsed, parseErr := strconv.ParseInt(canonicalValue, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf(
				"invalid %s: %w",
				envIdempotencyMaxEntries,
				parseErr,
			)
		}
		canonical = &parsed
	}

	var legacy *int64
	if legacyExists {
		parsed, parseErr := strconv.ParseInt(legacyValue, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf(
				"invalid %s: %w",
				envIdempotencyCapacity,
				parseErr,
			)
		}
		legacy = &parsed
	}

	if canonical != nil && legacy != nil {
		if *canonical != *legacy {
			return nil, fmt.Errorf(
				"%w: %s and %s have different values",
				ErrConflictingEnvironmentVariables,
				envIdempotencyMaxEntries,
				envIdempotencyCapacity,
			)
		}
		return canonical, nil
	}

	if canonical != nil {
		return canonical, nil
	}

	return legacy, nil
}

// parseMemorySize parses a byte quantity.
// Supported suffixes: B, KB, MB, GB, TB, KiB, MiB, GiB and TiB.
// Values without a suffix are interpreted as bytes.
func parseMemorySize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("value must not be empty")
	}

	number, suffix := splitNumericSuffix(value)
	if number == "" {
		return 0, fmt.Errorf("missing numeric value")
	}

	multiplier, ok := memoryUnitMultiplier(suffix)
	if !ok {
		return 0, fmt.Errorf("unsupported memory unit %q", suffix)
	}

	quantity, ok := new(big.Rat).SetString(number)
	if !ok || quantity.Sign() <= 0 {
		return 0, fmt.Errorf("memory size must be a positive number")
	}

	quantity.Mul(quantity, new(big.Rat).SetInt64(multiplier))

	if !quantity.IsInt() {
		return 0, fmt.Errorf("memory size must resolve to an integer number of bytes")
	}

	result := quantity.Num()
	if !result.IsInt64() {
		return 0, fmt.Errorf("memory size exceeds int64 range")
	}

	return result.Int64(), nil
}

func memoryUnitMultiplier(suffix string) (int64, bool) {
	switch strings.ToUpper(suffix) {
	case "", "B":
		return 1, true
	case "KB":
		return 1000, true
	case "MB":
		return 1000 * 1000, true
	case "GB":
		return 1000 * 1000 * 1000, true
	case "TB":
		return 1000 * 1000 * 1000 * 1000, true
	case "KIB":
		return 1 << 10, true
	case "MIB":
		return 1 << 20, true
	case "GIB":
		return 1 << 30, true
	case "TIB":
		return 1 << 40, true
	default:
		return 0, false
	}
}

// parseCPUCapacity parses CPU quantities into millicores.
// An m suffix means millicores; values without a suffix are cores.
func parseCPUCapacity(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("value must not be empty")
	}

	if strings.HasSuffix(strings.ToLower(value), "m") {
		number := strings.TrimSpace(value[:len(value)-1])
		parsed, err := strconv.ParseInt(number, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid millicore quantity: %w", err)
		}
		if parsed <= 0 {
			return 0, fmt.Errorf("CPU capacity must be positive")
		}
		return parsed, nil
	}

	cores, ok := new(big.Rat).SetString(value)
	if !ok || cores.Sign() <= 0 {
		return 0, fmt.Errorf("CPU capacity must be a positive core quantity")
	}

	millicores := new(big.Rat).Mul(
		cores,
		new(big.Rat).SetInt64(1000),
	)
	if !millicores.IsInt() {
		return 0, fmt.Errorf("CPU quantity must resolve to whole millicores")
	}

	result := millicores.Num()
	if !result.IsInt64() {
		return 0, fmt.Errorf("CPU capacity exceeds int64 range")
	}

	return result.Int64(), nil
}

func splitNumericSuffix(value string) (string, string) {
	index := 0

	for index < len(value) {
		character := value[index]
		if (character >= '0' && character <= '9') ||
			character == '.' ||
			character == '+' ||
			character == '-' {
			index++
			continue
		}
		break
	}

	return value[:index], strings.TrimSpace(value[index:])
}
