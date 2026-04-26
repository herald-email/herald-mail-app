package version

import (
	"strings"
	"testing"
)

func TestInfoStringUsesBuildMetadata(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, Date
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
	})

	Version = "v0.1.0-beta.1"
	Commit = "abc1234"
	Date = "2026-04-26T12:34:56Z"

	got := String("herald")

	for _, want := range []string{
		"herald v0.1.0-beta.1",
		"commit abc1234",
		"built 2026-04-26T12:34:56Z",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("String() = %q, want it to contain %q", got, want)
		}
	}
}

func TestInfoStringOmitsUnknownOptionalMetadata(t *testing.T) {
	originalVersion, originalCommit, originalDate := Version, Commit, Date
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
	})

	Version = "dev"
	Commit = "unknown"
	Date = "unknown"

	got := String("herald")
	if got != "herald dev" {
		t.Fatalf("String() = %q, want %q", got, "herald dev")
	}
}
