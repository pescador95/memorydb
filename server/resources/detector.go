package resources

import (
	"errors"
	"fmt"
)

var (
	ErrUnsupportedPlatform = errors.New("unsupported operating system")
	ErrResourceDetection   = errors.New("resource detection failed")
	ErrInvalidResources    = errors.New("invalid detected resources")
)

type ResourceDetectionSource string

const (
	ResourceSourceContainer ResourceDetectionSource = "container"
	ResourceSourceHost      ResourceDetectionSource = "host"
)

type DetectedResources struct {
	CPUCapacity    int64
	MemoryCapacity int64
	CPUSource      ResourceDetectionSource
	MemorySource   ResourceDetectionSource
}

func (r DetectedResources) Validate() error {
	if r.CPUCapacity <= 0 {
		return fmt.Errorf(
			"%w: CPU capacity must be positive",
			ErrInvalidResources,
		)
	}

	if r.MemoryCapacity <= 0 {
		return fmt.Errorf(
			"%w: memory capacity must be positive",
			ErrInvalidResources,
		)
	}

	if !validDetectionSource(r.CPUSource) {
		return fmt.Errorf(
			"%w: invalid CPU detection source %q",
			ErrInvalidResources,
			r.CPUSource,
		)
	}

	if !validDetectionSource(r.MemorySource) {
		return fmt.Errorf(
			"%w: invalid memory detection source %q",
			ErrInvalidResources,
			r.MemorySource,
		)
	}

	return nil
}

func validDetectionSource(source ResourceDetectionSource) bool {
	switch source {
	case ResourceSourceContainer, ResourceSourceHost:
		return true
	default:
		return false
	}
}

func DetectResources() (DetectedResources, error) {
	return detectResources()
}
