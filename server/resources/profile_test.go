package resources

import "testing"

func TestResourceProfileIsValid(t *testing.T) {
	tests := []struct {
		name    string
		profile ResourceProfile
		want    bool
	}{
		{"low", ResourceProfileLow, true},
		{"mid", ResourceProfileMid, true},
		{"high", ResourceProfileHigh, true},
		{"auto", ResourceProfileAuto, true},
		{"custom", ResourceProfileCustom, true},
		{"unknown", ResourceProfile("unknown"), false},
		{"empty", ResourceProfile(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.profile.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourceProfileValidate(t *testing.T) {
	tests := []struct {
		name    string
		profile ResourceProfile
		wantErr bool
	}{
		{"valid", ResourceProfileMid, false},
		{"invalid", ResourceProfile("unknown"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
