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

	var hasAttachment, hasUnsubscribe, hasHTML, hasThread, hasCrossParticipantThread, hasDemoAccountReplyThread bool
	subjects := make(map[string]int)
	threadParticipants := make(map[string]map[string]bool)
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
		if threadParticipants[normalized] == nil {
			threadParticipants[normalized] = make(map[string]bool)
		}
		threadParticipants[normalized][strings.ToLower(msg.Email.Sender)] = true
		if subjects[normalized] > 1 {
			hasThread = true
		}
		if len(threadParticipants[normalized]) > 1 {
			hasCrossParticipantThread = true
			for participant := range threadParticipants[normalized] {
				if strings.Contains(participant, "demo@demo.local") {
					hasDemoAccountReplyThread = true
				}
			}
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
	if !hasCrossParticipantThread {
		t.Fatal("expected at least one demo thread with multiple participants")
	}
	if !hasDemoAccountReplyThread {
		t.Fatal("expected at least one demo thread involving demo@demo.local")
	}
}

func TestMailboxOmitsPrivateDemoIdentityTerms(t *testing.T) {
	mailbox := Mailbox()
	forbidden := []string{"anthropic", "anton", "golubtsov", "tatiana", "tytiana"}

	assertClean := func(label, value string) {
		t.Helper()
		lower := strings.ToLower(value)
		for _, term := range forbidden {
			if strings.Contains(lower, term) {
				t.Fatalf("%s contains forbidden demo identity term %q: %q", label, term, value)
			}
		}
	}

	for _, msg := range mailbox.Messages {
		assertClean(msg.Email.MessageID+" sender", msg.Email.Sender)
		assertClean(msg.Email.MessageID+" subject", msg.Email.Subject)
		assertClean(msg.Email.MessageID+" message id", msg.Email.MessageID)
		assertClean(msg.Email.MessageID+" body", msg.Body.TextPlain)
		assertClean(msg.Email.MessageID+" html body", msg.Body.TextHTML)
		assertClean(msg.Email.MessageID+" from", msg.Body.From)
		assertClean(msg.Email.MessageID+" to", msg.Body.To)
		assertClean(msg.Email.MessageID+" cc", msg.Body.CC)
		assertClean(msg.Email.MessageID+" bcc", msg.Body.BCC)
		assertClean(msg.Email.MessageID+" body subject", msg.Body.Subject)
		for _, topic := range msg.Topics {
			assertClean(msg.Email.MessageID+" topic", topic)
		}
	}
	for _, contact := range mailbox.Contacts {
		assertClean(contact.Email+" email", contact.Email)
		assertClean(contact.Email+" display name", contact.DisplayName)
		assertClean(contact.Email+" company", contact.Company)
		for _, topic := range contact.Topics {
			assertClean(contact.Email+" topic", topic)
		}
	}
}

func TestMailboxIncludesLinkRenderingStressFixture(t *testing.T) {
	var found bool
	for _, msg := range Mailbox().Messages {
		if msg.Email.Subject != "Link rendering stress preview" {
			continue
		}
		found = true
		if !msg.Body.IsFromHTML {
			t.Fatal("link stress fixture should exercise HTML-derived markdown rendering")
		}
		if !strings.Contains(msg.Body.TextPlain, "[Display in your browser](") {
			t.Fatalf("link stress fixture should include an anchor-text markdown link, got:\n%s", msg.Body.TextPlain)
		}
		if !strings.Contains(msg.Body.TextPlain, "![Taskpad logo](") {
			t.Fatalf("link stress fixture should include an image markdown link, got:\n%s", msg.Body.TextPlain)
		}
		if !strings.Contains(msg.Body.TextPlain, "abcdefghijklmnopqrstuvwxyz0123456789") {
			t.Fatalf("link stress fixture should include a long naked URL, got:\n%s", msg.Body.TextPlain)
		}
	}
	if !found {
		t.Fatal("expected demo mailbox to include Link rendering stress preview fixture")
	}
}

func TestMailboxIncludesCreativeCommonsImageSampler(t *testing.T) {
	const subject = "Creative Commons image sampler for terminal previews"

	var found bool
	for _, msg := range Mailbox().Messages {
		if msg.Email.Subject != subject {
			continue
		}
		found = true
		if msg.Email.Sender != "Open Commons Gallery <images@opencommons.example>" {
			t.Fatalf("unexpected sampler sender: %q", msg.Email.Sender)
		}
		if msg.Email.Folder != "INBOX" {
			t.Fatalf("sampler folder = %q, want INBOX", msg.Email.Folder)
		}
		if len(msg.Body.InlineImages) != 4 {
			t.Fatalf("sampler inline image count = %d, want 4", len(msg.Body.InlineImages))
		}
		wantMIMEs := []string{"image/png", "image/png", "image/jpeg", "image/jpeg"}
		for i, want := range wantMIMEs {
			img := msg.Body.InlineImages[i]
			if img.MIMEType != want {
				t.Fatalf("image %d MIME type = %q, want %q", i+1, img.MIMEType, want)
			}
			if img.ContentID == "" {
				t.Fatalf("image %d has empty content ID", i+1)
			}
			if len(img.Data) == 0 {
				t.Fatalf("image %d has empty embedded data", i+1)
			}
		}
		body := strings.ToLower(msg.Body.TextPlain)
		for _, want := range []string{"creative commons", "cc0", "cc by 4.0", "46x21", "330px", "960px", "![remote commons thumbnail]("} {
			if !strings.Contains(body, strings.ToLower(want)) {
				t.Fatalf("sampler body missing %q:\n%s", want, msg.Body.TextPlain)
			}
		}
	}
	if !found {
		t.Fatalf("expected demo mailbox to include %q fixture", subject)
	}
}

func TestMailboxIncludesRichHTMLRenderingShowcase(t *testing.T) {
	const subject = "Rich HTML rendering showcase"

	var found bool
	for _, msg := range Mailbox().Messages {
		if msg.Email.Subject != subject {
			continue
		}
		found = true
		if !msg.Body.IsFromHTML {
			t.Fatal("rich HTML showcase should exercise HTML-derived preview rendering")
		}
		if msg.Body.TextHTML == "" {
			t.Fatal("rich HTML showcase should include original HTML")
		}
		for _, want := range []string{"<h1", "<strong>", "<em>", "<ul>", "<a href=", "<img"} {
			if !strings.Contains(strings.ToLower(msg.Body.TextHTML), strings.ToLower(want)) {
				t.Fatalf("rich HTML showcase HTML missing %q:\n%s", want, msg.Body.TextHTML)
			}
		}
		for _, want := range []string{"# HTML preview quality", "- Headings survive", "[Open dashboard](", "![Remote status chart]("} {
			if !strings.Contains(msg.Body.TextPlain, want) {
				t.Fatalf("rich HTML showcase markdown body missing %q:\n%s", want, msg.Body.TextPlain)
			}
		}
	}
	if !found {
		t.Fatalf("expected demo mailbox to include %q fixture", subject)
	}
}

func TestCreativeCommonsSamplerIncludesHTMLCIDPlacement(t *testing.T) {
	box := Mailbox()
	var found *Message
	for i := range box.Messages {
		if box.Messages[i].Email.Subject == "Creative Commons image sampler for terminal previews" {
			found = &box.Messages[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected sampler fixture")
	}
	if !found.Body.IsFromHTML {
		t.Fatal("sampler should exercise HTML-derived preview behavior")
	}
	for _, cid := range []string{"cc-by-sa-badge", "color-chart-330px", "bee-on-sunflower-330px", "changing-landscape-960px"} {
		if !strings.Contains(found.Body.TextHTML, "cid:"+cid) {
			t.Fatalf("sampler HTML missing cid reference %q:\n%s", cid, found.Body.TextHTML)
		}
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
