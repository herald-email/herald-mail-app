package smtp

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestBuildInlineImages_NoLocalPaths(t *testing.T) {
	html, inlines, err := BuildInlineImages("Hello **world**")
	if err != nil {
		t.Fatal(err)
	}
	if len(inlines) != 0 {
		t.Errorf("expected 0 inlines, got %d", len(inlines))
	}
	if !strings.Contains(html, "world") {
		t.Error("expected html to contain 'world'")
	}
}

func TestBuildInlineImages_LocalPath(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpFile.Write([]byte("fakepng")); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	md := fmt.Sprintf("![logo](%s)", tmpFile.Name())
	html, inlines, err := BuildInlineImages(md)
	if err != nil {
		t.Fatal(err)
	}
	if len(inlines) != 1 {
		t.Fatalf("expected 1 inline, got %d", len(inlines))
	}
	if !strings.Contains(html, "cid:") {
		t.Error("expected html to contain cid: reference")
	}
	if inlines[0].MIMEType != "image/png" {
		t.Errorf("expected image/png, got %s", inlines[0].MIMEType)
	}
}

func TestBuildInlineImages_HttpPathIgnored(t *testing.T) {
	md := "![logo](https://example.com/logo.png)"
	_, inlines, err := BuildInlineImages(md)
	if err != nil {
		t.Fatal(err)
	}
	if len(inlines) != 0 {
		t.Errorf("expected 0 inlines for http path, got %d", len(inlines))
	}
}

func TestBuildInlineImages_MultipleImages(t *testing.T) {
	tmp1, _ := os.CreateTemp("", "test1-*.png")
	tmp1.Write([]byte("png1"))
	tmp1.Close()
	defer os.Remove(tmp1.Name())

	tmp2, _ := os.CreateTemp("", "test2-*.jpg")
	tmp2.Write([]byte("jpg1"))
	tmp2.Close()
	defer os.Remove(tmp2.Name())

	md := fmt.Sprintf("![a](%s) ![b](%s)", tmp1.Name(), tmp2.Name())
	html, inlines, err := BuildInlineImages(md)
	if err != nil {
		t.Fatal(err)
	}
	if len(inlines) != 2 {
		t.Fatalf("expected 2 inlines, got %d", len(inlines))
	}
	if inlines[0].ContentID == inlines[1].ContentID {
		t.Error("expected unique Content-IDs for each inline image")
	}
	if inlines[0].MIMEType != "image/png" {
		t.Errorf("expected image/png for first image, got %s", inlines[0].MIMEType)
	}
	if inlines[1].MIMEType != "image/jpeg" {
		t.Errorf("expected image/jpeg for second image, got %s", inlines[1].MIMEType)
	}
	if strings.Count(html, "cid:") != 2 {
		t.Errorf("expected 2 cid: references in html, got %d", strings.Count(html, "cid:"))
	}
}

func TestBuildInlineImages_CIDSubstitution(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "test-*.gif")
	tmpFile.Write([]byte("gif"))
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	md := fmt.Sprintf("![anim](%s)", tmpFile.Name())
	html, inlines, err := BuildInlineImages(md)
	if err != nil {
		t.Fatal(err)
	}
	if len(inlines) != 1 {
		t.Fatalf("expected 1 inline")
	}
	expectedCIDRef := fmt.Sprintf(`src="cid:%s"`, inlines[0].ContentID)
	if !strings.Contains(html, expectedCIDRef) {
		t.Errorf("expected html to contain %q\ngot: %s", expectedCIDRef, html)
	}
	if strings.Contains(html, fmt.Sprintf(`src="%s"`, tmpFile.Name())) {
		t.Error("original src path should have been replaced with cid: reference")
	}
	if inlines[0].MIMEType != "image/gif" {
		t.Errorf("expected image/gif, got %s", inlines[0].MIMEType)
	}
}

func TestMimeTypeFromExt(t *testing.T) {
	cases := []struct {
		ext      string
		expected string
	}{
		{".png", "image/png"},
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".gif", "image/gif"},
		{".webp", "image/webp"},
		{".bmp", "image/octet-stream"},
		{"", "image/octet-stream"},
	}
	for _, c := range cases {
		got := mimeTypeFromExt(c.ext)
		if got != c.expected {
			t.Errorf("mimeTypeFromExt(%q) = %q, want %q", c.ext, got, c.expected)
		}
	}
}

func TestBuildDraftMessage(t *testing.T) {
	raw, err := BuildDraftMessage("from@example.com", "to@example.com", "Test Subject", "Hello body")
	if err != nil {
		t.Fatalf("BuildDraftMessage: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected non-empty message bytes")
	}
	msg := string(raw)
	if !strings.Contains(msg, "From: from@example.com") {
		t.Error("missing From header")
	}
	if !strings.Contains(msg, "To: to@example.com") {
		t.Error("missing To header")
	}
	if !strings.Contains(msg, "Subject: Test Subject") {
		t.Error("missing Subject header")
	}
	if !strings.Contains(msg, "MIME-Version: 1.0") {
		t.Error("missing MIME-Version header")
	}
	if !strings.Contains(msg, "Content-Type: text/plain") {
		t.Error("missing Content-Type header")
	}
	if !strings.Contains(msg, "Hello body") {
		t.Error("missing body content")
	}
}
