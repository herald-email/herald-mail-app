package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/muesli/termenv"
)

func TestRenderPreviewHeaderLines_StylesMetadataWithoutChangingText(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(oldProfile)

	email := &models.EmailData{
		Sender:  "Tech Weekly <newsletter@techweekly.example>",
		Subject: "This Week in Tech",
		Date:    time.Date(2026, 4, 22, 18, 39, 0, 0, time.UTC),
	}

	lines := renderPreviewHeaderLines(email, "news", true, 80, true)
	rendered := strings.Join(lines, "\n")
	stripped := stripANSI(rendered)

	for _, want := range []string{
		"From: Tech Weekly <newsletter@techweekly.example>",
		"Date: Wed, 22 Apr 2026 18:39",
		"Subj: This Week in Tech",
		"Tags: news",
		"Actions: u unsubscribe  h hide future mail",
	} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("expected stripped header to contain %q, got:\n%s", want, stripped)
		}
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected styled header to include ANSI color sequences, got:\n%s", rendered)
	}
}
