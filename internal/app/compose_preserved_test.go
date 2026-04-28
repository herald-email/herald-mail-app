package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"mail-processor/internal/models"
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

func TestComposeCtrlOCyclesPreservationMode(t *testing.T) {
	m := newPreservedComposeModel()

	model, _ := m.handleComposeKey(tea.KeyMsg{Type: tea.KeyCtrlO})
	updated := model.(*Model)
	if updated.composePreserved.mode != models.PreservationModeFidelity {
		t.Fatalf("mode after first ctrl+o = %q, want fidelity", updated.composePreserved.mode)
	}
	model, _ = updated.handleComposeKey(tea.KeyMsg{Type: tea.KeyCtrlO})
	updated = model.(*Model)
	if updated.composePreserved.mode != models.PreservationModePrivacy {
		t.Fatalf("mode after second ctrl+o = %q, want privacy", updated.composePreserved.mode)
	}
	model, _ = updated.handleComposeKey(tea.KeyMsg{Type: tea.KeyCtrlO})
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

	_, cmd := m.handleComposeKey(tea.KeyMsg{Type: tea.KeyCtrlS})

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

func TestRenderForwardComposeSelectedAttachmentUsesActiveFocusStyle(t *testing.T) {
	oldProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(oldProfile)

	m := newPreservedComposeModel()
	m.composeField = composeFieldForwardedAttachments
	m.composePreserved.selectedAttachment = 1

	rendered := m.renderComposePreservedSummary(80)
	if !strings.Contains(rendered, "\x1b[38;5;229") || !strings.Contains(rendered, "48;5;57") {
		t.Fatalf("selected forwarded attachment should use active foreground/background styling, got:\n%q", rendered)
	}
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
