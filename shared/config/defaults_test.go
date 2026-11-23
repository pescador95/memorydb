package config

import (
	"os"
	"testing"
)

func TestEnvOrDefault(t *testing.T) {
	const name = "MEMORYDB_TEST_ENV_OR_DEFAULT"

	t.Run("variable not set", func(t *testing.T) {
		original, existed := os.LookupEnv(name)

		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("Unsetenv() error = %v", err)
		}

		t.Cleanup(func() {
			if existed {
				if err := os.Setenv(name, original); err != nil {
					t.Errorf("Setenv() error = %v", err)
				}
			} else {
				if err := os.Unsetenv(name); err != nil {
					t.Errorf("Unsetenv() error = %v", err)
				}
			}
		})

		if got := EnvOrDefault(name, "default"); got != "default" {
			t.Errorf("EnvOrDefault() = %q, want %q", got, "default")
		}
	})

	t.Run("variable empty", func(t *testing.T) {
		t.Setenv(name, "")

		if got := EnvOrDefault(name, "default"); got != "default" {
			t.Errorf("EnvOrDefault() = %q, want %q", got, "default")
		}
	})

	t.Run("variable custom", func(t *testing.T) {
		t.Setenv(name, "custom")

		if got := EnvOrDefault(name, "default"); got != "custom" {
			t.Errorf("EnvOrDefault() = %q, want %q", got, "custom")
		}
	})
}
