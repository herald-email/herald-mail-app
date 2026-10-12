package app

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"net/mail"
	"strings"
	"testing"
)

func identityConfig() config.ComposeConfig {
	return config.ComposeConfig{Identities: []config.IdentityConfig{{Name: "Shopping", Address: "shop@example.test"}, {Name: "Public", Address: "public@example.test"}}, DefaultIdentity: "shop@example.test"}
}

func TestComposeIdentityDefaultAndSearch(t *testing.T) {
	m := New(&stubBackend{}, nil, "me@example.test", nil, false)
	m.cfg = &config.Config{Compose: identityConfig(), Credentials: config.CredentialsConfig{Username: "me@example.test"}}
	m.openBlankComposeFromCurrent()
	if !m.composeAccountPickerVisible() || !strings.Contains(m.composeFromAddress(), "shop@example.test") {
		t.Fatalf("default sender=%q", m.composeFromAddress())
	}
	m.composeTo.SetValue("recipient@example.test")
	m.composeBody.SetValue("Keep my text")
	m.composeField = composeFieldFrom
	for _, r := range "public" {
		m.handleComposeFromKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if !strings.Contains(m.composeFromAddress(), "public@example.test") {
		t.Fatalf("searched sender=%q", m.composeFromAddress())
	}
	if m.composeBody.Value() != "Keep my text" || m.composeTo.Value() != "recipient@example.test" {
		t.Fatal("selection changed draft")
	}
	m.setComposeSource("")
	if !strings.Contains(m.composeFromAddress(), "shop@example.test") {
		t.Fatal("new message did not reset default")
	}
}

func TestComposeIdentityAccountAndDraftRestore(t *testing.T) {
	m, _ := newComposeAccountTestModel("work-mail")
	m.cfg = &config.Config{Sources: []config.SourceConfig{{ID: "work-mail", Kind: "mail", Credentials: config.CredentialsConfig{Username: "work@example.test"}, Compose: identityConfig()}, {ID: "personal-mail", Kind: "mail", Credentials: config.CredentialsConfig{Username: "me@example.test"}}}}
	m.openBlankComposeFromCurrent()
	if !strings.Contains(m.composeFromAddress(), "shop@example.test") {
		t.Fatal(m.composeFromAddress())
	}
	m.openTimelineDraftCompose(&models.EmailData{SourceID: "work-mail", IsDraft: true}, &models.EmailBody{From: "Public <public@example.test>", TextPlain: "saved body"}, "")
	if !strings.Contains(m.composeFromAddress(), "public@example.test") || m.composeSelectedSourceID() != "work-mail" {
		t.Fatal("draft sender not restored")
	}
	m.setComposeSource("personal-mail")
	if m.composeFromAddress() != "me@example.test" {
		t.Fatal("alias leaked to personal account")
	}
}

func TestComposeIdentityReplyMatchesAccountRecipient(t *testing.T) {
	m, _ := newComposeAccountTestModel("work-mail")
	m.cfg = &config.Config{Sources: []config.SourceConfig{{ID: "work-mail", Kind: "mail", Compose: identityConfig()}}}
	m.openTimelineReplyCompose(&models.EmailData{SourceID: "work-mail", Sender: "sender@example.test"}, &models.EmailBody{To: "Public <public@example.test>"}, "", false)
	if !strings.Contains(m.composeFromAddress(), "public@example.test") {
		t.Fatal(m.composeFromAddress())
	}
	m.openTimelineForwardCompose(&models.EmailData{SourceID: "work-mail"}, &models.EmailBody{}, "")
	if !strings.Contains(m.composeFromAddress(), "shop@example.test") {
		t.Fatal("forward should use default")
	}
}

func TestComposeIdentitySendUsesSelectedSource(t *testing.T) {
	m, b := newComposeAccountTestModel("work-mail")
	m.cfg = &config.Config{Sources: []config.SourceConfig{{ID: "work-mail", Kind: "mail", Compose: identityConfig()}}}
	m.openBlankComposeFromCurrent()
	m.composeTo.SetValue("recipient@example.test")
	m.composeSubject.SetValue("test")
	m.composeBody.SetValue("body")
	result := m.sendCompose()().(ComposeStatusMsg)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(b.composeSends) != 1 || b.composeSends[0].SourceID != "work-mail" || !strings.Contains(b.composeSends[0].From, "shop@example.test") {
		t.Fatalf("send=%+v", b.composeSends)
	}
}

type identityDraftBackend struct {
	*stubBackend
	raw []byte
}

func (b *identityDraftBackend) SaveRawDraft(raw []byte) (uint32, string, error) {
	b.raw = raw
	return 1, "Drafts", nil
}
func TestComposeIdentityDraftPreservesAliasAndPrimary(t *testing.T) {
	b := &identityDraftBackend{stubBackend: &stubBackend{}}
	m := New(b, nil, "me@example.test", nil, false)
	m.cfg = &config.Config{Compose: identityConfig()}
	for _, address := range []string{"public@example.test", "me@example.test"} {
		m.openBlankComposeFromCurrent()
		m.restoreComposeIdentity(address)
		m.composeTo.SetValue("recipient@example.test")
		m.composeSubject.SetValue("draft")
		m.composeBody.SetValue("keep me")
		result := m.saveDraftCmd()().(DraftSavedMsg)
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		parsed, err := mail.ReadMessage(bytes.NewReader(b.raw))
		if err != nil {
			t.Fatal(err)
		}
		if mailboxAddress(parsed.Header.Get("From")) != address {
			t.Fatalf("raw From=%q", parsed.Header.Get("From"))
		}
		m.openTimelineDraftCompose(&models.EmailData{IsDraft: true}, &models.EmailBody{From: parsed.Header.Get("From"), TextPlain: "keep me"}, "")
		if mailboxAddress(m.composeFromAddress()) != address {
			t.Fatal("draft restore changed From")
		}
	}
}

func TestComposeIdentityReplyAllExcludesOwnAliasAndBackTabFocusesFrom(t *testing.T) {
	m := New(&stubBackend{}, nil, "me@example.test", nil, false)
	m.cfg = &config.Config{Compose: identityConfig()}
	m.openBlankComposeFromCurrent()
	m.handleComposeKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.composeField != composeFieldFrom {
		t.Fatalf("field=%v", m.composeField)
	}
	to, cc := m.replyAllRecipientFields(&models.EmailData{Sender: "sender@example.test"}, &models.EmailBody{To: "public@example.test", CC: "friend@example.test"})
	if strings.Contains(to+cc, "public@example.test") || !strings.Contains(cc, "friend@example.test") {
		t.Fatalf("reply-all=%s %s", to, cc)
	}
}

func TestComposeIdentityAIReviewKeepsFooterAt80Columns(t *testing.T) {
	for _, recipients := range []bool{false, true} {
		t.Run(fmt.Sprint(recipients), func(t *testing.T) {
			m := New(&stubBackend{}, nil, "me@example.test", &stubClassifier{}, false)
			m.loading = false
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			m.cfg = &config.Config{Compose: identityConfig()}
			m.activeTab = tabCompose
			m.composeAIPanel = true
			if recipients {
				m.composeCC.SetValue("cc@example.test")
				m.composeBCC.SetValue("bcc@example.test")
			}
			m.Update(AIAssistMsg{Original: "original text", Result: "edited text"})
			freezeComposeCursors(m)
			rendered := stripANSI(m.renderMainView())
			lines := strings.Split(rendered, "\n")
			if len(lines) != 24 {
				t.Fatalf("review has %d rows, want 24:\n%s", len(lines), rendered)
			}
			if !strings.Contains(strings.Join(lines[len(lines)-4:], "\n"), "ctrl+enter: accept") {
				t.Fatalf("missing review footer:\n%s", rendered)
			}
			for _, want := range []string{"From:", "edited text", "Changes"} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("missing %s", want)
				}
			}
		})
	}
}

func TestComposeIdentityAIReviewControlHintsSayAccept(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabCompose
	m.composeAIPanel = true
	m.composeAIResponse.SetValue("edited")
	hints := strings.Join(m.ctrlModifierHintSegments(ChromeState{}), " | ")
	if !strings.Contains(hints, "accept") || strings.Contains(hints, "send") {
		t.Fatalf("review hints=%s", hints)
	}
}
