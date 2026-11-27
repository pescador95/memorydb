//go:build linux

package resources

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectLinuxCPUWithCgroupV2(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "", map[string]string{
		"cpu.max": "50000 100000\n",
	})
	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"cpu.max": "100000 100000\n",
	})
	writeCgroupFiles(t, mountPoint, "container/child", map[string]string{
		"cpu.max": "200000 100000\n",
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
				path: "/container/child",
			},
		},
	}

	got, err := detectLinuxCPU(layout)
	if err != nil {
		t.Fatalf("detectLinuxCPU() error = %v", err)
	}

	if got.capacity != 500 {
		t.Errorf("capacity = %d, want 500 millicores", got.capacity)
	}

	if !got.fromContainer {
		t.Error("fromContainer = false, want true")
	}
}

func TestDetectLinuxCPUWithCgroupV1(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "", map[string]string{
		"cpu.cfs_quota_us":  "50000\n",
		"cpu.cfs_period_us": "100000\n",
	})
	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"cpu.cfs_quota_us":  "100000\n",
		"cpu.cfs_period_us": "100000\n",
	})
	writeCgroupFiles(t, mountPoint, "container/child", map[string]string{
		"cpu.cfs_quota_us":  "200000\n",
		"cpu.cfs_period_us": "100000\n",
	})

	layout := cgroupLayout{
		mounts: []cgroupMount{
			{
				root:        "/",
				mountPoint:  mountPoint,
				version:     cgroupV1,
				controllers: []string{"cpu", "cpuacct"},
			},
		},
		memberships: []cgroupMembership{
			{
				path:        "/container/child",
				controllers: []string{"cpu", "cpuacct"},
			},
		},
	}

	got, err := detectLinuxCPU(layout)
	if err != nil {
		t.Fatalf("detectLinuxCPU() error = %v", err)
	}

	if got.capacity != 500 {
		t.Errorf("capacity = %d, want 500 millicores", got.capacity)
	}

	if !got.fromContainer {
		t.Error("fromContainer = false, want true")
	}
}

func TestDetectLinuxMemoryWithCgroupV2(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "", map[string]string{
		"memory.max":     "2048\n",
		"memory.current": "512\n",
	})
	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"memory.max":     "4096\n",
		"memory.current": "1024\n",
	})
	writeCgroupFiles(t, mountPoint, "container/child", map[string]string{
		"memory.max":     "8192\n",
		"memory.current": "1024\n",
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
				path: "/container/child",
			},
		},
	}

	got, err := detectLinuxMemory(layout)
	if err != nil {
		t.Fatalf("detectLinuxMemory() error = %v", err)
	}

	if got.capacity != 1536 {
		t.Errorf("capacity = %d, want 1536 bytes", got.capacity)
	}

	if !got.fromContainer {
		t.Error("fromContainer = false, want true")
	}
}

func TestDetectLinuxMemoryWithCgroupV1(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "", map[string]string{
		"memory.limit_in_bytes": "2048\n",
		"memory.usage_in_bytes": "512\n",
	})
	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"memory.limit_in_bytes": "4096\n",
		"memory.usage_in_bytes": "1024\n",
	})
	writeCgroupFiles(t, mountPoint, "container/child", map[string]string{
		"memory.limit_in_bytes": "8192\n",
		"memory.usage_in_bytes": "1024\n",
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
		t.Errorf("capacity = %d, want 1536 bytes", got.capacity)
	}

	if !got.fromContainer {
		t.Error("fromContainer = false, want true")
	}
}

func TestDetectLinuxCPUWithoutCgroupLimits(t *testing.T) {
	got, err := detectLinuxCPU(cgroupLayout{})
	if err != nil {
		t.Fatalf("detectLinuxCPU() error = %v", err)
	}

	want := int64(runtime.NumCPU()) * 1000

	if got.capacity != want {
		t.Errorf("capacity = %d, want %d", got.capacity, want)
	}

	if got.fromContainer {
		t.Error("fromContainer = true, want false")
	}
}

func TestDetectLinuxMemoryWithoutCgroupLimits(t *testing.T) {
	got, err := detectLinuxMemory(cgroupLayout{})
	if err != nil {
		t.Fatalf("detectLinuxMemory() error = %v", err)
	}

	if got.capacity <= 0 {
		t.Errorf("capacity = %d, want a positive host memory capacity", got.capacity)
	}

	if got.fromContainer {
		t.Error("fromContainer = true, want false")
	}
}

func TestDetectLinuxCPUWithoutMatchingMembership(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"cpu.max": "50000 100000\n",
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
				controllers: []string{"memory"},
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

func TestDetectLinuxMemoryWithoutMatchingMembership(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"memory.max":     "1024\n",
		"memory.current": "128\n",
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
				path:        "/container",
				controllers: []string{"cpu"},
			},
		},
	}

	got, err := detectLinuxMemory(layout)
	if err != nil {
		t.Fatalf("detectLinuxMemory() error = %v", err)
	}

	if got.capacity <= 0 {
		t.Errorf("capacity = %d, want a positive host memory capacity", got.capacity)
	}

	if got.fromContainer {
		t.Error("fromContainer = true, want false")
	}
}

func TestDetectLinuxCPURejectsInvalidCgroupLimit(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"cpu.max": "invalid 100000\n",
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

	_, err := detectLinuxCPU(layout)
	if err == nil {
		t.Fatal("expected error for invalid CPU limit")
	}
}

func TestDetectLinuxMemoryRejectsMissingUsageFile(t *testing.T) {
	mountPoint := t.TempDir()

	writeCgroupFiles(t, mountPoint, "container", map[string]string{
		"memory.max": "1024\n",
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

	_, err := detectLinuxMemory(layout)
	if err == nil {
		t.Fatal("expected error for missing memory.current")
	}
}

func writeCgroupFiles(
	t *testing.T,
	mountPoint string,
	relativeDirectory string,
	files map[string]string,
) {
	t.Helper()

	directory := filepath.Join(mountPoint, relativeDirectory)

	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatalf("create cgroup directory %s: %v", directory, err)
	}

	for name, content := range files {
		writeResourceTestFile(t, directory, name, content)
	}
}

func TestDetectResourcesUsesCurrentLinuxEnvironment(t *testing.T) {
	got, err := DetectResources()
	if err != nil {
		t.Fatalf("DetectResources() error = %v", err)
	}

	if got.CPUCapacity <= 0 {
		t.Errorf("CPUCapacity = %d, want greater than zero", got.CPUCapacity)
	}

	if got.MemoryCapacity <= 0 {
		t.Errorf("MemoryCapacity = %d, want greater than zero", got.MemoryCapacity)
	}

	if got.CPUSource == "" {
		t.Error("CPUSource is empty")
	}

	if got.MemorySource == "" {
		t.Error("MemorySource is empty")
	}

	if err := got.Validate(); err != nil {
		t.Errorf("DetectedResources.Validate() error = %v", err)
	}
}
