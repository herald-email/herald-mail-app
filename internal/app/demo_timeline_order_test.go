package app

import (
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
)

func TestDemoTimelineRendersWelcomeThenOnboardingStepsAsTopRows(t *testing.T) {
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
		"✉ Welcome to Herald",
		"Step 1:",
		"Step 2:",
		"Step 3:",
		"Step 4:",
		"Step 5:",
		"Step 6:",
		"Step 7:",
		"Step 8:",
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
