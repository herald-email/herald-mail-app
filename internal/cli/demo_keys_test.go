package cli

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestRegisterTUIFlagsParsesDemoKeys(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	opts := registerTUIFlags(fs)

	if err := fs.Parse([]string{"--demo", "--demo-keys"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	if !*opts.demo {
		t.Fatal("expected --demo to parse")
	}
	if !*opts.demoKeys {
		t.Fatal("expected --demo-keys to parse")
	}
}

func TestDemoMultiAccountFlagImpliesDemoMode(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	opts := registerTUIFlags(fs)

	if err := fs.Parse([]string{"--demo-multi-account"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	if !shouldRunDemo(*opts.demo, *opts.demoMulti) {
		t.Fatal("expected --demo-multi-account to start demo mode")
	}
}

func TestPlainLaunchDoesNotRunDemoMode(t *testing.T) {
	if shouldRunDemo(false, false) {
		t.Fatal("plain launch should not start demo mode")
	}
}

func TestTUIFlagsParseOpenDeepLink(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	opts := registerTUIFlags(fs)

	if err := fs.Parse([]string{"--demo", "--open", "herald://mail/folder?folder=INBOX"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if opts.openDeepLink == nil || *opts.openDeepLink != "herald://mail/folder?folder=INBOX" {
		t.Fatalf("openDeepLink = %#v", opts.openDeepLink)
	}
}

func TestRegisterTUIFlagsParsesThemeOverride(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	opts := registerTUIFlags(fs)

	if err := fs.Parse([]string{"--demo", "-theme", "jade-signal"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	if got := *opts.theme; got != "jade-signal" {
		t.Fatalf("theme flag = %q, want jade-signal", got)
	}
}

func TestRegisterTUIFlagsParsesUnsafeLogs(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	opts := registerTUIFlags(fs)

	if err := fs.Parse([]string{"-debug", "-unsafe-logs"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	if !*opts.debug {
		t.Fatal("expected -debug to parse")
	}
	if !*opts.unsafeLogs {
		t.Fatal("expected -unsafe-logs to parse")
	}
}

func TestRegisterTUIFlagsAdvertisesDemoKeys(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	registerTUIFlags(fs)

	var buf bytes.Buffer
	fs.SetOutput(&buf)
	fs.PrintDefaults()

	help := buf.String()
	if !strings.Contains(help, "demo-keys") || !strings.Contains(help, "keypress overlay") {
		t.Fatalf("expected help to advertise demo keypress overlay, got:\n%s", help)
	}
}

func TestRegisterTUIFlagsAdvertisesThemeOverride(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	registerTUIFlags(fs)

	var buf bytes.Buffer
	fs.SetOutput(&buf)
	fs.PrintDefaults()

	help := buf.String()
	if !strings.Contains(help, "theme") || !strings.Contains(help, "built-in theme name or theme YAML file") {
		t.Fatalf("expected help to advertise theme override, got:\n%s", help)
	}
}

func TestRegisterTUIFlagsAdvertisesUnsafeLogs(t *testing.T) {
	fs := flag.NewFlagSet("herald", flag.ContinueOnError)
	registerTUIFlags(fs)

	var buf bytes.Buffer
	fs.SetOutput(&buf)
	fs.PrintDefaults()

	help := buf.String()
	if !strings.Contains(help, "unsafe-logs") || !strings.Contains(help, "unredacted private data") {
		t.Fatalf("expected help to advertise unsafe logs opt-in, got:\n%s", help)
	}
}
