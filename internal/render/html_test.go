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

func TestHTMLToMarkdownWithCIDImagesKeepsCIDImagesForPrintPreview(t *testing.T) {
	got := HTMLToMarkdownWithCIDImages(`<p>Before</p><img alt="Inline chart" src="cid:chart-001@example.test"><p>After</p>`)
	for _, want := range []string{"Before", "![Inline chart](cid:chart-001@example.test)", "After"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected CID image markdown %q, got:\n%s", want, got)
		}
	}
}

func TestHTMLToMarkdownWithCIDImagesPreservesImageSizingHintsForPrint(t *testing.T) {
	got := HTMLToMarkdownWithCIDImages(`<p>
		<img alt="App icon" src="https://example.test/icon.png" width="96" height="48" style="max-width:96px;height:auto">
	</p>`)
	for _, want := range []string{
		`<img src="https://example.test/icon.png"`,
		`alt="App icon"`,
		`width="96"`,
		`height="48"`,
		`style="max-width:96px;height:auto"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected printable image sizing fragment %q, got:\n%s", want, got)
		}
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

func TestHTMLToMarkdownKeepsDataTablesWithEmailStylingAttrs(t *testing.T) {
	got := HTMLToMarkdown(`<table width="100%" border="1" cellpadding="4" cellspacing="0">
		<tr><th>Applicant</th><th>Status</th></tr>
		<tr><td>Anton</td><td>Approved</td></tr>
	</table>`)

	for _, want := range []string{
		"| Applicant | Status |",
		"| --- | --- |",
		"| Anton | Approved |",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected styled data table row %q, got:\n%s", want, got)
		}
	}
}

func TestHTMLToMarkdownFlattensEmailLayoutTables(t *testing.T) {
	got := HTMLToMarkdown(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
		<tr>
			<td>
				<table>
					<tr><td><h1>GOV.UK</h1></td></tr>
					<tr><td>Dear Anton Golubtsov</td></tr>
					<tr><td>You have created your UKVI account.</td></tr>
					<tr><td><a href="https://example.test/sign-in">Sign in to your UKVI account</a></td></tr>
				</table>
			</td>
		</tr>
	</table>`)

	for _, want := range []string{
		"GOV.UK",
		"Dear Anton Golubtsov",
		"You have created your UKVI account.",
		"[Sign in to your UKVI account](https://example.test/sign-in)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("layout table markdown missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "|") || strings.Contains(got, "---") {
		t.Fatalf("layout table should not render as a markdown table:\n%s", got)
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

func TestNormalizeEmailTextArtifactsRemovesNBSPMojibakeButKeepsUnicode(t *testing.T) {
	got := NormalizeEmailTextArtifacts("Before\n\nÂ\u00a0\nPro Tip!\nName: Âge\nBad�char")
	for _, bad := range []string{"Â\u00a0", "\u00a0", "�"} {
		if strings.Contains(got, bad) {
			t.Fatalf("normalized text retained %q:\n%q", bad, got)
		}
	}
	for _, want := range []string{"Before", "Pro Tip!", "Name: Âge", "Bad char"} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized text missing %q:\n%q", want, got)
		}
	}
}

func TestHTMLToMarkdownNormalizesMojibakeNBSPSeparators(t *testing.T) {
	got := HTMLToMarkdown(`<p>Hi Anton,</p><p>Â&nbsp;</p><p>Pro Tip!</p>`)
	if strings.Contains(got, "Â") || strings.Contains(got, "\u00a0") || strings.Contains(got, "�") {
		t.Fatalf("HTML markdown retained charset artifact:\n%q", got)
	}
	if !strings.Contains(got, "Hi Anton") || !strings.Contains(got, "Pro Tip!") {
		t.Fatalf("HTML markdown lost expected content:\n%q", got)
	}
}
