package resources

import (
	"testing"
)

func TestLoadResourceConfigFromEnvRejectsInvalidLegacyIdempotencyCapacity(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envIdempotencyCapacity, "many")

	_, err := loadResourceConfigFromEnv(validDetectedResources())
	if err == nil {
		t.Fatal("expected error for invalid legacy idempotency capacity")
	}
}

func TestLoadResourceConfigFromEnvTreatsEmptyIdempotencyAliasesAsUnset(t *testing.T) {
	clearResourceEnvironment(t)
	t.Setenv(envIdempotencyMaxEntries, " ")
	t.Setenv(envIdempotencyCapacity, "")

	config, err := loadResourceConfigFromEnv(validDetectedResources())
	if err != nil {
		t.Fatalf("loadResourceConfigFromEnv: %v", err)
	}

	if config.Idempotency.MaxEntries != 100_000 {
		t.Errorf(
			"idempotency max entries = %d, want 100000",
			config.Idempotency.MaxEntries,
		)
	}
}

func TestParseCPUCapacityRejectsCoreOverflow(t *testing.T) {
	_, err := parseCPUCapacity("9223372036854776")
	if err == nil {
		t.Fatal("expected int64 overflow error")
	}
}
