package resources

import (
	"errors"
	"testing"
)

func TestDetectedResourcesValidate(t *testing.T) {
	tests := []struct {
		name      string
		resources DetectedResources
		wantErr   error
	}{
		{
			name: "valid container resources",
			resources: DetectedResources{
				CPUCapacity:    2000,
				MemoryCapacity: 2 << 30,
				CPUSource:      ResourceSourceContainer,
				MemorySource:   ResourceSourceContainer,
			},
		},
		{
			name: "valid host resources",
			resources: DetectedResources{
				CPUCapacity:    4000,
				MemoryCapacity: 8 << 30,
				CPUSource:      ResourceSourceHost,
				MemorySource:   ResourceSourceHost,
			},
		},
		{
			name: "CPU from container and memory from host",
			resources: DetectedResources{
				CPUCapacity:    2000,
				MemoryCapacity: 8 << 30,
				CPUSource:      ResourceSourceContainer,
				MemorySource:   ResourceSourceHost,
			},
		},
		{
			name: "CPU from host and memory from container",
			resources: DetectedResources{
				CPUCapacity:    4000,
				MemoryCapacity: 2 << 30,
				CPUSource:      ResourceSourceHost,
				MemorySource:   ResourceSourceContainer,
			},
		},
		{
			name: "zero CPU",
			resources: DetectedResources{
				MemoryCapacity: 2 << 30,
				CPUSource:      ResourceSourceContainer,
				MemorySource:   ResourceSourceContainer,
			},
			wantErr: ErrInvalidResources,
		},
		{
			name: "zero memory",
			resources: DetectedResources{
				CPUCapacity:  2000,
				CPUSource:    ResourceSourceContainer,
				MemorySource: ResourceSourceContainer,
			},
			wantErr: ErrInvalidResources,
		},
		{
			name: "invalid CPU source",
			resources: DetectedResources{
				CPUCapacity:    2000,
				MemoryCapacity: 2 << 30,
				CPUSource:      "unknown",
				MemorySource:   ResourceSourceContainer,
			},
			wantErr: ErrInvalidResources,
		},
		{
			name: "invalid memory source",
			resources: DetectedResources{
				CPUCapacity:    2000,
				MemoryCapacity: 2 << 30,
				CPUSource:      ResourceSourceContainer,
				MemorySource:   "unknown",
			},
			wantErr: ErrInvalidResources,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resources.Validate()

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}
