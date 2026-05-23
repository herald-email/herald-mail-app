package app

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	backendpkg "github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type previewPrewarmBackend struct {
	stubBackend
	cached        map[string]*models.EmailBody
	fetchIDs      []string
	cacheWriteIDs []string
}

func (b *previewPrewarmBackend) GetCachedPreviewBody(messageID string) (*models.EmailBody, error) {
	if b.cached == nil {
		return nil, nil
	}
	return b.cached[messageID], nil
}

func (b *previewPrewarmBackend) CachePreviewBody(messageID string, body *models.EmailBody) error {
	b.cacheWriteIDs = append(b.cacheWriteIDs, messageID)
	if b.cached == nil {
		b.cached = make(map[string]*models.EmailBody)
	}
	b.cached[messageID] = body
	return nil
}

func (b *previewPrewarmBackend) FetchPreviewBody(messageID, _ string, _ uint32) (*models.EmailBody, error) {
	b.fetchIDs = append(b.fetchIDs, messageID)
	return &models.EmailBody{TextPlain: "preview " + messageID}, nil
}

type previewPrewarmServiceBackend struct {
	stubBackend
	serviceIDs      []string
	serviceIntents  []backendpkg.MessageReadIntent
	serviceRefScope []models.MessageRef
}

func (b *previewPrewarmServiceBackend) GetMessagePreview(_ context.Context, ref models.MessageRef, intent backendpkg.MessageReadIntent) (backendpkg.MessageReadResult, error) {
	b.serviceIDs = append(b.serviceIDs, ref.MessageID)
	b.serviceIntents = append(b.serviceIntents, intent)
	b.serviceRefScope = append(b.serviceRefScope, ref)
	return backendpkg.MessageReadResult{
		Body:   &models.EmailBody{TextPlain: "preview " + ref.MessageID},
		Source: backendpkg.MessageReadSourceProvider,
	}, nil
}

func previewPrewarmEmails() []*models.EmailData {
	return []*models.EmailData{
		{MessageID: "msg-1", Folder: "INBOX", UID: 1},
		{MessageID: "msg-2", Folder: "INBOX", UID: 2},
		{MessageID: "msg-3", Folder: "INBOX", UID: 3},
		{MessageID: "archive-1", Folder: "Archive", UID: 4},
	}
}

func previewPrewarmMessagesForTest(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		messages := make([]tea.Msg, 0, len(batch))
		for _, child := range batch {
			if child == nil {
				continue
			}
			if childMsg := child(); childMsg != nil {
				messages = append(messages, childMsg)
			}
		}
		return messages
	}
	return []tea.Msg{msg}
}

func TestStartPreviewPrewarmerSkipsCachedMessagesAndWarmsOneAtATime(t *testing.T) {
	backend := &previewPrewarmBackend{
		cached: map[string]*models.EmailBody{
			"msg-1": {TextPlain: "already cached"},
		},
	}
	m := New(backend, nil, "", nil, false)
	m.currentFolder = "INBOX"
	m.backgroundWorkGeneration = 7
	m.timeline.emails = previewPrewarmEmails()

	cmd := m.startPreviewPrewarmerIfNeeded()
	if cmd == nil {
		t.Fatal("expected preview prewarmer command")
	}
	if !m.previewPrewarmActive {
		t.Fatal("expected preview prewarmer to be marked active")
	}

	first := cmd().(previewPrewarmMsg)
	if first.Done != 1 || first.Total != 3 || first.Skipped != 1 || first.Warmed != 0 {
		t.Fatalf("first progress = %#v, want one cached skip out of three", first)
	}
	if len(backend.fetchIDs) != 0 {
		t.Fatalf("cached message should not fetch, got fetches %#v", backend.fetchIDs)
	}

	model, nextCmd := m.Update(first)
	updated := model.(*Model)
	if nextCmd == nil {
		t.Fatal("expected next one-at-a-time prewarm command")
	}
	if updated.previewPrewarmDone != 1 || updated.previewPrewarmTotal != 3 {
		t.Fatalf("prewarm counters = %d/%d, want 1/3", updated.previewPrewarmDone, updated.previewPrewarmTotal)
	}

	second := nextCmd().(previewPrewarmMsg)
	if second.Done != 2 || second.Warmed != 1 || second.Skipped != 1 {
		t.Fatalf("second progress = %#v, want one warmed and one skipped", second)
	}
	if len(backend.fetchIDs) != 1 || backend.fetchIDs[0] != "msg-2" {
		t.Fatalf("fetchIDs = %#v, want [msg-2]", backend.fetchIDs)
	}
	if len(backend.cacheWriteIDs) != 1 || backend.cacheWriteIDs[0] != "msg-2" {
		t.Fatalf("cacheWriteIDs = %#v, want [msg-2]", backend.cacheWriteIDs)
	}
}

func TestPreviewPrewarmUsesCacheFirstServiceWhenAvailable(t *testing.T) {
	backend := &previewPrewarmServiceBackend{}
	m := New(backend, nil, "", nil, false)
	m.currentFolder = "INBOX"
	m.backgroundWorkGeneration = 17
	m.timeline.emails = previewPrewarmEmails()

	cmd := m.startPreviewPrewarmerIfNeeded()
	if cmd == nil {
		t.Fatal("expected service-backed preview prewarmer command")
	}

	first := cmd().(previewPrewarmMsg)
	if first.Done != 1 || first.Total != 3 || first.Warmed != 1 || first.Skipped != 0 {
		t.Fatalf("first service progress = %#v, want one warmed out of three", first)
	}
	if len(backend.serviceIDs) != 1 || backend.serviceIDs[0] != "msg-1" {
		t.Fatalf("serviceIDs = %#v, want [msg-1]", backend.serviceIDs)
	}
	if backend.serviceIntents[0].ViewID != "timeline-prewarm" {
		t.Fatalf("service intent = %#v, want timeline-prewarm", backend.serviceIntents[0])
	}
	if got := backend.serviceRefScope[0]; got.SourceID == "" || got.AccountID == "" || got.LocalID == "" {
		t.Fatalf("service ref missing default source/account/local scope: %#v", got)
	}
}

func TestPreviewPrewarmerIgnoresStaleFolderGeneration(t *testing.T) {
	backend := &previewPrewarmBackend{}
	m := New(backend, nil, "", nil, false)
	m.currentFolder = "Archive"
	m.backgroundWorkGeneration = 9

	model, cmd := m.Update(previewPrewarmMsg{
		Folder:     "INBOX",
		Generation: 8,
		Done:       1,
		Total:      2,
		Remaining: []previewPrewarmTarget{
			{MessageID: "msg-2", Folder: "INBOX", UID: 2},
		},
	})
	updated := model.(*Model)
	if cmd != nil {
		t.Fatal("stale prewarm result should not schedule another command")
	}
	if updated.previewPrewarmActive {
		t.Fatal("stale prewarm result should not reactivate the worker")
	}
	if len(backend.fetchIDs) != 0 {
		t.Fatalf("stale prewarm result fetched more previews: %#v", backend.fetchIDs)
	}
}

func TestStartPreviewPrewarmerRequiresPreviewBackendSupport(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	m.currentFolder = "INBOX"
	m.timeline.emails = previewPrewarmEmails()

	cmd := m.startPreviewPrewarmerIfNeeded()
	if cmd != nil {
		t.Fatal("unsupported backend should not start preview prewarming")
	}
	if m.previewPrewarmActive {
		t.Fatal("unsupported backend should not mark preview prewarming active")
	}
}

func TestTimelineLoadedStartsPreviewPrewarmerWithoutClassifier(t *testing.T) {
	backend := &previewPrewarmBackend{}
	m := New(backend, nil, "", nil, false)
	m.currentFolder = "INBOX"
	m.backgroundWorkGeneration = 12

	model, cmd, handled := m.handleTimelineMsg(TimelineLoadedMsg{Emails: previewPrewarmEmails()})
	if !handled {
		t.Fatal("expected TimelineLoadedMsg to be handled")
	}
	updated := model.(*Model)
	if cmd == nil {
		t.Fatal("expected TimelineLoadedMsg to schedule preview prewarming")
	}
	if !updated.previewPrewarmActive {
		t.Fatal("expected preview prewarming active after Timeline load")
	}

	messages := previewPrewarmMessagesForTest(cmd)
	if len(messages) != 1 {
		t.Fatalf("expected one immediate prewarm message, got %#v", messages)
	}
	msg := messages[0].(previewPrewarmMsg)
	if msg.Folder != "INBOX" || msg.Generation != 12 {
		t.Fatalf("prewarm message folder/generation = %s/%d, want INBOX/12", msg.Folder, msg.Generation)
	}
	if len(backend.fetchIDs) != 1 || backend.fetchIDs[0] != "msg-1" {
		t.Fatalf("Timeline prewarm fetchIDs = %#v, want [msg-1]", backend.fetchIDs)
	}
}

func TestSyncHydratedCompleteStartsPreviewPrewarmer(t *testing.T) {
	backend := &previewPrewarmBackend{}
	m := New(backend, nil, "", nil, false)
	m.currentFolder = "INBOX"
	m.syncGeneration = 21
	m.backgroundWorkGeneration = 21

	model, cmd := m.Update(SyncHydratedMsg{
		Folder:        "INBOX",
		Generation:    21,
		Emails:        previewPrewarmEmails(),
		FinishLoading: true,
		StatusMessage: "Found 3 senders",
	})
	updated := model.(*Model)
	if cmd == nil {
		t.Fatal("expected completed sync hydration to start preview prewarming")
	}
	if !updated.previewPrewarmActive {
		t.Fatal("expected preview prewarming active after completed sync hydration")
	}

	messages := previewPrewarmMessagesForTest(cmd)
	if len(messages) != 1 {
		t.Fatalf("expected one immediate prewarm message, got %#v", messages)
	}
	msg := messages[0].(previewPrewarmMsg)
	if msg.Folder != "INBOX" || msg.Generation != 21 {
		t.Fatalf("prewarm message folder/generation = %s/%d, want INBOX/21", msg.Folder, msg.Generation)
	}
	if len(backend.fetchIDs) != 1 || backend.fetchIDs[0] != "msg-1" {
		t.Fatalf("sync prewarm fetchIDs = %#v, want [msg-1]", backend.fetchIDs)
	}
}
