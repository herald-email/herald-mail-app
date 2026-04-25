package backend

import (
	"strings"
	"testing"

	"mail-processor/internal/demo"
)

func TestDemoBackendFetchesRichFixtureBodies(t *testing.T) {
	b := NewDemoBackend()
	emails, err := b.GetTimelineEmails("INBOX")
	if err != nil {
		t.Fatalf("GetTimelineEmails: %v", err)
	}

	var targetUID uint32
	for _, email := range emails {
		if email.HasAttachments {
			targetUID = email.UID
			break
		}
	}
	if targetUID == 0 {
		t.Fatal("expected a demo email with an attachment")
	}

	body, err := b.FetchEmailBody("INBOX", targetUID)
	if err != nil {
		t.Fatalf("FetchEmailBody: %v", err)
	}
	if body == nil {
		t.Fatal("expected body")
	}
	if strings.Contains(strings.ToLower(body.TextPlain), "lorem ipsum") {
		t.Fatalf("expected fixture body, got placeholder text: %q", body.TextPlain)
	}
	if len(body.Attachments) == 0 {
		t.Fatal("expected attachment metadata in fetched body")
	}
}

func TestDemoBackendSemanticSearchRanksInfrastructureResults(t *testing.T) {
	b := NewDemoBackend()
	vec, err := demo.NewAI().Embed("search_query: infrastructure budget risk")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	results, err := b.SearchSemanticChunked("INBOX", vec, 5, 0.25)
	if err != nil {
		t.Fatalf("SearchSemanticChunked: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected semantic results")
	}
	top := strings.ToLower(results[0].Email.Subject + " " + results[0].Email.Sender)
	if !strings.Contains(top, "northstar") && !strings.Contains(top, "infrastructure") && !strings.Contains(top, "budget") {
		t.Fatalf("expected infrastructure/budget result first, got %q", top)
	}
}

func TestDemoBackendContactDetailsUseSameFixtures(t *testing.T) {
	b := NewDemoBackend()
	contacts, err := b.ListContacts(20, "last_seen")
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(contacts) == 0 {
		t.Fatal("expected demo contacts")
	}

	emails, err := b.GetContactEmails(contacts[0].Email, 10)
	if err != nil {
		t.Fatalf("GetContactEmails: %v", err)
	}
	if len(emails) == 0 {
		t.Fatalf("expected recent emails for contact %s", contacts[0].Email)
	}
}
