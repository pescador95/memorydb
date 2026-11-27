package resources

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInsufficientResources  = errors.New("insufficient available resources")
	ErrInvalidResourceOptions = errors.New("invalid resource options")
)

// ResourceOptions defines the programmatic inputs used to build a
// ResourceConfig.
type ResourceOptions struct {
	Profile ResourceProfile

	// CPUCapacity is expressed in millicores.
	// It is required only for the custom profile.
	CPUCapacity int64

	// ProcessMemoryBudget is expressed in bytes.
	// It is required only for the custom profile.
	ProcessMemoryBudget int64

	Overrides ResourceOverrides
}

// ResourceOverrides contains optional overrides for calculated or preset
// resource configuration values.
//
// Nil fields preserve the values selected by the profile or calculator.
// Memory budget overrides must preserve the total process memory budget.
type ResourceOverrides struct {
	CPUCapacity         *int64
	ProcessMemoryBudget *int64

	StorageMaxEntries   *int64
	StorageMemoryBudget *int64
	StorageMaxKeySize   *int64
	StorageMaxValueSize *int64

	IdempotencyMaxEntries   *int64
	IdempotencyMemoryBudget *int64
	IdempotencyRetention    *time.Duration

	MaxConnections      *int
	MaxInFlightRequests *int
	MaxFramePayloadSize *int64

	ProcessingMemoryBudget *int64
	RuntimeMemoryBudget    *int64
}

// LoadResourceConfig detects the resources available to the process and
// builds a validated configuration for the requested profile.
func LoadResourceConfig(options ResourceOptions) (ResourceConfig, error) {
	detected, err := DetectResources()
	if err != nil {
		return ResourceConfig{}, fmt.Errorf(
			"%w: %w",
			ErrResourceDetection,
			err,
		)
	}

	return loadResourceConfig(options, detected)
}

// loadResourceConfig builds a resource configuration using an explicit
// detection result. It is separated from LoadResourceConfig to make profile
// calculation deterministic and independently testable.
func loadResourceConfig(
	options ResourceOptions,
	detected DetectedResources,
) (ResourceConfig, error) {
	if err := detected.Validate(); err != nil {
		return ResourceConfig{}, fmt.Errorf(
			"%w: %w",
			ErrInvalidResourceOptions,
			err,
		)
	}

	profile := options.Profile
	if profile == "" {
		profile = ResourceProfileMid
	}

	if err := profile.Validate(); err != nil {
		return ResourceConfig{}, fmt.Errorf(
			"%w: %w",
			ErrInvalidResourceOptions,
			err,
		)
	}

	var config ResourceConfig
	var err error

	switch profile {
	case ResourceProfileLow, ResourceProfileMid, ResourceProfileHigh:
		if options.CPUCapacity != 0 || options.ProcessMemoryBudget != 0 {
			return ResourceConfig{}, fmt.Errorf(
				"%w: CPU capacity and process memory budget can only be specified for the custom profile",
				ErrInvalidResourceOptions,
			)
		}

		var exists bool
		config, exists = defaultResourceProfiles[profile]
		if !exists {
			return ResourceConfig{}, fmt.Errorf(
				"%w: profile %q has no preset",
				ErrInvalidResourceOptions,
				profile,
			)
		}

	case ResourceProfileAuto:
		if options.CPUCapacity != 0 || options.ProcessMemoryBudget != 0 {
			return ResourceConfig{}, fmt.Errorf(
				"%w: CPU capacity and process memory budget are detected automatically",
				ErrInvalidResourceOptions,
			)
		}

		config, err = calculateResourceConfigForProfile(
			ResourceProfileAuto,
			detected.CPUCapacity,
			detected.MemoryCapacity,
		)
		if err != nil {
			return ResourceConfig{}, fmt.Errorf(
				"calculate auto resource configuration: %w",
				err,
			)
		}

	case ResourceProfileCustom:
		if options.CPUCapacity <= 0 {
			return ResourceConfig{}, fmt.Errorf(
				"%w: custom CPU capacity must be positive",
				ErrInvalidResourceOptions,
			)
		}

		if options.ProcessMemoryBudget <= 0 {
			return ResourceConfig{}, fmt.Errorf(
				"%w: custom process memory budget must be positive",
				ErrInvalidResourceOptions,
			)
		}

		if options.CPUCapacity > detected.CPUCapacity {
			return ResourceConfig{}, insufficientResources(
				"custom CPU capacity %dm exceeds detected capacity %dm",
				options.CPUCapacity,
				detected.CPUCapacity,
			)
		}

		if options.ProcessMemoryBudget > detected.MemoryCapacity {
			return ResourceConfig{}, insufficientResources(
				"custom process memory budget %d bytes exceeds detected capacity %d bytes",
				options.ProcessMemoryBudget,
				detected.MemoryCapacity,
			)
		}

		config, err = calculateResourceConfigForProfile(
			ResourceProfileCustom,
			options.CPUCapacity,
			options.ProcessMemoryBudget,
		)
		if err != nil {
			return ResourceConfig{}, fmt.Errorf(
				"calculate custom resource configuration: %w",
				err,
			)
		}
	}

	if err := applyResourceOverrides(&config, options.Overrides); err != nil {
		return ResourceConfig{}, err
	}

	if config.CPUCapacity > detected.CPUCapacity {
		return ResourceConfig{}, insufficientResources(
			"configured CPU capacity %dm exceeds detected capacity %dm",
			config.CPUCapacity,
			detected.CPUCapacity,
		)
	}

	if config.ProcessMemoryBudget > detected.MemoryCapacity {
		return ResourceConfig{}, insufficientResources(
			"configured process memory budget %d bytes exceeds detected capacity %d bytes",
			config.ProcessMemoryBudget,
			detected.MemoryCapacity,
		)
	}

	if err := config.Validate(); err != nil {
		return ResourceConfig{}, fmt.Errorf(
			"validate final resource configuration: %w",
			err,
		)
	}

	return config, nil
}

func applyResourceOverrides(
	config *ResourceConfig,
	overrides ResourceOverrides,
) error {
	if config == nil {
		return fmt.Errorf(
			"%w: resource configuration is nil",
			ErrInvalidResourceOptions,
		)
	}

	if overrides.CPUCapacity != nil {
		config.CPUCapacity = *overrides.CPUCapacity
	}

	if overrides.ProcessMemoryBudget != nil {
		config.ProcessMemoryBudget = *overrides.ProcessMemoryBudget
	}

	if overrides.StorageMaxEntries != nil {
		config.Storage.MaxEntries = *overrides.StorageMaxEntries
	}

	if overrides.StorageMemoryBudget != nil {
		config.Storage.MemoryBudget = *overrides.StorageMemoryBudget
	}

	if overrides.StorageMaxKeySize != nil {
		config.Storage.MaxKeySize = *overrides.StorageMaxKeySize
	}

	if overrides.StorageMaxValueSize != nil {
		config.Storage.MaxValueSize = *overrides.StorageMaxValueSize
	}

	if overrides.IdempotencyMaxEntries != nil {
		config.Idempotency.MaxEntries = *overrides.IdempotencyMaxEntries
	}

	if overrides.IdempotencyMemoryBudget != nil {
		config.Idempotency.MemoryBudget = *overrides.IdempotencyMemoryBudget
	}

	if overrides.IdempotencyRetention != nil {
		config.Idempotency.Retention = *overrides.IdempotencyRetention
	}

	if overrides.MaxConnections != nil {
		config.Network.MaxConnections = *overrides.MaxConnections
	}

	if overrides.MaxInFlightRequests != nil {
		config.Network.MaxInFlightRequests = *overrides.MaxInFlightRequests
	}

	if overrides.MaxFramePayloadSize != nil {
		config.Network.MaxFramePayloadSize = *overrides.MaxFramePayloadSize
	}

	if overrides.ProcessingMemoryBudget != nil {
		config.Processing.MemoryBudget = *overrides.ProcessingMemoryBudget
	}

	if overrides.RuntimeMemoryBudget != nil {
		config.Runtime.MemoryBudget = *overrides.RuntimeMemoryBudget
	}

	return nil
}

func insufficientResources(format string, args ...any) error {
	return fmt.Errorf(
		"%w: %s",
		ErrInsufficientResources,
		fmt.Sprintf(format, args...),
	)
}
