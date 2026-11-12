package Services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetProcessRSSBytes(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantKB  int64
		wantErr bool
	}{
		{
			name: "normal status file",
			content: `Name:	nginx
VmRSS:	  4096 kB
VmSize:	 20480 kB
`,
			wantKB: 4096,
		},
		{
			name: "missing VmRSS",
			content: `Name:	nginx
VmSize:	 20480 kB
`,
			wantErr: true,
		},
		{
			name: "malformed VmRSS line",
			content: `VmRSS:
`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			statusPath := filepath.Join(dir, "status")
			if err := os.WriteFile(statusPath, []byte(tc.content), 0644); err != nil {
				t.Fatalf("setup: %v", err)
			}

			got, err := readRSSBytesFromPath(statusPath)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := tc.wantKB * 1024
			if got != want {
				t.Errorf("got %d bytes, want %d", got, want)
			}
		})
	}
}

func TestThresholdDecision(t *testing.T) {
	const threshold = int64(5 * 1024 * 1024)

	cases := []struct {
		name       string
		totalRSS   int64
		portClosed bool
		wantClose  bool
		wantOpen   bool
	}{
		{"over threshold, port open → close", threshold + 1, false, true, false},
		{"over threshold, port already closed → no-op", threshold + 1, true, false, false},
		{"under threshold, port closed → open", threshold - 1, true, false, true},
		{"under threshold, port open → no-op", threshold - 1, false, false, false},
		{"exactly at threshold, port open → no-op", threshold, false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shouldClose := tc.totalRSS > threshold && !tc.portClosed
			shouldOpen := tc.totalRSS <= threshold && tc.portClosed

			if shouldClose != tc.wantClose {
				t.Errorf("shouldClose = %v, want %v", shouldClose, tc.wantClose)
			}
			if shouldOpen != tc.wantOpen {
				t.Errorf("shouldOpen = %v, want %v", shouldOpen, tc.wantOpen)
			}
		})
	}
}

func TestGetImageHostingRamLimitBytes(t *testing.T) {
	cases := []struct {
		name     string
		dbValue  string
		dbErr    bool
		wantBytes int64
	}{
		{"valid 5 MB", "5", false, 5 * 1024 * 1024},
		{"valid 10 MB", "10", false, 10 * 1024 * 1024},
		{"zero falls back to default", "0", false, imageHostingDefaultRamLimitBytes},
		{"negative falls back to default", "-1", false, imageHostingDefaultRamLimitBytes},
		{"non-integer falls back to default", "abc", false, imageHostingDefaultRamLimitBytes},
		{"whitespace trimmed", "  8  ", false, 8 * 1024 * 1024},
		{"db error falls back to default", "", true, imageHostingDefaultRamLimitBytes},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getRamLimitBytesFromValue(tc.dbValue, tc.dbErr)
			if got != tc.wantBytes {
				t.Errorf("got %d bytes, want %d", got, tc.wantBytes)
			}
		})
	}
}

func TestIsNginxImageHostingActiveOutput(t *testing.T) {
	cases := []struct {
		output string
		want   bool
	}{
		{"active\n", true},
		{"inactive\n", false},
		{"failed\n", false},
		{"activating\n", false},
		{"", false},
	}
	for _, tc := range cases {
		got := strings.TrimSpace(tc.output) == "active"
		if got != tc.want {
			t.Errorf("output %q: got %v, want %v", tc.output, got, tc.want)
		}
	}
}
