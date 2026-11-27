//go:build linux

package resources

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseCgroupMountInfoEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			name:  "blank lines",
			input: "\n  \t\n",
		},
		{
			name:    "incomplete mountinfo prefix",
			input:   `1 2 0:1 / /sys/fs/cgroup - cgroup2 cgroup rw`,
			wantErr: true,
		},
		{
			name:    "incomplete filesystem data",
			input:   `1 2 0:1 / /sys/fs/cgroup rw - cgroup2 cgroup`,
			wantErr: true,
		},
		{
			name:    "relative cgroup root",
			input:   `1 2 0:1 relative /sys/fs/cgroup rw - cgroup2 cgroup rw`,
			wantErr: true,
		},
		{
			name:    "relative cgroup mount point",
			input:   `1 2 0:1 / relative rw - cgroup2 cgroup rw`,
			wantErr: true,
		},
		{
			name:  "v1 mount without recognized controllers",
			input: `1 2 0:1 / /sys/fs/cgroup rw - cgroup cgroup rw,unknown_controller`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mounts, err := parseCgroupMountInfo(test.input)

			if test.wantErr {
				if err == nil {
					t.Fatal("expected parsing error")
				}
				return
			}

			if err != nil {
				t.Fatalf("parseCgroupMountInfo() error = %v", err)
			}

			if test.name == "v1 mount without recognized controllers" && len(mounts) != 0 {
				t.Fatalf("mounts = %v, want no mounts", mounts)
			}
		})
	}
}

func TestParseCgroupMountInfoScannerError(t *testing.T) {
	input := strings.Repeat("x", 1024*1024+1)

	_, err := parseCgroupMountInfo(input)
	if err == nil {
		t.Fatal("expected scanner error for oversized line")
	}

	if !strings.Contains(err.Error(), "scan mountinfo") {
		t.Fatalf("error = %v, want scanner error context", err)
	}
}

func TestParseCgroupMembershipEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			name:  "blank lines",
			input: "\n  \t\n",
		},
		{
			name:    "invalid hierarchy ID",
			input:   `invalid::/docker/container`,
			wantErr: true,
		},
		{
			name:    "relative membership path",
			input:   `0::docker/container`,
			wantErr: true,
		},
		{
			name:  "empty controller list",
			input: `5:,, :/docker/container`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			memberships, err := parseCgroupMembership(test.input)

			if test.wantErr {
				if err == nil {
					t.Fatal("expected parsing error")
				}
				return
			}

			if err != nil {
				t.Fatalf("parseCgroupMembership() error = %v", err)
			}

			if (test.input == "" || strings.TrimSpace(test.input) == "") && len(memberships) != 0 {
				t.Fatalf("memberships = %v, want no memberships", memberships)
			}

			if test.name == "empty controller list" {
				if len(memberships) != 1 {
					t.Fatalf("memberships = %d, want 1", len(memberships))
				}

				if len(memberships[0].controllers) != 0 {
					t.Errorf(
						"controllers = %v, want empty",
						memberships[0].controllers,
					)
				}
			}
		})
	}
}

func TestParseCgroupMembershipScannerError(t *testing.T) {
	input := strings.Repeat("x", 1024*1024+1)

	_, err := parseCgroupMembership(input)
	if err == nil {
		t.Fatal("expected scanner error for oversized line")
	}

	if !strings.Contains(err.Error(), "scan cgroup membership") {
		t.Fatalf("error = %v, want scanner error context", err)
	}
}

func TestResolveCgroupPathEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		mount      cgroupMount
		membership cgroupMembership
	}{
		{
			name: "unsupported version",
			mount: cgroupMount{
				root:       "/",
				mountPoint: "/sys/fs/cgroup",
				version:    3,
			},
			membership: cgroupMembership{
				path: "/docker/container",
			},
		},
		{
			name: "relative mount root",
			mount: cgroupMount{
				root:       "relative",
				mountPoint: "/sys/fs/cgroup",
				version:    cgroupV2,
			},
			membership: cgroupMembership{
				path: "/docker/container",
			},
		},
		{
			name: "relative mount point",
			mount: cgroupMount{
				root:       "/",
				mountPoint: "relative",
				version:    cgroupV2,
			},
			membership: cgroupMembership{
				path: "/docker/container",
			},
		},
		{
			name: "relative membership path",
			mount: cgroupMount{
				root:       "/",
				mountPoint: "/sys/fs/cgroup",
				version:    cgroupV2,
			},
			membership: cgroupMembership{
				path: "docker/container",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveCgroupPath(test.mount, test.membership)
			if err == nil {
				t.Fatal("expected resolution error")
			}
		})
	}
}

func TestSplitControllersEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "only whitespace",
			input: " \t ",
			want:  nil,
		},
		{
			name:  "empty elements",
			input: ",cpu,,memory,",
			want:  []string{"cpu", "memory"},
		},
		{
			name:  "trim surrounding whitespace",
			input: " cpu , memory ",
			want:  []string{"cpu", "memory"},
		},
		{
			name:  "single controller",
			input: "cpu",
			want:  []string{"cpu"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := splitControllers(test.input)

			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("splitControllers(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

func TestResolveCgroupPathNormalizesPaths(t *testing.T) {
	mount := cgroupMount{
		root:       "/docker/./container",
		mountPoint: "/sys/fs/cgroup/.",
		version:    cgroupV2,
	}

	membership := cgroupMembership{
		path: "/docker/container/child/..",
	}

	got, err := resolveCgroupPath(mount, membership)
	if err != nil {
		t.Fatalf("resolveCgroupPath() error = %v", err)
	}

	want := filepath.Clean("/sys/fs/cgroup")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}
