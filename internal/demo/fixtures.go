package demo

import (
	"encoding/base64"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// Message is one complete demo email fixture: metadata, body, expected AI tag,
// and topic words used by deterministic semantic search.
type Message struct {
	Email    models.EmailData
	Body     models.EmailBody
	Category ai.Category
	Topics   []string
}

// MailboxFixture is the shared fictional mailbox used by TUI and MCP demos.
type MailboxFixture struct {
	Messages []Message
	Contacts []models.ContactData
}

var baseTime = time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC)

// Mailbox returns a deep copy of the deterministic demo mailbox.
func Mailbox() MailboxFixture {
	fixture := buildMailbox()
	out := MailboxFixture{
		Messages: make([]Message, len(fixture.Messages)),
		Contacts: make([]models.ContactData, len(fixture.Contacts)),
	}
	for i, msg := range fixture.Messages {
		out.Messages[i] = cloneMessage(msg)
	}
	for i, contact := range fixture.Contacts {
		out.Contacts[i] = cloneContact(contact)
	}
	return out
}

// Emails returns demo email metadata as pointers for backend tables.
func Emails() []*models.EmailData {
	mailbox := Mailbox()
	emails := make([]*models.EmailData, 0, len(mailbox.Messages))
	for i := range mailbox.Messages {
		email := mailbox.Messages[i].Email
		emails = append(emails, &email)
	}
	return emails
}

// Contacts returns deterministic contact fixtures.
func Contacts() []models.ContactData {
	return Mailbox().Contacts
}

// BodyByUID returns a copied body fixture for the given UID.
func BodyByUID(uid uint32) (*models.EmailBody, bool) {
	for _, msg := range Mailbox().Messages {
		if msg.Email.UID == uid {
			body := cloneBody(msg.Body)
			return &body, true
		}
	}
	return nil, false
}

// BodyByMessageID returns a copied body fixture for the given message ID.
func BodyByMessageID(messageID string) (*models.EmailBody, bool) {
	for _, msg := range Mailbox().Messages {
		if msg.Email.MessageID == messageID {
			body := cloneBody(msg.Body)
			return &body, true
		}
	}
	return nil, false
}

// CategoryFor returns the expected demo category for a sender/subject pair.
func CategoryFor(sender, subject string) ai.Category {
	sender = strings.ToLower(strings.TrimSpace(sender))
	subject = strings.ToLower(strings.TrimSpace(subject))
	for _, msg := range Mailbox().Messages {
		if strings.ToLower(msg.Email.Sender) == sender && strings.ToLower(msg.Email.Subject) == subject {
			return msg.Category
		}
	}
	switch {
	case strings.Contains(subject, "receipt") || strings.Contains(subject, "invoice paid") || strings.Contains(subject, "statement"):
		return ai.CategoryTransactional
	case strings.Contains(subject, "deal") || strings.Contains(subject, "gift card") || strings.Contains(subject, "fare"):
		return ai.CategorySubscription
	case strings.Contains(subject, "digest") || strings.Contains(subject, "guide"):
		return ai.CategoryNewsletter
	case strings.Contains(subject, "mention"):
		return ai.CategorySocial
	case strings.Contains(subject, "security") || strings.Contains(subject, "appointment") || strings.Contains(subject, "budget"):
		return ai.CategoryImportant
	default:
		return ai.CategoryUnknown
	}
}

// Classifications returns expected category tags for every fixture message.
func Classifications() map[string]string {
	out := make(map[string]string)
	for _, msg := range Mailbox().Messages {
		out[msg.Email.MessageID] = string(msg.Category)
	}
	return out
}

// RecentSubjectsByContact returns newest subjects for a contact email.
func RecentSubjectsByContact(email string, limit int) []string {
	email = strings.ToLower(strings.TrimSpace(email))
	var subjects []string
	for _, msg := range Mailbox().Messages {
		if strings.Contains(strings.ToLower(msg.Email.Sender), email) {
			subjects = append(subjects, msg.Email.Subject)
		}
	}
	if limit > 0 && len(subjects) > limit {
		return subjects[:limit]
	}
	return subjects
}

// VectorForText returns a deterministic topic vector for semantic demos.
func VectorForText(text string) []float32 {
	text = strings.ToLower(text)
	dims := []struct {
		words []string
	}{
		{[]string{"infrastructure", "cluster", "compute", "storage", "cloud", "migration", "release"}},
		{[]string{"budget", "invoice", "statement", "tuition", "paid", "books", "forecast", "cost"}},
		{[]string{"risk", "security", "device", "notice", "failed", "interruption"}},
		{[]string{"travel", "fare", "cabin", "mountain", "coast", "ticket"}},
		{[]string{"newsletter", "digest", "guide", "issue", "systems", "containers"}},
		{[]string{"shopping", "order", "receipt", "delivery", "gift", "keyboard"}},
		{[]string{"health", "clinic", "appointment", "lab", "portal"}},
		{[]string{"finance", "ledger", "bank", "books", "retainer"}},
		{[]string{"code", "review", "build", "policy", "candidate"}},
		{[]string{"social", "mention", "roundup", "network"}},
	}
	vec := make([]float32, len(dims))
	for i, dim := range dims {
		for _, word := range dim.words {
			if strings.Contains(text, word) {
				vec[i] += 1
			}
		}
	}
	return vec
}

// SemanticResults ranks the provided emails against a deterministic query vector.
func SemanticResults(emails []*models.EmailData, queryVec []float32, limit int, minScore float64) []*models.SemanticSearchResult {
	vectorByID := make(map[string][]float32)
	for _, msg := range Mailbox().Messages {
		doc := msg.Email.Sender + " " + msg.Email.Subject + " " + msg.Body.TextPlain + " " + strings.Join(msg.Topics, " ")
		vectorByID[msg.Email.MessageID] = VectorForText(doc)
	}

	var results []*models.SemanticSearchResult
	for _, email := range emails {
		if email == nil {
			continue
		}
		score := cosine(queryVec, vectorByID[email.MessageID])
		if score < minScore {
			continue
		}
		results = append(results, &models.SemanticSearchResult{Email: email, Score: score})
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// ContactSemanticResults ranks contacts with the same deterministic vectors.
func ContactSemanticResults(queryVec []float32, limit int, minScore float64) []*models.ContactSearchResult {
	var results []*models.ContactSearchResult
	for _, contact := range Contacts() {
		doc := contact.DisplayName + " " + contact.Email + " " + contact.Company + " " + strings.Join(contact.Topics, " ")
		score := cosine(queryVec, VectorForText(doc))
		if score < minScore {
			continue
		}
		c := contact
		results = append(results, &models.ContactSearchResult{Contact: c, Score: score})
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, aa, bb float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		aa += av * av
		bb += bv * bv
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}

func buildMailbox() MailboxFixture {
	var messages []Message
	add := func(uid uint32, sender, subject, folder string, daysAgo int, size int, read, starred bool, category ai.Category, topics []string, body string, opts ...func(*Message)) {
		msg := Message{
			Email: models.EmailData{
				MessageID:      "demo-" + strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(subject, " ", "-"), ":", "")) + "@demo.local",
				UID:            uid,
				Sender:         sender,
				Subject:        subject,
				Date:           baseTime.AddDate(0, 0, -daysAgo),
				Size:           size,
				Folder:         folder,
				LastUpdated:    baseTime,
				IsRead:         read,
				IsStarred:      starred,
				HasAttachments: false,
			},
			Body: models.EmailBody{
				TextPlain: body,
			},
			Category: category,
			Topics:   topics,
		}
		for _, opt := range opts {
			opt(&msg)
		}
		messages = append(messages, msg)
	}

	withHTML := func(msg *Message) {
		msg.Body.IsFromHTML = true
	}
	withHTMLBody := func(html string) func(*Message) {
		return func(msg *Message) {
			msg.Body.IsFromHTML = true
			msg.Body.TextHTML = html
		}
	}
	withUnsub := func(url string) func(*Message) {
		return func(msg *Message) {
			msg.Body.ListUnsubscribe = "<" + url + ">"
			msg.Body.ListUnsubscribePost = "List-Unsubscribe=One-Click"
		}
	}
	withHeaders := func(from, to, cc, bcc string) func(*Message) {
		return func(msg *Message) {
			msg.Body.From = from
			msg.Body.To = to
			msg.Body.CC = cc
			msg.Body.BCC = bcc
			msg.Body.Subject = msg.Email.Subject
		}
	}
	withAttachment := func(filename, mime string, size int) func(*Message) {
		return func(msg *Message) {
			msg.Email.HasAttachments = true
			msg.Body.Attachments = append(msg.Body.Attachments, models.Attachment{
				Filename: filename,
				MIMEType: mime,
				Size:     size,
				PartPath: strconv.Itoa(2 + len(msg.Body.Attachments)),
				Data:     []byte("demo attachment: " + filename),
			})
		}
	}
	withInlineImage := func(contentID, mime string, data []byte) func(*Message) {
		return func(msg *Message) {
			msg.Body.InlineImages = append(msg.Body.InlineImages, models.InlineImage{
				ContentID: contentID,
				MIMEType:  mime,
				Data:      append([]byte(nil), data...),
			})
		}
	}
	withDate := func(date time.Time) func(*Message) {
		return func(msg *Message) {
			msg.Email.Date = date
		}
	}
	withMessageID := func(messageID string) func(*Message) {
		return func(msg *Message) {
			msg.Email.MessageID = messageID
		}
	}
	withDraft := func(to, cc, bcc string) func(*Message) {
		return func(msg *Message) {
			msg.Email.IsDraft = true
			msg.Email.MessageID = "demo-draft-" + msg.Email.MessageID
			msg.Body.From = msg.Email.Sender
			msg.Body.To = to
			msg.Body.CC = cc
			msg.Body.BCC = bcc
			msg.Body.Subject = msg.Email.Subject
		}
	}

	add(39, "Herald Welcome <welcome@herald.demo>", "✉ Welcome to Herald", "INBOX", 0, 10240, false, true, ai.CategoryImportant, []string{"onboarding", "welcome", "terminal email client", "inbox cleanup", "ai"},
		`Welcome to Herald.

Herald is a terminal email client for people who want fast keyboard navigation, inbox cleanup, rich previews, and AI-assisted triage without leaving the command line.

What you can try in demo mode
- Timeline shows a realistic mailbox and lets you read, search, reply, and manage threads.
- Compose can write Markdown, preview rendered HTML, and preserve original message formatting for replies and forwards.
- Cleanup groups repeated mail by sender or domain so bulk actions stay deliberate.
- Contacts, chat, rules, MCP, and SSH surfaces all use the same synthetic demo mailbox.

Demo mode is offline and deterministic. These messages are synthetic, attachments are safe fixtures, and no IMAP or SMTP account is touched.`,
		withMessageID("demo-welcome-to-herald@demo.local"),
		withDate(baseTime.Add(10*time.Hour)))
	add(31, "Herald Guide <guide@herald.demo>", "Step 1: Move around your inbox", "INBOX", 0, 11264, false, true, ai.CategoryImportant, []string{"onboarding", "navigation", "timeline", "search"},
		`Step 1 is a quick lap around Herald's Timeline.

Try now
- Use j/k or the up/down arrows to move within the Timeline or folders.
- Use horizontal movement with h/l, left/right arrows, Tab, or Shift+Tab to switch between folders, Timeline, and preview.
- Open an email preview with Enter, right arrow, l, or Tab.
- Press Esc to close a preview.
- Press 1/2/3 to jump between Timeline, Compose, and Cleanup.
- Press f to open the folder sidebar.
- Press / to search.
- Press ? when you want the current shortcut map.
- Use the mouse wheel over the Timeline or preview to scroll.
- Click Timeline rows to select or open mail, and click tab labels to switch sections.

What Herald is doing
Herald keeps the Timeline keyboard-first, but horizontal movement makes the surrounding panes feel spatial: folders, Timeline, and preview are neighboring places. The same rows, tabs, and scrollable panes can also be clicked in terminals that support mouse events. Demo mode is offline, so every message you open here is synthetic and safe to explore.`,
		withDate(baseTime.Add(9*time.Hour)))
	add(32, "Herald Compose Coach <compose@herald.demo>", "Step 2: Reply, write, and preview Markdown", "INBOX", 0, 14336, false, true, ai.CategoryImportant, []string{"onboarding", "compose", "reply", "markdown", "html"},
		`Step 2 shows how Herald turns a terminal compose screen into a real email workflow.

Try now
- Highlight this message and press R to start a reply.
- Write a few Markdown lines in the body.
- Press ctrl+p to preview the rendered message.
- Press ctrl+s to send. In demo mode, sending is simulated and does not contact SMTP.

What Herald is doing
Replies and forwards preserve original formatting, inline images, and attachments where possible. New Markdown you write is rendered HTML for email clients that support rich mail, and Herald also keeps a plain-text alternative so the message stays readable everywhere.`,
		withDate(baseTime.Add(8*time.Hour)),
		withHTMLBody(`<html><body>
<h1>Step 2: Reply, write, and preview Markdown</h1>
<p>Use this message to practice replies, Markdown preview, and safe demo sending.</p>
<ul>
<li>Replies preserve original formatting where possible.</li>
<li>Markdown sends as rendered HTML with a plain-text alternative.</li>
</ul>
</body></html>`))
	add(33, "Herald Attachments <attachments@herald.demo>", "Step 3: Open and save attachments", "INBOX", 0, 28672, false, true, ai.CategoryTransactional, []string{"onboarding", "attachments", "download", "files"},
		`Step 3 gives you a safe attachment message to practice with.

Try now
- Open this preview and look for the attachment list below the message body.
- Use [ and ] to move between attachments.
- Press s to save the selected attachment.
- In the Save to prompt, choose a path such as /tmp/herald-demo-attachment.txt.

What Herald is doing
The subject row shows an attachment marker when a message has files. Save actions use the selected attachment, not just the first one, so multi-file messages can be handled deliberately.`,
		withDate(baseTime.Add(7*time.Hour)),
		withAttachment("herald-demo-checklist.txt", "text/plain", 2048),
		withAttachment("herald-demo-routing.csv", "text/csv", 4096))
	add(40, "Herald Selection Coach <selection@herald.demo>", "Step 4: Select text from an email", "INBOX", 0, 10240, false, true, ai.CategoryImportant, []string{"onboarding", "text selection", "mouse capture", "copy", "full-screen preview"},
		`Step 4 shows how to select and copy text from a message when Herald owns the mouse.

Try now
- Open this email and press z for full-screen preview.
- Press m to release mouse capture.
- Use terminal-native selection to drag across email text and copy it.
- Press m again to restore mouse capture when you want Herald clicks and scrolling back.
- Press Esc to leave full-screen preview.

What Herald is doing
By default Herald captures mouse input so the app can handle mouse wheel scrolling, Timeline row clicks, tab clicks, and preview clicks. That capture can lock normal terminal text selection. Releasing mouse capture hands the pointer back to your terminal for copying text; full-screen preview gives you the cleanest body-only surface to select from, then m restores Herald's mouse controls.`,
		withDate(baseTime.Add(6*time.Hour)))
	add(34, "Herald Image Lab <images@herald.demo>", "Step 5: View inline images in full screen", "INBOX", 0, 270336, true, true, ai.CategoryNewsletter, []string{"onboarding", "images", "creative commons", "rendering", "terminal"},
		`Step 5 is the image rendering tour.

Try now
- Open this message and press z for full-screen reading.
- In Kitty or Ghostty, try ./bin/herald --demo -image-protocol=kitty.
- In iTerm2, try ./bin/herald --demo -image-protocol=iterm2.
- Scroll through the image section and watch for safe fallback links or text when raster graphics are unavailable.

What Herald is doing
This email includes embedded inline images, so Herald can render the local MIME bytes without downloading anything. Remote images are shown as safe placeholders until you press o to reveal them for this message.

Embedded Creative Commons images:
- CC BY-SA badge: 46x21 PNG, CC0 1.0, by Heflox. Source: https://commons.wikimedia.org/wiki/File:CC-BY-SA.png
- Color chart: 330px PNG thumbnail, CC0 1.0, by Ccompagnon with a simplified revision by Iketsi. Source: https://commons.wikimedia.org/wiki/File:ColorChart.svg
- Bee on sunflower: 330px JPEG thumbnail, CC BY 4.0, by Mbrickn. Source: https://commons.wikimedia.org/wiki/File:Bee_on_Sunflower.jpg
- Changing Landscape: 960px JPEG thumbnail, CC BY 4.0, by Mit.d.sheth. Source: https://commons.wikimedia.org/wiki/File:Changing_Landscape.jpg

Remote image link, intentionally not fetched until you press o:
![Remote Commons thumbnail](https://assets.herald.demo/color-chart-330px.png)`,
		withDate(baseTime.Add(5*time.Hour)),
		withHTMLBody(`<html><body>
<h1>Step 5: View inline images in full screen</h1>
<p>Open this message and press <strong>z</strong> for full-screen reading.</p>
<p><img alt="CC BY-SA badge" src="cid:cc-by-sa-badge"></p>
<p><img alt="Color chart" src="cid:color-chart-330px"></p>
<p><img alt="Bee on sunflower" src="cid:bee-on-sunflower-330px"></p>
<p><img alt="Changing landscape" src="cid:changing-landscape-960px"></p>
<h2>Embedded Creative Commons images</h2>
<ul>
<li>CC BY-SA badge: 46x21 PNG, CC0 1.0, by Heflox. Source: <a href="https://commons.wikimedia.org/wiki/File:CC-BY-SA.png">CC-BY-SA.png</a></li>
<li>Color chart: 330px PNG thumbnail, CC0 1.0, by Ccompagnon with a simplified revision by Iketsi. Source: <a href="https://commons.wikimedia.org/wiki/File:ColorChart.svg">ColorChart.svg</a></li>
<li>Bee on sunflower: 330px JPEG thumbnail, CC BY 4.0, by Mbrickn. Source: <a href="https://commons.wikimedia.org/wiki/File:Bee_on_Sunflower.jpg">Bee on Sunflower</a></li>
<li>Changing Landscape: 960px JPEG thumbnail, CC BY 4.0, by Mit.d.sheth. Source: <a href="https://commons.wikimedia.org/wiki/File:Changing_Landscape.jpg">Changing Landscape</a></li>
</ul>
<p>Remote image link, intentionally not fetched until you press <strong>o</strong>:</p>
<p><img alt="Remote Commons thumbnail" src="https://assets.herald.demo/color-chart-330px.png"></p>
</body></html>`),
		withInlineImage("cc-by-sa-badge", "image/png", demoCCBySABadgePNG),
		withInlineImage("color-chart-330px", "image/png", demoColorChartPNG),
		withInlineImage("bee-on-sunflower-330px", "image/jpeg", demoBeeOnSunflowerJPG),
		withInlineImage("changing-landscape-960px", "image/jpeg", demoChangingLandscapeJPG))
	add(35, "Herald Cleanup Coach <cleanup@herald.demo>", "Step 6: Clean up senders and domains safely", "INBOX", 0, 12288, false, true, ai.CategoryNewsletter, []string{"onboarding", "cleanup", "sender", "domain", "unsubscribe"},
		`Step 6 points you at Herald's bulk cleanup workflow.

Try now
- Press 3 to open Cleanup.
- Use j/k to move between senders.
- Press d to switch between sender and domain grouping.
- Press space to select a sender or domain.
- Preview before taking action, then use delete, archive, or unsubscribe when the hints show those actions.

What Herald is doing
Cleanup groups messages by sender or domain so repeated mail can be handled in batches. Herald keeps destructive actions deliberate: preview first, select what you mean, and use unsubscribe only when the message exposes a safe unsubscribe header.`,
		withDate(baseTime.Add(4*time.Hour)),
		withHTML,
		withUnsub("https://herald.demo/unsubscribe/cleanup-coach"))
	add(36, "Herald AI Rules <rules@herald.demo>", "Step 7: Classify mail and dry-run rules", "INBOX", 0, 16896, false, true, ai.CategoryImportant, []string{"onboarding", "ai", "rules", "prompts", "dry-run", "infrastructure", "budget", "risk"},
		`Step 7 introduces the offline demo AI, cleanup rules, automation rules, and reusable prompts.

Try now
- Press a to classify the current folder.
- Press /, type ? infrastructure budget risk, and press Enter for semantic search.
- Press C to open cleanup rules.
- Press W to open automation rules.
- Press P to open custom prompts.
- Use dry-run previews before running rules.

What Herald is doing
Demo AI is deterministic and offline, so classification, semantic search, quick replies, and rule previews work without Ollama.

Cleanup rules are saved filters for repeated mail. They match senders, domains, categories, or older messages and turn those matches into planned cleanup actions such as archive, delete, hide, or review.

Automation rules are for scheduled or repeated actions. They let Herald remember a condition and an action so recurring inbox maintenance can be previewed before it ever runs live.

Custom prompts are reusable AI instructions. Use them when you want the same analysis again, such as summarizing invoices, extracting follow-ups, or labeling risky infrastructure mail.

Dry-run previews are the safety layer for all of this: they show matched messages and planned actions before mail is changed.`,
		withDate(baseTime.Add(3*time.Hour)))
	add(37, "Herald Settings <settings@herald.demo>", "Step 8: Configure accounts, AI, and signatures", "INBOX", 0, 11008, true, true, ai.CategoryImportant, []string{"onboarding", "settings", "configuration", "signature", "embedding model"},
		`Step 8 shows where Herald configuration lives.

Try now
- Press S to open Settings.
- Review the account provider fields.
- Review the AI provider fields, including local Ollama and OpenAI-compatible options.
- Find the embedding model field used by semantic search.
- Add or review an email signature, then close Settings with Esc if you are only exploring.

What Herald is doing
The settings overlay writes the same YAML shape used by normal config files. Demo mode itself does not read your mailbox or send mail, but saving settings is still a real configuration action, so inspect safely and save only when you mean it.`,
		withDate(baseTime.Add(2*time.Hour)))
	add(38, "Herald Next Steps <next@herald.demo>", "Step 9: Explore contacts, chat, SSH, and MCP", "INBOX", 0, 9984, true, true, ai.CategoryNewsletter, []string{"onboarding", "contacts", "chat", "quick replies", "mcp", "ssh"},
		`Step 9 gives you a few extra paths to try after the core tour.

Try now
- Open Contacts and inspect a recent email from a contact.
- Press c to open the chat panel.
- Open a preview and try quick replies.
- Run herald mcp --demo to expose the same synthetic mailbox to an agent.
- Run herald ssh when you want the TUI served over SSH.

What Herald is doing
The demo mailbox is shared across the TUI and MCP demo surfaces, so search, stats, classifications, and previews all point at the same fictional data. Good practice searches include ? infrastructure budget risk, images, attachments, and cleanup.`,
		withDate(baseTime.Add(1*time.Hour)))
	add(41, "Herald Mail App <newsletter@herald.demo>", "[PREVIEW] Herald v0.5.0 — Calendar, and multi-account arrive", "INBOX", 0, 36864, true, true, ai.CategoryNewsletter, []string{"newsletter", "release", "calendar", "multi-account", "oauth", "cleanup"},
		`Hi everyone,

This one is a big release — the biggest since we started this newsletter. v0.5.0 lands the multi-account work we said was coming, and also opens up a whole new surface: **Calendar**. Herald is no longer just an email client.

A bit of plumbing made this possible: mail and calendar share a common notion of source/account that flows through reads, search, compose, sync, and writes. One caveat for now — you still add the mail side and the calendar side as separate sources, even when they come from the same provider, but Herald shows them together under one unified view. That's the unglamorous half; the visible half is below.

## Multi-account, finally

The v0.4 newsletter said this was next. It is now in. You get source badges on messages so you can see at a glance which account a thread came from, Mail.app-style account sections in the folder list, account-scoped signatures, and Compose that routes replies from the right address automatically. Adding and managing accounts happens in Settings.

If you were juggling separate config files and separate terminal windows as the workaround — you can stop.

Demo mode now ships with multiple synthetic accounts too, so you can try the whole multi-account experience — badges, sections, Compose routing — without setting up a single real one.

![Herald sidebar showing two demo accounts as separate sections with their own Inbox/Drafts/Folders, plus an All Inboxes view.](https://assets.buttondown.email/images/8a2e3bc3-a4ba-4b8b-bf1e-1c58484ae066.png?w=960&fit=max)

Multi-account timeline view (Tomorrow Desk theme)

## Calendar (preview)

Press 3 to open Calendar. The surfaces are: **Week Time-Grid** for spotting dense days and conflicts, **Day Agenda** for focus on one day with a detail drawer, **3-Day Command** for today + tomorrow + the next day with a side panel of open slots and conflicts, **Agenda List**, and **Calendar Search**, plus Event Detail with RSVP. The left rail filters by source and shows colored swatches per calendar.

It is mostly a preview surface in this release. Reading, RSVP, and basic event actions work. The smoother day-to-day flows — like "I got an .ics in an email, please put it on my calendar" — are what we are focusing on next. A couple of us are using it daily; we want to hear from anyone else who is.

[Calendar guide](https://docs.herald-mail.app/using-herald/calendar/?utm_source=herald-mail.app&utm_medium=email) · [What's new in v0.5](https://docs.herald-mail.app/whats-new-in-v0-5/?utm_source=herald-mail.app&utm_medium=email)

![Herald Calendar's 3-Day Command view with a mini month and color-coded calendars on the left, three days of events across a time grid, and a side panel showing the selected event, next-up, open slots, and conflicts.](https://assets.buttondown.email/images/7b1c6dbe-804d-4997-a843-984f7e86dcf8.png?w=960&fit=max)

Calendar 3-days view (Tokyo Dusk theme)

## Calendar providers in beta

CalDAV-based providers — Fastmail Calendar, iCloud Calendar, Yahoo Calendar, custom CalDAV — work with an app-specific password or the provider's CalDAV URL. **Google Calendar OAuth is under construction** in test mode; the public OAuth path is expected in a later beta, likely v0.6.0. We know Google Calendar without OAuth is rough.

[Provider setup docs](https://docs.herald-mail.app/provider-setup/?utm_source=herald-mail.app&utm_medium=email).

## Cleanup tab is gone — it moved into Timeline

Cleanup is no longer a separate top-level tab. Browsing what to clean now lives inside Timeline grouping: press G in Timeline to cycle through thread, sender, and domain grouping. Rule execution and dry-run previews still work the way they did. One fewer tab, same actions.

## What is coming next

- **Google Calendar OAuth** (and Gmail OAuth), so Calendar in particular works the way people expect, without the app-password dance.
- **Smarter email → calendar handoff.** When a message has an .ics, Herald should offer to add it to one of your configured calendars in one step, instead of making you download the file and hope your OS opens it in the right place. We've all hit this.
- **Polish on Calendar** based on whoever pokes at it during this beta — that part depends on you.

## Try it

brew tap herald-email/herald
brew install herald
herald --demo-multi-account

Press 3 to open Calendar with demo data — no real account needed.

If you hit something rough, or there is a feature you want, tell us. The roadmap moves fastest when we hear from people who are actually using it.

Full v0.5.0 notes: https://github.com/herald-email/herald-mail-app/releases/tag/v0.5.0-beta.1
What's new in v0.5: https://docs.herald-mail.app/whats-new-in-v0-5/
Site: https://herald-mail.app/ · Docs: https://docs.herald-mail.app/

Thanks for being on this list.

— Herald team

---
This is a sanitized demo copy of the Herald Mail App Newsletter. You can subscribe at https://buttondown.com/herald-mail.app or unsubscribe at https://herald.demo/unsubscribe/v050-preview.`,
		withDate(baseTime.Add(30*time.Minute)),
		withMessageID("demo-herald-v050-preview@demo.local"),
		withHeaders("Herald Mail App <newsletter@herald.demo>", "Rowan Finch <demo@demo.local>", "", ""),
		withUnsub("https://herald.demo/unsubscribe/v050-preview"),
		withHTMLBody(`<html><body>
<p>Hi everyone,</p>
<p>This one is a big release — the biggest since we started this newsletter. v0.5.0 lands the multi-account work we said was coming, and also opens up a whole new surface: <strong>Calendar</strong>. Herald is no longer just an email client.</p>
<p>A bit of plumbing made this possible: mail and calendar share a common notion of source/account that flows through reads, search, compose, sync, and writes. One caveat for now — you still add the mail side and the calendar side as separate sources, even when they come from the same provider, but Herald shows them together under one unified view. That's the unglamorous half; the visible half is below.</p>
<h2>Multi-account, finally</h2>
<p>The v0.4 newsletter said this was next. It is now in. You get source badges on messages so you can see at a glance which account a thread came from, Mail.app-style account sections in the folder list, account-scoped signatures, and Compose that routes replies from the right address automatically. Adding and managing accounts happens in Settings.</p>
<p>If you were juggling separate config files and separate terminal windows as the workaround — you can stop.</p>
<p>Demo mode now ships with multiple synthetic accounts too, so you can try the whole multi-account experience — badges, sections, Compose routing — without setting up a single real one.</p>
<figure>
<img alt="Herald sidebar showing two demo accounts as separate sections with their own Inbox/Drafts/Folders, plus an All Inboxes view." src="https://assets.buttondown.email/images/8a2e3bc3-a4ba-4b8b-bf1e-1c58484ae066.png?w=960&amp;fit=max">
<figcaption>Multi-account timeline view (Tomorrow Desk theme)</figcaption>
</figure>
<h2>Calendar (preview)</h2>
<p>Press <code>3</code> to open Calendar. The surfaces are: <strong>Week Time-Grid</strong> for spotting dense days and conflicts, <strong>Day Agenda</strong> for focus on one day with a detail drawer, <strong>3-Day Command</strong> for today + tomorrow + the next day with a side panel of open slots and conflicts, <strong>Agenda List</strong>, and <strong>Calendar Search</strong>, plus Event Detail with RSVP. The left rail filters by source and shows colored swatches per calendar.</p>
<p>It is mostly a preview surface in this release. Reading, RSVP, and basic event actions work. The smoother day-to-day flows — like "I got an <code>.ics</code> in an email, please put it on my calendar" — are what we are focusing on next. A couple of us are using it daily; we want to hear from anyone else who is.</p>
<p><a href="https://docs.herald-mail.app/using-herald/calendar/?utm_source=herald-mail.app&amp;utm_medium=email">Calendar guide</a> · <a href="https://docs.herald-mail.app/whats-new-in-v0-5/?utm_source=herald-mail.app&amp;utm_medium=email">What's new in v0.5</a></p>
<figure>
<img alt="Herald Calendar's 3-Day Command view with a mini month and color-coded calendars on the left, three days of events across a time grid, and a side panel showing the selected event, next-up, open slots, and conflicts." src="https://assets.buttondown.email/images/7b1c6dbe-804d-4997-a843-984f7e86dcf8.png?w=960&amp;fit=max">
<figcaption>Calendar 3-days view (Tokyo Dusk theme)</figcaption>
</figure>
<h2>Calendar providers in beta</h2>
<p>CalDAV-based providers — Fastmail Calendar, iCloud Calendar, Yahoo Calendar, custom CalDAV — work with an app-specific password or the provider's CalDAV URL. <strong>Google Calendar OAuth is under construction</strong> in test mode; the public OAuth path is expected in a later beta, likely v0.6.0. We know Google Calendar without OAuth is rough.</p>
<p><a href="https://docs.herald-mail.app/provider-setup/?utm_source=herald-mail.app&amp;utm_medium=email">Provider setup docs</a>.</p>
<h2>Cleanup tab is gone — it moved into Timeline</h2>
<p>Cleanup is no longer a separate top-level tab. Browsing what to clean now lives inside Timeline grouping: press <code>G</code> in Timeline to cycle through thread, sender, and domain grouping. Rule execution and dry-run previews still work the way they did. One fewer tab, same actions.</p>
<h2>What is coming next</h2>
<ul>
<li><strong>Google Calendar OAuth</strong> (and Gmail OAuth), so Calendar in particular works the way people expect, without the app-password dance.</li>
<li><strong>Smarter email → calendar handoff.</strong> When a message has an .ics, Herald should offer to add it to one of your configured calendars in one step, instead of making you download the file and hope your OS opens it in the right place. We've all hit this.</li>
<li><strong>Polish on Calendar</strong> based on whoever pokes at it during this beta — that part depends on you.</li>
</ul>
<h2>Try it</h2>
<pre><code>brew tap herald-email/herald
brew install herald
herald --demo-multi-account</code></pre>
<p>Press <code>3</code> to open Calendar with demo data — no real account needed.</p>
<p>If you hit something rough, or there is a feature you want, tell us. The roadmap moves fastest when we hear from people who are actually using it.</p>
<p>Full v0.5.0 notes: <a href="https://github.com/herald-email/herald-mail-app/releases/tag/v0.5.0-beta.1">https://github.com/herald-email/herald-mail-app/releases/tag/v0.5.0-beta.1</a><br>
What's new in v0.5: <a href="https://docs.herald-mail.app/whats-new-in-v0-5/">https://docs.herald-mail.app/whats-new-in-v0-5/</a><br>
Site: <a href="https://herald-mail.app/">https://herald-mail.app/</a> · Docs: <a href="https://docs.herald-mail.app/">https://docs.herald-mail.app/</a></p>
<p>Thanks for being on this list.</p>
<p>— Herald team</p>
<hr>
<p>This is a sanitized demo copy of the Herald Mail App Newsletter. You can <a href="https://buttondown.com/herald-mail.app">subscribe</a> or <a href="https://herald.demo/unsubscribe/v050-preview">unsubscribe</a>.</p>
</body></html>`))

	add(1, "Northstar Cloud <billing@northstar-cloud.example>", "Example: Project Orion usage alert", "INBOX", 0, 18432, false, true, ai.CategoryImportant, []string{"infrastructure", "budget", "risk", "cloud"},
		"Northstar Cloud detected a usage change on Project Orion.\n\nThe compute cluster is 18 percent above forecast and the attached invoice highlights the services driving the budget risk.\n\nReview before Friday so the infrastructure owner can right-size the workload.",
		withAttachment("northstar-orion-invoice.pdf", "application/pdf", 184320))
	add(26, "Rowan Finch <demo@demo.local>", "Re: Example: Thread with Cobalt Works", "INBOX", 0, 8704, false, true, ai.CategoryImportant, []string{"reply", "scheduling", "interview"},
		"Hi Mina,\n\nThanks for the update - looking forward to it. I'll keep an eye out for Rae's message.\n\nBest regards,\nRowan Finch",
		withHeaders("Rowan Finch <demo@demo.local>", "Mina Park <mina@cobalt-works.example>, Rae Stone <rae@cobalt-works.example>", "Hiring Panel <panel@cobalt-works.example>", ""))
	add(27, "Mina Park <mina@cobalt-works.example>", "Example: Thread with Cobalt Works", "INBOX", 1, 9216, true, false, ai.CategoryImportant, []string{"scheduling", "interview", "follow-up"},
		"Hi Rowan,\n\nThank you for taking the time to speak with me. For next steps, we'd like to invite you to complete our technical assessment. Rae will reach out separately with a scheduling link and more details on what to expect.\n\nPlease don't hesitate to reach out if you have any questions.\n\nCheers,\nMina",
		withHeaders("Mina Park <mina@cobalt-works.example>", "Rowan Finch <demo@demo.local>, Rae Stone <rae@cobalt-works.example>", "Hiring Panel <panel@cobalt-works.example>", ""))
	add(28, "Rowan Finch <demo@demo.local>", "Re: Example: Thread with Cobalt Works", "Drafts", 0, 6144, true, false, ai.CategoryImportant, []string{"draft", "scheduling", "interview"},
		"Hi Mina,\n\nThanks for the details and the scheduling link. I'll use it to select a time shortly.\n\nLooking forward to the next step.\n\nBest regards,\nRowan Finch",
		withDraft("mina@cobalt-works.example, rae@cobalt-works.example", "", ""))
	add(2, "Mara Vale <mara@forgepoint.example>", "Example: Storage policy migration review", "INBOX", 1, 9216, false, true, ai.CategoryImportant, []string{"code", "infrastructure", "migration"},
		"Mara shared the storage policy migration plan.\n\nThe risky bit is the cache backfill window. She needs a review on the rollback checklist and the release candidate notes before standup.")
	add(3, "Mara Vale <mara@forgepoint.example>", "Re: Example: Storage policy migration review", "INBOX", 2, 8704, true, false, ai.CategoryImportant, []string{"code", "infrastructure", "migration"},
		"Thanks for the first pass.\n\nI added the missing owner for the cold-storage bucket and clarified how we drain queue workers during the migration.")
	add(4, "Packet Press <newsletter@packetpress.example>", "Example: Systems newsletter with unsubscribe", "INBOX", 3, 7168, true, false, ai.CategoryNewsletter, []string{"newsletter", "infrastructure", "systems"},
		"# Weekly systems digest\n\n- Queue observability without dashboard sprawl\n- Cache invalidation drills that fit into a team retro\n- Latency budgets for small product teams\n\nRead the full issue at https://packetpress.example/issues/42",
		withHTML, withUnsub("https://packetpress.example/unsubscribe/demo"))
	add(5, "Trailpost Travel <offers@trailpost.example>", "Example: Travel offer with unsubscribe", "INBOX", 4, 6656, true, false, ai.CategorySubscription, []string{"travel", "promotion"},
		"Trailpost found open seats to three mountain towns this weekend.\n\nThese are promotional fares and expire tonight. No action is required unless you want to book travel.",
		withUnsub("https://trailpost.example/unsubscribe/demo"))
	add(6, "Harbor Ledger <alerts@harborledger.example>", "Example: Security notice", "INBOX", 5, 8192, false, true, ai.CategoryImportant, []string{"security", "finance", "risk"},
		"Harbor Ledger noticed a new device sign-in from Portland.\n\nIf this was you, no action is needed. If not, reset your password and review account activity.")
	add(7, "Greenhouse Clinic <care@greenhouse-clinic.example>", "Example: Appointment reminder", "INBOX", 6, 6144, true, false, ai.CategoryImportant, []string{"health", "appointment"},
		"Reminder: your appointment is Thursday at 10:30 AM.\n\nPlease bring your insurance card and arrive ten minutes early for check-in.")
	add(8, "Market Lane <receipts@marketlane.example>", "Example: Receipt with attachment", "Receipts", 7, 5120, true, false, ai.CategoryTransactional, []string{"shopping", "receipt"},
		"Thanks for your Market Lane order.\n\nYour receipt for the adjustable keyboard tray is attached for your records.",
		withAttachment("marketlane-receipt.txt", "text/plain", 2048))
	add(10, "PulseNet <notify@pulsenet.example>", "Example: Social mention roundup", "Social", 9, 4608, true, false, ai.CategorySocial, []string{"social", "mention"},
		"Three people mentioned your migration checklist this week.\n\nOpen PulseNet to review the discussion and mute the thread if it is no longer useful.")
	add(11, "Northstar Cloud <billing@northstar-cloud.example>", "Example: Budget forecast update", "INBOX", 10, 9728, false, false, ai.CategoryImportant, []string{"infrastructure", "budget", "cloud"},
		"The compute cluster forecast changed after new storage replication jobs started.\n\nNorthstar recommends checking reserved capacity before the end of the billing period.")
	add(16, "Mara Vale <mara@forgepoint.example>", "Example: Build failure alert", "INBOX", 15, 9984, false, true, ai.CategoryImportant, []string{"code", "risk", "release"},
		"The release candidate failed on the fixture import step.\n\nMara included the failing package name and asked for a second set of eyes before retrying the deploy.")
	add(25, "Taskpad Teams <teams@taskpad.example>", "Example: Link rendering stress preview", "INBOX", 24, 52224, true, false, ai.CategoryNewsletter, []string{"newsletter", "links", "rendering"},
		"# Free team trial inside\n\nSign in on your computer to unlock the team features.\n\n[Display in your browser](https://taskpad.mail.example/en/emails/team/onboarding/day0/creator-mobile?o=eyJmaXJzdF9uYW1lIjoiUm93YW4iLCJ3b3Jrc3BhY2VfaW52aXRlX2NvZGUiOiJrczRBQ1hDUDJTQmxPV0l3TkRka1lqVTROak14WldSbFpEQmpOemhtTnpnek5tTXhOekJrT0EiLCJ1bnN1YnNjcmliZV9saW5rIjoiZXhhbXBsZSJ9&s=-DM3t6fB_3TyPkavY9d1vRxPgY_VQR6z9k1KfuJjjFY)\n\n![Taskpad logo](https://taskpad.mail.example/_next/static/media/taskpad-logo.0-dsvhpw__1x7.png)\n\nOpen the workspace directly: https://app.taskpad.example/app/team/welcome/path/that/is/long/enough/to/prove/wrapping?utm_source=email&utm_medium=trial&token=abcdefghijklmnopqrstuvwxyz0123456789",
		withHTML,
		withInlineImage("taskpad-inline-logo", "image/png", demoPNG()))
	add(30, "Preview Lab <design@previewlab.example>", "Example: Rich HTML rendering showcase", "INBOX", 0, 32768, true, false, ai.CategoryNewsletter, []string{"newsletter", "html", "rendering", "preview"},
		"# HTML preview quality\n\n**Budget alert** for *Project Orion*.\n\n- Headings survive in compact previews\n- Lists keep their bullets\n- Links keep readable labels\n\n[Open dashboard](https://reports.example.test/orion?utm_source=email&token=abcdefghijklmnopqrstuvwxyz0123456789)\n\n![Remote status chart](https://reports.example.test/chart.png)\n\nThe same body should look good in Timeline, Cleanup, Contacts, and full-screen readers.",
		withHTMLBody(`<html><body>
<h1>HTML preview quality</h1>
<p><strong>Budget alert</strong> for <em>Project Orion</em>.</p>
<ul>
<li>Headings survive in compact previews</li>
<li>Lists keep their bullets</li>
<li>Links keep readable labels</li>
</ul>
<blockquote>Shared rendering should make every preview surface feel consistent.</blockquote>
<table><tr><th>Surface</th><th>Status</th></tr><tr><td>Timeline</td><td>Ready</td></tr><tr><td>Cleanup</td><td>Ready</td></tr><tr><td>Contacts</td><td>Ready</td></tr></table>
<p><a href="https://reports.example.test/orion?utm_source=email&amp;token=abcdefghijklmnopqrstuvwxyz0123456789">Open dashboard</a></p>
<p><img alt="Remote status chart" src="https://reports.example.test/chart.png"></p>
<p>The same body should look good in Timeline, Cleanup, Contacts, and full-screen readers.</p>
</body></html>`))
	return MailboxFixture{
		Messages: messages,
		Contacts: []models.ContactData{
			contact(1, "billing@northstar-cloud.example", "Northstar Cloud", "Northstar Cloud", []string{"cloud", "infrastructure", "budget"}, 4, 0, 12),
			contact(2, "mara@forgepoint.example", "Mara Vale", "Forgepoint Labs", []string{"code review", "storage migration", "release"}, 3, 2, 1),
			contact(3, "newsletter@packetpress.example", "Packet Press", "Packet Press", []string{"systems", "newsletter", "latency"}, 2, 0, 3),
			contact(4, "offers@trailpost.example", "Trailpost Travel", "Trailpost Travel", []string{"travel", "promotions"}, 2, 0, 4),
			contact(5, "alerts@harborledger.example", "Harbor Ledger", "Harbor Ledger", []string{"security", "finance"}, 2, 0, 5),
			contact(6, "care@greenhouse-clinic.example", "Greenhouse Clinic", "Greenhouse Clinic", []string{"health", "appointments"}, 1, 1, 6),
			contact(7, "orders@marketlane.example", "Market Lane", "Market Lane", []string{"shopping", "orders", "receipts"}, 3, 0, 7),
			contact(8, "design@previewlab.example", "Preview Lab", "Preview Lab", []string{"html preview", "rendering", "design"}, 1, 0, 0),
		},
	}
}

func demoPNG() []byte {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		return []byte("png")
	}
	return data
}

func contact(id int64, email, display, company string, topics []string, received, sent, daysAgo int) models.ContactData {
	return models.ContactData{
		ID:          id,
		Email:       email,
		DisplayName: display,
		Company:     company,
		Topics:      append([]string(nil), topics...),
		FirstSeen:   baseTime.AddDate(0, -6, 0),
		LastSeen:    baseTime.AddDate(0, 0, -daysAgo),
		EmailCount:  received,
		SentCount:   sent,
	}
}

func cloneMessage(msg Message) Message {
	msg.Email = cloneEmail(msg.Email)
	msg.Body = cloneBody(msg.Body)
	msg.Topics = append([]string(nil), msg.Topics...)
	return msg
}

func cloneEmail(email models.EmailData) models.EmailData {
	return email
}

func cloneContact(contact models.ContactData) models.ContactData {
	contact.Topics = append([]string(nil), contact.Topics...)
	if contact.Embedding != nil {
		contact.Embedding = append([]float32(nil), contact.Embedding...)
	}
	return contact
}

func cloneBody(body models.EmailBody) models.EmailBody {
	body.InlineImages = append([]models.InlineImage(nil), body.InlineImages...)
	for i := range body.InlineImages {
		body.InlineImages[i].Data = append([]byte(nil), body.InlineImages[i].Data...)
	}
	body.Attachments = append([]models.Attachment(nil), body.Attachments...)
	for i := range body.Attachments {
		body.Attachments[i].Data = append([]byte(nil), body.Attachments[i].Data...)
	}
	return body
}
