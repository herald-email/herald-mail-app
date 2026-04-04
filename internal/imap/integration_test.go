//go:build integration

package imap

import (
	"testing"

	"mail-processor/internal/cache"
	"mail-processor/internal/models"
	"mail-processor/internal/testutil"
)

// newIntegrationCache returns an in-memory SQLite cache for integration tests.
func newIntegrationCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("create test cache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestConnect_StandardAuth verifies that Client.Connect() can authenticate
// against a plain-TCP IMAP server using username/password credentials.
func TestConnect_StandardAuth(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 10)
	c := New(cfg, "", newIntegrationCache(t), progressCh)

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	t.Log("Connect succeeded")
}

// TestConnect_WrongPassword verifies that an incorrect password returns an
// authentication error from Connect().
func TestConnect_WrongPassword(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	cfg.Credentials.Password = "wrong-password"

	progressCh := make(chan models.ProgressInfo, 10)
	c := New(cfg, "", newIntegrationCache(t), progressCh)

	err := c.Connect()
	if err == nil {
		_ = c.Close()
		t.Fatal("expected Connect to fail with wrong password, but it succeeded")
	}
	t.Logf("Connect correctly rejected wrong password: %v", err)
}

// TestProcessEmailsIncremental_FirstRun verifies that on the first sync the
// client fetches all messages from INBOX and stores them in the cache.
// The mock server seeds INBOX with 5 messages.
func TestProcessEmailsIncremental_FirstRun(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 50)
	ch := newIntegrationCache(t)
	c := New(cfg, "", ch, progressCh)

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.ProcessEmailsIncremental("INBOX"); err != nil {
		t.Fatalf("ProcessEmailsIncremental: %v", err)
	}

	emails, err := ch.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate: %v", err)
	}
	if len(emails) != 5 {
		t.Errorf("expected 5 emails in cache after first sync, got %d", len(emails))
	}
	t.Logf("Synced %d emails on first run", len(emails))
}

// TestProcessEmailsIncremental_NoNewMail verifies that a second sync on an
// unchanged mailbox does not duplicate messages in the cache.
func TestProcessEmailsIncremental_NoNewMail(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 50)
	ch := newIntegrationCache(t)
	c := New(cfg, "", ch, progressCh)

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	// First sync
	if err := c.ProcessEmailsIncremental("INBOX"); err != nil {
		t.Fatalf("first ProcessEmailsIncremental: %v", err)
	}

	// Second sync — should be a no-op (syncStrategyNone)
	if err := c.ProcessEmailsIncremental("INBOX"); err != nil {
		t.Fatalf("second ProcessEmailsIncremental: %v", err)
	}

	emails, err := ch.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate: %v", err)
	}
	if len(emails) != 5 {
		t.Errorf("expected exactly 5 emails after second sync, got %d (possible duplication)", len(emails))
	}
}

// TestBatchFetchDetails_ProcessesAllEmails verifies that batchFetchDetails
// retrieves all seeded messages in a single round trip and stores them in the
// cache correctly. This exercises the batch-fetch path introduced to replace
// the serial processMessage loop.
func TestBatchFetchDetails_ProcessesAllEmails(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 50)
	ch := newIntegrationCache(t)
	c := New(cfg, "", ch, progressCh)

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	// ProcessEmailsIncremental now routes through batchFetchDetails internally.
	// A first run should find all 5 seeded messages and store them.
	if err := c.ProcessEmailsIncremental("INBOX"); err != nil {
		t.Fatalf("ProcessEmailsIncremental: %v", err)
	}

	emails, err := ch.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate: %v", err)
	}
	if len(emails) != 5 {
		t.Errorf("batchFetchDetails: expected 5 emails in cache, got %d", len(emails))
	}
	// All emails must have a non-empty sender and message ID.
	for i, e := range emails {
		if e.Sender == "" {
			t.Errorf("email[%d] has empty sender", i)
		}
		if e.MessageID == "" {
			t.Errorf("email[%d] has empty message ID", i)
		}
	}
	t.Logf("batchFetchDetails correctly cached %d emails", len(emails))
}

// TestMoveEmail verifies that MoveEmail copies a message to Archive,
// removes it from INBOX on the server, and deletes it from the cache.
func TestMoveEmail(t *testing.T) {
	_, cfg, stop := testutil.StartMockIMAPServer(t)
	defer stop()

	progressCh := make(chan models.ProgressInfo, 50)
	ch := newIntegrationCache(t)
	c := New(cfg, "", ch, progressCh)

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Sync so the cache is populated.
	if err := c.ProcessEmailsIncremental("INBOX"); err != nil {
		t.Fatalf("ProcessEmailsIncremental: %v", err)
	}

	// Pick a message to move — use the well-known seeded message ID.
	const targetID = "<test-msg-2@localhost>"

	if err := c.MoveEmail(targetID, "INBOX", "Archive"); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}

	// The message must no longer be in the cache.
	emails, err := ch.GetEmailsSortedByDate("INBOX")
	if err != nil {
		t.Fatalf("GetEmailsSortedByDate after move: %v", err)
	}
	for _, e := range emails {
		if e.MessageID == targetID {
			t.Errorf("message %q should have been removed from cache after MoveEmail", targetID)
		}
	}
	// Only 4 messages remain in the cache.
	if len(emails) != 4 {
		t.Errorf("expected 4 emails after move, got %d", len(emails))
	}
	t.Logf("MoveEmail succeeded; %d messages remain in cache", len(emails))
}
