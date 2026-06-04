package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type capturePreviewClipboardWriter struct {
	payload previewClipboardPayload
	calls   int
}

func (w *capturePreviewClipboardWriter) WritePreviewClipboard(payload previewClipboardPayload) (string, error) {
	w.payload = payload
	w.calls++
	return "captured", nil
}

func TestPreviewSelectionMovementClampsAndRestoresPreferredColumn(t *testing.T) {
	rows := plainSelectableRows([]string{"abcdef", "xy", "abcdef"})
	var sel previewSelectionState
	sel.begin(previewSelectionTimeline, 0, 5, rows)

	sel.move(rows, 0, 9)
	if sel.Cursor.Col != 6 || sel.PreferredCol != 6 {
		t.Fatalf("right clamp cursor/preferred = %d/%d, want 6/6", sel.Cursor.Col, sel.PreferredCol)
	}

	sel.move(rows, 0, -3)
	if sel.Cursor.Col != 3 || sel.PreferredCol != 3 {
		t.Fatalf("left move cursor/preferred = %d/%d, want 3/3", sel.Cursor.Col, sel.PreferredCol)
	}

	sel.move(rows, 1, 0)
	if sel.Cursor.Row != 1 || sel.Cursor.Col != 2 || sel.PreferredCol != 3 {
		t.Fatalf("short line cursor/preferred = row %d col %d preferred %d, want row 1 col 2 preferred 3", sel.Cursor.Row, sel.Cursor.Col, sel.PreferredCol)
	}

	sel.move(rows, 1, 0)
	if sel.Cursor.Row != 2 || sel.Cursor.Col != 3 || sel.PreferredCol != 3 {
		t.Fatalf("long line cursor/preferred = row %d col %d preferred %d, want row 2 col 3 preferred 3", sel.Cursor.Row, sel.Cursor.Col, sel.PreferredCol)
	}

	sel.move(rows, 0, -99)
	if sel.Cursor.Col != 0 || sel.PreferredCol != 0 {
		t.Fatalf("left clamp cursor/preferred = %d/%d, want 0/0", sel.Cursor.Col, sel.PreferredCol)
	}
}

func TestPreviewSelectionScrollFollowsCursor(t *testing.T) {
	rows := plainSelectableRows([]string{"0", "1", "2", "3", "4", "5"})
	var sel previewSelectionState
	sel.begin(previewSelectionTimeline, 0, 0, rows)
	scroll := 0

	for i := 0; i < 5; i++ {
		sel.move(rows, 1, 0)
	}
	sel.ensureCursorVisible(&scroll, 3, len(rows))
	if scroll != 3 {
		t.Fatalf("scroll after moving to row 5 = %d, want 3", scroll)
	}

	for i := 0; i < 4; i++ {
		sel.move(rows, -1, 0)
	}
	sel.ensureCursorVisible(&scroll, 3, len(rows))
	if scroll != 1 {
		t.Fatalf("scroll after moving back to row 1 = %d, want 1", scroll)
	}
}

func TestRenderPreviewSelectionTextShowsCursorAndKeepsVisibleText(t *testing.T) {
	sel := previewSelectionState{
		Active:       true,
		Surface:      previewSelectionTimeline,
		Selecting:    true,
		Anchor:       previewSelectionPoint{Row: 0, Col: 1},
		Cursor:       previewSelectionPoint{Row: 0, Col: 3},
		PreferredCol: 3,
	}

	rendered := renderPreviewSelectionText(defaultTheme, "abcd", 0, sel)
	if got := ansi.Strip(rendered); got != "abcd" {
		t.Fatalf("visible rendered text = %q, want abcd", got)
	}
	if rendered == "abcd" || !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("selection render should include ANSI cursor/highlight styling, got %q", rendered)
	}
}

func TestRenderPreviewCursorModeDoesNotHighlightAnchorRange(t *testing.T) {
	sel := previewSelectionState{
		Active:       true,
		Surface:      previewSelectionTimeline,
		Selecting:    false,
		Anchor:       previewSelectionPoint{Row: 0, Col: 0},
		Cursor:       previewSelectionPoint{Row: 0, Col: 3},
		PreferredCol: 3,
	}

	rendered := renderPreviewSelectionText(defaultTheme, "abcd", 0, sel)
	if got := ansi.Strip(rendered); got != "abcd" {
		t.Fatalf("visible rendered text = %q, want abcd", got)
	}
	if strings.Count(rendered, "\x1b[") == 0 {
		t.Fatalf("cursor mode should style the cursor, got %q", rendered)
	}
	if got := previewSelectionPlain(plainSelectableRows([]string{"abcd"}), sel); got != "" {
		t.Fatalf("cursor-only mode should not produce selected text, got %q", got)
	}
}

func TestPreviewSelectionPlainAndHTMLPayloads(t *testing.T) {
	rows := []previewSelectableRow{
		{Plain: "alpha", HTML: "<strong>alpha</strong>"},
		{Plain: "bravo", HTML: "<em>bravo</em>"},
		{Plain: "charlie", HTML: "<code>charlie</code>"},
	}

	partial := previewSelectionState{
		Active:    true,
		Surface:   previewSelectionTimeline,
		Selecting: true,
		Anchor:    previewSelectionPoint{Row: 0, Col: 2},
		Cursor:    previewSelectionPoint{Row: 2, Col: 1},
	}
	if got, want := previewSelectionPlain(rows, partial), "pha\nbravo\nch"; got != want {
		t.Fatalf("plain selection = %q, want %q", got, want)
	}

	if got := previewSelectionHTML(rows, partial); strings.Contains(got, "<strong>alpha</strong>") || !strings.Contains(got, "pha") {
		t.Fatalf("partial HTML should follow the selected text without whole-row markup, got %q", got)
	}

	fullRow := previewSelectionState{
		Active:    true,
		Surface:   previewSelectionTimeline,
		Selecting: true,
		Anchor:    previewSelectionPoint{Row: 1, Col: 0},
		Cursor:    previewSelectionPoint{Row: 1, Col: len([]rune(rows[1].Plain)) - 1},
	}
	if got := previewSelectionHTML(rows, fullRow); got != "<em>bravo</em>" {
		t.Fatalf("full-row HTML = %q, want source HTML fragment", got)
	}
}

func TestPreviewClipboardPayloadSingleImageAndMixedSelection(t *testing.T) {
	image := models.InlineImage{ContentID: "logo", MIMEType: "image/png", Data: []byte("png")}
	imageRow := previewSelectableRow{
		Plain:    previewSelectionImageLabel(&image, "Logo"),
		HTML:     "[image: Logo]",
		Image:    &image,
		ImageAlt: "Logo",
	}

	single := previewClipboardPayloadForRows([]previewSelectableRow{imageRow}, previewSelectionState{
		Active:    true,
		Surface:   previewSelectionTimeline,
		Selecting: true,
		Anchor:    previewSelectionPoint{Row: 0, Col: 0},
		Cursor:    previewSelectionPoint{Row: 0, Col: 0},
	})
	if single.Image == nil || single.ImageName != "Logo" {
		t.Fatalf("single-image payload image/name = %#v/%q, want image Logo", single.Image, single.ImageName)
	}

	mixedRows := []previewSelectableRow{{Plain: "Intro", HTML: "Intro"}, imageRow}
	mixed := previewClipboardPayloadForRows(mixedRows, previewSelectionState{
		Active:    true,
		Surface:   previewSelectionTimeline,
		Selecting: true,
		Anchor:    previewSelectionPoint{Row: 0, Col: 0},
		Cursor:    previewSelectionPoint{Row: 1, Col: len([]rune(imageRow.Plain)) - 1},
	})
	if mixed.Image != nil {
		t.Fatalf("mixed text+image selection should fall back to text placeholders, got image payload %#v", mixed.Image)
	}
	if !strings.Contains(mixed.Plain, "Intro") || !strings.Contains(mixed.Plain, "[image: Logo") {
		t.Fatalf("mixed fallback plain text missing text/image placeholder: %q", mixed.Plain)
	}
}

func TestCopyPreviewPayloadUsesInjectableClipboardWriter(t *testing.T) {
	old := activePreviewClipboardWriter
	writer := &capturePreviewClipboardWriter{}
	activePreviewClipboardWriter = writer
	defer func() { activePreviewClipboardWriter = old }()

	cmd := copyPreviewPayloadToClipboard(previewClipboardPayload{
		Plain: "plain",
		HTML:  "<p>plain</p>",
	})
	msg, ok := cmd().(PreviewClipboardMsg)
	if !ok {
		t.Fatalf("clipboard command returned %T, want PreviewClipboardMsg", cmd())
	}
	if msg.Err != nil || msg.Summary != "captured" {
		t.Fatalf("clipboard message = %#v, want captured success", msg)
	}
	if writer.calls != 1 || writer.payload.Plain != "plain" || writer.payload.HTML != "<p>plain</p>" {
		t.Fatalf("captured writer calls/payload = %d/%#v", writer.calls, writer.payload)
	}
}

func TestPreviewSelectionKeysActivateTimelineContactsAndComposeOriginal(t *testing.T) {
	timelineEmail := &models.EmailData{
		MessageID: "timeline-preview-select",
		Sender:    "sender@example.test",
		Subject:   "Preview select",
		Date:      time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}

	t.Run("timeline split", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		defer m.cleanup()
		m.activeTab = tabTimeline
		m.focusedPanel = panelPreview
		m.timeline.selectedEmail = timelineEmail
		m.timeline.bodyMessageID = timelineEmail.MessageID
		m.timeline.body = &models.EmailBody{TextPlain: "first line\nsecond line"}

		model, _, handled := m.handleTimelineKey(keyRunes("v"))
		if !handled {
			t.Fatal("expected Timeline v to be handled")
		}
		m = model.(*Model)
		if !m.previewSelection.activeOn(previewSelectionTimeline) {
			t.Fatal("Timeline split preview cursor did not activate")
		}
		if m.previewSelection.Selecting {
			t.Fatal("first v should show the Timeline split cursor without starting selection")
		}
		model, _, handled = m.handleTimelineKey(shortcutKeyPressMsg("right"))
		m = model.(*Model)
		if !handled || m.previewSelection.Cursor.Col != 1 {
			t.Fatalf("Timeline right key handled/cursor = %v/%d, want true/1", handled, m.previewSelection.Cursor.Col)
		}
	})

	t.Run("timeline full screen", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		defer m.cleanup()
		m.activeTab = tabTimeline
		m.focusedPanel = panelPreview
		m.timeline.fullScreen = true
		m.timeline.selectedEmail = timelineEmail
		m.timeline.bodyMessageID = timelineEmail.MessageID
		m.timeline.body = &models.EmailBody{TextPlain: "first line\nsecond line"}

		model, _, handled := m.handleTimelineKey(keyRunes("v"))
		m = model.(*Model)
		if !handled || !m.previewSelection.activeOn(previewSelectionTimeline) {
			t.Fatal("Timeline full-screen preview cursor did not activate")
		}
		if m.previewSelection.Selecting {
			t.Fatal("first v should show the Timeline full-screen cursor without starting selection")
		}
	})

	t.Run("contacts inline preview", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		defer m.cleanup()
		m.activeTab = tabContacts
		m.contactPreviewEmail = timelineEmail
		m.contactPreviewBody = &models.EmailBody{TextPlain: "contact line one\ncontact line two"}

		m, _ = m.handleContactsKey(keyRunes("v"))
		if !m.previewSelection.activeOn(previewSelectionContacts) {
			t.Fatal("Contacts inline preview cursor did not activate")
		}
		if m.previewSelection.Selecting {
			t.Fatal("first v should show the Contacts cursor without starting selection")
		}
	})

	t.Run("compose original message", func(t *testing.T) {
		m := newReplyPreservedComposeModel()
		m.windowWidth = 120
		m.windowHeight = 40
		m.composeField = composeFieldOriginalMessage

		model, _ := m.handleOriginalMessageKey(keyRunes("v"))
		m = model.(*Model)
		if !m.previewSelection.activeOn(previewSelectionComposeOriginal) {
			t.Fatal("Compose original-message cursor did not activate")
		}
		if m.previewSelection.Selecting {
			t.Fatal("first v should show the Compose original-message cursor without starting selection")
		}
	})
}

func TestPreviewCursorModeMovesBeforeSelectionAnchorStarts(t *testing.T) {
	email := &models.EmailData{
		MessageID: "timeline-preview-anchor",
		Sender:    "sender@example.test",
		Subject:   "Preview anchor",
		Date:      time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 120, 40)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "alpha\nbravo\ncharlie"}

	model, _, handled := m.handleTimelineKey(keyRunes("v"))
	m = model.(*Model)
	if !handled || !m.previewSelection.Active || m.previewSelection.Selecting {
		t.Fatalf("first v handled/active/selecting = %v/%v/%v, want true/true/false", handled, m.previewSelection.Active, m.previewSelection.Selecting)
	}

	for _, key := range []string{"l", "l", "j"} {
		model, _, handled = m.handleTimelineKey(keyRunes(key))
		m = model.(*Model)
		if !handled {
			t.Fatalf("expected cursor move key %q to be handled", key)
		}
	}
	if m.previewSelection.Cursor != (previewSelectionPoint{Row: 1, Col: 2}) {
		t.Fatalf("cursor before anchor = %#v, want row 1 col 2", m.previewSelection.Cursor)
	}
	if m.previewSelection.Anchor != m.previewSelection.Cursor {
		t.Fatalf("cursor-only movement should keep anchor at cursor, anchor=%#v cursor=%#v", m.previewSelection.Anchor, m.previewSelection.Cursor)
	}

	model, _, handled = m.handleTimelineKey(keyRunes("v"))
	m = model.(*Model)
	if !handled || !m.previewSelection.Selecting {
		t.Fatalf("second v handled/selecting = %v/%v, want true/true", handled, m.previewSelection.Selecting)
	}
	if m.previewSelection.Anchor != (previewSelectionPoint{Row: 1, Col: 2}) {
		t.Fatalf("selection anchor = %#v, want moved cursor row 1 col 2", m.previewSelection.Anchor)
	}

	model, _, handled = m.handleTimelineKey(keyRunes("j"))
	m = model.(*Model)
	if !handled {
		t.Fatal("expected selection move to be handled")
	}
	payload, ok := m.previewClipboardPayloadForSelection(previewSelectionTimeline)
	if !ok {
		t.Fatal("expected copyable selection after second v and movement")
	}
	if strings.Contains(payload.Plain, "alpha") {
		t.Fatalf("selection should start from moved cursor, got %q", payload.Plain)
	}
	if !strings.Contains(payload.Plain, "avo") || !strings.Contains(payload.Plain, "cha") {
		t.Fatalf("selection = %q, want text from bravo through charlie", payload.Plain)
	}
}

func TestPreviewSelectionCopyKeysUseCurrentSurface(t *testing.T) {
	old := activePreviewClipboardWriter
	writer := &capturePreviewClipboardWriter{}
	activePreviewClipboardWriter = writer
	defer func() { activePreviewClipboardWriter = old }()

	email := &models.EmailData{
		MessageID: "timeline-preview-copy",
		Sender:    "sender@example.test",
		Subject:   "Preview copy",
		Date:      time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 120, 40)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "copy me\nsecond line"}

	model, _, _ := m.handleTimelineKey(keyRunes("v"))
	m = model.(*Model)
	model, _, _ = m.handleTimelineKey(keyRunes("v"))
	m = model.(*Model)
	model, cmd, handled := m.handleTimelineKey(keyRunes("y"))
	m = model.(*Model)
	if !handled || cmd == nil {
		t.Fatalf("selection y handled/cmd = %v/%v, want true/non-nil", handled, cmd)
	}
	msg := cmd().(PreviewClipboardMsg)
	if msg.Err != nil {
		t.Fatalf("clipboard command error: %v", msg.Err)
	}
	if writer.calls != 1 || !strings.Contains(writer.payload.Plain, "c") {
		t.Fatalf("selection copy writer calls/payload = %d/%#v", writer.calls, writer.payload)
	}
	if m.previewSelection.Active {
		t.Fatal("copying active selection should exit preview selection mode")
	}

	model, _, _ = m.handleTimelineKey(keyRunes("y"))
	m = model.(*Model)
	model, cmd, handled = m.handleTimelineKey(keyRunes("y"))
	if !handled || cmd == nil {
		t.Fatalf("yy handled/cmd = %v/%v, want true/non-nil", handled, cmd)
	}
	_ = model
	msg = cmd().(PreviewClipboardMsg)
	if msg.Err != nil {
		t.Fatalf("yy clipboard command error: %v", msg.Err)
	}
	if !strings.Contains(writer.payload.Plain, "copy me") {
		t.Fatalf("yy copied %q, want current line", writer.payload.Plain)
	}
}

func TestPreviewCursorModeYDoesNotCopyTextBeforeSelectionStarts(t *testing.T) {
	old := activePreviewClipboardWriter
	writer := &capturePreviewClipboardWriter{}
	activePreviewClipboardWriter = writer
	defer func() { activePreviewClipboardWriter = old }()

	email := &models.EmailData{
		MessageID: "timeline-preview-cursor-copy",
		Sender:    "sender@example.test",
		Subject:   "Preview cursor copy",
		Date:      time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m := makeSizedModel(t, 120, 40)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{TextPlain: "copy me\nsecond line"}

	model, _, _ := m.handleTimelineKey(keyRunes("v"))
	m = model.(*Model)
	model, cmd, handled := m.handleTimelineKey(keyRunes("y"))
	m = model.(*Model)
	if !handled {
		t.Fatal("cursor-mode y should be handled")
	}
	if cmd != nil {
		t.Fatalf("cursor-mode y on text should wait for yy or second v, got command %T", cmd)
	}
	if writer.calls != 0 {
		t.Fatalf("cursor-mode y copied text before selection started: %#v", writer.payload)
	}
	if !m.timeline.pendingY {
		t.Fatal("cursor-mode y on text should still support yy line copy")
	}

	model, cmd, handled = m.handleTimelineKey(keyRunes("y"))
	m = model.(*Model)
	if !handled || cmd == nil {
		t.Fatalf("second y handled/cmd = %v/%v, want yy line copy", handled, cmd)
	}
	_ = m
	msg := cmd().(PreviewClipboardMsg)
	if msg.Err != nil {
		t.Fatalf("yy clipboard command error: %v", msg.Err)
	}
	if writer.calls != 1 || !strings.Contains(writer.payload.Plain, "copy me") {
		t.Fatalf("yy after cursor-mode y copied calls/payload = %d/%#v", writer.calls, writer.payload)
	}
}

func TestPreviewSelectionShortcutsDoNotStealComposeBodyOrTimelineSearchText(t *testing.T) {
	t.Run("compose body", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		defer m.cleanup()
		m.activeTab = tabCompose
		m.composeField = composeFieldBody
		m.composeBody.Focus()

		for _, key := range []string{"v", "y", "h", "j", "k", "l"} {
			model, _ := m.Update(keyRunes(key))
			m = model.(*Model)
		}

		if got := m.composeBody.Value(); got != "vyhjkl" {
			t.Fatalf("compose body value = %q, want literal preview shortcut text", got)
		}
		if m.previewSelection.Active {
			t.Fatal("compose body text entry should not activate preview selection")
		}
	})

	t.Run("timeline search", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		defer m.cleanup()
		m.activeTab = tabTimeline
		m.openTimelineSearch()

		for _, key := range []string{"v", "y", "h", "j", "k", "l"} {
			model, _ := m.handleKeyMsg(keyRunes(key))
			m = model.(*Model)
		}

		if got := m.timeline.searchInput.Value(); got != "vyhjkl" {
			t.Fatalf("timeline search value = %q, want literal preview shortcut text", got)
		}
		if m.previewSelection.Active {
			t.Fatal("timeline search text entry should not activate preview selection")
		}
	})

	t.Run("prompt editor", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		defer m.cleanup()
		m.activeTab = tabTimeline
		m.showPromptEditor = true
		m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
		_ = m.promptEditor.Init()

		for _, key := range []string{"v", "y", "h", "j", "k", "l"} {
			model, _ := m.Update(keyRunes(key))
			m = model.(*Model)
		}

		if got := m.promptEditor.name; got != "vyhjkl" {
			t.Fatalf("prompt editor name = %q, want literal preview shortcut text", got)
		}
		if m.previewSelection.Active || m.timeline.rangeMode {
			t.Fatal("prompt editor text entry should not activate preview selection or Timeline range mode")
		}
	})

	t.Run("rule editor", func(t *testing.T) {
		m := makeSizedModel(t, 120, 40)
		defer m.cleanup()
		m.activeTab = tabTimeline
		m.showRuleEditor = true
		m.ruleEditor = NewRuleEditor("", "", m.windowWidth, m.windowHeight)
		_ = m.ruleEditor.Init()
		_ = m.ruleEditor.form.NextField()

		var model tea.Model
		for _, key := range []string{"v", "y", "h", "j", "k", "l"} {
			model, _ = m.Update(keyRunes(key))
			m = model.(*Model)
		}

		if got := m.ruleEditor.triggerValue; got != "vyhjkl" {
			t.Fatalf("rule editor trigger value = %q, want literal preview shortcut text", got)
		}
		if m.previewSelection.Active || m.timeline.rangeMode {
			t.Fatal("rule editor text entry should not activate preview selection or Timeline range mode")
		}
	})
}

var _ tea.Msg = PreviewClipboardMsg{}
