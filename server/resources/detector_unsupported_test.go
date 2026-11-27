//go:build !linux && !windows

package resources

import (
	"errors"
	"testing"
)

func TestDetectResourcesUnsupportedPlatform(t *testing.T) {
	detected, err := DetectResources()

	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf(
			"error = %v, want ErrUnsupportedPlatform",
			err,
		)
	}

	if detected != (DetectedResources{}) {
		t.Errorf(
			"detected = %+v, want zero value",
			detected,
		)
	}
}
