package app

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type composeAccountStubBackend struct {
	*accountAwareStubBackend
	composeSends     []backend.ComposeSendRequest
	saveDraftSources []models.SourceID
	deleteDrafts     []string
	sendDrafts       []string
}

func (b *composeAccountStubBackend) SendCompose(req backend.ComposeSendRequest) error {
	b.composeSends = append(b.composeSends, req)
	return nil
}

func (b *composeAccountStubBackend) SaveDraftForAccount(sourceID models.SourceID, to, cc, bcc, subject, body string) (uint32, string, error) {
	b.saveDraftSources = append(b.saveDraftSources, sourceID)
	return 88, "Drafts", nil
}

func (b *composeAccountStubBackend) SaveRawDraftForAccount(sourceID models.SourceID, raw []byte) (uint32, string, error) {
	b.saveDraftSources = append(b.saveDraftSources, sourceID)
	return 89, "Drafts", nil
}

func (b *composeAccountStubBackend) DeleteDraftForAccount(sourceID models.SourceID, uid uint32, folder string) error {
	b.deleteDrafts = append(b.deleteDrafts, string(sourceID)+":"+folder)
	return nil
}

func (b *composeAccountStubBackend) SendDraftForAccount(sourceID models.SourceID, uid uint32, folder string) error {
	b.sendDrafts = append(b.sendDrafts, string(sourceID)+":"+folder)
	return nil
}

func newComposeAccountTestModel(active models.SourceID) (*Model, *composeAccountStubBackend) {
	base := newAccountAwareStubBackend([]backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Address: "work@example.test", Signature: "-- \nWork Signature"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Address: "me@example.test", Signature: "-- \nPersonal Signature"},
	})
	if active != "" {
		base.activeSource = active
	}
	b := &composeAccountStubBackend{accountAwareStubBackend: base}
	m := New(b, nil, "legacy@example.test", nil, false)
	if active != "" {
		_ = b.SwitchAccount(active)
		m.syncAccountIdentityFromBackend()
	}
	m.loading = false
	m.windowWidth = 120
	m.windowHeight = 40
	m.currentFolder = "INBOX"
	m.syncAccountsFromBackend()
	return m, b
}

func TestMultiAccountBlankComposeUsesActiveAccountSignature(t *testing.T) {
	m, _ := newComposeAccountTestModel("personal-mail")

	m.openBlankComposeFromCurrent()

	if got := m.composeBody.Value(); got != "\n\n-- \nPersonal Signature" {
		t.Fatalf("compose body = %q, want personal signature", got)
	}
}

func TestMultiAccountBlankComposeShowsFromPickerAndDefaultsToActiveAccount(t *testing.T) {
	m, _ := newComposeAccountTestModel("personal-mail")

	m.openBlankComposeFromCurrent()

	if got := m.composeSourceID; got != models.SourceID("personal-mail") {
		t.Fatalf("composeSourceID=%q, want personal-mail", got)
	}
	rendered := m.renderComposeView()
	for _, want := range []string{"From:", "Personal", "me@example.test"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("compose view missing %q:\n%s", want, rendered)
		}
	}
}

func TestSingleAccountComposeDoesNotRenderFromPicker(t *testing.T) {
	m := New(&stubBackend{}, nil, "rowan@example.test", nil, false)
	m.loading = false
	m.windowWidth = 120
	m.windowHeight = 40

	m.openBlankComposeFromCurrent()
	rendered := m.renderComposeView()

	if strings.Contains(rendered, "From:") {
		t.Fatalf("single-account compose rendered From picker:\n%s", rendered)
	}
}

func TestReplyAndForwardComposeUseSelectedMessageAccountForFrom(t *testing.T) {
	m, _ := newComposeAccountTestModel(backend.AllAccountsSourceID)
	email := scopedAppEmail(&models.EmailData{
		SourceID:  "personal-mail",
		AccountID: "personal",
		MessageID: "msg-1",
		UID:       7,
		Folder:    "INBOX",
		Sender:    "friend@example.test",
		Subject:   "hello",
	})
	body := &models.EmailBody{MessageID: "msg-1", TextPlain: "body"}

	m.openTimelineReplyCompose(email, body, "", false)
	if got := m.composeSourceID; got != models.SourceID("personal-mail") {
		t.Fatalf("reply composeSourceID=%q, want personal-mail", got)
	}
	if got := m.composeBody.Value(); got != "\n\n-- \nPersonal Signature" {
		t.Fatalf("reply body = %q, want personal signature", got)
	}

	m.openTimelineForwardCompose(email, body, "")
	if got := m.composeSourceID; got != models.SourceID("personal-mail") {
		t.Fatalf("forward composeSourceID=%q, want personal-mail", got)
	}
	if got := m.composeBody.Value(); got != "\n\n-- \nPersonal Signature" {
		t.Fatalf("forward body = %q, want personal signature", got)
	}
}

func TestQuickReplyUsesSelectedMessageAccountSignature(t *testing.T) {
	m, _ := newComposeAccountTestModel(backend.AllAccountsSourceID)
	m.timeline.selectedEmail = scopedAppEmail(&models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "msg-quick",
		Sender:    "friend@example.test",
		Subject:   "hello",
	})

	model, cmd := m.openQuickReply("Sounds good.")
	updated := model.(*Model)

	if cmd != nil {
		t.Fatalf("expected quick reply open to be synchronous, got %T", cmd)
	}
	if got := updated.composeBody.Value(); got != "Sounds good.\n\n\n-- \nWork Signature" {
		t.Fatalf("quick reply body = %q, want work signature", got)
	}
}

func TestComposeFromPickerChangesSourceWithoutStealingTextInput(t *testing.T) {
	m, _ := newComposeAccountTestModel("work-mail")
	m.openBlankComposeFromCurrent()

	model, _ := m.handleComposeKey(tea.KeyPressMsg{Text: "A", Code: 'A'})
	m = model.(*Model)
	if got := m.composeTo.Value(); got != "A" {
		t.Fatalf("To field after literal A=%q, want A", got)
	}
	if got := m.composeSourceID; got != models.SourceID("work-mail") {
		t.Fatalf("composeSourceID after typed A=%q, want work-mail", got)
	}

	m.focusComposeField(composeFieldFrom)
	model, _ = m.handleComposeKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m = model.(*Model)
	if got := m.composeSourceID; got != models.SourceID("personal-mail") {
		t.Fatalf("composeSourceID after From picker down=%q, want personal-mail", got)
	}
}

func TestComposeSendAndDraftUseSelectedAccount(t *testing.T) {
	m, b := newComposeAccountTestModel("work-mail")
	m.openBlankComposeFromCurrent()
	m.setComposeSource("personal-mail")
	m.composeTo.SetValue("friend@example.test")
	m.composeSubject.SetValue("Selected source")
	m.composeBody.SetValue("hello")

	msg := m.sendCompose()()
	status, ok := msg.(ComposeStatusMsg)
	if !ok {
		t.Fatalf("sendCompose returned %T", msg)
	}
	if status.Err != nil {
		t.Fatalf("sendCompose error: %v", status.Err)
	}
	if len(b.composeSends) != 1 {
		t.Fatalf("compose sends=%#v, want one", b.composeSends)
	}
	if got := b.composeSends[0].SourceID; got != models.SourceID("personal-mail") {
		t.Fatalf("send source=%q, want personal-mail", got)
	}
	if got := b.composeSends[0].From; got != "me@example.test" {
		t.Fatalf("send from=%q, want me@example.test", got)
	}

	draftMsg := m.saveDraftCmd()()
	if _, ok := draftMsg.(DraftSavedMsg); !ok {
		t.Fatalf("saveDraftCmd returned %T", draftMsg)
	}
	if !reflect.DeepEqual(b.saveDraftSources, []models.SourceID{"personal-mail"}) {
		t.Fatalf("save draft sources=%#v", b.saveDraftSources)
	}
}
