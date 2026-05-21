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

func TestLocalBackendFetchesVirtualMailScenarios(t *testing.T) {
	names := []testmail.ScenarioName{
		testmail.ScenarioPlainThread,
		testmail.ScenarioCalendlyInvite,
		testmail.ScenarioNewsletterTable,
		testmail.ScenarioReceiptHTML,
		testmail.ScenarioMalformedCharset,
		testmail.ScenarioInlineCIDImage,
		testmail.ScenarioLongLinkTracking,
		testmail.ScenarioUnsubscribeHeaders,
	}

	for _, name := range names {
		t.Run(string(name), func(t *testing.T) {
			seeded := testmail.StartScenario(t, name)
			backendByAccount := make(map[string]*LocalBackend)
			t.Cleanup(func() {
				for _, b := range backendByAccount {
					_ = b.Close()
				}
			})

			getBackend := func(account string) *LocalBackend {
				if b := backendByAccount[account]; b != nil {
					return b
				}
				virtualAccount := seeded.Lab.Account(account)
				if virtualAccount == nil {
					t.Fatalf("missing virtual account %q", account)
				}
				cacheName := strings.NewReplacer("@", "_", ".", "_").Replace(account) + ".db"
				b, err := NewLocal(virtualAccount.Config(filepath.Join(t.TempDir(), cacheName)), "", nil)
				if err != nil {
					t.Fatalf("NewLocal(%s): %v", account, err)
				}
				if err := b.imapClient.Connect(); err != nil {
					t.Fatalf("connect virtual IMAP for %s: %v", account, err)
				}
				backendByAccount[account] = b
				return b
			}

			for _, msg := range seeded.Messages {
				ref, ok := seeded.Refs[msg.Key]
				if !ok {
					t.Fatalf("missing ref for %q", msg.Key)
				}
				body, err := getBackend(msg.Account).FetchEmailBody(ref.Folder, ref.UID)
				if err != nil {
					t.Fatalf("FetchEmailBody(%s/%s/%d): %v", msg.Account, ref.Folder, ref.UID, err)
				}
				if body.Subject != msg.Subject {
					t.Fatalf("%s subject = %q, want %q", msg.Key, body.Subject, msg.Subject)
				}
				if body.MessageID != msg.MessageID {
					t.Fatalf("%s message ID = %q, want %q", msg.Key, body.MessageID, msg.MessageID)
				}
				if strings.TrimSpace(body.TextPlain) == "" && strings.TrimSpace(body.TextHTML) == "" && len(body.InlineImages) == 0 {
					t.Fatalf("%s fetched no previewable body content: %+v", msg.Key, body)
				}

				switch name {
				case testmail.ScenarioCalendlyInvite:
					if !strings.Contains(body.TextPlain, "Bob invited you") || !strings.Contains(body.TextHTML, "Join meeting") {
						t.Fatalf("calendly-like body lost expected text/html parts: %+v", body)
					}
				case testmail.ScenarioMalformedCharset:
					if !strings.Contains(body.TextPlain, "fall back without blanking") {
						t.Fatalf("malformed charset fallback missing readable text: %q", body.TextPlain)
					}
				case testmail.ScenarioInlineCIDImage:
					if len(body.InlineImages) != 1 || body.InlineImages[0].ContentID != "chart-001@herald.test" {
						t.Fatalf("inline CID images = %#v, want chart-001@herald.test", body.InlineImages)
					}
				case testmail.ScenarioUnsubscribeHeaders:
					switch msg.Key {
					case "one-click":
						if body.ListUnsubscribe != "<https://unsubscribe.herald.test/one-click>" {
							t.Fatalf("one-click List-Unsubscribe = %q", body.ListUnsubscribe)
						}
						if body.ListUnsubscribePost != "List-Unsubscribe=One-Click" {
							t.Fatalf("one-click List-Unsubscribe-Post = %q", body.ListUnsubscribePost)
						}
					case "mailto":
						if body.ListUnsubscribe != "<mailto:unsubscribe@herald.test?subject=unsubscribe>" {
							t.Fatalf("mailto List-Unsubscribe = %q", body.ListUnsubscribe)
						}
						if body.ListUnsubscribePost != "" {
							t.Fatalf("mailto List-Unsubscribe-Post = %q, want empty", body.ListUnsubscribePost)
						}
					case "no-header":
						if body.ListUnsubscribe != "" || body.ListUnsubscribePost != "" {
							t.Fatalf("no-header unsubscribe headers = %q / %q", body.ListUnsubscribe, body.ListUnsubscribePost)
						}
					}
				}
			}
		})
	}
}
