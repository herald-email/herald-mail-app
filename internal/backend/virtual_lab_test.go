package backend

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func TestLocalBackendUsesVirtualMailLabForSendDraftAndReply(t *testing.T) {
	lab := testmail.Start(t)
	alice := lab.Account(testmail.DefaultAliceAddress)
	bob := lab.Account(testmail.DefaultBobAddress)
	cfg := alice.Config(filepath.Join(t.TempDir(), "alice-cache.db"))

	b, err := NewLocal(cfg, "", nil)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	defer b.Close()
	if err := b.imapClient.Connect(); err != nil {
		t.Fatalf("connect virtual IMAP: %v", err)
	}

	if err := b.SendEmail(bob.Address, "Backend send", "Hello Bob.", alice.Address); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	lab.WaitForSubject(bob.Address, "INBOX", "Backend send")
	lab.WaitForSubject(alice.Address, "Sent", "Backend send")

	draftUID, draftFolder, err := b.SaveDraft(bob.Address, "", "", "Backend draft", "Draft body")
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	if draftUID == 0 || draftFolder != "Drafts" {
		t.Fatalf("draft saved to uid=%d folder=%q, want nonzero Drafts", draftUID, draftFolder)
	}
	lab.WaitForSubject(alice.Address, "Drafts", "Backend draft")

	originalRaw := []byte("From: " + bob.Address + "\r\n" +
		"To: " + alice.Address + "\r\n" +
		"Subject: Backend original\r\n" +
		"Date: Wed, 20 May 2026 10:00:00 -0700\r\n" +
		"Message-ID: <backend-original@herald.test>\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Original body.\r\n")
	original := alice.AppendEML("INBOX", originalRaw)
	if err := b.cache.CacheEmail(&models.EmailData{
		MessageID:   original.MessageID,
		UID:         original.UID,
		Sender:      bob.Address,
		Subject:     "Backend original",
		Date:        time.Date(2026, 5, 20, 10, 0, 0, 0, time.FixedZone("PDT", -7*60*60)),
		Folder:      "INBOX",
		LastUpdated: time.Now(),
	}); err != nil {
		t.Fatalf("cache original: %v", err)
	}

	if err := b.ReplyToEmail(original.MessageID, "Thanks for the note."); err != nil {
		t.Fatalf("ReplyToEmail: %v", err)
	}
	lab.WaitForSubject(bob.Address, "INBOX", "Re: Backend original")

	captured := lab.CapturedSMTP()
	if len(captured) < 2 {
		t.Fatalf("captured SMTP count = %d, want at least send and reply", len(captured))
	}
	reply := captured[len(captured)-1].Data
	if !bytes.Contains(reply, []byte("In-Reply-To: <backend-original@herald.test>")) {
		t.Fatalf("reply missing In-Reply-To header:\n%s", string(reply))
	}
	if !strings.Contains(string(reply), "Thanks for the note.") {
		t.Fatalf("reply missing top note:\n%s", string(reply))
	}
}
