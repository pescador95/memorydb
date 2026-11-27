package resources

import (
	"errors"
	"fmt"
	"math"
)

var ErrInvalidResourceConfig = errors.New("invalid resource configuration")

func (c ResourceConfig) Validate() error {
	if err := c.Profile.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidResourceConfig, err)
	}

	if c.ProcessMemoryBudget <= 0 {
		return invalidConfig("process memory budget must be positive")
	}

	if c.CPUCapacity <= 0 {
		return invalidConfig("CPU capacity must be positive")
	}

	if c.Storage.MaxEntries <= 0 {
		return invalidConfig("storage max entries must be positive")
	}

	if c.Storage.MemoryBudget <= 0 {
		return invalidConfig("storage memory budget must be positive")
	}

	if c.Storage.MaxKeySize <= 0 {
		return invalidConfig("storage max key size must be positive")
	}

	if c.Storage.MaxValueSize <= 0 {
		return invalidConfig("storage max value size must be positive")
	}

	if c.Idempotency.MaxEntries <= 0 {
		return invalidConfig("idempotency max entries must be positive")
	}

	if c.Idempotency.MemoryBudget <= 0 {
		return invalidConfig("idempotency memory budget must be positive")
	}

	if c.Idempotency.Retention <= 0 {
		return invalidConfig("idempotency retention must be positive")
	}

	if c.Network.MaxConnections <= 0 {
		return invalidConfig("max connections must be positive")
	}

	if c.Network.MaxInFlightRequests <= 0 {
		return invalidConfig("max in-flight requests must be positive")
	}

	if c.Network.MaxFramePayloadSize <= 0 {
		return invalidConfig("max frame payload size must be positive")
	}

	if c.Processing.MemoryBudget <= 0 {
		return invalidConfig("processing memory budget must be positive")
	}

	if c.Runtime.MemoryBudget <= 0 {
		return invalidConfig("runtime memory budget must be positive")
	}

	componentBudgets := []int64{
		c.Storage.MemoryBudget,
		c.Idempotency.MemoryBudget,
		c.Processing.MemoryBudget,
		c.Runtime.MemoryBudget,
	}

	var allocated int64

	for _, budget := range componentBudgets {
		if budget > math.MaxInt64-allocated {
			return invalidConfig("component memory budget sum overflows int64")
		}

		allocated += budget
	}

	if allocated != c.ProcessMemoryBudget {
		return invalidConfig(
			"component memory budgets do not equal process memory budget",
		)
	}

	if c.Storage.MaxValueSize > c.Network.MaxFramePayloadSize {
		return invalidConfig(
			"storage max value size exceeds maximum frame payload size",
		)
	}

	return nil
}

func invalidConfig(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidResourceConfig, message)
}
