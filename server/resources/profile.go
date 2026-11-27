package resources

import "fmt"

// ResourceProfile identifies the resource sizing strategy.
type ResourceProfile string

const (
	ResourceProfileLow    ResourceProfile = "low"
	ResourceProfileMid    ResourceProfile = "mid"
	ResourceProfileHigh   ResourceProfile = "high"
	ResourceProfileAuto   ResourceProfile = "auto"
	ResourceProfileCustom ResourceProfile = "custom"
)

// IsValid reports whether the resource profile is supported.
func (p ResourceProfile) IsValid() bool {
	switch p {
	case ResourceProfileLow,
		ResourceProfileMid,
		ResourceProfileHigh,
		ResourceProfileAuto,
		ResourceProfileCustom:
		return true
	default:
		return false
	}
}

// Validate checks whether the resource profile is supported.
func (p ResourceProfile) Validate() error {
	if !p.IsValid() {
		return fmt.Errorf("unsupported resource profile %q", p)
	}

	return nil
}
