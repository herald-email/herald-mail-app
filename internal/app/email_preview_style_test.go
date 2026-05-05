package app

import (
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestRenderPreviewHeaderLines_StylesMetadataWithoutChangingText(t *testing.T) {
	loc := time.FixedZone("PDT", -7*60*60)
	email := &models.EmailData{
		Sender:  "Tech Weekly <newsletter@techweekly.example>",
		Subject: "This Week in Tech",
		Date:    time.Date(2026, 4, 22, 11, 39, 0, 0, loc),
	}

	lines := renderPreviewHeaderLines(email, "news", true, 80, true)
	rendered := strings.Join(lines, "\n")
	stripped := stripANSI(rendered)

	for _, want := range []string{
		"From: Tech Weekly <newsletter@techweekly.example>",
		"Date: Wed, Apr 22, 2026 at 11:39 AM",
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
