package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func modifierHintTimelineModel(t *testing.T) *Model {
	t.Helper()
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)
	return m
}

func TestModifierHintPressReleaseSwitchesShiftLayer(t *testing.T) {
	m := modifierHintTimelineModel(t)

	model, _ := m.Update(tea.KeyboardEnhancementsMsg{Flags: ansi.KittyReportEventTypes})
	m = model.(*Model)
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeftShift})
	m = model.(*Model)

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "shift+tab: prev panel", "shift+↑/↓: range", "G: group", "R: sender", "D: delete now")
	if strings.Contains(hints, "ctrl+c: quit") {
		t.Fatalf("shift layer should not advertise ctrl actions, got:\n%s", hints)
	}

	model, _ = m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyLeftShift}))
	m = model.(*Model)

	hints = stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "?: help", "c: compose", "r: all")
	if strings.Contains(hints, "shift+tab: prev panel") {
		t.Fatalf("shift layer should clear after release, got:\n%s", hints)
	}
}

func TestModifierHintFallbackExpiresAfterModifiedKeypress(t *testing.T) {
	m := modifierHintTimelineModel(t)

	model, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = model.(*Model)
	if cmd == nil {
		t.Fatal("expected modified keypress to schedule fallback expiry")
	}

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "ctrl+c: quit", "ctrl+r: refresh", "ctrl+d/u: half-page")

	m.modifierHintFallbackToken = 42
	model, _ = m.Update(modifierHintExpiredMsg{Token: 42})
	m = model.(*Model)

	hints = stripANSI(m.renderKeyHints())
	if strings.Contains(hints, "ctrl+d/u: half-page") {
		t.Fatalf("ctrl fallback should clear after expiry, got:\n%s", hints)
	}
	requireHintSegments(t, hints, "?: help", "c: compose")
}

func TestModifierHintAltLayerKeepsDefaultHintsWithNotice(t *testing.T) {
	m := modifierHintTimelineModel(t)

	model, _ := m.Update(altKey('x'))
	m = model.(*Model)

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "alt: no actions here", "?: help", "c: compose")
}

func TestModifierHintLayerPrecedenceIsDeterministic(t *testing.T) {
	m := modifierHintTimelineModel(t)

	model, _ := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModShift | tea.ModAlt | tea.ModCtrl})
	m = model.(*Model)

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "ctrl+c: quit", "ctrl+r: refresh")
	if strings.Contains(hints, "alt: no actions here") || strings.Contains(hints, "shift+↑/↓: range") {
		t.Fatalf("ctrl should win over alt and shift layers, got:\n%s", hints)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModShift | tea.ModAlt})
	m = model.(*Model)

	hints = stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "alt: no actions here")
	if strings.Contains(hints, "shift+↑/↓: range") {
		t.Fatalf("alt should win over shift layer, got:\n%s", hints)
	}
}

func TestModifierHintCtrlSearchLayerShowsOnlyExistingActions(t *testing.T) {
	m := modifierHintTimelineModel(t)
	m.timeline.searchMode = true
	m.timeline.searchFocus = timelineSearchFocusInput

	model, _ := m.Update(tea.KeyPressMsg{Code: 'i', Mod: tea.ModCtrl})
	m = model.(*Model)

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "ctrl+i: server search", "ctrl+c: quit")
	if strings.Contains(hints, "ctrl+s") || strings.Contains(hints, "no-op") {
		t.Fatalf("ctrl search layer should only show existing actions, got:\n%s", hints)
	}
}

func TestModifierHintCtrlCalendarLayerShowsCalendarActions(t *testing.T) {
	b := &calendarAgendaStubBackend{available: true, events: testCalendarEvents()}
	m := New(b, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 50})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarEvents = normalizeCalendarEventsForDisplay(b.events)
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(b.events[0].Start)
	m.calendarDetail = m.selectedCalendarEvent()
	m.setCalendarView(calendarViewWeek)

	model, _ := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = model.(*Model)

	hints := stripANSI(m.renderKeyHints())
	requireHintSegments(t, hints, "ctrl+c: quit", "ctrl+r: refresh", "ctrl+u/d: page")
	if strings.TrimSpace(hints) == "ctrl+c: quit" {
		t.Fatalf("calendar ctrl layer should not collapse to quit only, got:\n%s", hints)
	}
}

func TestModifierHintsDoNotConfirmPendingDelete(t *testing.T) {
	m := modifierHintTimelineModel(t)
	confirmed := false
	m.pendingDeleteConfirm = true
	m.pendingDeleteDesc = "Delete 1 message?"
	m.pendingDeleteAction = func() tea.Cmd {
		confirmed = true
		return nil
	}

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeftShift})
	m = model.(*Model)
	if confirmed {
		t.Fatal("modifier key must not confirm pending delete")
	}
	if !m.pendingDeleteConfirm {
		t.Fatal("modifier key must leave pending delete confirmation open")
	}

	model, _ = m.Update(keyRunes("y"))
	m = model.(*Model)
	if !confirmed {
		t.Fatal("y should still confirm pending delete")
	}
	if m.pendingDeleteConfirm {
		t.Fatal("pending delete confirmation should close after y")
	}
}

func TestModifierHintsDoNotStealTextEntry(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeBody.Focus()

	model, _ := m.Update(altKey('x'))
	m = model.(*Model)

	if got := m.composeBody.Value(); got != "x" {
		t.Fatalf("alt-modified printable key should remain compose text, got %q", got)
	}
	if m.activeTab != tabCompose {
		t.Fatalf("alt text entry changed active tab to %d", m.activeTab)
	}

	m.activeTab = tabTimeline
	m.timeline.searchMode = true
	m.timeline.searchFocus = timelineSearchFocusInput
	m.timeline.searchInput.Focus()
	m.timeline.searchInput.SetValue("")
	m.timeline.emails = []*models.EmailData{{MessageID: "m1", Sender: "a@example.com", Subject: "A"}}
	m.updateTimelineTable()

	model, _ = m.Update(keyRunes("?"))
	m = model.(*Model)
	if got := m.timeline.searchInput.Value(); got != "?" {
		t.Fatalf("search prompt should keep literal question mark, got %q", got)
	}
	if m.showHelp {
		t.Fatal("question mark in search prompt should not open shortcut help")
	}

	m.activeTab = tabTimeline
	m.showPromptEditor = true
	m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
	_ = m.promptEditor.Init()

	model, _ = m.Update(altKey('z'))
	m = model.(*Model)
	if got := m.promptEditor.name; got != "z" {
		t.Fatalf("prompt editor should keep alt-modified printable text, got %q", got)
	}
	if m.showHelp {
		t.Fatal("alt-modified prompt editor text should not open shortcut help")
	}
}
