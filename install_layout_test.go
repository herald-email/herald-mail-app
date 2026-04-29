package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestModulePathIsCanonicalGitHubPath(t *testing.T) {
	cmd := exec.Command("go", "list", "-m")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -m failed: %v\n%s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), "github.com/herald-email/herald-mail-app"; got != want {
		t.Fatalf("module path = %q, want %q", got, want)
	}
}

func TestSourceInstallLayoutInstallsHeraldBinary(t *testing.T) {
	gobin := t.TempDir()

	cmd := exec.Command("go", "install", "./cmd/herald")
	cmd.Env = append(os.Environ(), "GOBIN="+gobin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go install ./cmd/herald failed: %v\n%s", err, out)
	}

	binaryName := "herald"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	if _, err := os.Stat(filepath.Join(gobin, binaryName)); err != nil {
		t.Fatalf("expected go install ./cmd/herald to create %q in %s: %v", binaryName, gobin, err)
	}
	if _, err := os.Stat(filepath.Join(gobin, "mail-processor")); err == nil {
		t.Fatalf("go install ./cmd/herald created legacy mail-processor binary in %s", gobin)
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking for legacy mail-processor binary: %v", err)
	}
}
