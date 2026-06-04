package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func mouseDrag(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func mouseRelease(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func makePreviewMouseTimelineModel(t *testing.T) *Model {
	t.Helper()
	m := makeMouseTimelineModel(t)
	m.timeline.selectedEmail = &models.EmailData{
		MessageID: "preview-mouse-1",
		Sender:    "Rae Stone <rae@cobalt-works.example>",
		Subject:   "Preview selection launch",
		Date:      time.Date(2026, 6, 3, 12, 15, 0, 0, time.UTC),
		Folder:    "INBOX",
		UID:       42,
	}
	m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
	m.timeline.body = &models.EmailBody{
		To:        "Mina Park <mina@cobalt-works.example>",
		CC:        "Rowan Finch <rowan@example.net>",
		TextPlain: "First body line with address body@example.com.\nSecond body line for drag selection.",
	}
	m.timeline.bodyLoading = false
	m.setFocusedPanel(panelPreview)
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	return m
}

func timelinePreviewContentPointForTest(t *testing.T, m *Model, row, col int) (int, int) {
	t.Helper()
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	top := m.mouseContentTop()
	x := 0
	if plan.SidebarVisible {
		x += sidebarContentWidth + 2 + panelGapWidth
	}
	tableRect := mouseRect{x: x, y: top, w: m.timelineTable.Width() + 2, h: m.mousePanelHeight()}
	previewRect := mouseRect{x: tableRect.x + tableRect.w + panelGapWidth, y: top, w: m.timeline.previewWidth, h: m.mousePanelHeight()}
	return previewRect.x + 2 + col, previewRect.y + 1 + row
}

func contactsPreviewContentPointForTest(t *testing.T, m *Model, row, col int) (int, int) {
	t.Helper()
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	rect := mouseRect{
		x: plan.Contacts.ListWidth + panelGapWidth,
		y: m.mouseContentTop(),
		w: plan.Contacts.DetailWidth,
		h: m.mousePanelHeight(),
	}
	return rect.x + 2 + col, rect.y + 1 + row
}

func TestPreviewMouseSelectionTimelineSplitSelectsHeaderAddressAndCopiesText(t *testing.T) {
	m := makePreviewMouseTimelineModel(t)
	startX, startY := timelinePreviewContentPointForTest(t, m, 0, 6)
	endX, endY := timelinePreviewContentPointForTest(t, m, 1, 64)

	model, _ := m.Update(mousePress(startX, startY))
	model, _ = model.(*Model).Update(mouseDrag(endX, endY))
	model, _ = model.(*Model).Update(mouseRelease(endX, endY))
	updated := model.(*Model)

	if !updated.previewSelection.Active {
		t.Fatal("expected split preview mouse drag to leave an active selection")
	}
	selected := updated.activePreviewSelectionPlainText()
	for _, want := range []string{"Rae Stone", "rae@cobalt-works.example", "Mina Park", "mina@cobalt-works.example"} {
		if !strings.Contains(selected, want) {
			t.Fatalf("selected text missing %q:\n%s", want, selected)
		}
	}

	model, cmd, handled := updated.handleTimelineKey(keyRunes("y"))
	if !handled {
		t.Fatal("expected y to handle the active mouse selection")
	}
	if cmd == nil {
		t.Fatal("expected y to return a clipboard command for the selected preview text")
	}
	if model.(*Model).previewSelection.Active {
		t.Fatal("expected y to clear the active mouse selection after copy")
	}
}

func TestPreviewMouseSelectionTimelineSplitCanSpanHeaderAndBody(t *testing.T) {
	m := makePreviewMouseTimelineModel(t)
	startX, startY := timelinePreviewContentPointForTest(t, m, 2, 6)
	endX, endY := timelinePreviewContentPointForTest(t, m, 8, 22)

	model, _ := m.Update(mousePress(startX, startY))
	model, _ = model.(*Model).Update(mouseDrag(endX, endY))
	updated := model.(*Model)

	selected := updated.activePreviewSelectionPlainText()
	for _, want := range []string{"Preview selection launch", "First body line"} {
		if !strings.Contains(selected, want) {
			t.Fatalf("selected text missing %q:\n%s", want, selected)
		}
	}

	model, _ = updated.Update(mouseWheelDown(endX, endY))
	updated = model.(*Model)
	if updated.timeline.bodyScrollOffset == 0 {
		t.Fatal("expected preview wheel to keep scrolling after mouse selection")
	}
}

func TestPreviewMouseSelectionTimelineFullScreenMapsHeaderAndBody(t *testing.T) {
	m := makePreviewMouseTimelineModel(t)
	m.timeline.fullScreen = true
	model, _ := m.Update(mousePress(6, 0))
	model, _ = model.(*Model).Update(mouseDrag(25, 8))
	updated := model.(*Model)

	if !updated.previewSelection.Active {
		t.Fatal("expected full-screen preview drag to activate selection")
	}
	selected := updated.activePreviewSelectionPlainText()
	for _, want := range []string{"Rae Stone", "First body line"} {
		if !strings.Contains(selected, want) {
			t.Fatalf("selected text missing %q:\n%s", want, selected)
		}
	}
}

func TestPreviewMouseSelectionContactsInlinePreviewSelectsHeaderAndBody(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabContacts
	m.contactsList = []models.ContactData{{Email: "rae@cobalt-works.example", DisplayName: "Rae Stone"}}
	m.contactsFiltered = m.contactsList
	m.contactDetail = &m.contactsList[0]
	m.contactFocusPanel = 1
	m.contactPreviewEmail = &models.EmailData{
		MessageID: "contact-preview-1",
		Sender:    "Rae Stone <rae@cobalt-works.example>",
		Subject:   "Inline contacts preview",
		Date:      time.Date(2026, 6, 3, 13, 0, 0, 0, time.UTC),
	}
	m.contactPreviewBody = &models.EmailBody{TextPlain: "Contact preview body with contact-body@example.com."}
	m.contactPreviewLoading = false

	startX, startY := contactsPreviewContentPointForTest(t, m, 0, 6)
	endX, endY := contactsPreviewContentPointForTest(t, m, 5, 26)
	model, _ := m.Update(mousePress(startX, startY))
	model, _ = model.(*Model).Update(mouseDrag(endX, endY))
	updated := model.(*Model)

	if !updated.previewSelection.Active {
		t.Fatal("expected Contacts inline preview drag to activate selection")
	}
	selected := updated.activePreviewSelectionPlainText()
	for _, want := range []string{"Rae Stone", "Inline contacts preview", "Contact preview body"} {
		if !strings.Contains(selected, want) {
			t.Fatalf("selected text missing %q:\n%s", want, selected)
		}
	}
}

func TestPreviewMouseSelectionEscClearsBeforeClosingPreview(t *testing.T) {
	m := makePreviewMouseTimelineModel(t)
	startX, startY := timelinePreviewContentPointForTest(t, m, 0, 6)
	endX, endY := timelinePreviewContentPointForTest(t, m, 1, 30)
	model, _ := m.Update(mousePress(startX, startY))
	model, _ = model.(*Model).Update(mouseDrag(endX, endY))
	updated := model.(*Model)

	model, cmd := updated.handleEscKey()
	updated = model.(*Model)
	if cmd != nil {
		t.Fatal("expected clearing preview selection not to request a command")
	}
	if updated.previewSelection.Active {
		t.Fatal("expected Esc to clear active preview selection")
	}
	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected first Esc to preserve open preview after clearing selection")
	}
}

func TestPreviewMouseSelectionContactsSearchStillTypesLiteralM(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabContacts
	m.contactSearchMode = "keyword"

	model, _ := m.Update(keyRunes("m"))
	updated := model.(*Model)
	if got := updated.contactSearch; got != "m" {
		t.Fatalf("contact search = %q, want literal m", got)
	}
	if updated.mouseSelectionMode {
		t.Fatal("literal m in Contacts search toggled mouse capture release")
	}
}

func TestContactsPreviewMouseModeToggleReleasesAndRestoresCapture(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabContacts
	m.contactPreviewEmail = &models.EmailData{MessageID: "contact-preview-mouse-mode"}

	model, cmd := m.Update(keyRunes("m"))
	updated := model.(*Model)
	if cmd != nil {
		t.Fatal("expected Contacts m fallback to update mouse capture through the next view")
	}
	if !updated.mouseSelectionMode {
		t.Fatal("expected m in Contacts preview to enter terminal mouse-selection mode")
	}
	if got := updated.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("MouseMode=%v, want MouseModeNone", got)
	}

	model, cmd = updated.Update(keyRunes("m"))
	updated = model.(*Model)
	if cmd != nil {
		t.Fatal("expected second Contacts m fallback to update mouse capture through the next view")
	}
	if updated.mouseSelectionMode {
		t.Fatal("expected second m in Contacts preview to restore TUI mouse capture")
	}
	if got := updated.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("MouseMode=%v, want MouseModeCellMotion", got)
	}
}

func TestContactsPreviewMouseSelectionHintsAdvertiseDragCopyAndMouseMode(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabContacts
	m.contactPreviewEmail = &models.EmailData{
		MessageID: "contact-preview-hints",
		Sender:    "Rae Stone <rae@cobalt-works.example>",
		Subject:   "Hint check",
		Date:      time.Date(2026, 6, 3, 13, 30, 0, 0, time.UTC),
	}
	m.contactPreviewBody = &models.EmailBody{TextPlain: "Hint body"}

	hints := stripANSI(m.renderKeyHints())
	for _, want := range []string{"drag: select", "y: copy selection", "m: mouse mode"} {
		if !strings.Contains(hints, want) {
			t.Fatalf("Contacts preview hints missing %q:\n%s", want, hints)
		}
	}

	m.previewSelection = previewSelectionState{
		Active:  true,
		Surface: previewSelectionContacts,
		Anchor:  previewSelectionPoint{Row: 0, Col: 0},
		Cursor:  previewSelectionPoint{Row: 1, Col: 4},
	}
	hints = stripANSI(m.renderKeyHints())
	for _, want := range []string{"drag: extend selection", "y: copy selection", "esc: clear selection"} {
		if !strings.Contains(hints, want) {
			t.Fatalf("active Contacts selection hints missing %q:\n%s", want, hints)
		}
	}

	m.clearPreviewSelection()
	m.mouseSelectionMode = true
	hints = stripANSI(m.renderKeyHints())
	if !strings.Contains(hints, "[mouse] select mode - m: restore TUI") {
		t.Fatalf("Contacts mouse-release hints missing restore affordance:\n%s", hints)
	}
}

func TestContactsShortcutHelpAdvertisesPreviewMouseSelection(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabContacts
	m.contactPreviewEmail = &models.EmailData{MessageID: "contact-preview-help"}

	section := m.contactsShortcutHelpSection()
	joined := section.Title
	for _, entry := range section.Entries {
		joined += "\n" + entry.Key + " " + entry.Desc
	}
	for _, want := range []string{"Mouse drag", "select visible header", "y copy", "m release"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Contacts shortcut help missing %q:\n%s", want, joined)
		}
	}
}
