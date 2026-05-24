package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type accountAwareStubBackend struct {
	stubBackend
	accounts     []backend.AccountInfo
	statuses     map[models.SourceID]backend.AccountStatus
	activeSource models.SourceID
	switchCalls  []models.SourceID
}

func newAccountAwareStubBackend(accounts []backend.AccountInfo) *accountAwareStubBackend {
	active := models.SourceID("")
	if len(accounts) > 0 {
		active = accounts[0].SourceID
	}
	return &accountAwareStubBackend{
		accounts:     accounts,
		statuses:     make(map[models.SourceID]backend.AccountStatus),
		activeSource: active,
	}
}

func (b *accountAwareStubBackend) Accounts() []backend.AccountInfo {
	out := make([]backend.AccountInfo, len(b.accounts))
	copy(out, b.accounts)
	return out
}

func (b *accountAwareStubBackend) ActiveAccount() backend.AccountInfo {
	for _, account := range b.accounts {
		if account.SourceID == b.activeSource {
			return account
		}
	}
	return backend.AccountInfo{}
}

func (b *accountAwareStubBackend) HasMultipleAccounts() bool {
	return len(b.accounts) > 1
}

func (b *accountAwareStubBackend) SwitchAccount(sourceID models.SourceID) error {
	b.switchCalls = append(b.switchCalls, sourceID)
	b.activeSource = sourceID
	return nil
}

func (b *accountAwareStubBackend) AccountStatuses() map[models.SourceID]backend.AccountStatus {
	out := make(map[models.SourceID]backend.AccountStatus, len(b.statuses))
	for id, st := range b.statuses {
		out[id] = st
	}
	return out
}

func accountSwitcherTestModel(b *accountAwareStubBackend) *Model {
	m := New(b, nil, "", nil, false)
	m.loading = false
	m.windowWidth = 120
	m.windowHeight = 40
	m.currentFolder = "INBOX"
	m.folders = []string{"INBOX", "Clients"}
	m.folderTree = buildFolderTree(m.folders)
	m.folderStatus = map[string]models.FolderStatus{"INBOX": {Unseen: 7, Total: 23}}
	m.syncAccountsFromBackend()
	return m
}

func TestSingleAccountSidebarDoesNotRenderAccountRail(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{{
		SourceID:    "default-mail",
		AccountID:   "default",
		DisplayName: "Default Mail",
		Provider:    "imap",
	}})
	m := accountSwitcherTestModel(b)

	sidebar := stripANSI(m.renderSidebar())
	if strings.Contains(sidebar, "Accounts") {
		t.Fatalf("single-account sidebar should not render account rail:\n%s", sidebar)
	}
	title := stripANSI(m.renderTitleBar(80))
	if strings.Contains(title, "Default Mail") {
		t.Fatalf("single-account title should not add account chrome: %q", title)
	}
	status := stripANSI(m.renderStatusBar())
	if strings.Contains(status, "Default Mail") {
		t.Fatalf("single-account status should not add account chrome: %q", status)
	}
}

func TestMultiAccountSidebarStatusAndSwitcherRenderAccountIdentity(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Provider: "imap", Address: "work@example.test"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Provider: "imap", Address: "me@example.test"},
	})
	b.statuses["work-mail"] = backend.AccountStatus{State: "live", Unread: 7, Total: 23}
	b.statuses["personal-mail"] = backend.AccountStatus{State: "auth", Error: "needs sign-in", Unread: 4, Total: 31}
	m := accountSwitcherTestModel(b)

	sidebar := stripANSI(m.renderSidebar())
	for _, want := range []string{"Accounts", "Work Mail", "Personal", "Folders - Work Mail"} {
		if !strings.Contains(sidebar, want) {
			t.Fatalf("sidebar missing %q:\n%s", want, sidebar)
		}
	}
	if title := stripANSI(m.renderTitleBar(120)); !strings.Contains(title, "Work Mail") {
		t.Fatalf("title missing active account: %q", title)
	}
	if status := stripANSI(m.renderStatusBar()); !strings.Contains(status, "Work Mail") {
		t.Fatalf("status missing active account: %q", status)
	}

	model, _ := m.handleKeyMsg(keyRunes("A"))
	updated := model.(*Model)
	if !updated.showAccountSwitcher {
		t.Fatal("expected A to open account switcher")
	}
	overlay := stripANSI(updated.renderAccountSwitcherOverlayView())
	for _, want := range []string{"Accounts", "Work Mail", "Personal", "needs sign-in"} {
		if !strings.Contains(overlay, want) {
			t.Fatalf("switcher overlay missing %q:\n%s", want, overlay)
		}
	}
}

func TestAccountSwitcherEnterSwitchesActiveAccountAndRestoresFolder(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	})
	m := accountSwitcherTestModel(b)
	m.currentFolder = "Clients"
	m.accountSelectedFolders["work-mail"] = "Clients"
	m.accountSelectedFolders["personal-mail"] = "Travel"

	model, _ := m.handleKeyMsg(keyRunes("A"))
	opened := model.(*Model)
	model, _ = opened.handleKeyMsg(keyRunes("j"))
	selected := model.(*Model)
	if selected.accountSwitcherCursor != 1 {
		t.Fatalf("cursor=%d, want 1", selected.accountSwitcherCursor)
	}
	model, cmd := selected.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	switched := model.(*Model)

	if !strings.EqualFold(string(b.activeSource), "personal-mail") {
		t.Fatalf("active source=%q, want personal-mail", b.activeSource)
	}
	if switched.showAccountSwitcher {
		t.Fatal("switcher should close after Enter")
	}
	if switched.currentFolder != "Travel" {
		t.Fatalf("currentFolder=%q, want restored Travel", switched.currentFolder)
	}
	if cmd == nil {
		t.Fatal("expected switch to schedule reload commands")
	}
}

func TestAccountRailEnterSwitchesActiveAccountAndRestoresFolder(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	})
	m := accountSwitcherTestModel(b)
	m.currentFolder = "Clients"
	m.accountSelectedFolders["work-mail"] = "Clients"
	m.accountSelectedFolders["personal-mail"] = "Travel"
	m.focusedPanel = panelSidebar
	m.sidebarCursor = 1

	cmd, handledAccount := m.selectSidebarFolder()
	if !handledAccount {
		t.Fatal("expected account rail row to be handled as an account switch")
	}
	if cmd == nil {
		t.Fatal("expected account rail switch to schedule reload commands")
	}
	if !strings.EqualFold(string(b.activeSource), "personal-mail") {
		t.Fatalf("active source=%q, want personal-mail", b.activeSource)
	}
	if m.currentFolder != "Travel" {
		t.Fatalf("currentFolder=%q, want restored Travel", m.currentFolder)
	}
}

func TestAccountSwitcherShortcutDoesNotStealTextEntry(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	})
	m := accountSwitcherTestModel(b)
	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeBody.Focus()

	model, _ := m.handleKeyMsg(keyRunes("A"))
	updated := model.(*Model)
	if updated.showAccountSwitcher {
		t.Fatal("A typed in compose opened account switcher")
	}
	if got := updated.composeBody.Value(); got != "A" {
		t.Fatalf("compose body=%q, want literal A", got)
	}

	updated.activeTab = tabTimeline
	updated.timeline.searchMode = true
	updated.timeline.searchFocus = timelineSearchFocusInput
	updated.timeline.searchInput.Focus()
	model, _ = updated.handleKeyMsg(keyRunes("A"))
	searching := model.(*Model)
	if searching.showAccountSwitcher {
		t.Fatal("A typed in search prompt opened account switcher")
	}
	if got := searching.timeline.searchInput.Value(); got != "A" {
		t.Fatalf("search input=%q, want literal A", got)
	}
}

func TestAccountSwitcherShortcutDoesNotStealEditorOverlays(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	})
	m := accountSwitcherTestModel(b)
	m.showPromptEditor = true
	m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)

	model, _ := m.Update(keyRunes("A"))
	updated := model.(*Model)
	if updated.showAccountSwitcher {
		t.Fatal("A typed in prompt editor opened account switcher")
	}
	if !updated.showPromptEditor {
		t.Fatal("prompt editor should remain active after typing A")
	}

	updated.showPromptEditor = false
	updated.promptEditor = nil
	updated.showRuleEditor = true
	updated.ruleEditor = NewRuleEditor("sender@example.test", "", updated.windowWidth, updated.windowHeight)

	model, _ = updated.Update(keyRunes("A"))
	ruleEditing := model.(*Model)
	if ruleEditing.showAccountSwitcher {
		t.Fatal("A typed in rule editor opened account switcher")
	}
	if !ruleEditing.showRuleEditor {
		t.Fatal("rule editor should remain active after typing A")
	}
}
