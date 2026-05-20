package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSanitizesAndValidatesFixture(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "raw.eml")
	out := filepath.Join(dir, "corpus", "sample", "message.eml")
	raw := "From: Real Person <real.person@example.com>\n" +
		"To: Other Person <other@example.net>\n" +
		"Subject: Token sample\n" +
		"Date: Tue, 01 Jan 2019 01:02:03 -0500\n" +
		"Message-ID: <raw@example.com>\n" +
		"Content-Type: text/plain; charset=utf-8\n\n" +
		"Open https://calendar.example.com/path?utm_source=mail and token sk-secretsecretsecret.\n"
	if err := os.WriteFile(in, []byte(raw), 0o600); err != nil {
		t.Fatalf("write raw input: %v", err)
	}

	if err := run([]string{"-in", in, "-out", out}); err != nil {
		t.Fatalf("run sanitize: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read sanitized output: %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{"example.com", "example.net", "utm_source", "sk-secret"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("sanitized output still contains %q:\n%s", forbidden, text)
		}
	}
	if !strings.Contains(text, "@herald.test") || !strings.Contains(text, "https://example.test/redacted-link-1") {
		t.Fatalf("sanitized output missing deterministic replacements:\n%s", text)
	}

	if err := run([]string{"-validate", filepath.Join(dir, "corpus")}); err != nil {
		t.Fatalf("run validate: %v", err)
	}
}
