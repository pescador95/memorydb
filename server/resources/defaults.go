package resources

import "time"

var defaultResourceProfiles = buildDefaultResourceProfiles()

func buildDefaultResourceProfiles() map[ResourceProfile]ResourceConfig {
	return map[ResourceProfile]ResourceConfig{
		ResourceProfileLow: buildPreset(
			ResourceProfileLow,
			256<<20,
			1000,
			100_000,
			10_000,
			256,
			16<<10,
			256,
			64,
			64<<10,
		),

		ResourceProfileMid: buildPreset(
			ResourceProfileMid,
			1<<30,
			2000,
			500_000,
			100_000,
			512,
			64<<10,
			1024,
			256,
			256<<10,
		),

		ResourceProfileHigh: buildPreset(
			ResourceProfileHigh,
			4<<30,
			4000,
			2_000_000,
			500_000,
			1024,
			256<<10,
			4096,
			1024,
			1<<20,
		),
	}
}

func buildPreset(
	profile ResourceProfile,
	processMemoryBudget int64,
	cpuCapacity int64,
	storageEntries int64,
	idempotencyEntries int64,
	maxKeySize int64,
	maxValueSize int64,
	maxConnections int,
	maxInFlightRequests int,
	maxFramePayloadSize int64,
) ResourceConfig {
	storageBudget := percentageOf(processMemoryBudget, storageMemoryPercentage)
	idempotencyBudget := percentageOf(processMemoryBudget, idempotencyMemoryPercentage)
	processingBudget := percentageOf(processMemoryBudget, processingMemoryPercentage)

	allocated := storageBudget + idempotencyBudget + processingBudget

	return ResourceConfig{
		Profile:             profile,
		ProcessMemoryBudget: processMemoryBudget,
		CPUCapacity:         cpuCapacity,

		Storage: StorageResourceConfig{
			MaxEntries:   storageEntries,
			MemoryBudget: storageBudget,
			MaxKeySize:   maxKeySize,
			MaxValueSize: maxValueSize,
		},

		Idempotency: IdempotencyResourceConfig{
			MaxEntries:   idempotencyEntries,
			MemoryBudget: idempotencyBudget,
			Retention:    24 * time.Hour,
		},

		Network: NetworkResourceConfig{
			MaxConnections:      maxConnections,
			MaxInFlightRequests: maxInFlightRequests,
			MaxFramePayloadSize: maxFramePayloadSize,
		},

		Processing: ProcessingResourceConfig{
			MemoryBudget: processingBudget,
		},

		Runtime: RuntimeResourceConfig{
			MemoryBudget: processMemoryBudget - allocated,
		},
	}
}
