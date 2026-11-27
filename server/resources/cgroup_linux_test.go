//go:build linux

package resources

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverCgroupLayoutFromFilesV2(t *testing.T) {
	directory := t.TempDir()

	mountInfoPath := filepath.Join(directory, "mountinfo")
	membershipPath := filepath.Join(directory, "cgroup")

	writeResourceTestFile(
		t,
		directory,
		"mountinfo",
		`29 23 0:26 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw`,
	)
	writeResourceTestFile(
		t,
		directory,
		"cgroup",
		`0::/docker/container-id`,
	)

	layout, err := discoverCgroupLayoutFromFiles(
		mountInfoPath,
		membershipPath,
	)
	if err != nil {
		t.Fatalf("discoverCgroupLayoutFromFiles() error = %v", err)
	}

	if len(layout.mounts) != 1 {
		t.Fatalf("mounts = %d, want 1", len(layout.mounts))
	}

	if len(layout.memberships) != 1 {
		t.Fatalf("memberships = %d, want 1", len(layout.memberships))
	}

	if layout.mounts[0].version != cgroupV2 {
		t.Errorf("mount version = %d, want %d", layout.mounts[0].version, cgroupV2)
	}

	if layout.memberships[0].path != "/docker/container-id" {
		t.Errorf(
			"membership path = %q, want %q",
			layout.memberships[0].path,
			"/docker/container-id",
		)
	}
}

func TestDiscoverCgroupLayoutFromFilesV1(t *testing.T) {
	directory := t.TempDir()

	mountInfoPath := filepath.Join(directory, "mountinfo")
	membershipPath := filepath.Join(directory, "cgroup")

	writeResourceTestFile(
		t,
		directory,
		"mountinfo",
		`30 23 0:27 / /sys/fs/cgroup/memory rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,memory`,
	)
	writeResourceTestFile(
		t,
		directory,
		"cgroup",
		`7:memory:/docker/container-id`,
	)

	layout, err := discoverCgroupLayoutFromFiles(
		mountInfoPath,
		membershipPath,
	)
	if err != nil {
		t.Fatalf("discoverCgroupLayoutFromFiles() error = %v", err)
	}

	if len(layout.mounts) != 1 {
		t.Fatalf("mounts = %d, want 1", len(layout.mounts))
	}

	if len(layout.memberships) != 1 {
		t.Fatalf("memberships = %d, want 1", len(layout.memberships))
	}

	if layout.mounts[0].version != cgroupV1 {
		t.Errorf("mount version = %d, want %d", layout.mounts[0].version, cgroupV1)
	}

	if !reflect.DeepEqual(
		layout.mounts[0].controllers,
		[]string{"memory"},
	) {
		t.Errorf(
			"mount controllers = %v, want [memory]",
			layout.mounts[0].controllers,
		)
	}
}

func TestDiscoverCgroupLayoutFromFilesMissingMountInfo(t *testing.T) {
	directory := t.TempDir()

	membershipPath := filepath.Join(directory, "cgroup")
	writeResourceTestFile(t, directory, "cgroup", `0::/docker/container-id`)

	_, err := discoverCgroupLayoutFromFiles(
		filepath.Join(directory, "missing-mountinfo"),
		membershipPath,
	)

	if !errors.Is(err, errCgroupLayoutUnavailable) {
		t.Fatalf(
			"error = %v, want errCgroupLayoutUnavailable",
			err,
		)
	}
}

func TestDiscoverCgroupLayoutFromFilesMissingMembership(t *testing.T) {
	directory := t.TempDir()

	mountInfoPath := filepath.Join(directory, "mountinfo")
	writeResourceTestFile(
		t,
		directory,
		"mountinfo",
		`29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw`,
	)

	_, err := discoverCgroupLayoutFromFiles(
		mountInfoPath,
		filepath.Join(directory, "missing-cgroup"),
	)

	if !errors.Is(err, errCgroupLayoutUnavailable) {
		t.Fatalf(
			"error = %v, want errCgroupLayoutUnavailable",
			err,
		)
	}
}

func TestDiscoverCgroupLayoutFromFilesNoCgroupMounts(t *testing.T) {
	directory := t.TempDir()

	mountInfoPath := filepath.Join(directory, "mountinfo")
	membershipPath := filepath.Join(directory, "cgroup")

	writeResourceTestFile(
		t,
		directory,
		"mountinfo",
		`28 23 0:25 / /proc rw,nosuid,nodev - proc proc rw`,
	)
	writeResourceTestFile(
		t,
		directory,
		"cgroup",
		`0::/docker/container-id`,
	)

	_, err := discoverCgroupLayoutFromFiles(
		mountInfoPath,
		membershipPath,
	)

	if !errors.Is(err, errCgroupLayoutUnavailable) {
		t.Fatalf(
			"error = %v, want errCgroupLayoutUnavailable",
			err,
		)
	}
}

func TestDiscoverCgroupLayoutFromFilesNoMemberships(t *testing.T) {
	directory := t.TempDir()

	mountInfoPath := filepath.Join(directory, "mountinfo")
	membershipPath := filepath.Join(directory, "cgroup")

	writeResourceTestFile(
		t,
		directory,
		"mountinfo",
		`29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw`,
	)
	writeResourceTestFile(t, directory, "cgroup", "")

	_, err := discoverCgroupLayoutFromFiles(
		mountInfoPath,
		membershipPath,
	)

	if !errors.Is(err, errCgroupLayoutUnavailable) {
		t.Fatalf(
			"error = %v, want errCgroupLayoutUnavailable",
			err,
		)
	}
}

func TestDiscoverCgroupLayoutFromFilesInvalidMountInfo(t *testing.T) {
	directory := t.TempDir()

	mountInfoPath := filepath.Join(directory, "mountinfo")
	membershipPath := filepath.Join(directory, "cgroup")

	writeResourceTestFile(
		t,
		directory,
		"mountinfo",
		`invalid mountinfo`,
	)
	writeResourceTestFile(
		t,
		directory,
		"cgroup",
		`0::/docker/container-id`,
	)

	_, err := discoverCgroupLayoutFromFiles(
		mountInfoPath,
		membershipPath,
	)

	if err == nil {
		t.Fatal("expected mountinfo parsing error")
	}

	if errors.Is(err, errCgroupLayoutUnavailable) {
		t.Fatalf("unexpected unavailable error: %v", err)
	}
}

func TestDiscoverCgroupLayoutFromFilesInvalidMembership(t *testing.T) {
	directory := t.TempDir()

	mountInfoPath := filepath.Join(directory, "mountinfo")
	membershipPath := filepath.Join(directory, "cgroup")

	writeResourceTestFile(
		t,
		directory,
		"mountinfo",
		`29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw`,
	)
	writeResourceTestFile(
		t,
		directory,
		"cgroup",
		`invalid-membership`,
	)

	_, err := discoverCgroupLayoutFromFiles(
		mountInfoPath,
		membershipPath,
	)

	if err == nil {
		t.Fatal("expected membership parsing error")
	}

	if errors.Is(err, errCgroupLayoutUnavailable) {
		t.Fatalf("unexpected unavailable error: %v", err)
	}
}

func TestParseCgroupMountInfoV2(t *testing.T) {
	input := `29 23 0:26 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw`

	mounts, err := parseCgroupMountInfo(input)
	if err != nil {
		t.Fatalf("parseCgroupMountInfo() error = %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}

	got := mounts[0]

	if got.version != cgroupV2 {
		t.Errorf("version = %d, want %d", got.version, cgroupV2)
	}

	if got.root != "/" {
		t.Errorf("root = %q, want %q", got.root, "/")
	}

	if got.mountPoint != "/sys/fs/cgroup" {
		t.Errorf(
			"mountPoint = %q, want %q",
			got.mountPoint,
			"/sys/fs/cgroup",
		)
	}

	if len(got.controllers) != 0 {
		t.Errorf("v2 controllers = %v, want empty", got.controllers)
	}
}

func TestParseCgroupMountInfoV1(t *testing.T) {
	input := `30 23 0:27 / /sys/fs/cgroup/memory rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,memory`

	mounts, err := parseCgroupMountInfo(input)
	if err != nil {
		t.Fatalf("parseCgroupMountInfo() error = %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}

	got := mounts[0]

	if got.version != cgroupV1 {
		t.Errorf("version = %d, want %d", got.version, cgroupV1)
	}

	if !reflect.DeepEqual(got.controllers, []string{"memory"}) {
		t.Errorf(
			"controllers = %v, want [memory]",
			got.controllers,
		)
	}
}

func TestParseCgroupMountInfoEscapedPath(t *testing.T) {
	input := `30 23 0:27 / /sys/fs/cgroup/my\040container rw - cgroup2 cgroup rw`

	mounts, err := parseCgroupMountInfo(input)
	if err != nil {
		t.Fatalf("parseCgroupMountInfo() error = %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}

	want := "/sys/fs/cgroup/my container"

	if mounts[0].mountPoint != want {
		t.Errorf(
			"mountPoint = %q, want %q",
			mounts[0].mountPoint,
			want,
		)
	}
}

func TestParseCgroupMountInfoIgnoresOtherFilesystems(t *testing.T) {
	input := `28 23 0:25 / /proc rw,nosuid,nodev - proc proc rw
29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw`

	mounts, err := parseCgroupMountInfo(input)
	if err != nil {
		t.Fatalf("parseCgroupMountInfo() error = %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 cgroup mount, got %d", len(mounts))
	}

	if mounts[0].version != cgroupV2 {
		t.Errorf("version = %d, want %d", mounts[0].version, cgroupV2)
	}
}

func TestParseCgroupMountInfoRejectsMalformedLine(t *testing.T) {
	input := `this is not valid mountinfo`

	_, err := parseCgroupMountInfo(input)
	if err == nil {
		t.Fatal("expected error for malformed mountinfo")
	}
}

func TestParseCgroupMembershipV2(t *testing.T) {
	input := `0::/docker/container-id`

	memberships, err := parseCgroupMembership(input)
	if err != nil {
		t.Fatalf("parseCgroupMembership() error = %v", err)
	}

	if len(memberships) != 1 {
		t.Fatalf("expected 1 membership, got %d", len(memberships))
	}

	got := memberships[0]

	if got.path != "/docker/container-id" {
		t.Errorf(
			"path = %q, want %q",
			got.path,
			"/docker/container-id",
		)
	}

	if len(got.controllers) != 0 {
		t.Errorf("controllers = %v, want empty", got.controllers)
	}
}

func TestParseCgroupMembershipV1(t *testing.T) {
	input := `5:cpu,cpuacct:/docker/container-id
7:memory:/docker/container-id`

	memberships, err := parseCgroupMembership(input)
	if err != nil {
		t.Fatalf("parseCgroupMembership() error = %v", err)
	}

	if len(memberships) != 2 {
		t.Fatalf("expected 2 memberships, got %d", len(memberships))
	}

	if !reflect.DeepEqual(
		memberships[0].controllers,
		[]string{"cpu", "cpuacct"},
	) {
		t.Errorf(
			"first controllers = %v, want [cpu cpuacct]",
			memberships[0].controllers,
		)
	}

	if !reflect.DeepEqual(
		memberships[1].controllers,
		[]string{"memory"},
	) {
		t.Errorf(
			"second controllers = %v, want [memory]",
			memberships[1].controllers,
		)
	}
}

func TestParseCgroupMembershipRejectsMalformedLine(t *testing.T) {
	input := `invalid-membership`

	_, err := parseCgroupMembership(input)
	if err == nil {
		t.Fatal("expected error for malformed membership")
	}
}

func TestResolveCgroupPathV2(t *testing.T) {
	mount := cgroupMount{
		root:       "/",
		mountPoint: "/sys/fs/cgroup",
		version:    cgroupV2,
	}

	membership := cgroupMembership{
		path: "/docker/container-id",
	}

	got, err := resolveCgroupPath(mount, membership)
	if err != nil {
		t.Fatalf("resolveCgroupPath() error = %v", err)
	}

	want := "/sys/fs/cgroup/docker/container-id"

	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestResolveCgroupPathV1(t *testing.T) {
	mount := cgroupMount{
		root:        "/",
		mountPoint:  "/sys/fs/cgroup/memory",
		controllers: []string{"memory"},
		version:     cgroupV1,
	}

	membership := cgroupMembership{
		path:        "/docker/container-id",
		controllers: []string{"memory"},
	}

	got, err := resolveCgroupPath(mount, membership)
	if err != nil {
		t.Fatalf("resolveCgroupPath() error = %v", err)
	}

	want := "/sys/fs/cgroup/memory/docker/container-id"

	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestResolveCgroupPathWithMountRoot(t *testing.T) {
	mount := cgroupMount{
		root:       "/docker/container-id",
		mountPoint: "/sys/fs/cgroup",
		version:    cgroupV2,
	}

	membership := cgroupMembership{
		path: "/docker/container-id/child",
	}

	got, err := resolveCgroupPath(mount, membership)
	if err != nil {
		t.Fatalf("resolveCgroupPath() error = %v", err)
	}

	want := "/sys/fs/cgroup/child"

	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestResolveCgroupPathRejectsMembershipOutsideMountRoot(t *testing.T) {
	mount := cgroupMount{
		root:       "/docker/container-a",
		mountPoint: "/sys/fs/cgroup",
		version:    cgroupV2,
	}

	membership := cgroupMembership{
		path: "/docker/container-b",
	}

	_, err := resolveCgroupPath(mount, membership)
	if err == nil {
		t.Fatal("expected error for membership outside mount root")
	}
}

func TestResolveCgroupPathRejectsDifferentControllers(t *testing.T) {
	mount := cgroupMount{
		root:        "/",
		mountPoint:  "/sys/fs/cgroup/cpu",
		controllers: []string{"cpu", "cpuacct"},
		version:     cgroupV1,
	}

	membership := cgroupMembership{
		path:        "/docker/container-id",
		controllers: []string{"memory"},
	}

	_, err := resolveCgroupPath(mount, membership)
	if err == nil {
		t.Fatal("expected error for different controllers")
	}
}

func TestResolveCgroupPathRejectsV1MembershipOnV2Mount(t *testing.T) {
	mount := cgroupMount{
		root:       "/",
		mountPoint: "/sys/fs/cgroup",
		version:    cgroupV2,
	}

	membership := cgroupMembership{
		path:        "/docker/container-id",
		controllers: []string{"cpu"},
	}

	_, err := resolveCgroupPath(mount, membership)
	if err == nil {
		t.Fatal("expected error for incompatible cgroup versions")
	}
}

func TestUnescapeMountInfoPath(t *testing.T) {
	input := `/path/with\040space\011tab\012newline\134backslash`

	want := "/path/with space\ttab\nnewline\\backslash"

	got := unescapeMountInfoPath(input)

	if got != want {
		t.Errorf("unescaped path = %q, want %q", got, want)
	}
}
