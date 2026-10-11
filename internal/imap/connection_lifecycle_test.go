package imap

import (
	"errors"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestConnectRetriesAfterFailedAuthentication(t *testing.T) {
	lab := testmail.Start(t, testmail.WithAccount("test@example.com", "password"))
	cfg := lab.Account("test@example.com").Config("")
	cfg.Credentials.Password = "wrong-password"
	c := New(cfg, "", nil, nil)
	defer c.Close()
	if err := c.Connect(); err == nil {
		t.Fatal("expected authentication failure")
	}
	if c.client != nil {
		t.Fatal("failed authentication retained a connection")
	}
	cfg.Credentials.Password = "password"
	if err := c.Connect(); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := c.ListFolders(); err != nil {
		t.Fatalf("authenticated operation: %v", err)
	}
	// Reconnect already owns the mutex; it must not call the locking wrapper.
	c.mu.Lock()
	err := c.Reconnect()
	c.mu.Unlock()
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
}

func TestMissingUIDErrorsOnlyAfterSuccessfulFetch(t *testing.T) {
	lab := testmail.Start(t, testmail.WithAccount("test@example.com", "password"))
	c := New(lab.Account("test@example.com").Config(""), "", nil, nil)
	defer c.Close()
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
	for _, preview := range []bool{false, true} {
		for _, folder := range []string{"INBOX", "folder not found"} {
			var err error
			if preview {
				_, err = c.FetchEmailPreviewBody(42, folder)
			} else {
				_, err = c.FetchEmailBody(42, folder)
			}
			if err == nil {
				t.Fatalf("preview=%v folder=%q: expected failure", preview, folder)
			}
			if got, want := errors.Is(err, ErrMessageNotFound), folder == "INBOX"; got != want {
				t.Fatalf("preview=%v folder=%q: missing=%v, want %v: %v", preview, folder, got, want, err)
			}
		}
	}
}
