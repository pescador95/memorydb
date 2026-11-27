//go:build linux

package resources

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLinuxCPURejectsCgroupFileReadError(t *testing.T) {
	mountPoint := t.TempDir()
	containerPath := filepath.Join(mountPoint, "container")

	if err := os.MkdirAll(containerPath, 0700); err != nil {
		t.Fatalf("create container directory: %v", err)
	}

	// Um diretório com o nome cpu.max faz os.ReadFile retornar erro.
	if err := os.Mkdir(filepath.Join(containerPath, "cpu.max"), 0700); err != nil {
		t.Fatalf("create cpu.max directory: %v", err)
	}

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
		t.Fatal("expected error when reading cpu.max directory")
	}
}
