package imap

import (
	"strings"
	"testing"
)

func TestHTMLToText_NestedDivs(t *testing.T) {
	html := `<div><div><div>text</div></div></div>`
	got := htmlToMarkdown(html)
	if got != "text" {
		t.Errorf("got %q, want %q", got, "text")
	}
}

func TestHTMLToText_ParagraphSeparation(t *testing.T) {
	html := `<p>A</p><p>B</p>`
	got := htmlToMarkdown(html)
	if got != "A\n\nB" {
		t.Errorf("got %q, want %q", got, "A\n\nB")
	}
}

func TestHTMLToText_NoTripleBlankLines(t *testing.T) {
	html := `<div><p>Hello</p></div><div><p>World</p></div>`
	got := htmlToMarkdown(html)
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("got triple newline in %q", got)
	}
}

func TestHTMLToText_TableCells(t *testing.T) {
	html := `<table><tr><td>A</td><td>B</td></tr></table>`
	got := htmlToMarkdown(html)
	if !strings.Contains(got, "A") || !strings.Contains(got, "B") {
		t.Errorf("expected A and B in %q", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Errorf("unexpected blank line in table output: %q", got)
	}
}

func TestHTMLToText_BRTag(t *testing.T) {
	html := `<p>line1<br>line2</p>`
	got := htmlToMarkdown(html)
	if !strings.Contains(got, "line1\nline2") {
		t.Errorf("expected single newline between lines, got %q", got)
	}
}

func TestHTMLToText_SkipsStyleScript(t *testing.T) {
	html := `<style>body{color:red}</style><p>Hello</p><script>alert(1)</script>`
	got := htmlToMarkdown(html)
	if strings.Contains(got, "body") || strings.Contains(got, "alert") {
		t.Errorf("style/script content leaked into output: %q", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Errorf("expected Hello in output: %q", got)
	}
}

func TestHTMLToText_EmptyInput(t *testing.T) {
	got := htmlToMarkdown("")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
