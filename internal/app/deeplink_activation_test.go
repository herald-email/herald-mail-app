package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/deeplink"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/notifications"
)

func TestApplyDeepLinkMessageOpensTimelinePreview(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	email := &models.EmailData{MessageID: "msg-1", Folder: "INBOX", UID: 7, Sender: "Alice <alice@example.com>", Subject: "Hello"}
	m.currentFolder = "INBOX"
	m.timeline.emails = []*models.EmailData{email}
	m.loading = false
	m.updateTimelineTable()

	cmd := m.applyDeepLinkTarget(deeplink.Target{Kind: deeplink.KindMessage, Folder: "INBOX", MessageID: "msg-1"})

	if m.activeTab != tabTimeline {
		t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
	}
	if m.timeline.selectedEmail == nil || m.timeline.selectedEmail.MessageID != "msg-1" {
		t.Fatalf("selectedEmail = %#v, want msg-1", m.timeline.selectedEmail)
	}
	if cmd == nil {
		t.Fatal("message activation should load the preview body")
	}
}

func TestApplyDeepLinkSearchPopulatesTimelineSearch(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.currentFolder = "INBOX"
	m.loading = false

	_ = m.applyDeepLinkTarget(deeplink.Target{Kind: deeplink.KindSearch, Folder: "INBOX", Query: "invoice"})

	if m.activeTab != tabTimeline {
		t.Fatalf("activeTab = %d, want Timeline", m.activeTab)
	}
	if !m.timeline.searchMode || m.timeline.searchInput.Value() != "invoice" {
		t.Fatalf("search state mode=%v value=%q, want active invoice search", m.timeline.searchMode, m.timeline.searchInput.Value())
	}
}

func TestApplyDeepLinkComposePrefillsFields(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)

	_ = m.applyDeepLinkTarget(deeplink.Target{Kind: deeplink.KindCompose, To: "friend@example.com", Subject: "Hello"})

	if m.activeTab != tabCompose {
		t.Fatalf("activeTab = %d, want Compose", m.activeTab)
	}
	if got := m.composeTo.Value(); got != "friend@example.com" {
		t.Fatalf("compose To = %q", got)
	}
	if got := m.composeSubject.Value(); got != "Hello" {
		t.Fatalf("compose Subject = %q", got)
	}
}

func TestNewMailNotificationUsesMessageOrFolderDeepLink(t *testing.T) {
	rec := notifications.NewRecorder()
	m := New(&stubBackend{}, nil, "", nil, false)
	m.SetNotifier(rec)
	cfg := &config.Config{}
	cfg.Notifications.Enabled = true
	cfg.Notifications.NewMail = true
	cfg.Notifications.SyncFailures = true
	m.SetConfig(cfg)

	one := NewEmailsMsg{Folder: "INBOX", Emails: []*models.EmailData{{MessageID: "msg-1", Folder: "INBOX", Sender: "Alice <alice@example.com>", Subject: "Hello"}}}
	runNotificationCmd(t, m.notifyNewMailCmd(one))
	if got := rec.Delivered()[0].DeepLink; !strings.Contains(got, "/message?") || !strings.Contains(got, "message_id=msg-1") {
		t.Fatalf("single-message deep link = %q, want message link", got)
	}

	many := NewEmailsMsg{Folder: "INBOX", Emails: []*models.EmailData{{MessageID: "msg-2", Folder: "INBOX"}, {MessageID: "msg-3", Folder: "INBOX"}}}
	runNotificationCmd(t, m.notifyNewMailCmd(many))
	if got := rec.Delivered()[1].DeepLink; !strings.Contains(got, "/folder?") || !strings.Contains(got, "folder=INBOX") {
		t.Fatalf("multi-message deep link = %q, want folder link", got)
	}
}

func TestSyncFailureNotificationUsesFolderDeepLink(t *testing.T) {
	rec := notifications.NewRecorder()
	m := New(&stubBackend{}, nil, "", nil, false)
	m.SetNotifier(rec)
	cfg := &config.Config{}
	cfg.Notifications.Enabled = true
	cfg.Notifications.NewMail = true
	cfg.Notifications.SyncFailures = true
	m.SetConfig(cfg)

	cmd := m.notifySyncFailureCmd(models.FolderSyncEvent{Folder: "Archive", Message: "sync failed"})
	runNotificationCmd(t, cmd)
	if got := rec.Delivered()[0].DeepLink; !strings.Contains(got, "/folder?") || !strings.Contains(got, "folder=Archive") {
		t.Fatalf("sync failure deep link = %q, want Archive folder link", got)
	}
}

func runNotificationCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected notification command")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("notification cmd returned unexpected msg %T", msg)
	}
}
