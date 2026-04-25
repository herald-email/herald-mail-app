package demo

import (
	"strings"
	"testing"
)

func TestMailboxCoversPublicDemoStories(t *testing.T) {
	mailbox := Mailbox()
	if len(mailbox.Messages) < 24 {
		t.Fatalf("expected at least 24 demo messages, got %d", len(mailbox.Messages))
	}
	if len(mailbox.Contacts) < 6 {
		t.Fatalf("expected at least 6 demo contacts, got %d", len(mailbox.Contacts))
	}

	var hasAttachment, hasUnsubscribe, hasHTML, hasThread bool
	subjects := make(map[string]int)
	for _, msg := range mailbox.Messages {
		lowerSender := strings.ToLower(msg.Email.Sender)
		for _, brand := range []string{"aws", "github", "airbnb", "shopify", "twitter"} {
			if strings.Contains(lowerSender, brand) {
				t.Fatalf("demo sender %q should be fictional, found brand %q", msg.Email.Sender, brand)
			}
		}
		if msg.Category == "" {
			t.Fatalf("message %s has no expected category", msg.Email.MessageID)
		}
		if strings.Contains(strings.ToLower(msg.Body.TextPlain), "lorem ipsum") {
			t.Fatalf("message %s still uses placeholder body text", msg.Email.MessageID)
		}
		if msg.Email.HasAttachments || len(msg.Body.Attachments) > 0 {
			hasAttachment = true
		}
		if msg.Body.ListUnsubscribe != "" {
			hasUnsubscribe = true
		}
		if msg.Body.IsFromHTML {
			hasHTML = true
		}
		normalized := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(msg.Email.Subject), "re: "), "fwd: ")
		subjects[normalized]++
		if subjects[normalized] > 1 {
			hasThread = true
		}
	}

	if !hasAttachment {
		t.Fatal("expected at least one demo message with an attachment")
	}
	if !hasUnsubscribe {
		t.Fatal("expected at least one demo message with unsubscribe headers")
	}
	if !hasHTML {
		t.Fatal("expected at least one demo message rendered from HTML")
	}
	if !hasThread {
		t.Fatal("expected at least one visible thread in demo messages")
	}
}

func TestDemoAIIsDeterministicAndOffline(t *testing.T) {
	ai := NewAI()

	cat, err := ai.Classify("Northstar Cloud <billing@northstar-cloud.example>", "Invoice and usage alert for Project Orion")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if cat != "imp" {
		t.Fatalf("expected Northstar Cloud to classify as imp, got %q", cat)
	}

	vec1, err := ai.Embed("search_query: infrastructure budget risk")
	if err != nil {
		t.Fatalf("Embed first call: %v", err)
	}
	vec2, err := ai.Embed("search_query: infrastructure budget risk")
	if err != nil {
		t.Fatalf("Embed second call: %v", err)
	}
	if len(vec1) == 0 || len(vec1) != len(vec2) {
		t.Fatalf("expected stable non-empty vectors, got %d and %d", len(vec1), len(vec2))
	}
	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Fatalf("embedding is not deterministic at index %d: %f != %f", i, vec1[i], vec2[i])
		}
	}

	replies, err := ai.GenerateQuickReplies("Mara Vale <mara@forgepoint.example>", "Can you review the migration?", "Please review the migration plan.")
	if err != nil {
		t.Fatalf("GenerateQuickReplies: %v", err)
	}
	if len(replies) != 3 {
		t.Fatalf("expected exactly 3 deterministic quick replies, got %d", len(replies))
	}
}
