package resources

import (
	"fmt"
	"math/big"
)

const (
	storageMemoryPercentage     int64 = 55
	idempotencyMemoryPercentage int64 = 15
	processingMemoryPercentage  int64 = 15
)

func percentageOf(total, percentage int64) int64 {
	if total <= 0 || percentage <= 0 {
		return 0
	}

	if percentage >= 100 {
		return total
	}

	return total/100*percentage +
		(total%100)*percentage/100
}

// calculateResourceConfig preserves the original internal entry point.
// The auto and custom profiles use the same sizing algorithm.
func calculateResourceConfig(
	cpuCapacity int64,
	processMemoryBudget int64,
) (ResourceConfig, error) {
	return calculateResourceConfigForProfile(
		ResourceProfileAuto,
		cpuCapacity,
		processMemoryBudget,
	)
}

// calculateResourceConfigForProfile applies the common sizing algorithm
// used by the auto and custom profiles.
func calculateResourceConfigForProfile(
	profile ResourceProfile,
	cpuCapacity int64,
	processMemoryBudget int64,
) (ResourceConfig, error) {
	if profile != ResourceProfileAuto && profile != ResourceProfileCustom {
		return ResourceConfig{}, fmt.Errorf(
			"unsupported calculated resource profile %q",
			profile,
		)
	}

	if cpuCapacity <= 0 {
		return ResourceConfig{}, fmt.Errorf("CPU capacity must be positive")
	}

	if processMemoryBudget <= 0 {
		return ResourceConfig{}, fmt.Errorf("process memory budget must be positive")
	}

	base, exists := defaultResourceProfiles[ResourceProfileMid]
	if !exists {
		return ResourceConfig{}, fmt.Errorf("mid resource profile is not defined")
	}

	storageBudget := percentageOf(
		processMemoryBudget,
		storageMemoryPercentage,
	)

	idempotencyBudget := percentageOf(
		processMemoryBudget,
		idempotencyMemoryPercentage,
	)

	processingBudget := percentageOf(
		processMemoryBudget,
		processingMemoryPercentage,
	)

	allocated := storageBudget +
		idempotencyBudget +
		processingBudget

	if allocated > processMemoryBudget {
		return ResourceConfig{}, fmt.Errorf(
			"component memory budgets exceed process budget",
		)
	}

	runtimeBudget := processMemoryBudget - allocated

	storageEntries, err := scaleEntries(
		base.Storage.MaxEntries,
		storageBudget,
		base.Storage.MemoryBudget,
	)
	if err != nil {
		return ResourceConfig{}, fmt.Errorf(
			"calculate storage max entries: %w",
			err,
		)
	}

	idempotencyEntries, err := scaleEntries(
		base.Idempotency.MaxEntries,
		idempotencyBudget,
		base.Idempotency.MemoryBudget,
	)
	if err != nil {
		return ResourceConfig{}, fmt.Errorf(
			"calculate idempotency max entries: %w",
			err,
		)
	}

	result := ResourceConfig{
		Profile:             profile,
		ProcessMemoryBudget: processMemoryBudget,
		CPUCapacity:         cpuCapacity,

		Storage: StorageResourceConfig{
			MaxEntries:   storageEntries,
			MemoryBudget: storageBudget,
			MaxKeySize:   base.Storage.MaxKeySize,
			MaxValueSize: base.Storage.MaxValueSize,
		},

		Idempotency: IdempotencyResourceConfig{
			MaxEntries:   idempotencyEntries,
			MemoryBudget: idempotencyBudget,
			Retention:    base.Idempotency.Retention,
		},

		Network: base.Network,

		Processing: ProcessingResourceConfig{
			MemoryBudget: processingBudget,
		},

		Runtime: RuntimeResourceConfig{
			MemoryBudget: runtimeBudget,
		},
	}

	if err := result.Validate(); err != nil {
		return ResourceConfig{}, fmt.Errorf(
			"validate calculated resource config: %w",
			err,
		)
	}

	return result, nil
}

// scaleEntries scales an entry limit proportionally to its memory budget.
// big.Int prevents intermediate multiplication overflow.
func scaleEntries(
	baseEntries int64,
	targetBudget int64,
	referenceBudget int64,
) (int64, error) {
	if baseEntries <= 0 || targetBudget <= 0 || referenceBudget <= 0 {
		return 0, fmt.Errorf(
			"entry count and memory budgets must be positive",
		)
	}

	numerator := new(big.Int).Mul(
		big.NewInt(baseEntries),
		big.NewInt(targetBudget),
	)

	result := numerator.Div(
		numerator,
		big.NewInt(referenceBudget),
	)

	if !result.IsInt64() {
		return 0, fmt.Errorf("scaled entry count exceeds int64 range")
	}

	entries := result.Int64()
	if entries <= 0 {
		return 0, fmt.Errorf("memory budget is too small for any entries")
	}

	return entries, nil
}
