package logger

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestLogsMaskPrivateDataByDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERALD_LOG_DIR", dir)
	resetLoggerState(t)

	var callbackMessages []string
	SetLogCallback(func(level, message string) {
		callbackMessages = append(callbackMessages, level+": "+message)
	})

	if err := Init(true); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Debug("sender=%s message_id=%s subject=%q config=%s token=%s", "person@example.com", "<secret-message@example.com>", "Project launch", "/Users/alice/.herald/conf.yaml", "refresh-token-123")
	Close()

	raw, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(raw)
	for _, leaked := range []string{"person@example.com", "secret-message@example.com", "Project launch", "/Users/alice/.herald/conf.yaml", "refresh-token-123"} {
		if strings.Contains(logText, leaked) {
			t.Fatalf("log leaked private value %q:\n%s", leaked, logText)
		}
		if strings.Contains(strings.Join(callbackMessages, "\n"), leaked) {
			t.Fatalf("callback leaked private value %q: %q", leaked, callbackMessages)
		}
	}
	if strings.Count(logText, "?????????") < 5 {
		t.Fatalf("expected masked values in log, got:\n%s", logText)
	}
	if !callbackMessagesContain(callbackMessages, "DEBUG: sender=?????????") {
		t.Fatalf("expected redacted DEBUG callback, got %#v", callbackMessages)
	}
}

func TestUnsafeLogsOptionPreservesPrivateData(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERALD_LOG_DIR", dir)
	resetLoggerState(t)

	if err := Init(true, WithUnsafeLogs(true)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Debug("sender=%s subject=%q", "person@example.com", "Project launch")
	Close()

	raw, err := os.ReadFile(Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(raw)
	for _, want := range []string{"person@example.com", "Project launch"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("unsafe logs should preserve %q:\n%s", want, logText)
		}
	}
}

func resetLoggerState(t *testing.T) {
	t.Helper()
	Close()
	infoLogger = nil
	errorLogger = nil
	debugLogger = nil
	logFile = nil
	logPath = ""
	debugMode = false
	unsafeLogs = false
	logCallback = nil
	t.Cleanup(func() {
		Close()
		infoLogger = nil
		errorLogger = nil
		debugLogger = nil
		logFile = nil
		logPath = ""
		debugMode = false
		unsafeLogs = false
		logCallback = nil
	})
}

func callbackMessagesContain(messages []string, needle string) bool {
	for _, message := range messages {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}
