package testmail

import (
	"path/filepath"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/smtp"
)

func TestLabRoutesSMTPDeliveryBetweenDefaultAccounts(t *testing.T) {
	lab := Start(t)
	alice := lab.Account("alice@herald.test")
	bob := lab.Account("bob@herald.test")
	if alice == nil || bob == nil {
		t.Fatalf("default lab accounts missing: alice=%v bob=%v", alice, bob)
	}

	cfg := alice.Config(filepath.Join(t.TempDir(), "alice-cache.db"))
	if err := smtp.New(cfg).Send(alice.Address, bob.Address, "Virtual hello", "Hello from Alice.", ""); err != nil {
		t.Fatalf("send through virtual SMTP: %v", err)
	}

	inboxRef := lab.WaitForSubject(bob.Address, "INBOX", "Virtual hello")
	if inboxRef.Account != bob.Address || inboxRef.Folder != "INBOX" || inboxRef.UID == 0 {
		t.Fatalf("unexpected Bob inbox ref: %+v", inboxRef)
	}
	sentRef := lab.WaitForSubject(alice.Address, "Sent", "Virtual hello")
	if sentRef.Account != alice.Address || sentRef.Folder != "Sent" || sentRef.UID == 0 {
		t.Fatalf("unexpected Alice sent ref: %+v", sentRef)
	}

	captured := lab.CapturedSMTP()
	if len(captured) != 1 {
		t.Fatalf("captured SMTP count = %d, want 1", len(captured))
	}
	if captured[0].From != alice.Address {
		t.Fatalf("captured from = %q, want %q", captured[0].From, alice.Address)
	}
	if len(captured[0].To) != 1 || captured[0].To[0] != bob.Address {
		t.Fatalf("captured recipients = %#v, want [%q]", captured[0].To, bob.Address)
	}
}
