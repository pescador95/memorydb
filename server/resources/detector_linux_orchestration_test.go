//go:build linux

package resources

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDetectResourcesFromLayout(t *testing.T) {
	t.Run("uses host resources when cgroup layout is unavailable", func(t *testing.T) {
		layoutErr := fmt.Errorf(
			"simulated unavailable layout: %w",
			errCgroupLayoutUnavailable,
		)

		got, err := detectResourcesFromLayout(cgroupLayout{}, layoutErr)
		if err != nil {
			t.Fatalf("detectResourcesFromLayout() error = %v", err)
		}

		if got.CPUCapacity <= 0 {
			t.Errorf("CPUCapacity = %d, want positive value", got.CPUCapacity)
		}

		if got.MemoryCapacity <= 0 {
			t.Errorf("MemoryCapacity = %d, want positive value", got.MemoryCapacity)
		}

		if got.CPUSource != ResourceSourceHost {
			t.Errorf("CPUSource = %q, want %q", got.CPUSource, ResourceSourceHost)
		}

		if got.MemorySource != ResourceSourceHost {
			t.Errorf("MemorySource = %q, want %q", got.MemorySource, ResourceSourceHost)
		}

		if err := got.Validate(); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("wraps unexpected cgroup discovery errors", func(t *testing.T) {
		discoveryErr := errors.New("simulated mountinfo read failure")

		_, err := detectResourcesFromLayout(
			cgroupLayout{},
			discoveryErr,
		)
		if err == nil {
			t.Fatal("expected error")
		}

		if !errors.Is(err, ErrResourceDetection) {
			t.Errorf("error = %v, want ErrResourceDetection", err)
		}

		if !errors.Is(err, discoveryErr) {
			t.Errorf("error = %v, want original discovery error", err)
		}
	})

	t.Run("wraps CPU detection errors", func(t *testing.T) {
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

		_, err := detectResourcesFromLayout(layout, nil)
		if err == nil {
			t.Fatal("expected CPU detection error")
		}

		if !errors.Is(err, ErrResourceDetection) {
			t.Errorf("error = %v, want ErrResourceDetection", err)
		}

		if !strings.Contains(err.Error(), "CPU") {
			t.Errorf("error = %v, want CPU context", err)
		}
	})

	t.Run("wraps memory detection errors", func(t *testing.T) {
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

		_, err := detectResourcesFromLayout(layout, nil)
		if err == nil {
			t.Fatal("expected memory detection error")
		}

		if !errors.Is(err, ErrResourceDetection) {
			t.Errorf("error = %v, want ErrResourceDetection", err)
		}

		if !strings.Contains(err.Error(), "memory") {
			t.Errorf("error = %v, want memory context", err)
		}
	})

	t.Run("identifies CPU capacity restricted by container", func(t *testing.T) {
		mountPoint := t.TempDir()

		writeCgroupFiles(t, mountPoint, "container", map[string]string{
			"cpu.max": "50000 100000\n",
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

		got, err := detectResourcesFromLayout(layout, nil)
		if err != nil {
			t.Fatalf("detectResourcesFromLayout() error = %v", err)
		}

		if got.CPUCapacity != 450 {
			t.Errorf("CPUCapacity = %d, want 450 millicores", got.CPUCapacity)
		}

		if got.CPUSource != ResourceSourceContainer {
			t.Errorf(
				"CPUSource = %q, want %q",
				got.CPUSource,
				ResourceSourceContainer,
			)
		}

		if err := got.Validate(); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})

	t.Run("identifies memory capacity restricted by container", func(t *testing.T) {
		mountPoint := t.TempDir()

		writeCgroupFiles(t, mountPoint, "container", map[string]string{
			"cpu.max":        "max 100000\n",
			"memory.max":     "2048\n",
			"memory.current": "512\n",
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

		got, err := detectResourcesFromLayout(layout, nil)
		if err != nil {
			t.Fatalf("detectResourcesFromLayout() error = %v", err)
		}

		if got.MemoryCapacity <= 0 {
			t.Errorf("MemoryCapacity = %d, want positive value", got.MemoryCapacity)
		}

		// O limite disponível no cgroup é 2048 - 512 = 1536 bytes.
		// A margem de segurança deve reduzir o orçamento final.
		if got.MemoryCapacity >= 1536 {
			t.Errorf(
				"MemoryCapacity = %d, want less than 1536 bytes after safety margin",
				got.MemoryCapacity,
			)
		}

		if got.MemorySource != ResourceSourceContainer {
			t.Errorf(
				"MemorySource = %q, want %q",
				got.MemorySource,
				ResourceSourceContainer,
			)
		}

		if got.CPUCapacity <= 0 {
			t.Errorf("CPUCapacity = %d, want positive host capacity", got.CPUCapacity)
		}

		if got.CPUSource != ResourceSourceHost {
			t.Errorf(
				"CPUSource = %q, want %q",
				got.CPUSource,
				ResourceSourceHost,
			)
		}

		if err := got.Validate(); err != nil {
			t.Errorf("Validate() error = %v", err)
		}
	})
}
