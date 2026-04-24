package logger

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInitWritesLogOutsideWorkingDirectory(t *testing.T) {
	tempRoot := t.TempDir()
	cwd := filepath.Join(tempRoot, "launch-dir")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldwd)
	})

	t.Setenv("HOME", tempRoot)
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempRoot, "state"))
	t.Setenv("LOCALAPPDATA", filepath.Join(tempRoot, "local-app-data"))
	t.Setenv("HERALD_LOG_DIR", "")

	if err := Init(false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Close()

	cwdLogs, err := filepath.Glob(filepath.Join(cwd, "herald_*.log"))
	if err != nil {
		t.Fatalf("glob cwd logs: %v", err)
	}
	if len(cwdLogs) != 0 {
		t.Fatalf("expected no logs in working directory, found %v", cwdLogs)
	}

	logDir, err := defaultLogDir()
	if err != nil {
		t.Fatalf("defaultLogDir: %v", err)
	}
	logs, err := filepath.Glob(filepath.Join(logDir, "herald_*.log"))
	if err != nil {
		t.Fatalf("glob log dir: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one log in %s, found %v", logDir, logs)
	}
}

func TestDefaultLogDirUsesPlatformUserLogLocation(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("HOME", tempRoot)
	t.Setenv("XDG_STATE_HOME", filepath.Join(tempRoot, "state"))
	t.Setenv("LOCALAPPDATA", filepath.Join(tempRoot, "local-app-data"))
	t.Setenv("HERALD_LOG_DIR", "")

	logDir, err := defaultLogDir()
	if err != nil {
		t.Fatalf("defaultLogDir: %v", err)
	}

	var want string
	switch runtime.GOOS {
	case "darwin":
		want = filepath.Join(tempRoot, "Library", "Logs", "Herald")
	case "windows":
		want = filepath.Join(tempRoot, "local-app-data", "Herald", "Logs")
	default:
		want = filepath.Join(tempRoot, "state", "herald", "logs")
	}

	if logDir != want {
		t.Fatalf("defaultLogDir() = %q, want %q", logDir, want)
	}
}

func TestDefaultLogDirHonorsOverride(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-logs")
	t.Setenv("HERALD_LOG_DIR", want)

	logDir, err := defaultLogDir()
	if err != nil {
		t.Fatalf("defaultLogDir: %v", err)
	}
	if logDir != want {
		t.Fatalf("defaultLogDir() = %q, want override %q", logDir, want)
	}
}
