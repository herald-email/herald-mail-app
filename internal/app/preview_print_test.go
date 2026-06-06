package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/printing"
)

type fakePreviewPrinter struct {
	result   printing.Result
	err      error
	requests []printing.Request
}

func (f *fakePreviewPrinter) Print(_ context.Context, req printing.Request) (printing.Result, error) {
	f.requests = append(f.requests, req)
	return f.result, f.err
}

func printTestModel() (*Model, *fakePreviewPrinter) {
	printer := &fakePreviewPrinter{result: printing.Result{Status: printing.StatusOpened, Message: "Print dialog opened"}}
	m := New(&stubBackend{}, nil, "", nil, false)
	m.SetPreviewPrinter(printer)
	m.loading = false
	m.windowWidth = 120
	m.windowHeight = 40
	email := &models.EmailData{
		MessageID: "print-msg",
		Sender:    "Preview Lab <preview@example.test>",
		Subject:   "Printable preview",
		Date:      time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
		UID:       42,
	}
	m.timeline.emails = []*models.EmailData{email}
	m.timeline.selectedEmail = email
	m.timeline.body = &models.EmailBody{
		From:      email.Sender,
		To:        "reader@example.test",
		Subject:   email.Subject,
		TextPlain: "Printable **body**",
	}
	m.focusedPanel = panelPreview
	return m, printer
}

func TestTimelinePreviewPrintChooserOpensAndCancels(t *testing.T) {
	m, _ := printTestModel()
	model, _, handled := m.handleTimelineKey(keyRunes("p"))
	if !handled {
		t.Fatal("expected p in loaded preview to open print chooser")
	}
	updated := model.(*Model)
	if !updated.previewPrintChooser {
		t.Fatal("print chooser was not opened")
	}
	rendered := stripANSI(updated.View().Content)
	for _, want := range []string{"Print Preview", "[1] Original Visual", "[2] Markdown Swiss", "[3] Markdown GitHub", "[4] Markdown Manuscript", "[5] Markdown Academic", "Esc cancel"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("print chooser missing %q:\n%s", want, rendered)
		}
	}

	model, _, handled = updated.handleOverlayKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatal("expected Esc to close print chooser")
	}
	updated = model.(*Model)
	if updated.previewPrintChooser {
		t.Fatal("print chooser remained open after Esc")
	}
	if !strings.Contains(updated.statusMessage, "Print canceled") {
		t.Fatalf("statusMessage = %q, want print cancellation", updated.statusMessage)
	}
}

func TestTimelineListPrintOpensChooserWhenPreviewBodyLoaded(t *testing.T) {
	m, _ := printTestModel()
	m.focusedPanel = panelTimeline
	model, _, handled := m.handleTimelineKey(keyRunes("p"))
	if !handled {
		t.Fatal("expected p from Timeline list focus to open print chooser when preview body is loaded")
	}
	if !model.(*Model).previewPrintChooser {
		t.Fatal("print chooser was not opened from Timeline list focus")
	}
}

func TestTimelineListPrintLoadsHighlightedRowThenOpensChooser(t *testing.T) {
	m, _ := printTestModel()
	email := m.timeline.emails[0]
	m.focusedPanel = panelTimeline
	m.timeline.selectedEmail = nil
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyLoading = false
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)

	model, cmd, handled := m.handleTimelineKey(keyRunes("p"))
	if !handled {
		t.Fatal("expected p from highlighted Timeline row to start print load")
	}
	updated := model.(*Model)
	if cmd == nil {
		t.Fatal("expected p from highlighted row to return a body load command")
	}
	if !updated.previewPrintPending {
		t.Fatal("expected pending print intent while body loads")
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != email.MessageID {
		t.Fatalf("selectedEmail = %#v, want highlighted row", updated.timeline.selectedEmail)
	}
	if !updated.timeline.bodyLoading {
		t.Fatal("expected body load to start")
	}
	if !strings.Contains(updated.statusMessage, "Loading preview for print") {
		t.Fatalf("statusMessage = %q, want print loading notice", updated.statusMessage)
	}

	body := &models.EmailBody{
		From:      email.Sender,
		To:        "reader@example.test",
		Subject:   email.Subject,
		TextPlain: "Loaded for print",
	}
	model, _, handled = updated.handleTimelineMsg(EmailBodyMsg{
		Body:      body,
		MessageID: email.MessageID,
		Folder:    email.Folder,
		UID:       email.UID,
	})
	if !handled {
		t.Fatal("expected body load message to be handled")
	}
	updated = model.(*Model)
	if updated.previewPrintPending {
		t.Fatal("pending print intent was not cleared")
	}
	if !updated.previewPrintChooser {
		t.Fatal("print chooser was not opened after body loaded")
	}
	rendered := stripANSI(updated.View().Content)
	if !strings.Contains(rendered, "Print Preview") || !strings.Contains(rendered, "[ OFF ]") {
		t.Fatalf("print chooser did not render after pending body load:\n%s", rendered)
	}
}

func TestPreviewPrintChooserCompactsAtMinimumSize(t *testing.T) {
	m, _ := printTestModel()
	m.windowWidth = 60
	m.windowHeight = 15
	model, _, handled := m.handleTimelineKey(keyRunes("p"))
	if !handled {
		t.Fatal("expected p in loaded preview to open print chooser")
	}
	rendered := stripANSI(model.(*Model).View().Content)
	for _, want := range []string{"Print Preview", "[1] Original Visual", "[2] Swiss", "[3] GitHub", "[4] Manuscript", "[5] Academic", "Esc cancel"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("compact print chooser missing %q:\n%s", want, rendered)
		}
	}
}

func TestPreviewPrintChooserUsesBadgesAndToggleSwitch(t *testing.T) {
	m, _ := printTestModel()
	model, _, _ := m.handleTimelineKey(keyRunes("p"))
	opened := model.(*Model)
	rendered := stripANSI(opened.View().Content)
	for _, want := range []string{"[1] Original Visual", "[2] Markdown Swiss", "[3] Markdown GitHub", "[4] Markdown Manuscript", "[5] Markdown Academic", "Remote Images", "[ OFF ]"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("print chooser missing polished control %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Remote Images Off") {
		t.Fatalf("remote image state should be rendered as a toggle, got:\n%s", rendered)
	}

	model, _, handled := opened.handleOverlayKey(keyRunes("i"))
	if !handled {
		t.Fatal("expected i to toggle remote image printing")
	}
	rendered = stripANSI(model.(*Model).View().Content)
	if !strings.Contains(rendered, "[ ON ]") || strings.Contains(rendered, "[ OFF ]") {
		t.Fatalf("remote image toggle did not render ON state:\n%s", rendered)
	}
}

func TestPreviewPrintChooserTogglesRemoteImagesForRequest(t *testing.T) {
	m, printer := printTestModel()
	model, _, _ := m.handleTimelineKey(keyRunes("p"))
	opened := model.(*Model)
	rendered := stripANSI(opened.View().Content)
	if !strings.Contains(rendered, "[ OFF ]") {
		t.Fatalf("print chooser missing default remote image toggle:\n%s", rendered)
	}

	model, _, handled := opened.handleOverlayKey(keyRunes("i"))
	if !handled {
		t.Fatal("expected i to toggle remote image printing")
	}
	toggled := model.(*Model)
	rendered = stripANSI(toggled.View().Content)
	if !strings.Contains(rendered, "[ ON ]") {
		t.Fatalf("print chooser did not show remote images enabled:\n%s", rendered)
	}

	model, cmd, handled := toggled.handleOverlayKey(keyRunes("3"))
	if !handled || cmd == nil {
		t.Fatalf("expected theme selection to return print command, handled=%v cmd=%v", handled, cmd)
	}
	msg := cmd().(PreviewPrintMsg)
	_, _ = model.(*Model).Update(msg)
	if len(printer.requests) != 1 {
		t.Fatalf("printer requests = %d, want 1", len(printer.requests))
	}
	if !printer.requests[0].AllowRemoteImages {
		t.Fatalf("print request did not carry remote image opt-in: %#v", printer.requests[0])
	}
	if got := printer.requests[0].Theme; got != printing.ThemeGitHub {
		t.Fatalf("print theme = %q, want GitHub", got)
	}
}

func TestTimelinePreviewPrintChooserSelectsModesAndReportsResults(t *testing.T) {
	m, printer := printTestModel()
	model, _, _ := m.handleTimelineKey(keyRunes("p"))
	opened := model.(*Model)
	model, cmd, handled := opened.handleOverlayKey(keyRunes("2"))
	if !handled || cmd == nil {
		t.Fatalf("expected mode selection to return print command, handled=%v cmd=%v", handled, cmd)
	}
	updated := model.(*Model)
	if updated.previewPrintChooser {
		t.Fatal("chooser should close while printing")
	}
	msg := cmd()
	printMsg, ok := msg.(PreviewPrintMsg)
	if !ok {
		t.Fatalf("print command returned %T", msg)
	}
	model, _ = updated.Update(printMsg)
	updated = model.(*Model)
	if got := printer.requests[0].Mode; got != printing.ModeRenderedMarkdown {
		t.Fatalf("print mode = %q, want rendered Markdown", got)
	}
	if got := printer.requests[0].Theme; got != printing.ThemeSwiss {
		t.Fatalf("print theme = %q, want Swiss", got)
	}
	if !strings.Contains(updated.statusMessage, "Print dialog opened") {
		t.Fatalf("statusMessage = %q", updated.statusMessage)
	}
}

func TestTimelinePreviewPrintChooserSelectsMarkdownThemes(t *testing.T) {
	tests := []struct {
		key   string
		theme printing.Theme
	}{
		{"2", printing.ThemeSwiss},
		{"3", printing.ThemeGitHub},
		{"4", printing.ThemeManuscript},
		{"5", printing.ThemeAcademic},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m, printer := printTestModel()
			model, _, _ := m.handleTimelineKey(keyRunes("p"))
			opened := model.(*Model)
			model, cmd, handled := opened.handleOverlayKey(keyRunes(tt.key))
			if !handled || cmd == nil {
				t.Fatalf("expected theme selection to return print command, handled=%v cmd=%v", handled, cmd)
			}
			msg := cmd().(PreviewPrintMsg)
			_, _ = model.(*Model).Update(msg)
			if len(printer.requests) != 1 {
				t.Fatalf("printer requests = %d, want 1", len(printer.requests))
			}
			req := printer.requests[0]
			if req.Mode != printing.ModeRenderedMarkdown || req.Theme != tt.theme {
				t.Fatalf("print request mode/theme = %q/%q, want %q/%q", req.Mode, req.Theme, printing.ModeRenderedMarkdown, tt.theme)
			}
		})
	}
}

func TestTimelinePreviewPrintHandlesUnavailableAndErrors(t *testing.T) {
	m, printer := printTestModel()
	m.timeline.body = nil
	model, _, handled := m.handleTimelineKey(keyRunes("p"))
	if !handled {
		t.Fatal("expected p in preview context with missing body to be handled")
	}
	if !strings.Contains(model.(*Model).statusMessage, "Print unavailable") {
		t.Fatalf("statusMessage = %q", model.(*Model).statusMessage)
	}

	m, printer = printTestModel()
	printer.err = errors.New("print helper failed")
	model, _, _ = m.handleTimelineKey(keyRunes("p"))
	model, cmd, handled := model.(*Model).handleOverlayKey(keyRunes("1"))
	if !handled || cmd == nil {
		t.Fatal("expected print command")
	}
	msg := cmd().(PreviewPrintMsg)
	model, _ = model.(*Model).Update(msg)
	if !strings.Contains(model.(*Model).statusMessage, "Print failed: print helper failed") {
		t.Fatalf("statusMessage = %q", model.(*Model).statusMessage)
	}
}

func TestContactsPreviewPrintUsesInlinePreviewBody(t *testing.T) {
	printer := &fakePreviewPrinter{result: printing.Result{Status: printing.StatusOpened, Message: "Print dialog opened"}}
	m := New(&stubBackend{}, nil, "", nil, false)
	m.SetPreviewPrinter(printer)
	m.loading = false
	m.activeTab = tabContacts
	m.windowWidth = 120
	m.windowHeight = 40
	email := &models.EmailData{MessageID: "contact-print", Sender: "Contact <c@example.test>", Subject: "Contact print", Date: time.Now()}
	m.contactPreviewEmail = email
	m.contactPreviewBody = &models.EmailBody{TextPlain: "Contact body"}
	model, _ := m.handleContactsKey(keyRunes("p"))
	if !model.previewPrintChooser {
		t.Fatal("expected p in Contacts inline preview to open print chooser")
	}
	teaModel, cmd, handled := model.handleOverlayKey(keyRunes("1"))
	if !handled || cmd == nil {
		t.Fatal("expected Contacts print command")
	}
	msg := cmd().(PreviewPrintMsg)
	_, _ = teaModel.(*Model).Update(msg)
	if len(printer.requests) != 1 || printer.requests[0].Email.MessageID != "contact-print" {
		t.Fatalf("printer requests = %#v", printer.requests)
	}
}

func TestPreviewPrintCommandIsValidCustomKeymapCommand(t *testing.T) {
	err := ValidateCustomKeymap([]byte(`
extends: vim
bindings:
  timeline:
    normal:
      P: preview.print
  contacts:
    normal:
      P: preview.print
`))
	if err != nil {
		t.Fatalf("preview.print should be accepted by custom keymap validation: %v", err)
	}
}

func TestPreviewPrintKeyStaysLiteralInTextEntrySurfaces(t *testing.T) {
	t.Run("compose body", func(t *testing.T) {
		m, _ := printTestModel()
		m.activeTab = tabCompose
		focusComposeTextField(m, composeFieldBody)
		model, _ := m.handleKeyMsg(keyRunes("p"))
		updated := model.(*Model)
		if got := updated.composeBody.Value(); got != "p" {
			t.Fatalf("compose body value = %q, want literal p", got)
		}
		if updated.previewPrintChooser {
			t.Fatal("compose body opened print chooser")
		}
	})

	t.Run("timeline search", func(t *testing.T) {
		m, _ := printTestModel()
		m.timeline.searchMode = true
		m.timeline.searchFocus = timelineSearchFocusInput
		m.timeline.searchInput.Focus()
		model, _ := m.handleKeyMsg(keyRunes("p"))
		updated := model.(*Model)
		if got := updated.timeline.searchInput.Value(); got != "p" {
			t.Fatalf("timeline search value = %q, want literal p", got)
		}
		if updated.previewPrintChooser {
			t.Fatal("timeline search opened print chooser")
		}
	})

	t.Run("contacts search", func(t *testing.T) {
		m, _ := printTestModel()
		m.activeTab = tabContacts
		m.contactSearchMode = "keyword"
		model, _ := m.handleKeyMsg(keyRunes("p"))
		updated := model.(*Model)
		if got := updated.contactSearch; got != "p" {
			t.Fatalf("contact search value = %q, want literal p", got)
		}
		if updated.previewPrintChooser {
			t.Fatal("contacts search opened print chooser")
		}
	})
}
