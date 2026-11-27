//go:build windows

package resources

import (
	"errors"
	"math"
	"syscall"
	"testing"
)

func TestDetectResources(t *testing.T) {
	detected, err := detectResources()
	if err != nil {
		t.Fatalf("detectResources() error = %v", err)
	}

	if err := detected.Validate(); err != nil {
		t.Fatalf("detected resources are invalid: %v", err)
	}

	if detected.CPUCapacity <= 0 {
		t.Errorf(
			"CPUCapacity = %d, want > 0",
			detected.CPUCapacity,
		)
	}

	if detected.MemoryCapacity <= 0 {
		t.Errorf(
			"MemoryCapacity = %d, want > 0",
			detected.MemoryCapacity,
		)
	}

	if detected.CPUSource != ResourceSourceHost {
		t.Errorf(
			"CPUSource = %q, want %q",
			detected.CPUSource,
			ResourceSourceHost,
		)
	}

	if detected.MemorySource != ResourceSourceHost {
		t.Errorf(
			"MemorySource = %q, want %q",
			detected.MemorySource,
			ResourceSourceHost,
		)
	}
}

func TestDetectAvailablePhysicalMemory(t *testing.T) {
	tests := []struct {
		name      string
		findErr   error
		result    uintptr
		callErr   error
		available uint64
		want      int64
		wantErr   bool
	}{
		{
			name:    "find function failure",
			findErr: errors.New("function not found"),
			wantErr: true,
		},
		{
			name:    "native call failure",
			result:  0,
			callErr: syscall.Errno(5),
			wantErr: true,
		},
		{
			name:    "native call failure without errno",
			result:  0,
			callErr: syscall.Errno(0),
			wantErr: true,
		},
		{
			name:      "zero available memory",
			result:    1,
			available: 0,
			wantErr:   true,
		},
		{
			name:      "available memory exceeds int64",
			result:    1,
			available: uint64(math.MaxInt64) + 1,
			wantErr:   true,
		},
		{
			name:      "success",
			result:    1,
			available: 8 << 30,
			want:      8 << 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setWindowsMemoryAPI(t, tt.findErr, func(status *memoryStatusEx) (uintptr, error) {
				status.availablePhysical = tt.available
				return tt.result, tt.callErr
			})

			got, err := detectAvailablePhysicalMemory()

			if (err != nil) != tt.wantErr {
				t.Fatalf("detectAvailablePhysicalMemory() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("detectAvailablePhysicalMemory() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDetectResourcesPropagatesMemoryDetectionFailure(t *testing.T) {
	setWindowsMemoryAPI(
		t,
		nil,
		func(*memoryStatusEx) (uintptr, error) {
			return 0, syscall.Errno(5)
		},
	)

	_, err := detectResources()
	if !errors.Is(err, ErrResourceDetection) {
		t.Fatalf("expected ErrResourceDetection, got %v", err)
	}
}

func setWindowsMemoryAPI(
	t *testing.T,
	findErr error,
	call func(*memoryStatusEx) (uintptr, error),
) {
	t.Helper()

	originalFind := findGlobalMemoryStatusEx
	originalCall := callGlobalMemoryStatusEx

	findGlobalMemoryStatusEx = func() error {
		return findErr
	}
	callGlobalMemoryStatusEx = call

	t.Cleanup(func() {
		findGlobalMemoryStatusEx = originalFind
		callGlobalMemoryStatusEx = originalCall
	})
}

func TestApplyWindowsSafetyMargin(t *testing.T) {
	tests := []struct {
		name          string
		value         int64
		marginPercent int64
		want          int64
	}{
		{
			name:          "10 percent",
			value:         1000,
			marginPercent: 10,
			want:          900,
		},
		{
			name:          "20 percent",
			value:         1000,
			marginPercent: 20,
			want:          800,
		},
		{
			name:          "fractional result",
			value:         101,
			marginPercent: 20,
			want:          81,
		},
		{
			name:          "zero margin",
			value:         1000,
			marginPercent: 0,
			want:          1000,
		},
		{
			name:          "negative margin",
			value:         1000,
			marginPercent: -1,
			want:          1000,
		},
		{
			name:          "zero value",
			value:         0,
			marginPercent: 20,
			want:          0,
		},
		{
			name:          "negative value",
			value:         -100,
			marginPercent: 20,
			want:          -100,
		},
		{
			name:          "full margin",
			value:         1000,
			marginPercent: 100,
			want:          0,
		},
		{
			name:          "margin above 100 percent",
			value:         1000,
			marginPercent: 150,
			want:          0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyWindowsSafetyMargin(
				tt.value,
				tt.marginPercent,
			)

			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}
