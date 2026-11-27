//go:build linux

package resources

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReadAvailableMemoryFromFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int64
		wantErr bool
	}{
		{
			name: "valid MemAvailable",
			content: "MemTotal: 8388608 kB\n" +
				"MemFree: 102400 kB\n" +
				"MemAvailable: 1048576 kB\n",
			want: 1073741824,
		},
		{
			name:    "MemAvailable with additional fields",
			content: "MemAvailable: 1024 kB extra\n",
			want:    1048576,
		},
		{
			name:    "MemAvailable missing",
			content: "MemTotal: 8388608 kB\nMemFree: 102400 kB\n",
			wantErr: true,
		},
		{
			name:    "zero value",
			content: "MemAvailable: 0 kB\n",
			wantErr: true,
		},
		{
			name:    "negative value",
			content: "MemAvailable: -1 kB\n",
			wantErr: true,
		},
		{
			name:    "non numeric value",
			content: "MemAvailable: invalid kB\n",
			wantErr: true,
		},
		{
			name:    "integer overflow",
			content: "MemAvailable: 9223372036854775807 kB\n",
			wantErr: true,
		},
		{
			name:    "missing numeric field",
			content: "MemAvailable:\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "meminfo")

			if err := os.WriteFile(path, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write test file: %v", err)
			}

			got, err := readAvailableMemoryFromFile(path)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if got != tt.want {
				t.Errorf("available memory = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReadAvailableMemoryFromFileRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-meminfo")

	_, err := readAvailableMemoryFromFile(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadAvailableMemoryFromFileRejectsScannerError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meminfo")

	content := strings.Repeat("x", 70*1024) +
		"\nMemAvailable: 1024 kB\n"

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := readAvailableMemoryFromFile(path)
	if err == nil {
		t.Fatal("expected scanner error for oversized line")
	}
}

func TestReadAvailableMemoryFromFileChecksMaximumConversionBoundary(t *testing.T) {
	maxKilobytes := int64(math.MaxInt64 / 1024)

	tests := []struct {
		name    string
		value   int64
		want    int64
		wantErr bool
	}{
		{
			name:  "maximum safe conversion",
			value: maxKilobytes,
			want:  maxKilobytes * 1024,
		},
		{
			name:    "first overflowing conversion",
			value:   maxKilobytes + 1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "meminfo")
			content := "MemAvailable: " + strconv.FormatInt(tt.value, 10) + " kB\n"

			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatalf("write test file: %v", err)
			}

			got, err := readAvailableMemoryFromFile(path)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if got != tt.want {
				t.Errorf("available memory = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReadHostAvailableMemory(t *testing.T) {
	got, err := readHostAvailableMemory()
	if err != nil {
		t.Fatalf("readHostAvailableMemory() error = %v", err)
	}

	if got <= 0 {
		t.Errorf("available memory = %d, want a positive value", got)
	}
}
