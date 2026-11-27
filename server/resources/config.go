package resources

import "time"

type ResourceConfig struct {
	Profile             ResourceProfile
	ProcessMemoryBudget int64
	CPUCapacity         int64 // millicores

	Storage     StorageResourceConfig
	Idempotency IdempotencyResourceConfig
	Network     NetworkResourceConfig
	Processing  ProcessingResourceConfig
	Runtime     RuntimeResourceConfig
}

type StorageResourceConfig struct {
	MaxEntries   int64
	MemoryBudget int64
	MaxKeySize   int64
	MaxValueSize int64
}

type IdempotencyResourceConfig struct {
	MaxEntries   int64
	MemoryBudget int64
	Retention    time.Duration
}

type NetworkResourceConfig struct {
	MaxConnections      int
	MaxInFlightRequests int
	MaxFramePayloadSize int64
}

type ProcessingResourceConfig struct {
	MemoryBudget int64
}

type RuntimeResourceConfig struct {
	MemoryBudget int64
}
