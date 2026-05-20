package testmail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusFixturesValidate(t *testing.T) {
	root := filepath.Join("testdata", "corpus")
	if err := ValidateCorpus(root); err != nil {
		t.Fatalf("ValidateCorpus(%s): %v", root, err)
	}
}

func TestCorpusScenariosCoverPlannedRealisticCases(t *testing.T) {
	scenarios, err := LoadCorpus(filepath.Join("testdata", "corpus"))
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	want := []string{
		"plain-thread",
		"calendly-invite",
		"newsletter-table",
		"receipt-html",
		"malformed-charset",
		"inline-cid-image",
		"long-link-tracking",
	}
	for _, name := range want {
		scenario, ok := scenarios[name]
		if !ok {
			t.Fatalf("missing corpus scenario %q", name)
		}
		if len(scenario.Messages) == 0 {
			t.Fatalf("scenario %q has no messages", name)
		}
	}
}

func TestCalendlyLikeCorpusPreservesCalendarShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "corpus", "calendly-invite", "invite.eml"))
	if err != nil {
		t.Fatalf("read calendly-like fixture: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"Content-Type: text/calendar", "BEGIN:VCALENDAR", "METHOD:REQUEST", "ATTENDEE", "ORGANIZER"} {
		if !strings.Contains(text, want) {
			t.Fatalf("calendar fixture missing %q:\n%s", want, text)
		}
	}
}
