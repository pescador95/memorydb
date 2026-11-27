//go:build !linux && !windows

package resources

import "fmt"

func detectResources() (DetectedResources, error) {
	return DetectedResources{}, fmt.Errorf(
		"%w: resource detection is not implemented for this operating system",
		ErrUnsupportedPlatform,
	)
}
