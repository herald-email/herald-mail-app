package backend

import (
	"context"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type arrivalMailSource struct {
	*fakeMailSource
	backend *LocalBackend
	entered chan struct{}
	release chan struct{}
}

func (s *arrivalMailSource) ProcessEmailsIncremental(context.Context, string) error {
	return s.backend.cache.CacheEmail(&models.EmailData{MessageID: "new", Folder: "INBOX", Date: time.Now()})
}
func (s *arrivalMailSource) StartBackgroundReconcile(ctx context.Context, folder string, out chan<- map[string]bool) {
	close(s.entered)
	select {
	case <-s.release:
		out <- map[string]bool{"old": true, "new": true}
	case <-ctx.Done():
	}
	close(out)
}
func TestSyncArrivalVisibleBeforeReconcile(t *testing.T) {
	b := &LocalBackend{cache: newMemoryCache(t), cfg: &config.Config{}, lastFetchCurrent: make(map[int64]int)}
	for _, id := range []string{"old", "stale"} {
		if err := b.cache.CacheEmail(&models.EmailData{MessageID: id, Folder: "INBOX", Date: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	b.setValidIDsForFolder("INBOX", map[string]bool{"old": true})
	source := &arrivalMailSource{fakeMailSource: newFakeMailSource(), backend: b, entered: make(chan struct{}), release: make(chan struct{})}
	b.mailSource = source
	done := make(chan struct{})
	go func() { b.runLoad(folderLoadRequest{Folder: "INBOX", Generation: 1}); close(done) }()
	<-source.entered
	emails, err := b.GetTimelineEmails("INBOX")
	close(source.release)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	ids := messageIDSet(emails)
	if !ids["new"] || !ids["old"] || ids["stale"] {
		t.Fatalf("visible IDs before reconcile = %v", ids)
	}
}

func TestReconcileDoesNotHideArrivalCachedWhileRunning(t *testing.T) {
	b := &LocalBackend{cache: newMemoryCache(t)}
	if err := b.cache.CacheEmail(&models.EmailData{MessageID: "old", Folder: "INBOX"}); err != nil {
		t.Fatal(err)
	}
	ch := b.prepareValidIDsChannelForFolder("INBOX")
	if err := b.cache.CacheEmail(&models.EmailData{MessageID: "new", Folder: "INBOX"}); err != nil {
		t.Fatal(err)
	}
	ch <- map[string]bool{"old": true}
	close(ch)
	ids := <-b.ValidIDsCh()
	if !ids["new"] {
		t.Fatalf("reconcile hid arrival: %v", ids)
	}
}

func TestUserLoadCancelsBackgroundReconcile(t *testing.T) {
	b := &LocalBackend{cache: newMemoryCache(t), cfg: &config.Config{}, lastFetchCurrent: make(map[int64]int), loadCoordinator: newLatestWinsLoadCoordinator()}
	source := &arrivalMailSource{fakeMailSource: newFakeMailSource(), backend: b, entered: make(chan struct{}), release: make(chan struct{})}
	b.mailSource = source
	b.Load("INBOX")
	req, _ := b.loadCoordinator.DrainPending()
	done := make(chan struct{})
	go func() { b.runLoad(req); close(done) }()
	<-source.entered
	b.Load("Archive")
	select {
	case <-done:
	case <-time.After(time.Second):
		close(source.release)
		<-done
		t.Fatal("user load waited behind background reconcile")
	}
}

func TestGmailPollingArrivalRemainsVisibleInTimelineSnapshot(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()
	c := newMemoryCache(t)
	source := newTestGmailAPIMailSource(t, fake.URL(), c)
	if err := source.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	const arrivalID = "<polled@example.com>"
	fake.messagePayloads["msg-2"] = gmailMessagePayloadFor("msg-2", arrivalID, []string{"INBOX"}, "102")
	fake.historyResponse = map[string]any{
		"historyId": "102",
		"history": []map[string]any{{
			"messagesAdded": []map[string]any{{"message": map[string]string{"id": "msg-2"}}},
		}},
	}
	if err := c.CacheEmail(&models.EmailData{MessageID: "stale", Folder: "INBOX"}); err != nil {
		t.Fatal(err)
	}
	b := &LocalBackend{mailSource: source, cache: c, newEmailsCh: make(chan models.NewEmailsNotification, 1)}
	b.setValidIDsForFolder("INBOX", map[string]bool{"<api-1@example.com>": true})
	b.StartPolling("INBOX", 1)
	defer b.StopPolling()
	select {
	case notification := <-b.NewEmailsCh():
		b.StopPolling()
		if notification.Folder != "INBOX" || len(notification.Emails) != 1 || notification.Emails[0].MessageID != arrivalID {
			t.Fatalf("unexpected poll notification: %+v", notification)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Gmail poll did not deliver new mail")
	}
	emails, err := b.GetTimelineEmails("INBOX")
	if err != nil {
		t.Fatal(err)
	}
	ids := messageIDSet(emails)
	if !ids[arrivalID] || !ids["<api-1@example.com>"] || ids["stale"] {
		t.Fatalf("unexpected timeline membership after Gmail poll: %v", ids)
	}
}
