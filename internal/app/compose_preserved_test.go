package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func newPreservedComposeModel() *Model {
	m := New(&stubBackend{}, nil, "me@example.com", nil, false)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composePreserved = &composePreservedContext{
		kind: models.PreservedMessageKindForward,
		mode: models.PreservationModeSafe,
		email: &models.EmailData{
			MessageID: "msg-preserved",
			Sender:    "sender@example.com",
			Subject:   "Styled mail",
			Date:      time.Date(2026, 4, 28, 9, 30, 0, 0, time.UTC),
		},
		body: &models.EmailBody{
			TextPlain: "plain",
			TextHTML:  "<p>html</p>",
			InlineImages: []models.InlineImage{
				{ContentID: "logo", MIMEType: "image/png", Data: []byte("png")},
			},
		},
		forwardedAttachments: []models.ForwardedAttachment{
			{Attachment: models.Attachment{Filename: "report.pdf", MIMEType: "application/pdf", Data: []byte("pdf")}, Include: true},
			{Attachment: models.Attachment{Filename: "secret.pdf", MIMEType: "application/pdf", Data: []byte("secret")}, Include: true},
		},
	}
	return m
}

func newReplyPreservedComposeModel() *Model {
	m := New(&stubBackend{}, nil, "me@example.com", nil, false)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeTo.SetValue("sender@example.com")
	m.composeSubject.SetValue("Re: Styled mail")
	m.composeBody.SetValue("Thanks, I will review this.")
	m.replyContextEmail = &models.EmailData{
		MessageID: "msg-reply",
		Sender:    "sender@example.com",
		Subject:   "Styled mail",
		Date:      time.Date(2026, 4, 28, 9, 30, 0, 0, time.UTC),
	}
	m.composePreserved = &composePreservedContext{
		kind:  models.PreservedMessageKindReply,
		mode:  models.PreservationModeSafe,
		email: m.replyContextEmail,
		body: &models.EmailBody{
			TextPlain: "Original reply context line one.\nOriginal reply context line two.",
			TextHTML:  "<p>Original reply context line one.</p><p>Original reply context line two.</p>",
			InlineImages: []models.InlineImage{
				{ContentID: "logo", MIMEType: "image/png", Data: []byte("png")},
			},
		},
	}
	return m
}

func TestComposeCtrlOCyclesPreservationMode(t *testing.T) {
	m := newPreservedComposeModel()

	model, _ := m.handleComposeKey(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	updated := model.(*Model)
	if updated.composePreserved.mode != models.PreservationModeFidelity {
		t.Fatalf("mode after first ctrl+o = %q, want fidelity", updated.composePreserved.mode)
	}
	model, _ = updated.handleComposeKey(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	updated = model.(*Model)
	if updated.composePreserved.mode != models.PreservationModePrivacy {
		t.Fatalf("mode after second ctrl+o = %q, want privacy", updated.composePreserved.mode)
	}
	model, _ = updated.handleComposeKey(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	updated = model.(*Model)
	if updated.composePreserved.mode != models.PreservationModeSafe {
		t.Fatalf("mode after third ctrl+o = %q, want safe", updated.composePreserved.mode)
	}
}

func TestComposeForwardedAttachmentFocusRemovesSelectedAttachment(t *testing.T) {
	m := newPreservedComposeModel()
	m.composeField = composeFieldForwardedAttachments
	m.composePreserved.selectedAttachment = 1

	model, _ := m.handleComposeKey(keyRunes("x"))
	updated := model.(*Model)

	if updated.composePreserved.forwardedAttachments[1].Include {
		t.Fatalf("expected selected forwarded attachment to be excluded: %#v", updated.composePreserved.forwardedAttachments)
	}
	if !updated.composePreserved.forwardedAttachments[0].Include {
		t.Fatalf("non-selected attachment should remain included: %#v", updated.composePreserved.forwardedAttachments)
	}
}

func TestComposeForwardedAttachmentFocusTogglesSelectedAttachment(t *testing.T) {
	m := newPreservedComposeModel()
	m.composeField = composeFieldForwardedAttachments
	m.composePreserved.selectedAttachment = 1

	model, _ := m.handleComposeKey(keyRunes("x"))
	updated := model.(*Model)
	if updated.composePreserved.forwardedAttachments[1].Include {
		t.Fatalf("expected selected forwarded attachment to be excluded after first x: %#v", updated.composePreserved.forwardedAttachments)
	}

	model, _ = updated.handleComposeKey(keyRunes("x"))
	updated = model.(*Model)
	if !updated.composePreserved.forwardedAttachments[1].Include {
		t.Fatalf("expected selected forwarded attachment to be included after second x: %#v", updated.composePreserved.forwardedAttachments)
	}
}

func TestComposeForwardedAttachmentFocusDoesNotRestoreUnavailableAttachment(t *testing.T) {
	m := newPreservedComposeModel()
	m.composeField = composeFieldForwardedAttachments
	m.composePreserved.selectedAttachment = 1
	m.composePreserved.forwardedAttachments[1].Include = false
	m.composePreserved.forwardedAttachments[1].Attachment.Data = nil

	model, _ := m.handleComposeKey(keyRunes("x"))
	updated := model.(*Model)

	if updated.composePreserved.forwardedAttachments[1].Include {
		t.Fatalf("unavailable forwarded attachment should stay excluded: %#v", updated.composePreserved.forwardedAttachments)
	}
}

func TestComposeForwardedAttachmentFocusKeepsGlobalSendShortcut(t *testing.T) {
	m := newPreservedComposeModel()
	m.composeField = composeFieldForwardedAttachments
	m.composeTo.SetValue("you@example.com")
	m.composeSubject.SetValue("Fwd: Styled mail")

	_, cmd := m.handleComposeKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	if cmd == nil {
		t.Fatal("expected ctrl+s to keep sending while forwarded attachment list is focused")
	}
}

func TestRenderComposePreservedSummary(t *testing.T) {
	m := newPreservedComposeModel()
	rendered := stripANSI(m.renderComposePreservedSummary(80))

	for _, want := range []string{"Preserved forward", "Safe", "HTML", "1 inline", "2 attachments"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("summary missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderForwardComposeSeparatesResponseAndOriginalMessage(t *testing.T) {
	m := newPreservedComposeModel()
	m.loading = false
	m.composeBody.SetValue("Please review this.")
	m.updateTableDimensions(100, 32)
	freezeComposeCursors(m)

	rendered := stripANSI(m.renderComposeView())
	for _, want := range []string{"Response", "Original message", "sender@example.com", "Styled mail", "plain"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("forward compose render missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(m.composeBody.Value(), "plain") {
		t.Fatalf("compose body should contain only the response note, got %q", m.composeBody.Value())
	}
}

func TestRenderReplyComposeSeparatesResponseAndOriginalMessage(t *testing.T) {
	m := newReplyPreservedComposeModel()
	m.loading = false
	m.updateTableDimensions(100, 32)
	freezeComposeCursors(m)

	rendered := stripANSI(m.renderComposeView())
	for _, want := range []string{"Response", "Original message", "sender@example.com", "Styled mail", "Original reply context line one"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("reply compose render missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(m.composeBody.Value(), "Original reply context") {
		t.Fatalf("compose body should contain only the response note, got %q", m.composeBody.Value())
	}
}

func TestReplyComposeOriginalMessagePaneScrollsWithoutEditingResponse(t *testing.T) {
	m := newReplyPreservedComposeModel()
	m.loading = false
	m.composePreserved.body.TextPlain = strings.Join([]string{
		"Line 01 original context",
		"Line 02 original context",
		"Line 03 original context",
		"Line 04 original context",
		"Line 05 original context",
		"Line 06 original context",
		"Line 07 original context",
		"Line 08 original context",
		"Line 09 original context",
	}, "\n")
	m.updateTableDimensions(100, 32)

	model, _ := m.handleComposeKey(tea.KeyPressMsg{Code: tea.KeyTab})
	updated := model.(*Model)
	beforeBody := updated.composeBody.Value()
	for i := 0; i < 10; i++ {
		model, _ = updated.handleComposeKey(keyRunes("j"))
		updated = model.(*Model)
	}

	if updated.composeBody.Value() != beforeBody {
		t.Fatalf("scrolling original pane edited response body: %q", updated.composeBody.Value())
	}
	rendered := stripANSI(updated.renderComposeView())
	if !strings.Contains(rendered, "Line 07 original context") {
		t.Fatalf("expected scrolled original context to show later lines, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Line 01 original context") {
		t.Fatalf("expected top original context line to scroll out, got:\n%s", rendered)
	}
}

func TestRenderForwardComposeSelectedAttachmentUsesActiveFocusStyle(t *testing.T) {
	m := newPreservedComposeModel()
	m.composeField = composeFieldForwardedAttachments
	m.composePreserved.selectedAttachment = 1

	rendered := m.renderComposePreservedSummary(80)
	if !strings.Contains(rendered, "\x1b[38;5;229") || !strings.Contains(rendered, "48;5;57") {
		t.Fatalf("selected forwarded attachment should use active foreground/background styling, got:\n%q", rendered)
	}
}

func TestReplyComposePreservedViewFits80x24(t *testing.T) {
	m := newReplyPreservedComposeModel()
	m.loading = false
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	freezeComposeCursors(m)

	rendered := m.renderMainView()
	assertFitsWidth(t, 80, rendered)
	assertFitsHeight(t, 24, rendered)
	stripped := stripANSI(rendered)
	for _, want := range []string{"Response", "Original message"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("reply compose at 80x24 missing %q:\n%s", want, stripped)
		}
	}
}

func TestReplyComposeOriginalMessagePaneIsBoundedAndBalanced(t *testing.T) {
	m := newReplyPreservedComposeModel()
	m.loading = false
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	m = updated.(*Model)
	freezeComposeCursors(m)

	rendered := stripANSI(m.renderMainView())
	responseHeight, originalHeight := composePreservedPaneHeights(t, rendered)
	if originalHeight < responseHeight-1 {
		t.Fatalf("expected original pane to be roughly balanced with response pane, response=%d original=%d\n%s", responseHeight, originalHeight, rendered)
	}
	if !strings.Contains(rendered, "┌") || !strings.Contains(rendered, "Original message") {
		t.Fatalf("expected original message to render inside a bordered pane, got:\n%s", rendered)
	}
}

func composePreservedPaneHeights(t *testing.T, rendered string) (responseHeight, originalHeight int) {
	t.Helper()
	lines := strings.Split(rendered, "\n")
	responseDivider := -1
	for i, line := range lines {
		if strings.Contains(line, "Response") {
			responseDivider = i
			break
		}
	}
	if responseDivider < 0 {
		t.Fatalf("response divider not found:\n%s", rendered)
	}
	responseStart := findLineWithPrefix(lines, responseDivider+1, "┌")
	responseEnd := findLineWithPrefix(lines, responseStart+1, "└")
	if responseStart < 0 || responseEnd < 0 {
		t.Fatalf("response pane border not found:\n%s", rendered)
	}
	responseHeight = responseEnd - responseStart + 1

	originalStart := findLineWithPrefix(lines, responseEnd+1, "┌")
	originalEnd := findLineWithPrefix(lines, originalStart+1, "└")
	if originalStart < 0 || originalEnd < 0 {
		t.Fatalf("original pane border not found after response pane:\n%s", rendered)
	}
	originalHeight = originalEnd - originalStart + 1
	return responseHeight, originalHeight
}

func findLineWithPrefix(lines []string, start int, prefix string) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), prefix) {
			return i
		}
	}
	return -1
}

func TestForwardComposePreservedViewFits80x24(t *testing.T) {
	m := newPreservedComposeModel()
	m.loading = false
	m.composeBody.SetValue("Please review this.")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	freezeComposeCursors(m)

	rendered := m.renderMainView()
	assertFitsWidth(t, 80, rendered)
	assertFitsHeight(t, 24, rendered)
	stripped := stripANSI(rendered)
	for _, want := range []string{"Response", "Original message"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("forward compose at 80x24 missing %q:\n%s", want, stripped)
		}
	}
}

func TestOpenTimelineForwardComposeReflowsPreservedRowsAtWideSize(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	email := &models.EmailData{
		MessageID: "wide-forward",
		Sender:    "Northstar Cloud <billing@northstar-cloud.example>",
		Subject:   "Invoice and usage alert for Project Orion",
		Date:      time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
	}
	body := &models.EmailBody{
		TextPlain: "Northstar Cloud detected a usage change on Project Orion. The compute cluster is above forecast and the attached invoice highlights the services driving the budget risk.",
		Attachments: []models.Attachment{
			{Filename: "northstar-orion-invoice.pdf", MIMEType: "application/pdf", Data: []byte("pdf")},
		},
	}

	m.openTimelineForwardCompose(email, body, "")
	freezeComposeCursors(m)
	rendered := m.renderMainView()

	assertFitsWidth(t, 220, rendered)
	assertFitsHeight(t, 50, rendered)
	stripped := stripANSI(rendered)
	for _, want := range []string{"Herald", "To:", "Response", "Original message"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("wide forward compose missing %q:\n%s", want, stripped)
		}
	}
}

func TestBuildPreservedComposeRequestOmitsRemovedForwardedAttachments(t *testing.T) {
	m := newPreservedComposeModel()
	m.composeBody.SetValue("FYI **team**")
	m.composePreserved.forwardedAttachments[1].Include = false

	req, err := m.buildPreservedComposeRequest("me@example.com", "you@example.com", "Fwd: Styled mail", nil)
	if err != nil {
		t.Fatalf("buildPreservedComposeRequest: %v", err)
	}
	if req.Kind != models.PreservedMessageKindForward {
		t.Fatalf("kind = %q, want forward", req.Kind)
	}
	if req.TopNoteMarkdown != "FYI **team**" {
		t.Fatalf("top note = %q", req.TopNoteMarkdown)
	}
	if req.Original.TextHTML != "<p>html</p>" {
		t.Fatalf("original HTML = %q", req.Original.TextHTML)
	}
	if len(req.OmittedOriginalAttachmentNames) != 1 || req.OmittedOriginalAttachmentNames[0] != "secret.pdf" {
		t.Fatalf("omitted attachments = %#v, want secret.pdf", req.OmittedOriginalAttachmentNames)
	}
}

type rawDraftBackend struct {
	stubBackend
	rawDrafts [][]byte
}

func (b *rawDraftBackend) SaveRawDraft(raw []byte) (uint32, string, error) {
	b.rawDrafts = append(b.rawDrafts, raw)
	return uint32(len(b.rawDrafts)), "Drafts", nil
}

func TestSaveDraftCmdForPreservedComposeSavesAssembledMIME(t *testing.T) {
	backend := &rawDraftBackend{}
	m := newPreservedComposeModel()
	m.backend = backend
	m.composeTo.SetValue("you@example.com")
	m.composeSubject.SetValue("Fwd: Styled mail")
	m.composeBody.SetValue("FYI")

	msg := m.saveDraftCmd()().(DraftSavedMsg)

	if msg.Err != nil {
		t.Fatalf("saveDraftCmd returned error: %v", msg.Err)
	}
	if len(backend.rawDrafts) != 1 {
		t.Fatalf("expected one raw draft save, got %d", len(backend.rawDrafts))
	}
	raw := string(backend.rawDrafts[0])
	if !strings.Contains(raw, "multipart/alternative") || !strings.Contains(raw, "<p>html</p>") {
		t.Fatalf("raw draft did not include assembled preserved MIME:\n%s", raw)
	}
}
