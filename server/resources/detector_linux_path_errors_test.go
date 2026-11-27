//go:build linux

package resources

import (
	"strings"
	"testing"
)

func TestDetectLinuxCPURejectsMembershipOutsideMountRoot(t *testing.T) {
	mountPoint := t.TempDir()

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:       "/tenant/container",
				mountPoint: mountPoint,
				version:    cgroupV2,
			},
		},
		memberships: []cgroupMembership{
			{
				path: "/other/container",
			},
		},
	}

	_, err := detectLinuxCPU(layout)
	if err == nil {
		t.Fatal("expected error for membership outside cgroup mount root")
	}

	if !strings.Contains(err.Error(), "resolve CPU cgroup path") {
		t.Errorf("error = %q, want CPU cgroup path context", err)
	}

	if !strings.Contains(err.Error(), "membership is outside the mounted cgroup subtree") {
		t.Errorf("error = %q, want membership outside mount root", err)
	}
}

func TestDetectLinuxMemoryRejectsMembershipOutsideMountRoot(t *testing.T) {
	mountPoint := t.TempDir()

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:       "/tenant/container",
				mountPoint: mountPoint,
				version:    cgroupV2,
			},
		},
		memberships: []cgroupMembership{
			{
				path: "/other/container",
			},
		},
	}

	_, err := detectLinuxMemory(layout)
	if err == nil {
		t.Fatal("expected error for membership outside cgroup mount root")
	}

	if !strings.Contains(err.Error(), "resolve memory cgroup path") {
		t.Errorf("error = %q, want memory cgroup path context", err)
	}

	if !strings.Contains(err.Error(), "membership is outside the mounted cgroup subtree") {
		t.Errorf("error = %q, want membership outside mount root", err)
	}
}
