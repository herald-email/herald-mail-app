package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
)

func TestDemoTimelineRendersOnboardingStepsAsTopEightRows(t *testing.T) {
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
	if len(rows) < 8 {
		t.Fatalf("expected at least 8 timeline rows, got %d", len(rows))
	}

	for i := 0; i < 8; i++ {
		want := fmt.Sprintf("Step %d:", i+1)
		gotSubject := stripANSI(rows[i][2])
		if !strings.Contains(gotSubject, want) {
			t.Fatalf("timeline row %d subject = %q, want it to contain %q", i+1, gotSubject, want)
		}
	}
}
