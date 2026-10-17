package config

import (
	"context"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLoadPasswordCommand(t *testing.T) {
	for _, password := range []string{`{command: "printf password"}`, `{command: ["printf", "password"]}`} {
		t.Run(password, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "conf.yaml")
			data := "credentials:\n  username: user@example.test\n  password: " + password + "\nserver:\n  host: localhost\n  port: 1143\n"
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err != nil {
				t.Fatalf("Load password.command: %v", err)
			}
		})
	}
}

func TestPasswordParsingAndRoundTrip(t *testing.T) {
	for _, input := range []string{
		`password: " literal password "`,
		`password: 123456`,
		`password: {command: "printf '%s' password"}`,
		`password: {command: ["pass", "show", "mail/my account", ""]}`,
	} {
		t.Run(input, func(t *testing.T) {
			var before CredentialsConfig
			if err := yaml.Unmarshal([]byte("username: user\n"+input), &before); err != nil {
				t.Fatal(err)
			}
			data, err := yaml.Marshal(before)
			if err != nil {
				t.Fatal(err)
			}
			var after CredentialsConfig
			if err := yaml.Unmarshal(data, &after); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("credentials changed during round trip")
			}
		})
	}
}

func TestPasswordCommandRejectsInvalidConfigWithoutLeakingValues(t *testing.T) {
	for _, input := range []string{
		`[]`, `{}`, `{command: ""}`, `{command: "   "}`, `{command: []}`,
		`{command: [""]}`, `{command: ["   "]}`, `{command: null}`, `{command: 123}`,
		`{command: ["sensitive", 123]}`, `{command: ["sensitive", {}]}`,
		`{command: {sensitive: value}}`, `{sensitive: value}`, `{command: "sensitive", extra: value}`,
		`{command: "sensitive", command: "other"}`, `{command: ["sensitive", "\0"]}`,
	} {
		t.Run(input, func(t *testing.T) {
			var c CredentialsConfig
			err := yaml.Unmarshal([]byte("password: "+input), &c)
			if err == nil {
				t.Fatal("accepted invalid password command")
			}
			if strings.Contains(err.Error(), "sensitive") {
				t.Fatal("error exposed a credential value")
			}
		})
	}
}

// The test binary supplies a portable helper, without relying on an installed
// password manager or interpreting arguments through a shell.
func TestPasswordCommandProcess(t *testing.T) {
	args := os.Args
	for len(args) > 0 && args[0] != "--password-helper" {
		args = args[1:]
	}
	if len(args) == 0 {
		return
	}
	args = args[1:]
	if args[0] != "" {
		f, err := os.OpenFile(args[0], os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(2)
		}
		_, _ = f.WriteString("called\n")
		_ = f.Close()
	}
	switch args[1] {
	case "print":
		fmt.Print(args[2])
	case "fail":
		fmt.Print("stdout-secret")
		fmt.Fprint(os.Stderr, "stderr-secret")
		os.Exit(7)
	case "sleep":
		time.Sleep(10 * time.Second)
	case "nul":
		fmt.Print("pass\x00word")
	case "large":
		fmt.Print(strings.Repeat("x", maxPasswordOutput+1))
	}
	os.Exit(0)
}

func helperCredentials(t *testing.T, marker, action, output string) CredentialsConfig {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return CredentialsConfig{Username: "user", PasswordCommand: &PasswordCommand{Args: []string{exe, "-test.run=^TestPasswordCommandProcess$", "--", "--password-helper", marker, action, output}}}
}

func TestResolvePasswordCommand(t *testing.T) {
	for _, tc := range []struct{ name, action, output, want, errorText string }{
		{"whitespace", "print", "  password \t\r\n", "  password \t", ""},
		{"literal argv", "print", "$(echo unsafe); $HOME ~ *", "$(echo unsafe); $HOME ~ *", ""},
		{"failure", "fail", "", "", "exit code 7"},
		{"empty", "print", "", "", "empty password"},
		{"newlines only", "print", "\r\n", "", "empty password"},
		{"multiline", "print", "first\nsecond", "", "single password line"},
		{"NUL output", "nul", "", "", "without NUL"},
		{"large", "large", "", "", "exceeds 64 KiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := helperCredentials(t, "", tc.action, tc.output)
			got, err := c.ResolvePassword(context.Background())
			if tc.errorText == "" {
				if err != nil || got != tc.want {
					t.Fatalf("resolution mismatch: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.errorText) || got != "" {
				t.Fatalf("expected safe error %q, got %v", tc.errorText, err)
			}
			if err != nil && (strings.Contains(err.Error(), "stdout-secret") || strings.Contains(err.Error(), "stderr-secret")) {
				t.Fatal("command output exposed in error")
			}
			if c.Password != "" {
				t.Fatal("resolved secret cached in configuration")
			}
		})
	}
}

func TestPasswordCommandErrorsAndCancellation(t *testing.T) {
	c := CredentialsConfig{PasswordCommand: &PasswordCommand{Args: []string{filepath.Join(t.TempDir(), "secret-command-name")}}}
	if _, err := c.ResolvePassword(context.Background()); err == nil || strings.Contains(err.Error(), "secret-command-name") {
		t.Fatalf("unsafe start error: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "called")
	c = helperCredentials(t, marker, "print", "password")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ResolvePassword(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("canceled command executed")
	}
	c = helperCredentials(t, "", "sleep", "")
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.ResolvePassword(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout did not bound execution")
	}
}

func TestPasswordCommandShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell syntax")
	}
	c := CredentialsConfig{PasswordCommand: &PasswordCommand{Shell: `value='shell password'; printf '%s\n' "$value"`}}
	if got, err := c.ResolvePassword(context.Background()); err != nil || got != "shell password" {
		t.Fatalf("shell resolution: %v", err)
	}
	c.PasswordCommand = &PasswordCommand{Shell: "sleep 10 & wait"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.ResolvePassword(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shell timeout: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("shell descendants held command open")
	}
}

func TestPasswordCommandConfigOperationsDoNotExecute(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "called")
			credentials := helperCredentials(t, marker, "print", "resolved-secret")
			cfg := &Config{Credentials: credentials, Server: ServerConfig{Host: "localhost", Port: 1143}, SMTP: ServerConfig{Host: "localhost", Port: 1025}}
			if explicit {
				cfg.Sources = []SourceConfig{{ID: "mail", Kind: "mail", Provider: "imap", Credentials: credentials, IMAP: cfg.Server, SMTP: cfg.SMTP}}
				cfg.Credentials = CredentialsConfig{}
			}
			path := filepath.Join(t.TempDir(), "conf.yaml")
			if err := cfg.Save(path); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			sources := loaded.NormalizedSources()
			if len(sources) != 1 || !reflect.DeepEqual(sources[0].Credentials, credentials) {
				t.Fatal("source normalization lost command")
			}
			if err := loaded.Save(path); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("config operation executed password command")
			}
			if got, err := sources[0].Credentials.ResolvePassword(context.Background()); err != nil || got != "resolved-secret" {
				t.Fatalf("resolve after reload: %v", err)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatal("helper did not execute at authentication")
			}
			if err := loaded.Save(path); err != nil {
				t.Fatal(err)
			}
			after, err := Load(path)
			if err != nil || !reflect.DeepEqual(after.NormalizedSources()[0].Credentials, credentials) {
				t.Fatal("resolution changed saved credentials")
			}
		})
	}
}

func TestLiteralPasswordAndCredentialFormatting(t *testing.T) {
	c := CredentialsConfig{Password: " literal-secret \n"}
	if got, err := c.ResolvePassword(context.Background()); err != nil || got != c.Password {
		t.Fatal("literal password changed")
	}
	c.PasswordCommand = &PasswordCommand{Shell: "command-secret"}
	for _, value := range []any{c, c.PasswordCommand} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
			if strings.Contains(fmt.Sprintf(format, value), "secret") {
				t.Fatal("formatting exposed credentials")
			}
		}
	}
	if _, err := yaml.Marshal(c); err == nil {
		t.Fatal("accepted ambiguous literal and command")
	}
	if _, err := c.ResolvePassword(context.Background()); err == nil {
		t.Fatal("resolved ambiguous credential")
	}
}
