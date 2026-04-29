package render

import (
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestHTMLToMarkdownPreservesPreviewStructure(t *testing.T) {
	got := HTMLToMarkdown(`<html><body>
		<h1>Preview Title</h1>
		<p><strong>Bold</strong> and <em>italic</em> plus <code>code</code>.</p>
		<ul><li>First item</li><li>Second item</li></ul>
		<p><a href="https://example.test/report?utm_source=email">Open report</a></p>
		<p><img alt="Remote chart" src="https://example.test/chart.png"></p>
	</body></html>`)

	for _, want := range []string{
		"# Preview Title",
		"**Bold**",
		"*italic*",
		"`code`",
		"- First item",
		"- Second item",
		"[Open report](https://example.test/report?utm_source=email)",
		"![Remote chart](https://example.test/chart.png)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("HTMLToMarkdown missing %q:\n%s", want, got)
		}
	}
}

func TestHTMLToMarkdownKeepsAnchorWrappedRemoteImages(t *testing.T) {
	got := HTMLToMarkdown(`<a href="https://example.test"><img alt="Logo" src="https://example.test/logo.png"></a>`)
	if !strings.Contains(got, "![Logo](https://example.test/logo.png)") {
		t.Fatalf("expected linked remote image to stay visible, got %q", got)
	}
}

func TestHTMLToMarkdownKeepsPunctuationTightAfterInlineFormatting(t *testing.T) {
	got := HTMLToMarkdown(`<p><strong>Budget alert</strong> for <em>Project Orion</em>.</p>`)
	if want := "**Budget alert** for *Project Orion*."; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestHTMLToMarkdownConvertsTablesToMarkdownTables(t *testing.T) {
	got := HTMLToMarkdown(`<table>
		<tr><th>Name</th><th>Power</th></tr>
		<tr><td>Carrots</td><td>9001</td></tr>
		<tr><td>Ramen</td><td>9002</td></tr>
	</table>`)

	for _, want := range []string{
		"| Name | Power |",
		"| --- | --- |",
		"| Carrots | 9001 |",
		"| Ramen | 9002 |",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected markdown table row %q, got:\n%s", want, got)
		}
	}
}

func TestEmailBodyMarkdownPrefersHTMLOverPlainFallback(t *testing.T) {
	body := &models.EmailBody{
		TextPlain: "stale plain fallback",
		TextHTML:  `<h1>Fresh HTML</h1><ul><li>Shared renderer</li></ul>`,
	}
	got := EmailBodyMarkdown(body)
	if !strings.Contains(got, "# Fresh HTML") || !strings.Contains(got, "- Shared renderer") {
		t.Fatalf("expected HTML-derived markdown, got:\n%s", got)
	}
	if strings.Contains(got, "stale plain fallback") {
		t.Fatalf("expected HTML to win over plain fallback, got:\n%s", got)
	}
}
