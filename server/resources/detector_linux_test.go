//go:build linux

package resources

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestApplySafetyMargin(t *testing.T) {
	tests := []struct {
		name       string
		value      int64
		percentage int64
		want       int64
	}{
		{"ten percent", 1000, 10, 900},
		{"twenty percent", 1000, 20, 800},
		{"zero value", 0, 20, 0},
		{"zero percentage", 1000, 0, 1000},
		{"full percentage", 1000, 100, 0},
		{"negative percentage", 1000, -10, 1000},
		{"maximum int64", math.MaxInt64, 20, math.MaxInt64 - (math.MaxInt64/100)*20 - ((math.MaxInt64%100)*20)/100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applySafetyMargin(tt.value, tt.percentage)
			if got != tt.want {
				t.Errorf("applySafetyMargin() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCalculateCPUCapacity(t *testing.T) {
	tests := []struct {
		name      string
		quota     int64
		period    int64
		want      int64
		wantFound bool
		wantErr   bool
	}{
		{"half CPU", 50000, 100000, 500, true, false},
		{"two CPUs", 200000, 100000, 2000, true, false},
		{"fraction below one millicore", 1, 100000, 1, true, false},
		{"zero quota", 0, 100000, 0, false, true},
		{"negative quota", -1, 100000, 0, false, true},
		{"zero period", 100000, 0, 0, false, true},
		{"negative period", 100000, -1, 0, false, true},
		{"overflow-safe calculation", math.MaxInt64, 1, math.MaxInt64, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := calculateCPUCapacity(tt.quota, tt.period)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("capacity = %d, want %d", got, tt.want)
			}

			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

func TestCgroupDirectoriesWithinMount(t *testing.T) {
	got, err := cgroupDirectoriesWithinMount(
		"/sys/fs/cgroup",
		"/sys/fs/cgroup/docker/container/child",
	)
	if err != nil {
		t.Fatalf("cgroupDirectoriesWithinMount() error = %v", err)
	}

	want := []string{
		"/sys/fs/cgroup/docker/container/child",
		"/sys/fs/cgroup/docker/container",
		"/sys/fs/cgroup/docker",
		"/sys/fs/cgroup",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("directories = %v, want %v", got, want)
	}
}

func TestCgroupDirectoriesWithinMountRejectsEscape(t *testing.T) {
	_, err := cgroupDirectoriesWithinMount(
		"/sys/fs/cgroup",
		"/sys/fs/cgroup-other/container",
	)
	if err == nil {
		t.Fatal("expected error for path outside mount")
	}
}

func TestReadCPUQuotaV2At(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "cpu.max", "50000 100000\n")

	got, found, err := readCPUQuotaV2At(directory)
	if err != nil {
		t.Fatalf("readCPUQuotaV2At() error = %v", err)
	}

	if !found || got != 500 {
		t.Errorf("capacity = %d, found = %v, want 500 and true", got, found)
	}
}

func TestReadCPUQuotaV2AtUnlimited(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "cpu.max", "max 100000\n")

	got, found, err := readCPUQuotaV2At(directory)
	if err != nil {
		t.Fatalf("readCPUQuotaV2At() error = %v", err)
	}

	if found || got != 0 {
		t.Errorf("capacity = %d, found = %v, want 0 and false", got, found)
	}
}

func TestReadCPUQuotaV1At(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "cpu.cfs_quota_us", "150000\n")
	writeResourceTestFile(t, directory, "cpu.cfs_period_us", "100000\n")

	got, found, err := readCPUQuotaV1At(directory)
	if err != nil {
		t.Fatalf("readCPUQuotaV1At() error = %v", err)
	}

	if !found || got != 1500 {
		t.Errorf("capacity = %d, found = %v, want 1500 and true", got, found)
	}
}

func TestReadCPUQuotaV1AtUnlimited(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "cpu.cfs_quota_us", "-1\n")

	got, found, err := readCPUQuotaV1At(directory)
	if err != nil {
		t.Fatalf("readCPUQuotaV1At() error = %v", err)
	}

	if found || got != 0 {
		t.Errorf("capacity = %d, found = %v, want 0 and false", got, found)
	}
}

func TestReadMemoryLimitV2At(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "memory.max", "1073741824\n")
	writeResourceTestFile(t, directory, "memory.current", "268435456\n")

	got, found, err := readMemoryLimitV2At(directory)
	if err != nil {
		t.Fatalf("readMemoryLimitV2At() error = %v", err)
	}

	if !found || got != 805306368 {
		t.Errorf("available = %d, found = %v, want 805306368 and true", got, found)
	}
}

func TestReadMemoryLimitV2AtUnlimited(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "memory.max", "max\n")

	got, found, err := readMemoryLimitV2At(directory)
	if err != nil {
		t.Fatalf("readMemoryLimitV2At() error = %v", err)
	}

	if found || got != 0 {
		t.Errorf("available = %d, found = %v, want 0 and false", got, found)
	}
}

func TestReadMemoryLimitV1At(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "memory.limit_in_bytes", "1073741824\n")
	writeResourceTestFile(t, directory, "memory.usage_in_bytes", "268435456\n")

	got, found, err := readMemoryLimitV1At(directory)
	if err != nil {
		t.Fatalf("readMemoryLimitV1At() error = %v", err)
	}

	if !found || got != 805306368 {
		t.Errorf("available = %d, found = %v, want 805306368 and true", got, found)
	}
}

func TestReadMemoryLimitV1AtUnlimited(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "memory.limit_in_bytes", "9223372036854771712\n")

	got, found, err := readMemoryLimitV1At(directory)
	if err != nil {
		t.Fatalf("readMemoryLimitV1At() error = %v", err)
	}

	if found || got != 0 {
		t.Errorf("available = %d, found = %v, want 0 and false", got, found)
	}
}

func TestReadMemoryLimitRejectsExhaustedLimit(t *testing.T) {
	directory := t.TempDir()

	writeResourceTestFile(t, directory, "memory.max", "1024\n")
	writeResourceTestFile(t, directory, "memory.current", "1024\n")

	_, _, err := readMemoryLimitV2At(directory)
	if err == nil {
		t.Fatal("expected error for exhausted memory limit")
	}
}

func TestCalculateAvailableMemoryRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
		usage int64
	}{
		{"zero limit", 0, 0},
		{"negative limit", -1, 0},
		{"negative usage", 1024, -1},
		{"usage equals limit", 1024, 1024},
		{"usage exceeds limit", 1024, 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := calculateAvailableMemory(tt.limit, tt.usage, "test")
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func writeResourceTestFile(t *testing.T, directory, name, content string) {
	t.Helper()

	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test file %s: %v", path, err)
	}
}

func TestFindMembership(t *testing.T) {
	tests := []struct {
		name        string
		mount       cgroupMount
		memberships []cgroupMembership
		want        cgroupMembership
		wantFound   bool
	}{
		{
			name: "v2 unified membership",
			mount: cgroupMount{
				version: cgroupV2,
			},
			memberships: []cgroupMembership{
				{
					path: "/docker/container",
				},
			},
			want: cgroupMembership{
				path: "/docker/container",
			},
			wantFound: true,
		},
		{
			name: "v2 skips v1 memberships",
			mount: cgroupMount{
				version: cgroupV2,
			},
			memberships: []cgroupMembership{
				{
					path:        "/docker/cpu",
					controllers: []string{"cpu"},
				},
				{
					path: "/docker/unified",
				},
			},
			want: cgroupMembership{
				path: "/docker/unified",
			},
			wantFound: true,
		},
		{
			name: "v1 matching controller",
			mount: cgroupMount{
				version:     cgroupV1,
				controllers: []string{"cpu", "cpuacct"},
			},
			memberships: []cgroupMembership{
				{
					path:        "/docker/memory",
					controllers: []string{"memory"},
				},
				{
					path:        "/docker/cpu",
					controllers: []string{"cpu", "cpuacct"},
				},
			},
			want: cgroupMembership{
				path:        "/docker/cpu",
				controllers: []string{"cpu", "cpuacct"},
			},
			wantFound: true,
		},
		{
			name: "v1 matching any controller",
			mount: cgroupMount{
				version:     cgroupV1,
				controllers: []string{"cpu", "cpuacct"},
			},
			memberships: []cgroupMembership{
				{
					path:        "/docker/cpu",
					controllers: []string{"cpu"},
				},
			},
			want: cgroupMembership{
				path:        "/docker/cpu",
				controllers: []string{"cpu"},
			},
			wantFound: true,
		},
		{
			name: "v1 no matching controller",
			mount: cgroupMount{
				version:     cgroupV1,
				controllers: []string{"cpu"},
			},
			memberships: []cgroupMembership{
				{
					path:        "/docker/memory",
					controllers: []string{"memory"},
				},
			},
			wantFound: false,
		},
		{
			name: "v1 empty mount controllers",
			mount: cgroupMount{
				version: cgroupV1,
			},
			memberships: []cgroupMembership{
				{
					path:        "/docker/cpu",
					controllers: []string{"cpu"},
				},
			},
			wantFound: false,
		},
		{
			name: "v1 empty membership controllers",
			mount: cgroupMount{
				version:     cgroupV1,
				controllers: []string{"cpu"},
			},
			memberships: []cgroupMembership{
				{
					path: "/docker/cpu",
				},
			},
			wantFound: false,
		},
		{
			name: "no memberships",
			mount: cgroupMount{
				version: cgroupV2,
			},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := findMembership(tt.mount, tt.memberships)

			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("membership = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMountHasController(t *testing.T) {
	tests := []struct {
		name       string
		mount      cgroupMount
		controller string
		want       bool
	}{
		{
			name: "v2 supports controller lookup",
			mount: cgroupMount{
				version: cgroupV2,
			},
			controller: "cpu",
			want:       true,
		},
		{
			name: "v2 supports unknown controller lookup",
			mount: cgroupMount{
				version: cgroupV2,
			},
			controller: "unknown",
			want:       true,
		},
		{
			name: "v1 controller exists",
			mount: cgroupMount{
				version:     cgroupV1,
				controllers: []string{"cpu", "cpuacct"},
			},
			controller: "cpu",
			want:       true,
		},
		{
			name: "v1 controller does not exist",
			mount: cgroupMount{
				version:     cgroupV1,
				controllers: []string{"memory"},
			},
			controller: "cpu",
			want:       false,
		},
		{
			name: "v1 empty controllers",
			mount: cgroupMount{
				version: cgroupV1,
			},
			controller: "cpu",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mountHasController(tt.mount, tt.controller)

			if got != tt.want {
				t.Errorf("mountHasController() = %v, want %v", got, tt.want)
			}
		})
	}
}
