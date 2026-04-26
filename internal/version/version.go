package version

import "fmt"

// These variables are set by release builds using -ldflags -X.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns a compact, human-readable build identifier.
func String(binary string) string {
	out := fmt.Sprintf("%s %s", binary, Version)
	if Commit != "" && Commit != "unknown" {
		out += fmt.Sprintf(" (commit %s", Commit)
		if Date != "" && Date != "unknown" {
			out += fmt.Sprintf(", built %s", Date)
		}
		out += ")"
		return out
	}
	if Date != "" && Date != "unknown" {
		out += fmt.Sprintf(" (built %s)", Date)
	}
	return out
}
