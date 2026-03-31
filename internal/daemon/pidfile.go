package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// WritePID writes the current process PID to the given file, creating parent
// directories as needed.
func WritePID(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("pidfile: create dir: %w", err)
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// ReadPID reads and returns the PID stored in path.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("pidfile: malformed pid in %s: %w", path, err)
	}
	return pid, nil
}

// RemovePID deletes the pid file, ignoring "not found" errors.
func RemovePID(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// IsRunning returns true if a process with the pid stored in path is alive.
// Returns false when the file does not exist or the pid is stale.
func IsRunning(path string) bool {
	pid, err := ReadPID(path)
	if err != nil {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; send signal 0 to check liveness.
	return proc.Signal(os.Signal(nil)) == nil
}
