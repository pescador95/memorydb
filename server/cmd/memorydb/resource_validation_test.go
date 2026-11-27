package main

import (
	"strings"
	"testing"

	"github.com/pescador95/memorydb/server/resources"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

func TestValidateProtocolFrameLimitsAcceptsDefaultProfiles(t *testing.T) {
	tests := []struct {
		name         string
		maxKeySize   int64
		maxValueSize int64
		maxFrameSize int64
	}{
		{
			name:         "low",
			maxKeySize:   256,
			maxValueSize: 16 << 10,
			maxFrameSize: 64 << 10,
		},
		{
			name:         "mid",
			maxKeySize:   512,
			maxValueSize: 64 << 10,
			maxFrameSize: 256 << 10,
		},
		{
			name:         "high",
			maxKeySize:   1024,
			maxValueSize: 256 << 10,
			maxFrameSize: 1 << 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := resources.ResourceConfig{
				Storage: resources.StorageResourceConfig{
					MaxKeySize:   tt.maxKeySize,
					MaxValueSize: tt.maxValueSize,
				},
				Network: resources.NetworkResourceConfig{
					MaxFramePayloadSize: tt.maxFrameSize,
				},
			}

			if err := validateProtocolFrameLimits(config); err != nil {
				t.Fatalf("validateProtocolFrameLimits() error = %v", err)
			}
		})
	}
}

func TestValidateProtocolFrameLimitsRejectsOversizedMessages(t *testing.T) {
	config := resources.ResourceConfig{
		Storage: resources.StorageResourceConfig{
			MaxKeySize:   1,
			MaxValueSize: 64 << 10,
		},
		Network: resources.NetworkResourceConfig{
			MaxFramePayloadSize: 64 << 10,
		},
	}

	err := validateProtocolFrameLimits(config)
	if err == nil {
		t.Fatal("expected error for messages exceeding frame payload limit")
	}

	if !strings.Contains(err.Error(), "SET request") {
		t.Errorf("expected SET request size error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "GET response") {
		t.Errorf("expected GET response size error, got: %v", err)
	}
}

func TestValidateProtocolFrameLimitsRejectsKeyLargerThanFrame(t *testing.T) {
	config := resources.ResourceConfig{
		Storage: resources.StorageResourceConfig{
			MaxKeySize:   1024,
			MaxValueSize: 128,
		},
		Network: resources.NetworkResourceConfig{
			MaxFramePayloadSize: 512,
		},
	}

	err := validateProtocolFrameLimits(config)
	if err == nil {
		t.Fatal("expected error for key larger than frame")
	}

	if !strings.Contains(err.Error(), "maximum key size") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProtocolFrameLimitsRejectsValueLargerThanFrame(t *testing.T) {
	config := resources.ResourceConfig{
		Storage: resources.StorageResourceConfig{
			MaxKeySize:   128,
			MaxValueSize: 1024,
		},
		Network: resources.NetworkResourceConfig{
			MaxFramePayloadSize: 512,
		},
	}

	err := validateProtocolFrameLimits(config)
	if err == nil {
		t.Fatal("expected error for value larger than frame")
	}

	if !strings.Contains(err.Error(), "maximum value size") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProtocolFrameLimitsRejectsFrameAboveTransportLimit(t *testing.T) {
	config := resources.ResourceConfig{
		Storage: resources.StorageResourceConfig{
			MaxKeySize:   128,
			MaxValueSize: 256,
		},
		Network: resources.NetworkResourceConfig{
			MaxFramePayloadSize: int64(transporttcp.MaxFrameSize) + 1,
		},
	}

	err := validateProtocolFrameLimits(config)
	if err == nil {
		t.Fatal("expected error for frame above transport limit")
	}

	if !strings.Contains(err.Error(), "exceeds transport limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProtocolFrameLimitsRejectsInvalidSizes(t *testing.T) {
	tests := []struct {
		name   string
		config resources.ResourceConfig
	}{
		{
			name: "zero frame",
			config: resources.ResourceConfig{
				Storage: resources.StorageResourceConfig{
					MaxKeySize:   1,
					MaxValueSize: 1,
				},
			},
		},
		{
			name: "zero key",
			config: resources.ResourceConfig{
				Storage: resources.StorageResourceConfig{
					MaxValueSize: 1,
				},
				Network: resources.NetworkResourceConfig{
					MaxFramePayloadSize: 1024,
				},
			},
		},
		{
			name: "zero value",
			config: resources.ResourceConfig{
				Storage: resources.StorageResourceConfig{
					MaxKeySize: 1,
				},
				Network: resources.NetworkResourceConfig{
					MaxFramePayloadSize: 1024,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateProtocolFrameLimits(tt.config); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
