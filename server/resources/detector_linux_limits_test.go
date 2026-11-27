//go:build linux

package resources

import (
	"runtime"
	"testing"
)

func TestDetectLinuxCPUWithUnlimitedCgroupV2(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"cpu.max": "max 100000\n",
	})

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:       "/",
				mountPoint: mountPoint,
				version:    cgroupV2,
			},
		},
		memberships: []cgroupMembership{
			{
				path: "/container",
			},
		},
	}

	got, err := detectLinuxCPU(layout)
	if err != nil {
		t.Fatalf("detectLinuxCPU() error = %v", err)
	}

	want := int64(runtime.NumCPU()) * 1000

	if got.capacity != want {
		t.Errorf("capacity = %d, want host capacity %d", got.capacity, want)
	}

	if got.fromContainer {
		t.Error("fromContainer = true, want false")
	}
}

func TestDetectLinuxCPUWithUnlimitedCgroupV1(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"cpu.cfs_quota_us": "-1\n",
	})

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:        "/",
				mountPoint:  mountPoint,
				version:     cgroupV1,
				controllers: []string{"cpu"},
			},
		},
		memberships: []cgroupMembership{
			{
				path:        "/container",
				controllers: []string{"cpu"},
			},
		},
	}

	got, err := detectLinuxCPU(layout)
	if err != nil {
		t.Fatalf("detectLinuxCPU() error = %v", err)
	}

	want := int64(runtime.NumCPU()) * 1000

	if got.capacity != want {
		t.Errorf("capacity = %d, want host capacity %d", got.capacity, want)
	}

	if got.fromContainer {
		t.Error("fromContainer = true, want false")
	}
}

func TestDetectLinuxMemoryWithUnlimitedCgroupV2(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"memory.max": "max\n",
	})

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:       "/",
				mountPoint: mountPoint,
				version:    cgroupV2,
			},
		},
		memberships: []cgroupMembership{
			{
				path: "/container",
			},
		},
	}

	got, err := detectLinuxMemory(layout)
	if err != nil {
		t.Fatalf("detectLinuxMemory() error = %v", err)
	}

	if got.capacity <= 0 {
		t.Errorf("capacity = %d, want positive host memory capacity", got.capacity)
	}

	if got.fromContainer {
		t.Error("fromContainer = true, want false")
	}
}

func TestDetectLinuxMemoryWithUnlimitedCgroupV1(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"memory.limit_in_bytes": "1152921504606846976\n",
	})

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:        "/",
				mountPoint:  mountPoint,
				version:     cgroupV1,
				controllers: []string{"memory"},
			},
		},
		memberships: []cgroupMembership{
			{
				path:        "/container",
				controllers: []string{"memory"},
			},
		},
	}

	got, err := detectLinuxMemory(layout)
	if err != nil {
		t.Fatalf("detectLinuxMemory() error = %v", err)
	}

	if got.capacity <= 0 {
		t.Errorf("capacity = %d, want positive host memory capacity", got.capacity)
	}

	if got.fromContainer {
		t.Error("fromContainer = true, want false")
	}
}
