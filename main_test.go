package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"mail-processor/internal/app"
)

func TestConfigNeedsOnboarding_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")

	needs, err := configNeedsOnboarding(path)
	if err != nil {
		t.Fatalf("configNeedsOnboarding returned error: %v", err)
	}
	if !needs {
		t.Fatalf("expected missing config to require onboarding")
	}
}

func TestConfigNeedsOnboarding_EmptyOrWhitespaceFile(t *testing.T) {
	cases := map[string]string{
		"empty":      "",
		"whitespace": "   \n\t",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "empty.yaml")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatalf("write temp config: %v", err)
			}

			needs, err := configNeedsOnboarding(path)
			if err != nil {
				t.Fatalf("configNeedsOnboarding returned error: %v", err)
			}
			if !needs {
				t.Fatalf("expected empty or whitespace-only config to require onboarding")
			}
		})
	}
}

func TestConfigNeedsOnboarding_NonEmptyFileDoesNotTriggerOnboarding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("credentials:\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	needs, err := configNeedsOnboarding(path)
	if err != nil {
		t.Fatalf("configNeedsOnboarding returned error: %v", err)
	}
	if needs {
		t.Fatalf("expected non-empty config file to skip onboarding and fail later via normal validation")
	}
}

func TestEnsurePrivateConfigDir_TightensExistingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-style permission bits are not reliable on Windows")
	}

	dir := filepath.Join(t.TempDir(), ".herald")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir loose config dir: %v", err)
	}

	if err := ensurePrivateConfigDir(dir); err != nil {
		t.Fatalf("ensurePrivateConfigDir() returned error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("expected config dir permissions 0700, got %04o", perm)
	}
}

func TestParseImageProtocolFlagAcceptsSupportedModes(t *testing.T) {
	for _, value := range []string{"auto", "iterm2", "kitty", "links", "placeholder", "off"} {
		mode, err := parseImageProtocolFlag(value)
		if err != nil {
			t.Fatalf("parseImageProtocolFlag(%q) unexpected error: %v", value, err)
		}
		if string(mode) != value {
			t.Fatalf("parseImageProtocolFlag(%q) = %q", value, mode)
		}
	}
}

func TestParseImageProtocolFlagRejectsInvalidMode(t *testing.T) {
	if _, err := parseImageProtocolFlag("sixel"); err == nil {
		t.Fatal("parseImageProtocolFlag(\"sixel\") returned nil error, want invalid mode error")
	}
}

func TestParseImageProtocolFlagReturnsAppMode(t *testing.T) {
	mode, err := parseImageProtocolFlag("kitty")
	if err != nil {
		t.Fatalf("parseImageProtocolFlag(\"kitty\"): %v", err)
	}
	if mode != app.PreviewImageModeKitty {
		t.Fatalf("mode = %q, want %q", mode, app.PreviewImageModeKitty)
	}
}

func TestRootCommandFromArgsRoutesSubcommands(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		want     rootCommand
		wantArgs []string
	}{
		{
			name:     "default tui",
			args:     []string{"herald"},
			want:     rootCommandTUI,
			wantArgs: nil,
		},
		{
			name:     "mcp",
			args:     []string{"herald", "mcp", "--demo"},
			want:     rootCommandMCP,
			wantArgs: []string{"--demo"},
		},
		{
			name:     "ssh",
			args:     []string{"herald", "ssh", "-addr", ":2223"},
			want:     rootCommandSSH,
			wantArgs: []string{"-addr", ":2223"},
		},
		{
			name:     "serve unchanged",
			args:     []string{"herald", "serve", "-config", "test.yaml"},
			want:     rootCommandServe,
			wantArgs: []string{"-config", "test.yaml"},
		},
		{
			name:     "unknown remains tui argument",
			args:     []string{"herald", "mailbox"},
			want:     rootCommandTUI,
			wantArgs: []string{"mailbox"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotArgs := rootCommandFromArgs(tt.args)
			if got != tt.want {
				t.Fatalf("rootCommandFromArgs(%v) command = %v, want %v", tt.args, got, tt.want)
			}
			if !slices.Equal(gotArgs, tt.wantArgs) {
				t.Fatalf("rootCommandFromArgs(%v) args = %v, want %v", tt.args, gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestRootHelpTextAdvertisesMCPAndSSHSubcommands(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	fs.Bool("debug", false, "Enable debug logging")
	fs.Bool("version", false, "Show version information")

	var buf bytes.Buffer
	printRootHelp(&buf, "herald", fs)
	help := buf.String()

	for _, want := range []string{
		"herald mcp",
		"herald ssh",
		"herald-mcp-server",
		"herald-ssh-server",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}
