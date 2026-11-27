//go:build linux

package resources

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	cpuSafetyPercent    int64 = 10
	memorySafetyPercent int64 = 20
)

type detectedLimit struct {
	capacity      int64
	fromContainer bool
}

func detectResources() (DetectedResources, error) {
	layout, layoutErr := discoverCgroupLayout()
	return detectResourcesFromLayout(layout, layoutErr)
}

func detectResourcesFromLayout(
	layout cgroupLayout,
	layoutErr error,
) (DetectedResources, error) {
	if layoutErr != nil {
		if !errors.Is(layoutErr, errCgroupLayoutUnavailable) {
			return DetectedResources{}, fmt.Errorf(
				"%w: discover cgroup layout: %w",
				ErrResourceDetection,
				layoutErr,
			)
		}

		// Sem metadados de cgroups, a detecção continua utilizando
		// os recursos disponíveis no host.
		layout = cgroupLayout{}
	}

	cpu, err := detectLinuxCPU(layout)
	if err != nil {
		return DetectedResources{}, fmt.Errorf(
			"%w: CPU: %w",
			ErrResourceDetection,
			err,
		)
	}

	memory, err := detectLinuxMemory(layout)
	if err != nil {
		return DetectedResources{}, fmt.Errorf(
			"%w: memory: %w",
			ErrResourceDetection,
			err,
		)
	}

	cpuSource := ResourceSourceHost
	if cpu.fromContainer {
		cpuSource = ResourceSourceContainer
	}

	memorySource := ResourceSourceHost
	if memory.fromContainer {
		memorySource = ResourceSourceContainer
	}

	result := DetectedResources{
		CPUCapacity: applySafetyMargin(
			cpu.capacity,
			cpuSafetyPercent,
		),
		MemoryCapacity: applySafetyMargin(
			memory.capacity,
			memorySafetyPercent,
		),
		CPUSource:    cpuSource,
		MemorySource: memorySource,
	}

	if err := result.Validate(); err != nil {
		return DetectedResources{}, err
	}

	return result, nil
}

func detectLinuxCPU(layout cgroupLayout) (detectedLimit, error) {
	hostCapacity := int64(runtime.NumCPU()) * 1000
	if hostCapacity <= 0 {
		return detectedLimit{}, fmt.Errorf("invalid host CPU capacity")
	}

	minimum := hostCapacity

	for _, mount := range layout.mounts {
		if !mountHasController(mount, "cpu") {
			continue
		}

		membership, found := findMembership(
			mount,
			layout.memberships,
		)
		if !found {
			continue
		}

		groupPath, err := resolveCgroupPath(mount, membership)
		if err != nil {
			return detectedLimit{}, fmt.Errorf(
				"resolve CPU cgroup path: %w",
				err,
			)
		}

		directories, err := cgroupDirectoriesWithinMount(
			mount.mountPoint,
			groupPath,
		)
		if err != nil {
			return detectedLimit{}, err
		}

		for _, directory := range directories {
			var capacity int64
			var foundLimit bool

			switch mount.version {
			case cgroupV2:
				capacity, foundLimit, err = readCPUQuotaV2At(directory)
			case cgroupV1:
				capacity, foundLimit, err = readCPUQuotaV1At(directory)
			default:
				continue
			}

			if err != nil {
				return detectedLimit{}, err
			}

			if foundLimit && capacity < minimum {
				minimum = capacity
			}
		}
	}

	return detectedLimit{
		capacity:      minimum,
		fromContainer: minimum < hostCapacity,
	}, nil
}

func detectLinuxMemory(layout cgroupLayout) (detectedLimit, error) {
	hostMemory, err := readHostAvailableMemory()
	if err != nil {
		return detectedLimit{}, err
	}

	minimum := hostMemory

	for _, mount := range layout.mounts {
		if !mountHasController(mount, "memory") {
			continue
		}

		membership, found := findMembership(
			mount,
			layout.memberships,
		)
		if !found {
			continue
		}

		groupPath, err := resolveCgroupPath(mount, membership)
		if err != nil {
			return detectedLimit{}, fmt.Errorf(
				"resolve memory cgroup path: %w",
				err,
			)
		}

		directories, err := cgroupDirectoriesWithinMount(
			mount.mountPoint,
			groupPath,
		)
		if err != nil {
			return detectedLimit{}, err
		}

		for _, directory := range directories {
			var available int64
			var foundLimit bool

			switch mount.version {
			case cgroupV2:
				available, foundLimit, err = readMemoryLimitV2At(directory)
			case cgroupV1:
				available, foundLimit, err = readMemoryLimitV1At(directory)
			default:
				continue
			}

			if err != nil {
				return detectedLimit{}, err
			}

			if foundLimit && available < minimum {
				minimum = available
			}
		}
	}

	return detectedLimit{
		capacity:      minimum,
		fromContainer: minimum < hostMemory,
	}, nil
}

// findMembership encontra a membership correspondente à montagem.
func findMembership(
	mount cgroupMount,
	memberships []cgroupMembership,
) (cgroupMembership, bool) {
	for _, membership := range memberships {
		if mount.version == cgroupV2 {
			if len(membership.controllers) == 0 {
				return membership, true
			}
			continue
		}

		if hasControllerIntersection(
			mount.controllers,
			membership.controllers,
		) {
			return membership, true
		}
	}

	return cgroupMembership{}, false
}

func mountHasController(mount cgroupMount, controller string) bool {
	if mount.version == cgroupV2 {
		// Em cgroup v2, os controllers são compartilhados
		// na hierarquia unificada. A ausência de um arquivo
		// específico será tratada como ausência daquele limite.
		return true
	}

	for _, candidate := range mount.controllers {
		if candidate == controller {
			return true
		}
	}

	return false
}

// cgroupDirectoriesWithinMount retorna o diretório atual e seus ancestrais,
// sem ultrapassar a raiz da montagem.
func cgroupDirectoriesWithinMount(
	mountPoint,
	groupPath string,
) ([]string, error) {
	mountPoint = filepath.Clean(mountPoint)
	groupPath = filepath.Clean(groupPath)

	if !filepath.IsAbs(mountPoint) || !filepath.IsAbs(groupPath) {
		return nil, fmt.Errorf("cgroup paths must be absolute")
	}

	relative, err := filepath.Rel(mountPoint, groupPath)
	if err != nil {
		return nil, fmt.Errorf("resolve cgroup directory: %w", err)
	}

	if relative == ".." ||
		strings.HasPrefix(
			relative,
			".."+string(filepath.Separator),
		) {
		return nil, fmt.Errorf(
			"cgroup path %q escapes mount point %q",
			groupPath,
			mountPoint,
		)
	}

	directories := make([]string, 0, 8)
	current := groupPath

	for {
		directories = append(directories, current)

		if current == mountPoint {
			break
		}

		parent := filepath.Dir(current)
		if parent == current {
			return nil, fmt.Errorf("could not reach cgroup mount point")
		}

		current = parent
	}

	return directories, nil
}

func applySafetyMargin(value, percentage int64) int64 {
	if value <= 0 || percentage <= 0 {
		return value
	}

	if percentage >= 100 {
		return 0
	}

	margin := (value/100)*percentage +
		((value%100)*percentage)/100

	return value - margin
}

func readCPUQuotaV2At(directory string) (int64, bool, error) {
	path := filepath.Join(directory, "cpu.max")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read %s: %w", path, err)
	}

	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return 0, false, fmt.Errorf("invalid cpu.max in %s", directory)
	}

	if fields[0] == "max" {
		return 0, false, nil
	}

	quota, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || quota <= 0 {
		return 0, false, fmt.Errorf("invalid CPU quota in %s", directory)
	}

	period, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || period <= 0 {
		return 0, false, fmt.Errorf("invalid CPU period in %s", directory)
	}

	return calculateCPUCapacity(quota, period)
}

func readCPUQuotaV1At(directory string) (int64, bool, error) {
	quotaPath := filepath.Join(directory, "cpu.cfs_quota_us")

	quota, err := readIntegerFile(quotaPath)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	if quota < 0 {
		return 0, false, nil
	}
	if quota == 0 {
		return 0, false, fmt.Errorf("invalid CPU quota in %s", directory)
	}

	periodPath := filepath.Join(directory, "cpu.cfs_period_us")
	period, err := readIntegerFile(periodPath)
	if err != nil {
		return 0, false, err
	}
	if period <= 0 {
		return 0, false, fmt.Errorf("invalid CPU period in %s", directory)
	}

	return calculateCPUCapacity(quota, period)
}

// calculateCPUCapacity converte quota/period para millicores.
// Usa big.Int para evitar overflow em quota*1000.
func calculateCPUCapacity(
	quota,
	period int64,
) (int64, bool, error) {
	if quota <= 0 || period <= 0 {
		return 0, false, fmt.Errorf("quota and period must be positive")
	}

	var numerator, denominator big.Int

	numerator.Mul(big.NewInt(quota), big.NewInt(1000))
	denominator.SetInt64(period)
	numerator.Div(&numerator, &denominator)

	if !numerator.IsInt64() {
		return int64(^uint64(0) >> 1), true, nil
	}

	capacity := numerator.Int64()
	if capacity <= 0 {
		capacity = 1
	}

	return capacity, true, nil
}
