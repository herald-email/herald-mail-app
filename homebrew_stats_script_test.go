package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomebrewInstallStatsReportsSupportedAndUnsupportedWindows(t *testing.T) {
	tempDir := t.TempDir()
	stubPath := filepath.Join(tempDir, "curl")
	stub := `#!/usr/bin/env bash
set -euo pipefail

url="${@: -1}"
case "$url" in
  *"/3d.json"|*"/7d.json")
    exit 22
    ;;
  *"/30d.json")
    cat <<'JSON'
{"start_date":"2026-04-18","end_date":"2026-05-18","items":[{"formula":"herald-email/herald/herald","count":"13","percent":"0"}]}
JSON
    ;;
  *"/90d.json")
    cat <<'JSON'
{"start_date":"2026-02-17","end_date":"2026-05-18","items":[{"formula":"herald-email/herald/herald","count":"21","percent":"0.01"}]}
JSON
    ;;
  *)
    echo "unexpected URL: $url" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(stubPath, []byte(stub), 0o700); err != nil {
		t.Fatalf("write curl stub: %v", err)
	}

	cmd := exec.Command("bash", "scripts/homebrew-install-stats.sh")
	cmd.Env = append(os.Environ(), "PATH="+tempDir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("homebrew-install-stats.sh failed: %v\n%s", err, out)
	}

	output := string(out)
	wantSubstrings := []string{
		"Formula: herald-email/herald/herald",
		"Category: install-on-request",
		"3d\t-\t-\t-\t-\tunsupported",
		"7d\t-\t-\t-\t-\tunsupported",
		"30d\t2026-04-18\t2026-05-18\t13\t0\tok",
		"90d\t2026-02-17\t2026-05-18\t21\t0.01\tok",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
