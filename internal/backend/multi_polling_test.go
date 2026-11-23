package backend

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestMultiPollingDeliversGmailAndIMAPToAllInboxes(t *testing.T) {
	fake := newFakeGmailAPIServer(t)
	defer fake.Close()
	gmailCache := newMemoryCache(t)
	gmail := newTestGmailAPIMailSource(t, fake.URL(), gmailCache)
	if err := gmail.ProcessEmailsIncremental(context.Background(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	fake.messagePayloads["msg-2"] = gmailMessagePayloadFor("msg-2", "<gmail-poll@example.test>", []string{"INBOX"}, "102")
	fake.historyResponse = map[string]any{
		"historyId": "102",
		"history": []map[string]any{{
			"messagesAdded": []map[string]any{{"message": map[string]string{"id": "msg-2"}}},
		}},
	}

	lab := testmail.Start(t)
	alice := lab.Account(testmail.DefaultAliceAddress)
	imap, err := NewLocal(alice.Config(filepath.Join(t.TempDir(), "imap.db")), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	gmailBackend := &LocalBackend{mailSource: gmail, cache: gmailCache, newEmailsCh: make(chan models.NewEmailsNotification, 10)}
	multi, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "gmail", Provider: "gmail"}, Backend: gmailBackend},
		{Info: AccountInfo{SourceID: "imap", Provider: "imap"}, Backend: imap},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = multi.Close() })
	if err := multi.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatal(err)
	}
	appendIMAP := func(name string) {
		t.Helper()
		// Deliver through the same IMAP connection. The virtual server's
		// in-memory mailbox cannot be changed directly while a poll reads it.
		messageID := "<" + name + "@example.test>"
		raw := []byte(fmt.Sprintf("From: sender@example.test\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\n\r\nTest\r\n", alice.Address, name, time.Now().Format(time.RFC1123Z), messageID))
		_, folder, err := imap.mailSource.AppendDraft(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := imap.mailSource.MoveEmail(context.Background(), messageID, folder, "INBOX"); err != nil {
			t.Fatal(err)
		}
	}
	multi.StartPolling("INBOX", 1)
	appendIMAP("imap-poll")
	seen := map[models.SourceID]string{}
	timeout := time.After(5 * time.Second)
	for len(seen) < 2 {
		select {
		case event := <-multi.NewEmailsCh():
			seen[event.SourceID] = event.Emails[0].Subject
		case <-timeout:
			t.Fatalf("expected arrivals from both accounts, got %v", seen)
		}
	}
	if seen["gmail"] == "" || seen["imap"] != "imap-poll" {
		t.Fatalf("unexpected arrivals: %v", seen)
	}
	if err := multi.SwitchAccount("gmail"); err != nil {
		t.Fatal(err)
	}
	multi.StartPolling("INBOX", 1)
	appendIMAP("imap-away")
	deadline := time.Now().Add(5 * time.Second)
	for {
		emails, err := imap.GetTimelineEmails("INBOX")
		if err != nil {
			t.Fatal(err)
		}
		for _, email := range emails {
			if email.Subject == "imap-away" {
				if err := multi.SwitchAccount(AllAccountsSourceID); err != nil {
					t.Fatal(err)
				}
				all, err := multi.GetTimelineEmails("INBOX")
				if err != nil || !messageIDSet(all)["<gmail-poll@example.test>"] || !messageIDSet(all)[email.MessageID] {
					t.Fatalf("combined cache is missing polled arrivals: %v, %v", messageIDSet(all), err)
				}
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("IMAP account stopped receiving mail while Gmail was selected")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

type blockedInboxPoll struct {
	*fakeMailSource
	started chan struct{}
	stopped chan struct{}
}

func (p *blockedInboxPoll) PollForNewEmails(ctx context.Context, _ string, _ time.Time) ([]*models.EmailData, error) {
	close(p.started)
	<-ctx.Done()
	close(p.stopped)
	return nil, ctx.Err()
}

func TestMultiPollingSlowAccountDoesNotBlockOtherMailbox(t *testing.T) {
	blocked := &blockedInboxPoll{fakeMailSource: newFakeMailSource(), started: make(chan struct{}), stopped: make(chan struct{})}
	healthy := newFakeMailSource()
	healthy.pollEmails = []*models.EmailData{{MessageID: "healthy-arrival", Folder: "INBOX", Date: time.Now()}}
	local := func(source MailSource) *LocalBackend {
		return &LocalBackend{mailSource: source, cache: newMemoryCache(t), newEmailsCh: make(chan models.NewEmailsNotification, 1)}
	}
	multi, err := NewMultiBackend([]AccountBackend{
		{Info: AccountInfo{SourceID: "slow"}, Backend: local(blocked)},
		{Info: AccountInfo{SourceID: "healthy"}, Backend: local(healthy)},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = multi.Close() })
	if err := multi.SwitchAccount(AllAccountsSourceID); err != nil {
		t.Fatal(err)
	}
	multi.StartPolling("INBOX", 1)
	select {
	case <-blocked.started:
	case <-time.After(4 * time.Second):
		t.Fatal("slow account never started its poll")
	}
	select {
	case event := <-multi.NewEmailsCh():
		if event.SourceID != "healthy" {
			t.Fatalf("unexpected arrival from %s", event.SourceID)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("one blocked account prevented mail from arriving for the other")
	}
	multi.StopPolling()
	select {
	case <-blocked.stopped:
	case <-time.After(time.Second):
		t.Fatal("stopping polling did not cancel the blocked request")
	}
}
