package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestRenderEmailPreview_AttachmentsHaveDividerAndBodyGap(t *testing.T) {
	m := makeSizedModel(t, 160, 40)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.previewWidth = 80
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "msg-attach",
		Sender:    "sender@example.com",
		Subject:   "Attachment spacing",
		Date:      time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m.timeline.bodyMessageID = "msg-attach"
	m.timeline.body = &models.EmailBody{
		TextPlain: "Body starts here.\n\nSecond paragraph.",
		Attachments: []models.Attachment{
			{Filename: "report.pdf", MIMEType: "application/pdf", Size: 2048},
		},
	}

	raw := m.renderEmailPreview()
	rendered := stripANSI(raw)
	lines := strings.Split(rendered, "\n")

	dividerIdx := indexLineContaining(lines, "Attachments ( [ and ] for selection, s for save )")
	if dividerIdx < 0 {
		t.Fatalf("expected attachment divider label, got:\n%s", rendered)
	}
	attachIdx := indexLineContaining(lines, "[attach] report.pdf")
	if attachIdx < 0 {
		t.Fatalf("expected attachment row, got:\n%s", rendered)
	}
	bodyIdx := indexLineContaining(lines, "Body starts here")
	if bodyIdx < 0 {
		t.Fatalf("expected body content, got:\n%s", rendered)
	}
	if dividerIdx != attachIdx+1 {
		t.Fatalf("attachment divider should be directly below attachment rows, divider=%d attach=%d:\n%s", dividerIdx, attachIdx, rendered)
	}
	if bodyIdx < dividerIdx+2 {
		t.Fatalf("expected a blank content row between divider and body, divider=%d body=%d:\n%s", dividerIdx, bodyIdx, rendered)
	}

	rawDivider := rawLineContaining(raw, "Attachments ( [ and ] for selection, s for save )")
	for _, muted := range []string{"\x1b[2m", "\x1b[90m"} {
		if strings.Contains(rawDivider, muted) {
			t.Fatalf("attachment divider should render bright like the header rule, got raw line %q", rawDivider)
		}
	}
}

func TestPreviewAttachmentDivider_UsesRuleGlyphsAtNarrowSplitWidth(t *testing.T) {
	line := previewAttachmentDivider(37)
	if ansi.StringWidth(line) > 37 {
		t.Fatalf("divider width = %d, want <= 37: %q", ansi.StringWidth(line), line)
	}
	for _, want := range []string{"─", "Attachments", "[/]", "s"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected compact divider to include %q, got %q", want, line)
		}
	}
}

func indexLineContaining(lines []string, needle string) int {
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func rawLineContaining(rendered, needle string) string {
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(stripANSI(line), needle) {
			return line
		}
	}
	return ""
}
