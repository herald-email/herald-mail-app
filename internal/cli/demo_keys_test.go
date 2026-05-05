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
