package resources

import (
	"math"
	"testing"
)

func TestPercentageOf(t *testing.T) {
	tests := []struct {
		name       string
		total      int64
		percentage int64
		want       int64
	}{
		{"55 percent", 1000, 55, 550},
		{"15 percent", 1000, 15, 150},
		{"fractional result", 101, 15, 15},
		{"zero percentage", 1000, 0, 0},
		{"negative percentage", 1000, -1, 0},
		{"zero total", 0, 55, 0},
		{"negative total", -1000, 55, 0},
		{"full percentage", 1000, 100, 1000},
		{"percentage above 100", 1000, 150, 1000},
		{"large value", math.MaxInt64, 55, 5072854620270126693},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percentageOf(tt.total, tt.percentage); got != tt.want {
				t.Errorf("percentageOf() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestScaleEntries(t *testing.T) {
	tests := []struct {
		name         string
		baseEntries  int64
		targetBudget int64
		reference    int64
		want         int64
		wantErr      bool
	}{
		{"same budget", 500_000, 100, 100, 500_000, false},
		{"double budget", 500_000, 200, 100, 1_000_000, false},
		{"half budget", 500_000, 50, 100, 250_000, false},
		{"fractional result", 3, 2, 4, 1, false},
		{"too small", 1, 1, 2, 0, true},
		{"zero entries", 0, 100, 100, 0, true},
		{"negative entries", -1, 100, 100, 0, true},
		{"zero target", 1, 0, 1, 0, true},
		{"negative target", 1, -1, 1, 0, true},
		{"zero reference", 1, 1, 0, 0, true},
		{"negative reference", 1, 1, -1, 0, true},
		{
			name:         "result exceeds int64",
			baseEntries:  math.MaxInt64,
			targetBudget: math.MaxInt64,
			reference:    1,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scaleEntries(
				tt.baseEntries,
				tt.targetBudget,
				tt.reference,
			)

			if (err != nil) != tt.wantErr {
				t.Fatalf("scaleEntries() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("scaleEntries() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCalculateResourceConfig(t *testing.T) {
	config, err := calculateResourceConfig(2000, 2<<30)
	if err != nil {
		t.Fatalf("calculateResourceConfig() error = %v", err)
	}

	if config.Profile != ResourceProfileAuto {
		t.Errorf("Profile = %q, want auto", config.Profile)
	}

	if config.CPUCapacity != 2000 {
		t.Errorf("CPUCapacity = %d, want 2000", config.CPUCapacity)
	}

	if config.ProcessMemoryBudget != 2<<30 {
		t.Errorf("ProcessMemoryBudget = %d, want %d", config.ProcessMemoryBudget, 2<<30)
	}

	if config.Storage.MemoryBudget != percentageOf(2<<30, 55) {
		t.Errorf("unexpected storage memory budget")
	}

	if config.Idempotency.MemoryBudget != percentageOf(2<<30, 15) {
		t.Errorf("unexpected idempotency memory budget")
	}

	if config.Processing.MemoryBudget != percentageOf(2<<30, 15) {
		t.Errorf("unexpected processing memory budget")
	}

	total := config.Storage.MemoryBudget +
		config.Idempotency.MemoryBudget +
		config.Processing.MemoryBudget +
		config.Runtime.MemoryBudget

	if total != config.ProcessMemoryBudget {
		t.Errorf("component budgets sum = %d, want %d", total, config.ProcessMemoryBudget)
	}

	if err := config.Validate(); err != nil {
		t.Errorf("calculated config is invalid: %v", err)
	}
}

func TestCalculateResourceConfigForProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile ResourceProfile
	}{
		{"auto", ResourceProfileAuto},
		{"custom", ResourceProfileCustom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := calculateResourceConfigForProfile(
				tt.profile,
				2000,
				2<<30,
			)
			if err != nil {
				t.Fatalf("calculateResourceConfigForProfile() error = %v", err)
			}

			if config.Profile != tt.profile {
				t.Errorf("Profile = %q, want %q", config.Profile, tt.profile)
			}

			if config.CPUCapacity != 2000 {
				t.Errorf("CPUCapacity = %d, want 2000", config.CPUCapacity)
			}

			if config.ProcessMemoryBudget != 2<<30 {
				t.Errorf("ProcessMemoryBudget = %d, want %d", config.ProcessMemoryBudget, 2<<30)
			}

			if err := config.Validate(); err != nil {
				t.Errorf("calculated config is invalid: %v", err)
			}
		})
	}
}

func TestCalculateResourceConfigForProfileRejectsUnsupportedProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile ResourceProfile
	}{
		{"low", ResourceProfileLow},
		{"mid", ResourceProfileMid},
		{"high", ResourceProfileHigh},
		{"unknown", ResourceProfile("unknown")},
		{"empty", ResourceProfile("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := calculateResourceConfigForProfile(
				tt.profile,
				2000,
				2<<30,
			)

			if err == nil {
				t.Fatal("expected error for unsupported calculated profile")
			}
		})
	}
}

func TestCalculateResourceConfigRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		cpu    int64
		memory int64
	}{
		{"zero CPU", 0, 1 << 30},
		{"negative CPU", -1, 1 << 30},
		{"zero memory", 1000, 0},
		{"negative memory", 1000, -1},
		{"insufficient memory", 1000, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := calculateResourceConfig(tt.cpu, tt.memory)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
