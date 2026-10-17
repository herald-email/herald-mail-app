package smtp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestPasswordCommandSMTPAuthentication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper")
	}
	for _, mode := range []string{"check", "send", "implicit TLS"} {
		t.Run(mode, func(t *testing.T) {
			var server *authCaptureSMTPServer
			if mode == "implicit TLS" {
				server = startImplicitTLSAuthCaptureSMTPServer(t)
			} else {
				server = startAuthCaptureSMTPServer(t)
			}
			cfg := smtpCheckConfig(server.addr)
			cfg.Credentials.Username = "user@example.test"
			cfg.Credentials.PasswordCommand = &config.PasswordCommand{Shell: `printf 'command password\r\n'`}
			c := New(cfg)
			var err error
			if mode == "send" {
				err = c.sendPlain(server.addr, "user@example.test", []string{"friend@example.test"}, "Subject: hello\r\n\r\nBody")
			} else {
				if mode == "implicit TLS" {
					cfg.Vendor = "protonmail"
				}
				err = c.Check(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(server.authLine, "AUTH PLAIN "))
			if err != nil || string(payload) != "\x00user@example.test\x00command password" {
				t.Fatal("SMTP did not authenticate with command output")
			}
			if cfg.Credentials.Password != "" {
				t.Fatal("SMTP persisted resolved password")
			}
		})
	}
}

func TestPasswordCommandSMTPFailureAndRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper")
	}
	lab := testmail.Start(t)
	cfg := lab.Account(testmail.DefaultAliceAddress).Config("")
	cfg.Credentials.Password = ""
	c := New(cfg)
	for _, command := range []string{`printf 'stdout-secret'; printf 'stderr-secret' >&2; exit 9`, `printf ''`, `printf wrong-password`} {
		cfg.Credentials.PasswordCommand = &config.PasswordCommand{Shell: command}
		err := c.Check(context.Background())
		if err == nil {
			t.Fatal("expected authentication failure")
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "printf") || strings.Contains(err.Error(), "wrong-password") {
			t.Fatal("error exposed command or output")
		}
	}
	cfg.Credentials.PasswordCommand = &config.PasswordCommand{Shell: `printf password`}
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(testmail.DefaultAliceAddress, testmail.DefaultBobAddress, "Command auth", "Body", ""); err != nil {
		t.Fatal(err)
	}
	lab.WaitForSubject(testmail.DefaultBobAddress, "INBOX", "Command auth")
}

func TestPasswordCommandSMTPIsLazyAndOAuthSkipsIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper")
	}
	marker := filepath.Join(t.TempDir(), "called")
	cfg := smtpCheckConfig("127.0.0.1:0")
	cfg.Credentials.PasswordCommand = &config.PasswordCommand{Args: []string{"sh", "-c", `printf called > "$1"; printf password`, "helper", marker}}
	c := New(cfg)
	if err := c.sendPlain("127.0.0.1:0", "a@example.test", []string{"b@example.test"}, "Body"); err == nil {
		t.Fatal("expected connection failure")
	}
	cfg.Gmail.Email = "oauth@example.test"
	cfg.Gmail.AccessToken = "access-token"
	cfg.Gmail.RefreshToken = "refresh-token"
	cfg.Gmail.TokenExpiry = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if _, err := c.auth(context.Background(), "localhost"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("non-password-auth operation executed helper")
	}
}

func TestPasswordCommandSMTPFailureDoesNotAttemptAuth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper")
	}
	for _, send := range []bool{false, true} {
		t.Run(fmt.Sprint(send), func(t *testing.T) {
			server := startAuthCaptureSMTPServer(t)
			cfg := smtpCheckConfig(server.addr)
			cfg.Credentials.PasswordCommand = &config.PasswordCommand{Shell: `printf stdout-secret; printf stderr-secret >&2; exit 7`}
			c := New(cfg)
			var err error
			if send {
				err = c.sendPlain(server.addr, "a@example.test", []string{"b@example.test"}, "Body")
			} else {
				err = c.Check(context.Background())
			}
			if err == nil || !strings.Contains(err.Error(), "exit code 7") {
				t.Fatalf("expected helper failure: %v", err)
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "printf") {
				t.Fatal("helper failure exposed command output")
			}
			if server.authLine != "" {
				t.Fatal("SMTP attempted AUTH after helper failed")
			}
		})
	}
}
