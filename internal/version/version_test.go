package version

import (
	"strings"
	"testing"
)

func TestInfo(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		date    string
		want    string
	}{
		{"default", "dev", "none", "unknown", "invgate-cli vdev (commit: none, built: unknown)"},
		{"release", "v0.1.0", "abc123", "2026-07-03", "invgate-cli vv0.1.0 (commit: abc123, built: 2026-07-03)"},
		{"empty", "", "", "", "invgate-cli v (commit: , built: )"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			Commit = tt.commit
			Date = tt.date
			got := Info()
			if got != tt.want {
				t.Errorf("Info() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInfoContainsParts(t *testing.T) {
	Version = "1.2.3"
	Commit = "deadbeef"
	Date = "2026-01-15"
	info := Info()
	for _, part := range []string{"invgate-cli", "1.2.3", "deadbeef", "2026-01-15"} {
		if !strings.Contains(info, part) {
			t.Errorf("Info() = %q, expected to contain %q", info, part)
		}
	}
}