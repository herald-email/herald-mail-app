package demo

import (
	"net/mail"
	"sort"
	"strings"
	"testing"
	"time"
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
	forbidden := []string{
		"anthropic",
		"anton",
		"golubtsov",
		"logrusadm",
		"gmail.com",
		"c674584b-ef13-4a23-8c15-8a7a9f0773f7",
		"68fa7079-9012-4ea6-a2db-bb7073165084",
		"55e58bef-7e82-4786-ab34-dbf28017a01f",
		"tatiana",
		"tytiana",
	}

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

func TestMailboxPlacesV050PreviewAfterOnboarding(t *testing.T) {
	messages := append([]Message(nil), Mailbox().Messages...)
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].Email.Date.After(messages[j].Email.Date)
	})

	step9 := -1
	for i, msg := range messages {
		if msg.Email.Subject == "Step 9: Explore contacts, chat, SSH, and MCP" {
			step9 = i
			break
		}
	}
	if step9 < 0 {
		t.Fatal("expected Step 9 onboarding message")
	}
	if step9+1 >= len(messages) {
		t.Fatal("expected a message after Step 9")
	}

	got := messages[step9+1]
	if got.Email.Subject != "[PREVIEW] Herald v0.5.0 — Calendar, and multi-account arrive" {
		t.Fatalf("message after Step 9 subject = %q, want v0.5 preview newsletter", got.Email.Subject)
	}
	if got.Email.Sender != "Herald Mail App <newsletter@herald.demo>" {
		t.Fatalf("preview sender = %q, want sanitized Herald demo sender", got.Email.Sender)
	}
	if got.Body.To != "Rowan Finch <demo@demo.local>" {
		t.Fatalf("preview To = %q, want demo recipient", got.Body.To)
	}
	if got.Body.ListUnsubscribe != "<https://herald.demo/unsubscribe/v050-preview>" {
		t.Fatalf("preview List-Unsubscribe = %q, want sanitized demo unsubscribe URL", got.Body.ListUnsubscribe)
	}
	if !messages[step9].Email.Date.After(got.Email.Date) {
		t.Fatalf("preview date %s should sort after Step 9 date %s", got.Email.Date, messages[step9].Email.Date)
	}
	if !strings.Contains(got.Body.TextPlain, "Calendar (preview)") || !strings.Contains(got.Body.TextPlain, "multi-account") {
		t.Fatalf("preview body does not look like the v0.5 newsletter: %q", got.Body.TextPlain)
	}
}

func TestMailboxIncludesOrderedHeraldOnboardingSteps(t *testing.T) {
	messages := append([]Message(nil), Mailbox().Messages...)
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].Email.Date.After(messages[j].Email.Date)
	})

	want := []struct {
		subject   string
		sender    string
		messageID string
	}{
		{subject: "✉ Welcome to Herald", sender: "Herald Welcome <welcome@herald.demo>", messageID: "demo-welcome-to-herald@demo.local"},
		{subject: "Step 1: Move around your inbox", sender: "Herald Guide <guide@herald.demo>"},
		{subject: "Step 2: Reply, write, and preview Markdown", sender: "Herald Compose Coach <compose@herald.demo>"},
		{subject: "Step 3: Open and save attachments", sender: "Herald Attachments <attachments@herald.demo>"},
		{subject: "Step 4: Select text from an email", sender: "Herald Selection Coach <selection@herald.demo>"},
		{subject: "Step 5: View inline images in full screen", sender: "Herald Image Lab <images@herald.demo>"},
		{subject: "Step 6: Clean up senders and domains safely", sender: "Herald Cleanup Coach <cleanup@herald.demo>"},
		{subject: "Step 7: Classify mail and dry-run rules", sender: "Herald AI Rules <rules@herald.demo>"},
		{subject: "Step 8: Configure accounts, AI, and signatures", sender: "Herald Settings <settings@herald.demo>"},
		{subject: "Step 9: Explore contacts, chat, SSH, and MCP", sender: "Herald Next Steps <next@herald.demo>"},
	}

	if len(messages) < len(want) {
		t.Fatalf("expected at least %d demo messages, got %d", len(want), len(messages))
	}
	for i, w := range want {
		got := messages[i].Email
		if got.Subject != w.subject {
			t.Fatalf("message %d subject = %q, want %q", i+1, got.Subject, w.subject)
		}
		if got.Sender != w.sender {
			t.Fatalf("message %d sender = %q, want %q", i+1, got.Sender, w.sender)
		}
		if got.Folder != "INBOX" {
			t.Fatalf("message %d folder = %q, want INBOX", i+1, got.Folder)
		}
		if w.messageID != "" && got.MessageID != w.messageID {
			t.Fatalf("message %d message ID = %q, want %q", i+1, got.MessageID, w.messageID)
		}
	}
	for i := 1; i < len(want); i++ {
		if !messages[i-1].Email.Date.After(messages[i].Email.Date) {
			t.Fatalf("onboarding dates are not strictly descending at messages %d and %d: %s then %s", i, i+1, messages[i-1].Email.Date, messages[i].Email.Date)
		}
	}
}

func TestSupportingDemoMessagesAreExamplesAndNotTooMany(t *testing.T) {
	mailbox := Mailbox()
	if len(mailbox.Messages) > 28 {
		t.Fatalf("expected a focused demo mailbox with at most 28 messages, got %d", len(mailbox.Messages))
	}

	for _, msg := range mailbox.Messages {
		subject := msg.Email.Subject
		if subject == "✉ Welcome to Herald" || strings.HasPrefix(subject, "Step ") {
			continue
		}
		if subject == "[PREVIEW] Herald v0.5.0 — Calendar, and multi-account arrive" {
			continue
		}
		normalized := strings.TrimPrefix(strings.TrimPrefix(subject, "Re: "), "Fwd: ")
		if !strings.HasPrefix(normalized, "Example: ") {
			t.Fatalf("supporting demo subject %q should start with Example:", subject)
		}
	}
}

func TestMailboxIncludesCobaltWorksMultiRecipientHeaders(t *testing.T) {
	var foundMultiTo, foundCC bool
	for _, msg := range Mailbox().Messages {
		normalized := strings.TrimPrefix(strings.ToLower(msg.Email.Subject), "re: ")
		if normalized != "example: thread with cobalt works" {
			continue
		}
		if addrs := mustParseDemoAddresses(t, msg.Body.To); len(addrs) > 1 {
			foundMultiTo = true
		}
		if addrs := mustParseDemoAddresses(t, msg.Body.CC); len(addrs) > 0 {
			foundCC = true
		}
	}
	if !foundMultiTo {
		t.Fatal("expected Cobalt Works demo thread to include a message with multiple To recipients")
	}
	if !foundCC {
		t.Fatal("expected Cobalt Works demo thread to include a visible Cc recipient")
	}
}

func mustParseDemoAddresses(t *testing.T, value string) []*mail.Address {
	t.Helper()
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(value)
	if err != nil {
		t.Fatalf("invalid demo address list %q: %v", value, err)
	}
	return addrs
}

func TestCalendarEventsAreSeededAroundCurrentDate(t *testing.T) {
	events := CalendarEvents()
	if len(events) == 0 {
		t.Fatal("expected demo calendar events")
	}

	now := time.Now()
	start := calendarFixtureDayStart(now)
	end := start.AddDate(0, 0, 5)
	var inWindow int
	for _, event := range events {
		if event.Start.IsZero() {
			t.Fatalf("event %q has zero start", event.Title)
		}
		if !event.Start.Before(start) && event.Start.Before(end) {
			inWindow++
		}
	}
	if inWindow < 5 {
		t.Fatalf("events in current 5-day demo window = %d, want at least 5", inWindow)
	}
}

func TestMailboxOnboardingBodiesTeachCoreFeatures(t *testing.T) {
	cases := []struct {
		subject        string
		wants          []string
		attachments    int
		inlineImages   int
		htmlCIDSnippet string
	}{
		{
			subject: "✉ Welcome to Herald",
			wants:   []string{"terminal email client", "inbox cleanup", "ai", "demo mode", "synthetic", "timeline", "compose", "cleanup", "contacts"},
		},
		{
			subject: "Step 1: Move around your inbox",
			wants:   []string{"try now", "j/k", "up/down arrows", "h/l", "left/right arrows", "tab", "shift+tab", "folders", "timeline", "preview", "enter", "right arrow", "open an email preview", "mouse wheel", "timeline rows", "tab labels", "esc", "1/2/3", "?", "what herald is doing"},
		},
		{
			subject: "Step 2: Reply, write, and preview Markdown",
			wants:   []string{"try now", "r", "ctrl+p", "ctrl+s", "preserve original formatting", "rendered html", "plain-text"},
		},
		{
			subject:     "Step 3: Open and save attachments",
			wants:       []string{"try now", "[", "]", "s", "save to", "selected attachment"},
			attachments: 2,
		},
		{
			subject: "Step 4: Select text from an email",
			wants:   []string{"try now", "text selection", "mouse capture", "m", "release mouse", "restore mouse", "full-screen preview", "z", "terminal-native selection"},
		},
		{
			subject:        "Step 5: View inline images in full screen",
			wants:          []string{"creative commons", "z", "kitty", "iterm2", "remote images", "not fetched", "46x21", "330px", "960px", "![remote commons thumbnail]("},
			inlineImages:   4,
			htmlCIDSnippet: "cid:cc-by-sa-badge",
		},
		{
			subject: "Step 6: Clean up senders and domains safely",
			wants:   []string{"try now", "3", "space", "sender", "domain", "unsubscribe", "preview"},
		},
		{
			subject: "Step 7: Classify mail and dry-run rules",
			wants:   []string{"try now", "a", "? infrastructure budget risk", "cleanup rules", "saved filters", "automation rules", "scheduled or repeated actions", "custom prompts", "reusable ai instructions", "dry-run", "matched messages", "planned actions"},
		},
		{
			subject: "Step 8: Configure accounts, AI, and signatures",
			wants:   []string{"try now", "s", "settings", "provider", "embedding model", "signature"},
		},
		{
			subject: "Step 9: Explore contacts, chat, SSH, and MCP",
			wants:   []string{"try now", "contacts", "chat panel", "quick replies", "herald mcp --demo", "herald ssh"},
		},
	}

	for _, tc := range cases {
		msg := demoMessageBySubject(t, tc.subject)
		body := strings.ToLower(msg.Body.TextPlain)
		for _, want := range tc.wants {
			if !strings.Contains(body, strings.ToLower(want)) {
				t.Fatalf("%s body missing %q:\n%s", tc.subject, want, msg.Body.TextPlain)
			}
		}
		if tc.attachments > 0 && len(msg.Body.Attachments) != tc.attachments {
			t.Fatalf("%s attachment count = %d, want %d", tc.subject, len(msg.Body.Attachments), tc.attachments)
		}
		if tc.inlineImages > 0 && len(msg.Body.InlineImages) != tc.inlineImages {
			t.Fatalf("%s inline image count = %d, want %d", tc.subject, len(msg.Body.InlineImages), tc.inlineImages)
		}
		if tc.htmlCIDSnippet != "" && !strings.Contains(msg.Body.TextHTML, tc.htmlCIDSnippet) {
			t.Fatalf("%s HTML missing %q:\n%s", tc.subject, tc.htmlCIDSnippet, msg.Body.TextHTML)
		}
	}
}

func demoMessageBySubject(t *testing.T, subject string) Message {
	t.Helper()
	for _, msg := range Mailbox().Messages {
		if msg.Email.Subject == subject {
			return msg
		}
	}
	t.Fatalf("expected demo mailbox to include %q", subject)
	return Message{}
}

func TestMailboxIncludesLinkRenderingStressFixture(t *testing.T) {
	var found bool
	for _, msg := range Mailbox().Messages {
		if msg.Email.Subject != "Example: Link rendering stress preview" {
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
		t.Fatal("expected demo mailbox to include Example: Link rendering stress preview fixture")
	}
}

func TestMailboxIncludesCreativeCommonsImageSampler(t *testing.T) {
	const subject = "Step 5: View inline images in full screen"

	var found bool
	for _, msg := range Mailbox().Messages {
		if msg.Email.Subject != subject {
			continue
		}
		found = true
		if msg.Email.Sender != "Herald Image Lab <images@herald.demo>" {
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
	const subject = "Example: Rich HTML rendering showcase"

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
		if box.Messages[i].Email.Subject == "Step 5: View inline images in full screen" {
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

	cat, err := ai.Classify("Northstar Cloud <billing@northstar-cloud.example>", "Example: Project Orion usage alert")
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
