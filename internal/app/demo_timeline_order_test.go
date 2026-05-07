package app

import (
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
)

func TestDemoTimelineRendersWelcomeThenOnboardingExamplesAsTopRows(t *testing.T) {
	b := backend.NewDemoBackend()
	emails, err := b.GetTimelineEmails("INBOX")
	if err != nil {
		t.Fatalf("GetTimelineEmails: %v", err)
	}

	m := New(b, nil, "demo@demo.local", nil, false)
	m.activeTab = tabTimeline
	m.timeline.emails = emails
	m.updateTableDimensions(220, 50)
	m.updateTimelineTable()

	rows := m.timelineTable.Rows()
	wantSubjects := []string{
		":sparkles: :mailbox: Welcome to Herald",
		"Example 1:",
		"Example 2:",
		"Example 3:",
		"Example 4:",
		"Example 5:",
		"Example 6:",
		"Example 7:",
		"Example 8:",
	}
	if len(rows) < len(wantSubjects) {
		t.Fatalf("expected at least %d timeline rows, got %d", len(wantSubjects), len(rows))
	}

	for i, want := range wantSubjects {
		gotSubject := stripANSI(rows[i][2])
		if !strings.Contains(gotSubject, want) {
			t.Fatalf("timeline row %d subject = %q, want it to contain %q", i+1, gotSubject, want)
		}
	}
}
