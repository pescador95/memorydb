package main

import (
	"strings"
	"testing"
)

func TestRunRejectsInvalidConfigurationBeforeListening(t *testing.T) {
	t.Setenv("MEMORYDB_SHUTDOWN_TIMEOUT", "invalid-duration")

	err := run()
	if err == nil {
		t.Fatal("expected configuration error")
	}

	if !strings.Contains(err.Error(), "load server configuration") {
		t.Fatalf("expected configuration loading error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "MEMORYDB_SHUTDOWN_TIMEOUT") {
		t.Fatalf("expected shutdown timeout error, got: %v", err)
	}

	if strings.Contains(err.Error(), "listen on") {
		t.Fatalf("expected configuration validation before listening, got: %v", err)
	}
}
