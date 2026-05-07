package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
)

func makeDemoWelcomeModel(t *testing.T, width, height int) *Model {
	t.Helper()
	b := backend.NewDemoBackend()
	emails, err := b.GetTimelineEmails("INBOX")
	if err != nil {
		t.Fatalf("GetTimelineEmails: %v", err)
	}

	m := New(b, nil, "demo@demo.local", nil, false)
	m.activeTab = tabTimeline
	m.timeline.emails = emails
	m.loading = false
	m.updateTableDimensions(width, height)
	m.updateTimelineTable()
	return m
}

func TestDemoWelcomeOverlayVisibleOnlyInDemo(t *testing.T) {
	demo := makeDemoWelcomeModel(t, 120, 40)
	rendered := stripANSI(demo.View().Content)
	for _, want := range []string{
		"Welcome to Herald Demo",
		"safe synthetic mailbox",
		"Open the first email",
		"Esc / Space / Enter: continue",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected demo welcome overlay to include %q, got:\n%s", want, rendered)
		}
	}

	regular := makeSizedModel(t, 120, 40)
	regular.activeTab = tabTimeline
	regular.timeline.emails = mockEmails()
	regular.updateTimelineTable()
	if strings.Contains(stripANSI(regular.View().Content), "Welcome to Herald Demo") {
		t.Fatalf("non-demo sessions must not show demo welcome overlay:\n%s", stripANSI(regular.View().Content))
	}
}

func TestDemoWelcomeOverlayDismissKeysDoNotReachTimeline(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{name: "escape", msg: tea.KeyPressMsg{Code: tea.KeyEsc}},
		{name: "space", msg: keyRunes(" ")},
		{name: "enter", msg: tea.KeyPressMsg{Code: tea.KeyEnter}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeDemoWelcomeModel(t, 120, 40)
			initialCursor := m.timelineTable.Cursor()

			model, _ := m.Update(tt.msg)
			updated := model.(*Model)
			rendered := stripANSI(updated.View().Content)

			if strings.Contains(rendered, "Welcome to Herald Demo") {
				t.Fatalf("expected %s to dismiss demo welcome overlay, got:\n%s", tt.name, rendered)
			}
			if updated.timeline.selectedEmail != nil {
				t.Fatalf("%s should not open the first Timeline email while dismissing overlay", tt.name)
			}
			if got := updated.timelineTable.Cursor(); got != initialCursor {
				t.Fatalf("%s should not move Timeline cursor while dismissing overlay, got %d want %d", tt.name, got, initialCursor)
			}
			if updated.timelineSelectedCount() != 0 {
				t.Fatalf("%s should not toggle Timeline selection while dismissing overlay", tt.name)
			}
		})
	}
}

func TestDemoWelcomeOverlaySwallowsNonDismissKeys(t *testing.T) {
	m := makeDemoWelcomeModel(t, 120, 40)
	initialCursor := m.timelineTable.Cursor()

	model, _ := m.Update(keyRunes("j"))
	updated := model.(*Model)
	rendered := stripANSI(updated.View().Content)

	if !strings.Contains(rendered, "Welcome to Herald Demo") {
		t.Fatalf("expected non-dismiss key to leave demo welcome overlay visible, got:\n%s", rendered)
	}
	if got := updated.timelineTable.Cursor(); got != initialCursor {
		t.Fatalf("non-dismiss key should not move Timeline cursor, got %d want %d", got, initialCursor)
	}
	if updated.timeline.selectedEmail != nil {
		t.Fatal("non-dismiss key should not open Timeline preview")
	}
}

func TestDemoWelcomeOverlayFitsCanonicalSizes(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 220, height: 50},
		{width: 80, height: 24},
	} {
		t.Run("", func(t *testing.T) {
			m := makeDemoWelcomeModel(t, size.width, size.height)
			rendered := m.View().Content
			assertFitsWidth(t, size.width, rendered)
			assertFitsHeight(t, size.height, rendered)
			if !strings.Contains(stripANSI(rendered), "Welcome to Herald Demo") {
				t.Fatalf("expected demo welcome overlay at %dx%d, got:\n%s", size.width, size.height, stripANSI(rendered))
			}
		})
	}
}

func TestDemoWelcomeOverlayDefersToMinimumSizeGuard(t *testing.T) {
	m := makeDemoWelcomeModel(t, 50, 15)
	rendered := stripANSI(m.View().Content)

	if strings.Contains(rendered, "Welcome to Herald Demo") {
		t.Fatalf("minimum-size guard should hide demo welcome overlay, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "60") || !strings.Contains(strings.ToLower(rendered), "resize") {
		t.Fatalf("expected minimum-size guard at 50x15, got:\n%s", rendered)
	}
}

func TestDemoWelcomeOverlayDismissHintUsesActionForeground(t *testing.T) {
	panel := renderDemoWelcomePanel(80)
	want := lipgloss.NewStyle().
		Foreground(defaultTheme.Severity.Success.ForegroundColor()).
		Bold(true).
		Render("Esc / Space / Enter: continue")

	if !strings.Contains(panel, want) {
		t.Fatalf("expected dismiss hint to use action foreground style %q, got:\n%q", want, panel)
	}
}
