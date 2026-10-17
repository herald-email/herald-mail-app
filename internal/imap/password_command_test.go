package imap

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestPasswordCommandAuthenticationAndReconnect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper")
	}
	lab := testmail.Start(t)
	cfg := lab.Account(testmail.DefaultAliceAddress).Config("")
	marker := filepath.Join(t.TempDir(), "calls")
	cfg.Credentials.Password = ""
	cfg.Credentials.PasswordCommand = &config.PasswordCommand{Args: []string{"sh", "-c", `printf 'called\n' >> "$1"; printf 'password\r\n'`, "helper", marker}}
	c := New(cfg, "", nil, nil)
	defer c.Close()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("constructor executed command")
	}
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListFolders(); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(marker)
	if err != nil || string(calls) != "called\n" {
		t.Fatal("existing connection re-executed helper")
	}
	c.mu.Lock()
	err = c.Reconnect()
	c.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	calls, err = os.ReadFile(marker)
	if err != nil || string(calls) != "called\ncalled\n" {
		t.Fatal("reconnect did not resolve fresh password")
	}
	if cfg.Credentials.Password != "" {
		t.Fatal("authentication persisted resolved password")
	}
}

func TestPasswordCommandFailureClosesConnectionAndCanRetry(t *testing.T) {
	t.Setenv("HERALD_LOG_DIR", t.TempDir())
	if err := logger.Init(true, logger.WithUnsafeLogs(true)); err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper")
	}
	lab := testmail.Start(t)
	cfg := lab.Account(testmail.DefaultAliceAddress).Config("")
	cfg.Credentials.Password = ""
	c := New(cfg, "", nil, nil)
	defer c.Close()
	for _, command := range []string{`printf 'stdout-secret'; printf 'stderr-secret' >&2; exit 9`, `printf ''`, `printf wrong-password`} {
		cfg.Credentials.PasswordCommand = &config.PasswordCommand{Shell: command}
		err := c.Connect()
		if err == nil {
			t.Fatal("expected authentication failure")
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "printf") || strings.Contains(err.Error(), "wrong-password") {
			t.Fatal("error exposed command or output")
		}
		logger.Error("connection failed: %v", err)
		if c.client != nil {
			t.Fatal("failed authentication retained connection")
		}
	}
	logs, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"stdout-secret", "stderr-secret", "printf", "wrong-password"} {
		if strings.Contains(string(logs), secret) {
			t.Fatal("authentication logs exposed command or output")
		}
	}
	cfg.Credentials.PasswordCommand = &config.PasswordCommand{Shell: `printf password`}
	if err := c.Connect(); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestPasswordCommandIMAPFailedDialDoesNotExecute(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "called")
	cfg := &config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}, Credentials: config.CredentialsConfig{Username: "user", PasswordCommand: &config.PasswordCommand{Args: []string{"sh", "-c", `printf called > "$1"; printf password`, "helper", marker}}}}
	c := New(cfg, "", nil, nil)
	defer c.Close()
	if err := c.Connect(); err == nil {
		t.Fatal("expected failed dial")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("failed connection executed helper")
	}
}
