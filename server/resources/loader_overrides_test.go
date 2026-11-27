package resources

import (
	"errors"
	"testing"
	"time"
)

func TestApplyResourceOverridesAppliesAllFields(t *testing.T) {
	config := defaultResourceProfiles[ResourceProfileMid]

	overrides := ResourceOverrides{
		CPUCapacity:             int64Pointer(1500),
		ProcessMemoryBudget:     int64Pointer(512 << 20),
		StorageMaxEntries:       int64Pointer(200_000),
		StorageMemoryBudget:     int64Pointer(200 << 20),
		StorageMaxKeySize:       int64Pointer(1024),
		StorageMaxValueSize:     int64Pointer(32 << 10),
		IdempotencyMaxEntries:   int64Pointer(25_000),
		IdempotencyMemoryBudget: int64Pointer(50 << 20),
		IdempotencyRetention:    durationPointer(12 * time.Hour),
		MaxConnections:          intPointer(512),
		MaxInFlightRequests:     intPointer(128),
		MaxFramePayloadSize:     int64Pointer(128 << 10),
		ProcessingMemoryBudget:  int64Pointer(100 << 20),
		RuntimeMemoryBudget:     int64Pointer(100 << 20),
	}

	if err := applyResourceOverrides(&config, overrides); err != nil {
		t.Fatalf("applyResourceOverrides() error = %v", err)
	}

	if config.CPUCapacity != 1500 {
		t.Errorf("CPUCapacity = %d, want 1500", config.CPUCapacity)
	}

	if config.ProcessMemoryBudget != 512<<20 {
		t.Errorf("ProcessMemoryBudget = %d, want %d", config.ProcessMemoryBudget, 512<<20)
	}

	if config.Storage.MaxEntries != 200_000 {
		t.Errorf("Storage.MaxEntries = %d, want 200000", config.Storage.MaxEntries)
	}

	if config.Storage.MemoryBudget != 200<<20 {
		t.Errorf("Storage.MemoryBudget = %d, want %d", config.Storage.MemoryBudget, 200<<20)
	}

	if config.Storage.MaxKeySize != 1024 {
		t.Errorf("Storage.MaxKeySize = %d, want 1024", config.Storage.MaxKeySize)
	}

	if config.Storage.MaxValueSize != 32<<10 {
		t.Errorf("Storage.MaxValueSize = %d, want %d", config.Storage.MaxValueSize, 32<<10)
	}

	if config.Idempotency.MaxEntries != 25_000 {
		t.Errorf("Idempotency.MaxEntries = %d, want 25000", config.Idempotency.MaxEntries)
	}

	if config.Idempotency.MemoryBudget != 50<<20 {
		t.Errorf("Idempotency.MemoryBudget = %d, want %d", config.Idempotency.MemoryBudget, 50<<20)
	}

	if config.Idempotency.Retention != 12*60*60*1e9 {
		t.Errorf("Idempotency.Retention = %s, want 12h", config.Idempotency.Retention)
	}

	if config.Network.MaxConnections != 512 {
		t.Errorf("Network.MaxConnections = %d, want 512", config.Network.MaxConnections)
	}

	if config.Network.MaxInFlightRequests != 128 {
		t.Errorf("Network.MaxInFlightRequests = %d, want 128", config.Network.MaxInFlightRequests)
	}

	if config.Network.MaxFramePayloadSize != 128<<10 {
		t.Errorf("Network.MaxFramePayloadSize = %d, want %d", config.Network.MaxFramePayloadSize, 128<<10)
	}

	if config.Processing.MemoryBudget != 100<<20 {
		t.Errorf("Processing.MemoryBudget = %d, want %d", config.Processing.MemoryBudget, 100<<20)
	}

	if config.Runtime.MemoryBudget != 100<<20 {
		t.Errorf("Runtime.MemoryBudget = %d, want %d", config.Runtime.MemoryBudget, 100<<20)
	}
}

func TestApplyResourceOverridesRejectsNilConfig(t *testing.T) {
	err := applyResourceOverrides(nil, ResourceOverrides{})

	if err == nil {
		t.Fatal("expected error for nil config")
	}

	if !errorsIsInvalidResourceOptions(err) {
		t.Errorf("error = %v, want ErrInvalidResourceOptions", err)
	}
}

func errorsIsInvalidResourceOptions(err error) bool {
	return err != nil && errors.Is(err, ErrInvalidResourceOptions)
}
