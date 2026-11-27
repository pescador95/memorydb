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

func readAvailableMemoryFromFile(path string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())

		if len(fields) < 2 || fields[0] != "MemAvailable:" {
			continue
		}

		kilobytes, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || kilobytes <= 0 {
			return 0, fmt.Errorf("invalid MemAvailable value")
		}

		if kilobytes > int64(^uint64(0)>>1)/1024 {
			return 0, fmt.Errorf("MemAvailable value overflows int64")
		}

		return kilobytes * 1024, nil
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}

	return 0, fmt.Errorf("MemAvailable not found in %s", path)
}

func readMemoryLimitV2At(directory string) (int64, bool, error) {
	limitPath := filepath.Join(directory, "memory.max")

	limit, unlimited, err := readIntegerOrMax(limitPath)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	if unlimited {
		return 0, false, nil
	}

	if limit <= 0 {
		return 0, false, fmt.Errorf(
			"invalid memory limit in %s: limit must be positive",
			directory,
		)
	}

	usagePath := filepath.Join(directory, "memory.current")
	usage, err := readIntegerFile(usagePath)
	if err != nil {
		return 0, false, err
	}

	return calculateAvailableMemory(limit, usage, directory)
}

func readMemoryLimitV1At(directory string) (int64, bool, error) {
	limitPath := filepath.Join(directory, "memory.limit_in_bytes")

	limit, err := readIntegerFile(limitPath)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	if limit <= 0 || limit >= 1<<60 {
		return 0, false, nil
	}

	usagePath := filepath.Join(directory, "memory.usage_in_bytes")
	usage, err := readIntegerFile(usagePath)
	if err != nil {
		return 0, false, err
	}

	return calculateAvailableMemory(limit, usage, directory)
}

func calculateAvailableMemory(
	limit,
	usage int64,
	directory string,
) (int64, bool, error) {
	if limit <= 0 || usage < 0 {
		return 0, false, fmt.Errorf("invalid memory values in %s", directory)
	}

	if usage >= limit {
		return 0, false, fmt.Errorf("memory limit exhausted in %s", directory)
	}

	return limit - usage, true, nil
}

func readHostAvailableMemory() (int64, error) {
	return readAvailableMemoryFromFile("/proc/meminfo")
}

func readIntegerFile(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	value, err := strconv.ParseInt(
		strings.TrimSpace(string(data)),
		10,
		64,
	)
	if err != nil {
		return 0, fmt.Errorf("invalid integer in %s: %w", path, err)
	}

	return value, nil
}

// readIntegerOrMax distingue um limite numérico do valor especial "max".
func readIntegerOrMax(path string) (int64, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false, err
	}

	value := strings.TrimSpace(string(data))
	if value == "max" {
		return 0, true, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid value in %s: %w", path, err)
	}

	return parsed, false, nil
}
