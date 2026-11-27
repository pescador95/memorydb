//go:build linux

package resources

import (
	"reflect"
	"testing"
)

func TestCgroupDirectoriesWithinMountEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		mountPoint string
		groupPath  string
		want       []string
		wantErr    bool
	}{
		{
			name:       "relative mount point",
			mountPoint: "sys/fs/cgroup",
			groupPath:  "sys/fs/cgroup/container",
			wantErr:    true,
		},
		{
			name:       "relative group path",
			mountPoint: "/sys/fs/cgroup",
			groupPath:  "sys/fs/cgroup/container",
			wantErr:    true,
		},
		{
			name:       "group path equals mount point",
			mountPoint: "/sys/fs/cgroup",
			groupPath:  "/sys/fs/cgroup",
			want:       []string{"/sys/fs/cgroup"},
		},
		{
			name:       "root mount point",
			mountPoint: "/",
			groupPath:  "/",
			want:       []string{"/"},
		},
		{
			name:       "nested group path",
			mountPoint: "/sys/fs/cgroup",
			groupPath:  "/sys/fs/cgroup/docker/container",
			want: []string{
				"/sys/fs/cgroup/docker/container",
				"/sys/fs/cgroup/docker",
				"/sys/fs/cgroup",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cgroupDirectoriesWithinMount(
				tt.mountPoint,
				tt.groupPath,
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("cgroupDirectoriesWithinMount() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("directories = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractCgroupControllers(t *testing.T) {
	tests := []struct {
		name         string
		mountOptions string
		superOptions string
		want         []string
	}{
		{
			name:         "controllers from mount options",
			mountOptions: "rw,cpu,cpuacct",
			want:         []string{"cpu", "cpuacct"},
		},
		{
			name:         "controllers from super options",
			superOptions: "rw,memory,pids",
			want:         []string{"memory", "pids"},
		},
		{
			name:         "controllers from both option sets",
			mountOptions: "rw,cpu,cpuacct",
			superOptions: "rw,memory",
			want:         []string{"cpu", "cpuacct", "memory"},
		},
		{
			name:         "unknown controllers are ignored",
			mountOptions: "rw,unknown_controller,cpu",
			superOptions: "another_unknown,memory",
			want:         []string{"cpu", "memory"},
		},
		{
			name:         "named controller is preserved",
			mountOptions: "rw,name=systemd",
			want:         []string{"name=systemd"},
		},
		{
			name:         "duplicate controllers are removed",
			mountOptions: "rw,cpu,memory,cpu",
			superOptions: "rw,memory,pids,cpu",
			want:         []string{"cpu", "memory", "pids"},
		},
		{
			name: "empty options",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCgroupControllers(
				tt.mountOptions,
				tt.superOptions,
			)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controllers = %v, want %v", got, tt.want)
			}
		})
	}
}
