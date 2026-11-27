//go:build linux

package resources

import (
	"testing"
)

func TestDetectLinuxCPUWithMissingChildLimitCgroupV2(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"cpu.max": "50000 100000\n",
	})
	writeCgroupFiles(t, mountPoint, "container/child", nil)

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:       "/",
				mountPoint: mountPoint,
				version:    cgroupV2,
			},
		},
		memberships: []cgroupMembership{
			{path: "/container/child"},
		},
	}

	got, err := detectLinuxCPU(layout)
	if err != nil {
		t.Fatalf("detectLinuxCPU() error = %v", err)
	}

	if got.capacity != 500 {
		t.Errorf("capacity = %d, want 500 millicores from parent", got.capacity)
	}

	if !got.fromContainer {
		t.Error("fromContainer = false, want true")
	}
}

func TestDetectLinuxCPUWithMissingChildLimitCgroupV1(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"cpu.cfs_quota_us":  "50000\n",
		"cpu.cfs_period_us": "100000\n",
	})
	writeCgroupFiles(t, mountPoint, "container/child", nil)

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
				path:        "/container/child",
				controllers: []string{"cpu"},
			},
		},
	}

	got, err := detectLinuxCPU(layout)
	if err != nil {
		t.Fatalf("detectLinuxCPU() error = %v", err)
	}

	if got.capacity != 500 {
		t.Errorf("capacity = %d, want 500 millicores from parent", got.capacity)
	}

	if !got.fromContainer {
		t.Error("fromContainer = false, want true")
	}
}

func TestDetectLinuxMemoryWithMissingChildLimitCgroupV2(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"memory.max":     "2048\n",
		"memory.current": "512\n",
	})
	writeCgroupFiles(t, mountPoint, "container/child", nil)

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:       "/",
				mountPoint: mountPoint,
				version:    cgroupV2,
			},
		},
		memberships: []cgroupMembership{
			{path: "/container/child"},
		},
	}

	got, err := detectLinuxMemory(layout)
	if err != nil {
		t.Fatalf("detectLinuxMemory() error = %v", err)
	}

	if got.capacity != 1536 {
		t.Errorf("capacity = %d, want 1536 bytes from parent", got.capacity)
	}

	if !got.fromContainer {
		t.Error("fromContainer = false, want true")
	}
}

func TestDetectLinuxMemoryWithMissingChildLimitCgroupV1(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"memory.limit_in_bytes": "2048\n",
		"memory.usage_in_bytes": "512\n",
	})
	writeCgroupFiles(t, mountPoint, "container/child", nil)

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
				path:        "/container/child",
				controllers: []string{"memory"},
			},
		},
	}

	got, err := detectLinuxMemory(layout)
	if err != nil {
		t.Fatalf("detectLinuxMemory() error = %v", err)
	}

	if got.capacity != 1536 {
		t.Errorf("capacity = %d, want 1536 bytes from parent", got.capacity)
	}

	if !got.fromContainer {
		t.Error("fromContainer = false, want true")
	}
}
