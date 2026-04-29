package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderHomebrewFormulaUsesImmutableReleaseAssets(t *testing.T) {
	tempDir := t.TempDir()
	checksumsPath := filepath.Join(tempDir, "checksums.txt")
	outputPath := filepath.Join(tempDir, "herald.rb")

	checksums := strings.Join([]string{
		"380187a42200d184dfd803c2fc69491d61a5eea989a175d14f5dd1ac761722ad  herald-v0.1.0-beta.1-darwin-amd64.tar.gz",
		"273e48e3b871d7c800801f2fe9ed4610ea9a5ff6c77c5850fb78b300e0fab7e9  herald-v0.1.0-beta.1-darwin-arm64.tar.gz",
	}, "\n") + "\n"
	if err := os.WriteFile(checksumsPath, []byte(checksums), 0o600); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	cmd := exec.Command("bash", ".github/scripts/render-homebrew-formula.sh", "v0.1.0-beta.1", checksumsPath, outputPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("render-homebrew-formula.sh failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read formula: %v", err)
	}
	formula := string(data)

	wantSubstrings := []string{
		`class Herald < Formula`,
		`desc "Terminal email client and inbox cleanup tool"`,
		`homepage "https://github.com/herald-email/herald-mail-app"`,
		`license "FSL-1.1-ALv2"`,
		`on_arm do`,
		`url "https://github.com/herald-email/herald-mail-app/releases/download/v0.1.0-beta.1/herald-v0.1.0-beta.1-darwin-arm64.tar.gz"`,
		`sha256 "273e48e3b871d7c800801f2fe9ed4610ea9a5ff6c77c5850fb78b300e0fab7e9"`,
		`on_intel do`,
		`url "https://github.com/herald-email/herald-mail-app/releases/download/v0.1.0-beta.1/herald-v0.1.0-beta.1-darwin-amd64.tar.gz"`,
		`sha256 "380187a42200d184dfd803c2fc69491d61a5eea989a175d14f5dd1ac761722ad"`,
		`bin.install "herald"`,
		`bin.install "herald-mcp-server"`,
		`bin.install "herald-ssh-server"`,
		`system bin/"herald", "--version"`,
		`system bin/"herald", "mcp", "--version"`,
		`system bin/"herald", "ssh", "--version"`,
		`system bin/"herald-mcp-server", "--version"`,
		`system bin/"herald-ssh-server", "--version"`,
		`assert_match "\"tools\"", shell_output("echo '#{request}' | #{bin}/herald mcp --demo")`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(formula, want) {
			t.Fatalf("formula missing %q:\n%s", want, formula)
		}
	}
	if strings.Contains(formula, "beta-latest") {
		t.Fatalf("formula must use immutable versioned assets, got:\n%s", formula)
	}
}

func TestRenderHomebrewFormulaFailsWhenChecksumIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	checksumsPath := filepath.Join(tempDir, "checksums.txt")
	outputPath := filepath.Join(tempDir, "herald.rb")

	checksums := "273e48e3b871d7c800801f2fe9ed4610ea9a5ff6c77c5850fb78b300e0fab7e9  herald-v0.1.0-beta.1-darwin-arm64.tar.gz\n"
	if err := os.WriteFile(checksumsPath, []byte(checksums), 0o600); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	cmd := exec.Command("bash", ".github/scripts/render-homebrew-formula.sh", "v0.1.0-beta.1", checksumsPath, outputPath)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("render-homebrew-formula.sh unexpectedly succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "missing checksum for herald-v0.1.0-beta.1-darwin-amd64.tar.gz") {
		t.Fatalf("expected missing amd64 checksum error, got:\n%s", out)
	}
}
