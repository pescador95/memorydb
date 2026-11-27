package resources

import "errors"

var errCgroupLayoutUnavailable = errors.New("cgroup layout unavailable")

type cgroupMount struct {
	root        string
	mountPoint  string
	controllers []string
	version     int
}

type cgroupMembership struct {
	path        string
	controllers []string
}

type cgroupLayout struct {
	mounts      []cgroupMount
	memberships []cgroupMembership
}
