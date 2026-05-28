package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type accountAwareStubBackend struct {
	stubBackend
	accounts      []backend.AccountInfo
	statuses      map[models.SourceID]backend.AccountStatus
	snapshots     []backend.AccountFolderSnapshot
	timeline      map[string][]*models.EmailData
	activeSource  models.SourceID
	switchCalls   []models.SourceID
	statusCalls   int
	timelineCalls int
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
	if b.activeSource == backend.AllAccountsSourceID {
		return backend.AccountInfo{SourceID: backend.AllAccountsSourceID, DisplayName: "All Accounts", Provider: "virtual"}
	}
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
	b.statusCalls++
	out := make(map[models.SourceID]backend.AccountStatus, len(b.statuses))
	for id, st := range b.statuses {
		out[id] = st
	}
	return out
}

func (b *accountAwareStubBackend) GetTimelineEmails(folder string) ([]*models.EmailData, error) {
	b.timelineCalls++
	if b.timeline == nil {
		return nil, nil
	}
	return b.timeline[folder], nil
}

func (b *accountAwareStubBackend) ListAccountFolderSnapshots() ([]backend.AccountFolderSnapshot, error) {
	out := make([]backend.AccountFolderSnapshot, len(b.snapshots))
	copy(out, b.snapshots)
	return out, nil
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
	if len(b.snapshots) > 0 {
		m.accountFolderSnapshots = b.snapshots
	}
	return m
}

func accountSwitcherSnapshots(accounts []backend.AccountInfo) []backend.AccountFolderSnapshot {
	snapshots := make([]backend.AccountFolderSnapshot, 0, len(accounts))
	for _, account := range accounts {
		snapshots = append(snapshots, backend.AccountFolderSnapshot{
			Account: account,
			Folders: []string{
				"INBOX",
				"Drafts",
				"Sent",
				"Receipts",
				"Projects/Launch",
			},
			Status: map[string]models.FolderStatus{
				"INBOX":           {Unseen: 7, Total: 23},
				"Drafts":          {Unseen: 0, Total: 2},
				"Sent":            {Unseen: 0, Total: 5},
				"Receipts":        {Unseen: 1, Total: 4},
				"Projects/Launch": {Unseen: 0, Total: 8},
			},
		})
	}
	return snapshots
}

func findSidebarItem(t *testing.T, m *Model, label string, account models.SourceID, folder string) int {
	t.Helper()
	for i, item := range m.visibleSidebarItems() {
		if item.label != label {
			continue
		}
		if account != "" && item.account != account {
			continue
		}
		if folder != "" && item.fullPath != folder {
			continue
		}
		return i
	}
	t.Fatalf("sidebar item label=%q account=%q folder=%q not found in %#v", label, account, folder, m.visibleSidebarItems())
	return -1
}

func scopedAppEmail(email *models.EmailData) *models.EmailData {
	ref := email.MessageRef()
	email.SourceID = ref.SourceID
	email.AccountID = ref.AccountID
	email.LocalID = ref.LocalID
	return email
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
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Provider: "imap", Address: "work@example.test"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Provider: "imap", Address: "me@example.test"},
	}
	b := newAccountAwareStubBackend(accounts)
	b.snapshots = accountSwitcherSnapshots(accounts)
	b.statuses["work-mail"] = backend.AccountStatus{State: "live", Unread: 7, Total: 23}
	b.statuses["personal-mail"] = backend.AccountStatus{State: "auth", Error: "needs sign-in", Unread: 4, Total: 31}
	m := accountSwitcherTestModel(b)

	sidebar := stripANSI(m.renderSidebar())
	for _, want := range []string{"Favorites", "All Inboxes", "All Drafts", "All Sent", "Work Mail", "Personal", "Folders", "Receipts"} {
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

func TestMultiAccountSidebarLabelsIncludeKnownAddress(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Address: "work@example.test"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Address: "me@example.test"},
	}
	b := newAccountAwareStubBackend(accounts)
	b.snapshots = accountSwitcherSnapshots(accounts)
	m := accountSwitcherTestModel(b)

	foundSection := false
	foundFavoriteChild := false
	for _, item := range m.visibleSidebarItems() {
		if item.label == "Work Mail (work@example.test)" && item.account == "work-mail" {
			foundSection = true
		}
		if item.label == "Personal (me@example.test)" && item.account == "personal-mail" && item.fullPath == "INBOX" {
			foundFavoriteChild = true
		}
	}
	if !foundSection {
		t.Fatalf("expected account section label to include address; items=%#v", m.visibleSidebarItems())
	}
	if !foundFavoriteChild {
		t.Fatalf("expected favorite account child label to include address; items=%#v", m.visibleSidebarItems())
	}
}

func TestSidebarNavigationSkipsNonSelectableHeaders(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Address: "work@example.test"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Address: "me@example.test"},
	}
	b := newAccountAwareStubBackend(accounts)
	b.snapshots = accountSwitcherSnapshots(accounts)
	m := accountSwitcherTestModel(b)
	m.focusedPanel = panelSidebar
	m.sidebarCursor = 0

	model, _ := m.handleNavigation(1)
	moved := model.(*Model)
	items := moved.visibleSidebarItems()
	if items[moved.sidebarCursor].kind == sidebarItemHeader {
		t.Fatalf("sidebar cursor landed on non-selectable header at %d: %#v", moved.sidebarCursor, items[moved.sidebarCursor])
	}
	if items[moved.sidebarCursor].label != "All Inboxes" {
		t.Fatalf("sidebar cursor label=%q, want first selectable All Inboxes", items[moved.sidebarCursor].label)
	}
}

func TestAccountSidebarSectionsCollapse(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Address: "work@example.test"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Address: "me@example.test"},
	}
	b := newAccountAwareStubBackend(accounts)
	b.snapshots = accountSwitcherSnapshots(accounts)
	m := accountSwitcherTestModel(b)
	m.focusedPanel = panelSidebar
	m.sidebarCursor = -1
	for i, item := range m.visibleSidebarItems() {
		if item.kind == sidebarItemGroup && item.label == "Work Mail (work@example.test)" && item.account == "work-mail" {
			m.sidebarCursor = i
			break
		}
	}
	if m.sidebarCursor < 0 {
		t.Fatalf("work account section not found in %#v", m.visibleSidebarItems())
	}
	before := len(m.visibleSidebarItems())

	m.toggleSidebarNode()

	after := len(m.visibleSidebarItems())
	if after >= before {
		t.Fatalf("collapsing account section did not reduce visible items: before=%d after=%d", before, after)
	}
	for _, item := range m.visibleSidebarItems() {
		if item.account == "work-mail" && item.fullPath == "Receipts" {
			t.Fatalf("collapsed account section still shows child folder: %#v", item)
		}
	}
}

func TestRenderSidebarUsesLayoutContentHeight(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail", Address: "work@example.test"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal", Address: "me@example.test"},
	}
	b := newAccountAwareStubBackend(accounts)
	b.snapshots = accountSwitcherSnapshots(accounts)
	m := accountSwitcherTestModel(b)
	m.windowWidth = 120
	m.windowHeight = 24
	m.loading = true
	m.syncCountsSettled = false
	m.progressInfo.Message = "Checking sync state in INBOX (23 messages on server)..."
	m.timeline.emails = mockEmails()

	sidebarLines := strings.Split(stripANSI(m.renderSidebar()), "\n")
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	if len(sidebarLines) > plan.ContentHeight {
		t.Fatalf("sidebar rendered %d rows, content height is %d:\n%s", len(sidebarLines), plan.ContentHeight, strings.Join(sidebarLines, "\n"))
	}
}

func TestStaleScopedFolderMessagesDoNotRepaintCurrentAccount(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	}
	b := newAccountAwareStubBackend(accounts)
	m := accountSwitcherTestModel(b)
	m.activeSourceID = "personal-mail"
	m.currentFolder = "INBOX"
	m.folders = []string{"INBOX", "Travel"}
	m.folderTree = buildFolderTree(m.folders)

	model, _ := m.Update(FoldersLoadedMsg{
		SourceID: backend.AllAccountsSourceID,
		Folders:  []string{"INBOX", "Archive"},
	})
	updated := model.(*Model)
	if strings.Join(updated.folders, ",") != "INBOX,Travel" {
		t.Fatalf("stale all-account folders repainted current account folders: %#v", updated.folders)
	}
}

func TestStaleScopedTimelineLoadedMsgIgnored(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	})
	m := accountSwitcherTestModel(b)
	m.activeSourceID = "personal-mail"
	m.currentFolder = "INBOX"
	personal := []*models.EmailData{{MessageID: "personal", Sender: "me@example.test", Subject: "Personal", Folder: "INBOX", Date: time.Now()}}
	m.timeline.emails = personal

	model, _, handled := m.handleTimelineMsg(TimelineLoadedMsg{
		SourceID: backend.AllAccountsSourceID,
		Folder:   "INBOX",
		Emails:   []*models.EmailData{{MessageID: "all", Sender: "work@example.test", Subject: "All", Folder: "INBOX", Date: time.Now()}},
	})
	if !handled {
		t.Fatal("expected TimelineLoadedMsg to be handled")
	}
	updated := model.(*Model)
	if len(updated.timeline.emails) != 1 || updated.timeline.emails[0].MessageID != "personal" {
		t.Fatalf("stale all-account timeline repainted current account: %#v", updated.timeline.emails)
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
	if selected.accountSwitcherCursor != 2 {
		t.Fatalf("cursor=%d, want 2", selected.accountSwitcherCursor)
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

func TestAccountSwitchShowsCachedTimelineWithoutRefreshingStatuses(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	}
	b := newAccountAwareStubBackend(accounts)
	cached := []*models.EmailData{{
		MessageID: "personal-inbox",
		SourceID:  "personal-mail",
		Sender:    "friend@example.test",
		Subject:   "Cached personal inbox",
		Folder:    "INBOX",
		Date:      time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC),
	}}
	b.timeline = map[string][]*models.EmailData{"INBOX": cached}
	m := accountSwitcherTestModel(b)
	b.activeSource = backend.AllAccountsSourceID
	m.activeSourceID = backend.AllAccountsSourceID
	m.timeline.emails = []*models.EmailData{{
		MessageID: "work-client",
		SourceID:  "work-mail",
		Sender:    "client@example.test",
		Subject:   "Work client",
		Folder:    "Clients",
		Date:      time.Date(2026, 5, 27, 8, 0, 0, 0, time.UTC),
	}, {
		MessageID: "personal-old",
		SourceID:  "personal-mail",
		Sender:    "old@example.test",
		Subject:   "Old personal",
		Folder:    "Clients",
		Date:      time.Date(2026, 5, 27, 7, 0, 0, 0, time.UTC),
	}}
	m.updateTableDimensions(120, 40)
	if !hasColumnTitle(m.timelineTable.Columns(), "Acct") {
		t.Fatal("test setup expected unified timeline to show Acct column")
	}
	m.currentFolder = "Clients"
	m.accountSelectedFolders[backend.AllAccountsSourceID] = "Clients"
	m.accountSelectedFolders["personal-mail"] = "INBOX"
	b.statusCalls = 0
	b.timelineCalls = 0

	cmd := m.switchActiveAccount("personal-mail")

	if cmd == nil {
		t.Fatal("expected account switch to still schedule background sync")
	}
	if b.statusCalls != 0 {
		t.Fatalf("account switch synchronously refreshed account statuses %d time(s)", b.statusCalls)
	}
	if b.timelineCalls != 1 {
		t.Fatalf("account switch should hydrate one cached timeline slice, got %d calls", b.timelineCalls)
	}
	if len(m.timeline.emails) != 1 || m.timeline.emails[0].MessageID != "personal-inbox" {
		t.Fatalf("expected cached personal timeline immediately after switch, got %#v", m.timeline.emails)
	}
	if hasColumnTitle(m.timelineTable.Columns(), "Acct") {
		t.Fatalf("specific-account cached timeline kept stale Acct column: %#v", tableColumnTitles(m.timelineTable.Columns()))
	}
	if m.timelineTable.Cursor() != 0 {
		t.Fatalf("expected timeline cursor reset to first cached row, got %d", m.timelineTable.Cursor())
	}
}

func TestAccountRailEnterSwitchesActiveAccountAndRestoresFolder(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	}
	b := newAccountAwareStubBackend(accounts)
	b.snapshots = accountSwitcherSnapshots(accounts)
	m := accountSwitcherTestModel(b)
	m.currentFolder = "Clients"
	m.accountSelectedFolders["work-mail"] = "Clients"
	m.accountSelectedFolders["personal-mail"] = "Travel"
	m.focusedPanel = panelSidebar
	m.sidebarCursor = findSidebarItem(t, m, "Personal", "personal-mail", "INBOX")

	cmd, handledAccount := m.selectSidebarFolder()
	if !handledAccount {
		t.Fatal("expected account child folder row to be handled as an account switch")
	}
	if cmd == nil {
		t.Fatal("expected account folder switch to schedule reload commands")
	}
	if !strings.EqualFold(string(b.activeSource), "personal-mail") {
		t.Fatalf("active source=%q, want personal-mail", b.activeSource)
	}
	if m.currentFolder != "INBOX" {
		t.Fatalf("currentFolder=%q, want selected INBOX", m.currentFolder)
	}
}

func TestActiveAccountSidebarChildSelectionLoadsSelectedFolder(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	}
	b := newAccountAwareStubBackend(accounts)
	b.snapshots = accountSwitcherSnapshots(accounts)
	m := accountSwitcherTestModel(b)
	m.currentFolder = "INBOX"
	m.accountSelectedFolders["work-mail"] = "INBOX"
	m.focusedPanel = panelSidebar
	m.sidebarCursor = findSidebarItem(t, m, "Drafts", "work-mail", "Drafts")

	cmd, handledAccount := m.selectSidebarFolder()
	if !handledAccount {
		t.Fatal("expected active account child folder row to be handled directly")
	}
	if cmd == nil {
		t.Fatal("expected active account folder selection to schedule reload commands")
	}
	if !strings.EqualFold(string(b.activeSource), "work-mail") {
		t.Fatalf("active source=%q, want work-mail", b.activeSource)
	}
	if m.currentFolder != "Drafts" {
		t.Fatalf("currentFolder=%q, want selected Drafts", m.currentFolder)
	}
	if got := m.accountSelectedFolders["work-mail"]; got != "Drafts" {
		t.Fatalf("remembered work-mail folder=%q, want Drafts", got)
	}
}

func TestAllAccountsRailAndSwitcherEntrySwitchUnifiedScope(t *testing.T) {
	accounts := []backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	}
	b := newAccountAwareStubBackend(accounts)
	b.snapshots = accountSwitcherSnapshots(accounts)
	m := accountSwitcherTestModel(b)
	m.currentFolder = "Clients"
	m.accountSelectedFolders["work-mail"] = "Clients"
	m.accountSelectedFolders["personal-mail"] = "Travel"
	m.focusedPanel = panelSidebar
	m.sidebarCursor = findSidebarItem(t, m, "All Inboxes", backend.AllAccountsSourceID, "INBOX")

	sidebar := stripANSI(m.renderSidebar())
	if !strings.Contains(sidebar, "All Inboxes") {
		t.Fatalf("multi-account sidebar missing All Inboxes row:\n%s", sidebar)
	}

	cmd, handledAccount := m.selectSidebarFolder()
	if !handledAccount {
		t.Fatal("expected All Inboxes row to be handled as an account switch")
	}
	if cmd == nil {
		t.Fatal("expected All Inboxes switch to schedule reload commands")
	}
	if b.activeSource != backend.AllAccountsSourceID {
		t.Fatalf("active source=%q, want all accounts", b.activeSource)
	}
	if m.currentFolder != "INBOX" {
		t.Fatalf("currentFolder=%q, want unified INBOX", m.currentFolder)
	}
	if title := stripANSI(m.renderTitleBar(120)); !strings.Contains(title, "All Accounts") {
		t.Fatalf("title missing All Accounts scope: %q", title)
	}

	model, _ := m.handleKeyMsg(keyRunes("A"))
	opened := model.(*Model)
	overlay := stripANSI(opened.renderAccountSwitcherOverlayView())
	if !strings.Contains(overlay, "All Accounts") {
		t.Fatalf("switcher overlay missing All Accounts:\n%s", overlay)
	}
}

func TestUnifiedTimelineRendersAccountBadgesAndKeepsDuplicateSelections(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{
		{SourceID: "work-mail", AccountID: "work", DisplayName: "Work Mail"},
		{SourceID: "personal-mail", AccountID: "personal", DisplayName: "Personal"},
	})
	m := accountSwitcherTestModel(b)
	m.activeSourceID = backend.AllAccountsSourceID
	m.timeline.senderWidth = 28
	m.timeline.subjectWidth = 44
	now := time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC)
	m.timeline.emails = []*models.EmailData{
		scopedAppEmail(&models.EmailData{SourceID: "work-mail", AccountID: "work", MessageID: "same-message", UID: 11, Folder: "INBOX", Sender: "work@example.test", Subject: "Work note", Date: now}),
		scopedAppEmail(&models.EmailData{SourceID: "personal-mail", AccountID: "personal", MessageID: "same-message", UID: 22, Folder: "INBOX", Sender: "me@example.test", Subject: "Personal note", Date: now.Add(-time.Minute)}),
	}
	m.updateTableDimensions(120, 40)

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected two unified rows, got %d: %#v", len(rows), rows)
	}
	renderedRows := stripANSI(strings.Join([]string{strings.Join(rows[0], " "), strings.Join(rows[1], " ")}, "\n"))
	for _, want := range []string{"Work Mail", "Personal"} {
		if !strings.Contains(renderedRows, want) {
			t.Fatalf("unified rows missing account badge %q:\n%s", want, renderedRows)
		}
	}

	m.timelineTable.SetCursor(0)
	m.toggleTimelineSelection()
	m.timelineTable.SetCursor(1)
	m.toggleTimelineSelection()
	if got := len(m.timeline.selectedMessageIDs); got != 2 {
		t.Fatalf("selected duplicate message IDs count=%d, want 2; selected=%#v", got, m.timeline.selectedMessageIDs)
	}
	if m.timeline.selectedMessageIDs["same-message"] {
		t.Fatalf("unified selection should use scoped keys, got %#v", m.timeline.selectedMessageIDs)
	}
}

func TestSingleAccountTimelineDoesNotRenderAccountBadges(t *testing.T) {
	b := newAccountAwareStubBackend([]backend.AccountInfo{{
		SourceID:    "default-mail",
		AccountID:   "default",
		DisplayName: "Default Mail",
	}})
	m := accountSwitcherTestModel(b)
	m.timeline.senderWidth = 28
	m.timeline.subjectWidth = 44
	m.timeline.emails = []*models.EmailData{{
		MessageID: "solo",
		Sender:    "sender@example.test",
		Subject:   "Hello",
		Date:      time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}}

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d: %#v", len(rows), rows)
	}
	if rendered := stripANSI(strings.Join(rows[0], " ")); strings.Contains(rendered, "Default Mail") || strings.Contains(rendered, "Acct") {
		t.Fatalf("single-account row should not show account badge: %q", rendered)
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
