//go:build linux

package resources

import (
	"testing"
)

type linuxLimitReaderTestCase struct {
	name      string
	files     map[string]string
	want      int64
	wantFound bool
	wantErr   bool
}

type linuxLimitReader func(string) (int64, bool, error)

func runLinuxLimitReaderCases(
	t *testing.T,
	reader linuxLimitReader,
	tests []linuxLimitReaderTestCase,
) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()

			for name, content := range tt.files {
				writeResourceTestFile(t, directory, name, content)
			}

			got, found, err := reader(directory)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if got != tt.want {
				t.Errorf("value = %d, want %d", got, tt.want)
			}

			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
		})
	}
}

func TestReadCPUQuotaV2AtEdgeCases(t *testing.T) {
	runLinuxLimitReaderCases(t, readCPUQuotaV2At, []linuxLimitReaderTestCase{
		{
			name: "missing cpu.max",
		},
		{
			name: "invalid field count",
			files: map[string]string{
				"cpu.max": "50000\n",
			},
			wantErr: true,
		},
		{
			name: "invalid quota",
			files: map[string]string{
				"cpu.max": "invalid 100000\n",
			},
			wantErr: true,
		},
		{
			name: "zero quota",
			files: map[string]string{
				"cpu.max": "0 100000\n",
			},
			wantErr: true,
		},
		{
			name: "invalid period",
			files: map[string]string{
				"cpu.max": "50000 invalid\n",
			},
			wantErr: true,
		},
		{
			name: "zero period",
			files: map[string]string{
				"cpu.max": "50000 0\n",
			},
			wantErr: true,
		},
	})
}

func TestReadCPUQuotaV1AtEdgeCases(t *testing.T) {
	runLinuxLimitReaderCases(t, readCPUQuotaV1At, []linuxLimitReaderTestCase{
		{
			name: "missing quota",
		},
		{
			name: "invalid quota",
			files: map[string]string{
				"cpu.cfs_quota_us": "invalid\n",
			},
			wantErr: true,
		},
		{
			name: "zero quota",
			files: map[string]string{
				"cpu.cfs_quota_us": "0\n",
			},
			wantErr: true,
		},
		{
			name: "missing period",
			files: map[string]string{
				"cpu.cfs_quota_us": "50000\n",
			},
			wantErr: true,
		},
		{
			name: "invalid period",
			files: map[string]string{
				"cpu.cfs_quota_us":  "50000\n",
				"cpu.cfs_period_us": "invalid\n",
			},
			wantErr: true,
		},
		{
			name: "zero period",
			files: map[string]string{
				"cpu.cfs_quota_us":  "50000\n",
				"cpu.cfs_period_us": "0\n",
			},
			wantErr: true,
		},
	})
}

func TestReadMemoryLimitV2AtEdgeCases(t *testing.T) {
	runLinuxLimitReaderCases(t, readMemoryLimitV2At, []linuxLimitReaderTestCase{
		{
			name: "missing memory.max",
		},
		{
			name: "invalid memory.max",
			files: map[string]string{
				"memory.max": "invalid\n",
			},
			wantErr: true,
		},
		{
			name: "missing memory.current",
			files: map[string]string{
				"memory.max": "2048\n",
			},
			wantErr: true,
		},
		{
			name: "invalid memory.current",
			files: map[string]string{
				"memory.max":     "2048\n",
				"memory.current": "invalid\n",
			},
			wantErr: true,
		},
		{
			name: "negative memory.current",
			files: map[string]string{
				"memory.max":     "2048\n",
				"memory.current": "-1\n",
			},
			wantErr: true,
		},
	})
}

func TestReadMemoryLimitV1AtEdgeCases(t *testing.T) {
	runLinuxLimitReaderCases(t, readMemoryLimitV1At, []linuxLimitReaderTestCase{
		{
			name: "missing memory.limit_in_bytes",
		},
		{
			name: "invalid memory.limit_in_bytes",
			files: map[string]string{
				"memory.limit_in_bytes": "invalid\n",
			},
			wantErr: true,
		},
		{
			name: "missing memory.usage_in_bytes",
			files: map[string]string{
				"memory.limit_in_bytes": "2048\n",
			},
			wantErr: true,
		},
		{
			name: "invalid memory.usage_in_bytes",
			files: map[string]string{
				"memory.limit_in_bytes": "2048\n",
				"memory.usage_in_bytes": "invalid\n",
			},
			wantErr: true,
		},
		{
			name: "negative memory.usage_in_bytes",
			files: map[string]string{
				"memory.limit_in_bytes": "2048\n",
				"memory.usage_in_bytes": "-1\n",
			},
			wantErr: true,
		},
	})
}
