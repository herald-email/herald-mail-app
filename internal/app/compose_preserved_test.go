package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
