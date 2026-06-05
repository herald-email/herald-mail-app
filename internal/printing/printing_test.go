package printing

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/demo"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func samplePrintEmail() *models.EmailData {
	return &models.EmailData{
		MessageID: "print-1",
		Sender:    "Alice <alice@example.test>",
		Subject:   `Budget <alert> & review`,
		Date:      time.Date(2026, 6, 5, 9, 30, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
}

func samplePrintBody() *models.EmailBody {
	return &models.EmailBody{
		From:      "Alice <alice@example.test>",
		To:        "Team <team@example.test>",
		CC:        "Finance <finance@example.test>",
		Subject:   `Budget <alert> & review`,
		TextPlain: "Plain fallback",
		TextHTML: `<html><body>
			<h1 onclick="bad()">Budget <em>alert</em></h1>
			<script>steal()</script>
			<p>Open <a href="javascript:alert(1)">bad</a> and <a href="https://safe.example.test/path?utm_source=email&id=42">safe</a>.</p>
			<img src="cid:chart1" alt="Chart">
			<img src="https://tracker.example.test/open.gif" alt="tracker">
		</body></html>`,
		InlineImages: []models.InlineImage{{
			ContentID: "chart1",
			MIMEType:  "image/png",
			Data:      []byte{0x89, 0x50, 0x4e, 0x47},
		}},
		Attachments: []models.Attachment{{
			Filename: "report.pdf",
			MIMEType: "application/pdf",
			Size:     2048,
		}},
	}
}

func TestBuildHTMLDocumentOriginalVisualSanitizesAndEmbedsCIDImages(t *testing.T) {
	html, err := BuildHTMLDocument(Request{
		Email: samplePrintEmail(),
		Body:  samplePrintBody(),
		Mode:  ModeOriginalVisual,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{
		"Alice &lt;alice@example.test&gt;",
		"Team &lt;team@example.test&gt;",
		"Finance &lt;finance@example.test&gt;",
		"Budget &lt;alert&gt; &amp; review",
		"Budget <em>alert</em>",
		`src="data:image/png;base64,iVBORw=="`,
		"report.pdf",
		"application/pdf",
		"2.0 KiB",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("print HTML missing %q:\n%s", want, html)
		}
	}
	for _, forbidden := range []string{
		"<script",
		"onclick=",
		"javascript:",
		`src="https://tracker.example.test/open.gif"`,
		"utm_source=email",
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("print HTML contained forbidden %q:\n%s", forbidden, html)
		}
	}
}

func TestBuildHTMLDocumentOriginalVisualAllowsRemoteImagesWhenExplicitlyRequested(t *testing.T) {
	body := samplePrintBody()
	body.TextHTML = `<html><body>
		<p>Original layout image:</p>
		<img src="https://cdn.example.test/newsletter/logo.png?utm_source=email" alt="Newsletter logo" width="96" height="48">
	</body></html>`
	html, err := BuildHTMLDocument(Request{
		Email:             samplePrintEmail(),
		Body:              body,
		Mode:              ModeOriginalVisual,
		AllowRemoteImages: true,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{
		`img-src data: https: http:`,
		`<img src="https://cdn.example.test/newsletter/logo.png`,
		`alt="Newsletter logo"`,
		`width="96"`,
		`height="48"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("original visual print HTML with remote images missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "Remote image blocked") || strings.Contains(html, "utm_source=email") {
		t.Fatalf("original visual print HTML did not opt in cleanly to remote images:\n%s", html)
	}
}

func TestBuildHTMLDocumentRenderedMarkdownUsesHeraldReadingRepresentation(t *testing.T) {
	body := samplePrintBody()
	body.TextPlain = "stale plain fallback"
	body.TextHTML = `<h1>HTML wins</h1><p><strong>Budget alert</strong> for <em>Project Orion</em>.</p>`
	html, err := BuildHTMLDocument(Request{
		Email: samplePrintEmail(),
		Body:  body,
		Mode:  ModeRenderedMarkdown,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{"HTML wins", "<strong>Budget alert</strong>", "<em>Project Orion</em>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered Markdown print HTML missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "stale plain fallback") || strings.Contains(html, "# HTML wins") {
		t.Fatalf("rendered Markdown print HTML did not use rendered preview representation:\n%s", html)
	}
}

func TestBuildHTMLDocumentRenderedMarkdownPreservesImageSizingHints(t *testing.T) {
	body := samplePrintBody()
	body.TextHTML = `<p>
		<img alt="App icon" src="https://example.test/icon.png" width="96" height="48" style="max-width:96px;height:auto">
	</p>`
	html, err := BuildHTMLDocument(Request{
		Email:             samplePrintEmail(),
		Body:              body,
		Mode:              ModeRenderedMarkdown,
		Theme:             ThemeSwiss,
		AllowRemoteImages: true,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{
		`<img src="https://example.test/icon.png"`,
		`alt="App icon"`,
		`width="96"`,
		`height="48"`,
		`style="max-width:96px;height:auto"`,
		`max-height: min(520px, 68vh)`,
		`body:not([data-print-theme="original"]) .message-body img { display: block;`,
		`class="print-image-frame"`,
		`break-inside: avoid`,
		`page-break-inside: avoid`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered Markdown print HTML missing image sizing fragment %q:\n%s", want, html)
		}
	}
}

func TestBuildHTMLDocumentOriginalVisualDoesNotWrapSenderImagesInPrintFrames(t *testing.T) {
	body := samplePrintBody()
	body.TextHTML = `<p><img alt="Sender layout image" src="https://example.test/layout.png" width="96" height="48"></p>`
	html, err := BuildHTMLDocument(Request{
		Email:             samplePrintEmail(),
		Body:              body,
		Mode:              ModeOriginalVisual,
		AllowRemoteImages: true,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	if strings.Contains(html, `class="print-image-frame"`) {
		t.Fatalf("original visual print HTML should preserve sender image structure:\n%s", html)
	}
	if !strings.Contains(html, `<p><img`) {
		t.Fatalf("original visual print HTML did not preserve paragraph image structure:\n%s", html)
	}
}

func TestBuildHTMLDocumentRenderedMarkdownRendersDataTables(t *testing.T) {
	body := samplePrintBody()
	body.TextHTML = `<table>
		<tr><th>Item</th><th>Qty</th><th>Total</th></tr>
		<tr><td>Adult</td><td>2</td><td>GBP 64.00</td></tr>
		<tr><td>Child</td><td>2</td><td>GBP 32.00</td></tr>
	</table>`
	html, err := BuildHTMLDocument(Request{
		Email: samplePrintEmail(),
		Body:  body,
		Mode:  ModeRenderedMarkdown,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{"<table>", "<th>Item</th>", "<td>Adult</td>", "<td>GBP 64.00</td>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered Markdown print HTML missing table fragment %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "| Item | Qty | Total |") {
		t.Fatalf("rendered Markdown print HTML left table as pipe text:\n%s", html)
	}
}

func TestBuildHTMLDocumentRenderedMarkdownAppliesSelectedTheme(t *testing.T) {
	body := samplePrintBody()
	body.TextHTML = `<h1>Receipt</h1><p>Markdown styled print.</p>`
	html, err := BuildHTMLDocument(Request{
		Email: samplePrintEmail(),
		Body:  body,
		Mode:  ModeRenderedMarkdown,
		Theme: ThemeAcademic,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{
		`data-print-theme="academic"`,
		"Crimson Text",
		"border-left: 4px solid #2f5d50",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered Markdown print HTML missing theme fragment %q:\n%s", want, html)
		}
	}
}

func TestBuildHTMLDocumentRenderedMarkdownThemesEmitOnlySelectedCSS(t *testing.T) {
	body := samplePrintBody()
	seen := map[Theme]string{}
	for _, option := range MarkdownThemes() {
		html, err := BuildHTMLDocument(Request{
			Email: samplePrintEmail(),
			Body:  body,
			Mode:  ModeRenderedMarkdown,
			Theme: option.ID,
		})
		if err != nil {
			t.Fatalf("BuildHTMLDocument(%s) error: %v", option.ID, err)
		}
		selected := `body[data-print-theme="` + string(option.ID) + `"]`
		if !strings.Contains(html, selected) {
			t.Fatalf("%s theme HTML missing selected theme CSS %q:\n%s", option.ID, selected, html)
		}
		for _, other := range MarkdownThemes() {
			if other.ID == option.ID {
				continue
			}
			unselected := `body[data-print-theme="` + string(other.ID) + `"]`
			if strings.Contains(html, unselected) {
				t.Fatalf("%s theme HTML includes unselected theme CSS %q", option.ID, unselected)
			}
		}
		normalized := strings.ReplaceAll(html, `data-print-theme="`+string(option.ID)+`"`, `data-print-theme=""`)
		for otherTheme, otherHTML := range seen {
			if normalized == otherHTML {
				t.Fatalf("%s theme HTML matches %s after normalizing selected theme attribute", option.ID, otherTheme)
			}
		}
		seen[option.ID] = normalized
	}
}

func TestBuildHTMLDocumentRenderedMarkdownEmbedsDemoCIDImages(t *testing.T) {
	msg := findDemoMessage(t, "Step 5: View inline images in full screen")
	html, err := BuildHTMLDocument(Request{
		Email: &msg.Email,
		Body:  &msg.Body,
		Mode:  ModeRenderedMarkdown,
		Theme: ThemeGitHub,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{
		"CC BY-SA badge",
		"Color chart",
		"Bee on sunflower",
		"Changing landscape",
		`src="data:image/png;base64,`,
		`src="data:image/jpeg;base64,`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered Markdown print HTML missing image fragment %q", want)
		}
	}
	if strings.Contains(html, `src="cid:`) {
		t.Fatalf("rendered Markdown print HTML leaked cid image source:\n%s", html)
	}
}

func TestBuildHTMLDocumentRenderedMarkdownShowsDemoRemoteImagePlaceholders(t *testing.T) {
	msg := findDemoMessage(t, "[PREVIEW] Herald v0.5.0 — Calendar, and multi-account arrive")
	html, err := BuildHTMLDocument(Request{
		Email: &msg.Email,
		Body:  &msg.Body,
		Mode:  ModeRenderedMarkdown,
		Theme: ThemeSwiss,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{
		"Remote image blocked",
		"Herald sidebar showing two demo accounts",
		"Herald Calendar&#39;s 3-Day Command view",
		"assets.buttondown.email/images/8a2e3bc3-a4ba-4b8b-bf1e-1c58484ae066.png",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered Markdown print HTML missing remote image placeholder fragment %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, `<img src="https://`) || strings.Contains(html, `src="https://assets.buttondown.email`) {
		t.Fatalf("rendered Markdown print HTML emitted loadable remote image:\n%s", html)
	}
}

func TestBuildHTMLDocumentRenderedMarkdownAllowsDemoRemoteImagesWhenExplicitlyRequested(t *testing.T) {
	msg := findDemoMessage(t, "[PREVIEW] Herald v0.5.0 — Calendar, and multi-account arrive")
	html, err := BuildHTMLDocument(Request{
		Email:             &msg.Email,
		Body:              &msg.Body,
		Mode:              ModeRenderedMarkdown,
		Theme:             ThemeSwiss,
		AllowRemoteImages: true,
	})
	if err != nil {
		t.Fatalf("BuildHTMLDocument error: %v", err)
	}
	for _, want := range []string{
		`img-src data: https: http:`,
		`<img src="https://assets.buttondown.email/images/8a2e3bc3-a4ba-4b8b-bf1e-1c58484ae066.png`,
		`<img src="https://assets.buttondown.email/images/7b1c6dbe-804d-4997-a843-984f7e86dcf8.png`,
		`alt="Herald sidebar showing two demo accounts`,
		`alt="Herald Calendar&#39;s 3-Day Command view`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered Markdown print HTML with remote images missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "Remote image blocked") {
		t.Fatalf("rendered Markdown print HTML still blocked remote image after opt-in:\n%s", html)
	}
}

func TestMarkdownPrintThemesProvideChooserOptions(t *testing.T) {
	themes := MarkdownThemes()
	if len(themes) != 4 {
		t.Fatalf("MarkdownThemes length = %d, want 4", len(themes))
	}
	for i, theme := range themes {
		if theme.Key != string('2'+rune(i)) {
			t.Fatalf("theme %d key = %q, want %q", i, theme.Key, string('2'+rune(i)))
		}
		if theme.ID == "" || theme.Name == "" {
			t.Fatalf("theme %d missing id/name: %#v", i, theme)
		}
	}
}

func findDemoMessage(t *testing.T, subject string) demo.Message {
	t.Helper()
	for _, msg := range demo.Mailbox().Messages {
		if msg.Email.Subject == subject {
			return msg
		}
	}
	t.Fatalf("demo message %q not found", subject)
	return demo.Message{}
}

func TestWriteTempHTMLUsesPrivatePermissions(t *testing.T) {
	path, err := WriteTempHTML("<html><body>private</body></html>")
	if err != nil {
		t.Fatalf("WriteTempHTML error: %v", err)
	}
	defer os.Remove(path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat temp file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("temp file permissions = %#o, want 0600", got)
	}
}

func TestUnsupportedPrinterReturnsBoundedUnsupportedResult(t *testing.T) {
	result, err := UnsupportedPrinter{Reason: "ssh session"}.Print(context.Background(), Request{
		Email: samplePrintEmail(),
		Body:  samplePrintBody(),
		Mode:  ModeOriginalVisual,
	})
	if err != nil {
		t.Fatalf("unsupported printer should not return transport error: %v", err)
	}
	if result.Status != StatusUnsupported || !strings.Contains(result.Message, "ssh session") {
		t.Fatalf("unsupported result = %#v", result)
	}
}
