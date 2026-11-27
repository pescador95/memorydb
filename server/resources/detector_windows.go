//go:build windows

package resources

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	windowsCPUSafetyMargin    int64 = 10
	windowsMemorySafetyMargin int64 = 20
	windowsMillicoresPerCPU   int64 = 1000
)

var (
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")

	globalMemoryStatusExProc = kernel32DLL.NewProc("GlobalMemoryStatusEx")

	findGlobalMemoryStatusEx = func() error {
		return globalMemoryStatusExProc.Find()
	}

	callGlobalMemoryStatusEx = func(status *memoryStatusEx) (uintptr, error) {
		result, _, callErr := globalMemoryStatusExProc.Call(
			uintptr(unsafe.Pointer(status)),
		)
		return result, callErr
	}
)

// memoryStatusEx representa a estrutura MEMORYSTATUSEX da API do Windows.
type memoryStatusEx struct {
	length            uint32
	memoryLoad        uint32
	totalPhysical     uint64
	availablePhysical uint64
	totalPageFile     uint64
	availablePageFile uint64
	totalVirtual      uint64
	availableVirtual  uint64
	availableExtended uint64
}

// detectResources detecta os recursos disponíveis no host Windows.
//
// Esta implementação ainda não consulta limites de Windows Job Objects.
// Portanto, a origem é identificada como host.
func detectResources() (DetectedResources, error) {
	cpuCount := runtime.NumCPU()
	if cpuCount <= 0 {
		return DetectedResources{}, fmt.Errorf(
			"%w: invalid logical CPU count",
			ErrResourceDetection,
		)
	}

	cpuCapacity := int64(cpuCount) * windowsMillicoresPerCPU

	memoryCapacity, err := detectAvailablePhysicalMemory()
	if err != nil {
		return DetectedResources{}, fmt.Errorf(
			"%w: detect physical memory: %w",
			ErrResourceDetection,
			err,
		)
	}

	detected := DetectedResources{
		CPUCapacity: applyWindowsSafetyMargin(
			cpuCapacity,
			windowsCPUSafetyMargin,
		),
		MemoryCapacity: applyWindowsSafetyMargin(
			memoryCapacity,
			windowsMemorySafetyMargin,
		),
		CPUSource:    ResourceSourceHost,
		MemorySource: ResourceSourceHost,
	}

	if err := detected.Validate(); err != nil {
		return DetectedResources{}, fmt.Errorf(
			"%w: %w",
			ErrResourceDetection,
			err,
		)
	}

	return detected, nil
}

// detectAvailablePhysicalMemory retorna a memória física disponível
// para o sistema operacional no momento da consulta.
func detectAvailablePhysicalMemory() (int64, error) {
	if err := findGlobalMemoryStatusEx(); err != nil {
		return 0, fmt.Errorf(
			"find GlobalMemoryStatusEx: %w",
			err,
		)
	}

	status := memoryStatusEx{
		length: uint32(unsafe.Sizeof(memoryStatusEx{})),
	}

	result, callErr := callGlobalMemoryStatusEx(&status)

	if result == 0 {
		if callErr != syscall.Errno(0) {
			return 0, fmt.Errorf(
				"GlobalMemoryStatusEx: %w",
				callErr,
			)
		}

		return 0, errors.New(
			"GlobalMemoryStatusEx returned failure",
		)
	}

	if status.availablePhysical == 0 {
		return 0, errors.New(
			"no available physical memory reported",
		)
	}

	if status.availablePhysical > math.MaxInt64 {
		return 0, errors.New(
			"available physical memory exceeds int64 range",
		)
	}

	return int64(status.availablePhysical), nil
}

// applyWindowsSafetyMargin desconta uma porcentagem do recurso detectado.
func applyWindowsSafetyMargin(
	value,
	marginPercent int64,
) int64 {
	if value <= 0 || marginPercent <= 0 {
		return value
	}

	if marginPercent >= 100 {
		return 0
	}

	margin := value/100*marginPercent +
		value%100*marginPercent/100

	return value - margin
}
