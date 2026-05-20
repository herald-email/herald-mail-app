package repoassert

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func repoRoot(t testing.TB) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repoassert helper")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func repoPath(t testing.TB, elems ...string) string {
	t.Helper()

	parts := append([]string{repoRoot(t)}, elems...)
	return filepath.Join(parts...)
}

func repoCommand(t testing.TB, name string, args ...string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = repoRoot(t)
	return cmd
}
