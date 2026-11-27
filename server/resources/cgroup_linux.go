//go:build linux

package resources

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	cgroupV1 = 1
	cgroupV2 = 2
)

var knownCgroupControllers = map[string]struct{}{
	"cpu":        {},
	"cpuacct":    {},
	"cpuset":     {},
	"memory":     {},
	"blkio":      {},
	"io":         {},
	"devices":    {},
	"freezer":    {},
	"net_cls":    {},
	"net_prio":   {},
	"perf_event": {},
	"hugetlb":    {},
	"pids":       {},
	"rdma":       {},
	"misc":       {},
}

// discoverCgroupLayout identifica as montagens e a hierarquia cgroup
// às quais o processo atual pertence.
func discoverCgroupLayout() (cgroupLayout, error) {
	return discoverCgroupLayoutFromFiles(
		"/proc/self/mountinfo",
		"/proc/self/cgroup",
	)
}

// discoverCgroupLayoutFromFiles permite descobrir o layout a partir de
// arquivos informados, mantendo a lógica independente dos caminhos de /proc.
func discoverCgroupLayoutFromFiles(
	mountInfoPath,
	membershipPath string,
) (cgroupLayout, error) {
	mountInfo, err := os.ReadFile(mountInfoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cgroupLayout{}, fmt.Errorf(
				"%w: %s does not exist",
				errCgroupLayoutUnavailable,
				mountInfoPath,
			)
		}

		return cgroupLayout{}, fmt.Errorf(
			"read cgroup mountinfo: %w",
			err,
		)
	}

	membershipInfo, err := os.ReadFile(membershipPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cgroupLayout{}, fmt.Errorf(
				"%w: %s does not exist",
				errCgroupLayoutUnavailable,
				membershipPath,
			)
		}

		return cgroupLayout{}, fmt.Errorf(
			"read cgroup membership: %w",
			err,
		)
	}

	mounts, err := parseCgroupMountInfo(string(mountInfo))
	if err != nil {
		return cgroupLayout{}, fmt.Errorf(
			"parse cgroup mountinfo: %w",
			err,
		)
	}

	memberships, err := parseCgroupMembership(string(membershipInfo))
	if err != nil {
		return cgroupLayout{}, fmt.Errorf(
			"parse cgroup membership: %w",
			err,
		)
	}

	if len(mounts) == 0 {
		return cgroupLayout{}, fmt.Errorf(
			"%w: no cgroup mounts found",
			errCgroupLayoutUnavailable,
		)
	}

	if len(memberships) == 0 {
		return cgroupLayout{}, fmt.Errorf(
			"%w: no cgroup memberships found",
			errCgroupLayoutUnavailable,
		)
	}

	return cgroupLayout{
		mounts:      mounts,
		memberships: memberships,
	}, nil
}

// parseCgroupMountInfo interpreta as linhas de /proc/self/mountinfo.
//
// O formato relevante é:
//
// mountID parentID major:minor root mountPoint mountOptions ...
// - filesystem source superOptions
func parseCgroupMountInfo(data string) ([]cgroupMount, error) {
	var mounts []cgroupMount

	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1024*1024)

	lineNumber := 0

	for scanner.Scan() {
		lineNumber++

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		separator := strings.Index(line, " - ")
		if separator < 0 {
			return nil, fmt.Errorf(
				"line %d: missing mountinfo separator",
				lineNumber,
			)
		}

		beforeSeparator := strings.Fields(line[:separator])
		afterSeparator := strings.Fields(line[separator+3:])

		if len(beforeSeparator) < 6 {
			return nil, fmt.Errorf(
				"line %d: incomplete mountinfo prefix",
				lineNumber,
			)
		}

		if len(afterSeparator) < 3 {
			return nil, fmt.Errorf(
				"line %d: incomplete mountinfo filesystem data",
				lineNumber,
			)
		}

		filesystem := afterSeparator[0]

		var version int
		switch filesystem {
		case "cgroup":
			version = cgroupV1
		case "cgroup2":
			version = cgroupV2
		default:
			continue
		}

		root := unescapeMountInfoPath(beforeSeparator[3])
		mountPoint := unescapeMountInfoPath(beforeSeparator[4])

		if !filepath.IsAbs(root) {
			return nil, fmt.Errorf(
				"line %d: cgroup mount root is not absolute",
				lineNumber,
			)
		}

		if !filepath.IsAbs(mountPoint) {
			return nil, fmt.Errorf(
				"line %d: cgroup mount point is not absolute",
				lineNumber,
			)
		}

		mount := cgroupMount{
			root:       filepath.Clean(root),
			mountPoint: filepath.Clean(mountPoint),
			version:    version,
		}

		if version == cgroupV1 {
			mount.controllers = extractCgroupControllers(
				beforeSeparator[5],
				afterSeparator[2],
			)

			if len(mount.controllers) == 0 {
				continue
			}
		}

		mounts = append(mounts, mount)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan mountinfo: %w", err)
	}

	return mounts, nil
}

// parseCgroupMembership interpreta /proc/self/cgroup.
//
// Exemplos:
//
// 0::/docker/container-id
// 5:cpu,cpuacct:/docker/container-id
// 7:memory:/docker/container-id
func parseCgroupMembership(data string) ([]cgroupMembership, error) {
	var memberships []cgroupMembership

	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1024*1024)

	lineNumber := 0

	for scanner.Scan() {
		lineNumber++

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf(
				"line %d: invalid cgroup membership format",
				lineNumber,
			)
		}

		if _, err := strconv.Atoi(parts[0]); err != nil {
			return nil, fmt.Errorf(
				"line %d: invalid hierarchy ID: %w",
				lineNumber,
				err,
			)
		}

		path := parts[2]

		if !filepath.IsAbs(path) {
			return nil, fmt.Errorf(
				"line %d: membership path is not absolute",
				lineNumber,
			)
		}

		membership := cgroupMembership{
			path: filepath.Clean(path),
		}

		if parts[1] != "" {
			membership.controllers = splitControllers(parts[1])
		}

		memberships = append(memberships, membership)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan cgroup membership: %w", err)
	}

	return memberships, nil
}

// resolveCgroupPath associa uma montagem a uma membership e retorna
// o caminho correspondente dentro do filesystem montado.
func resolveCgroupPath(
	mount cgroupMount,
	membership cgroupMembership,
) (string, error) {
	if mount.version != cgroupV1 && mount.version != cgroupV2 {
		return "", fmt.Errorf(
			"unsupported cgroup version: %d",
			mount.version,
		)
	}

	if !filepath.IsAbs(mount.root) ||
		!filepath.IsAbs(mount.mountPoint) ||
		!filepath.IsAbs(membership.path) {
		return "", fmt.Errorf("cgroup paths must be absolute")
	}

	if mount.version == cgroupV2 {
		if len(membership.controllers) != 0 {
			return "", fmt.Errorf("v2 mount cannot match v1 membership")
		}
	} else if !hasControllerIntersection(
		mount.controllers,
		membership.controllers,
	) {
		return "", fmt.Errorf("mount and membership controllers do not match")
	}

	root := filepath.Clean(mount.root)
	membershipPath := filepath.Clean(membership.path)
	mountPoint := filepath.Clean(mount.mountPoint)

	relativePath, err := filepath.Rel(root, membershipPath)
	if err != nil {
		return "", fmt.Errorf(
			"resolve membership relative to mount root: %w",
			err,
		)
	}

	if relativePath == ".." ||
		strings.HasPrefix(
			relativePath,
			".."+string(filepath.Separator),
		) {
		return "", fmt.Errorf(
			"membership is outside the mounted cgroup subtree",
		)
	}

	resolvedPath := filepath.Clean(
		filepath.Join(mountPoint, relativePath),
	)

	relativeToMount, err := filepath.Rel(
		mountPoint,
		resolvedPath,
	)
	if err != nil {
		return "", fmt.Errorf(
			"validate resolved cgroup path: %w",
			err,
		)
	}

	if relativeToMount == ".." ||
		strings.HasPrefix(
			relativeToMount,
			".."+string(filepath.Separator),
		) {
		return "", fmt.Errorf("resolved cgroup path escapes mount point")
	}

	return resolvedPath, nil
}

func extractCgroupControllers(
	mountOptions,
	superOptions string,
) []string {
	candidates := append(
		splitControllers(mountOptions),
		splitControllers(superOptions)...,
	)

	seen := make(map[string]struct{}, len(candidates))
	controllers := make([]string, 0, len(candidates))

	for _, controller := range candidates {
		if _, known := knownCgroupControllers[controller]; !known &&
			!strings.HasPrefix(controller, "name=") {
			continue
		}

		if _, exists := seen[controller]; exists {
			continue
		}

		seen[controller] = struct{}{}
		controllers = append(controllers, controller)
	}

	return controllers
}

func splitControllers(value string) []string {
	if value == "" {
		return nil
	}

	var controllers []string

	for _, item := range strings.Split(value, ",") {
		controller := strings.TrimSpace(item)
		if controller == "" {
			continue
		}

		controllers = append(controllers, controller)
	}

	return controllers
}

func hasControllerIntersection(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}

	controllers := make(map[string]struct{}, len(left))

	for _, controller := range left {
		controllers[controller] = struct{}{}
	}

	for _, controller := range right {
		if _, exists := controllers[controller]; exists {
			return true
		}
	}

	return false
}

// unescapeMountInfoPath decodifica os escapes octais usados pelo
// /proc/self/mountinfo nos campos de caminho.
func unescapeMountInfoPath(value string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)

	return replacer.Replace(value)
}
