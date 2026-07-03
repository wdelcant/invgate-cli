package version

import "fmt"

// Version, Commit, and Date are set at build time via ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info returns a human-readable version string.
func Info() string {
	return fmt.Sprintf("invgate-cli v%s (commit: %s, built: %s)", Version, Commit, Date)
}