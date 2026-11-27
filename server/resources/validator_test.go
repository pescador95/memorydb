package resources

import (
	"errors"
	"math"
	"testing"
)

func TestDefaultResourceProfilesValidate(t *testing.T) {
	for profile, config := range defaultResourceProfiles {
		t.Run(string(profile), func(t *testing.T) {
			if config.Profile != profile {
				t.Fatalf("profile = %q, want %q", config.Profile, profile)
			}

			if err := config.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestResourceConfigValidateRejectsInvalidConfigurations(t *testing.T) {
	base := defaultResourceProfiles[ResourceProfileMid]

	tests := []struct {
		name   string
		mutate func(*ResourceConfig)
	}{
		{
			name: "invalid profile",
			mutate: func(c *ResourceConfig) {
				c.Profile = "unknown"
			},
		},
		{
			name: "zero CPU",
			mutate: func(c *ResourceConfig) {
				c.CPUCapacity = 0
			},
		},
		{
			name: "zero process memory",
			mutate: func(c *ResourceConfig) {
				c.ProcessMemoryBudget = 0
			},
		},
		{
			name: "zero storage entries",
			mutate: func(c *ResourceConfig) {
				c.Storage.MaxEntries = 0
			},
		},
		{
			name: "zero storage memory",
			mutate: func(c *ResourceConfig) {
				c.Storage.MemoryBudget = 0
			},
		},
		{
			name: "zero storage max key size",
			mutate: func(c *ResourceConfig) {
				c.Storage.MaxKeySize = 0
			},
		},
		{
			name: "zero storage max value size",
			mutate: func(c *ResourceConfig) {
				c.Storage.MaxValueSize = 0
			},
		},
		{
			name: "zero idempotency entries",
			mutate: func(c *ResourceConfig) {
				c.Idempotency.MaxEntries = 0
			},
		},
		{
			name: "zero idempotency memory",
			mutate: func(c *ResourceConfig) {
				c.Idempotency.MemoryBudget = 0
			},
		},
		{
			name: "zero idempotency retention",
			mutate: func(c *ResourceConfig) {
				c.Idempotency.Retention = 0
			},
		},
		{
			name: "zero connections",
			mutate: func(c *ResourceConfig) {
				c.Network.MaxConnections = 0
			},
		},
		{
			name: "zero in-flight requests",
			mutate: func(c *ResourceConfig) {
				c.Network.MaxInFlightRequests = 0
			},
		},
		{
			name: "zero frame payload size",
			mutate: func(c *ResourceConfig) {
				c.Network.MaxFramePayloadSize = 0
			},
		},
		{
			name: "zero processing memory",
			mutate: func(c *ResourceConfig) {
				c.Processing.MemoryBudget = 0
			},
		},
		{
			name: "zero runtime memory",
			mutate: func(c *ResourceConfig) {
				c.Runtime.MemoryBudget = 0
			},
		},
		{
			name: "component budgets do not match process budget",
			mutate: func(c *ResourceConfig) {
				c.Runtime.MemoryBudget--
			},
		},
		{
			name: "component memory budget sum overflows int64",
			mutate: func(c *ResourceConfig) {
				c.Storage.MemoryBudget = math.MaxInt64
				c.Idempotency.MemoryBudget = 1
			},
		},
		{
			name: "value exceeds frame",
			mutate: func(c *ResourceConfig) {
				c.Storage.MaxValueSize = c.Network.MaxFramePayloadSize + 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := base
			tt.mutate(&config)

			err := config.Validate()
			if !errors.Is(err, ErrInvalidResourceConfig) {
				t.Fatalf("expected ErrInvalidResourceConfig, got %v", err)
			}
		})
	}
}
