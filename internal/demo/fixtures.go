package demo

import (
	"encoding/base64"
	"math"
	"sort"
	"strings"
	"time"

	"mail-processor/internal/ai"
	"mail-processor/internal/models"
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
	withUnsub := func(url string) func(*Message) {
		return func(msg *Message) {
			msg.Body.ListUnsubscribe = "<" + url + ">"
			msg.Body.ListUnsubscribePost = "List-Unsubscribe=One-Click"
		}
	}
	withAttachment := func(filename, mime string, size int) func(*Message) {
		return func(msg *Message) {
			msg.Email.HasAttachments = true
			msg.Body.Attachments = append(msg.Body.Attachments, models.Attachment{
				Filename: filename,
				MIMEType: mime,
				Size:     size,
				PartPath: "2",
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

	add(1, "Northstar Cloud <billing@northstar-cloud.example>", "Invoice and usage alert for Project Orion", "INBOX", 0, 18432, false, true, ai.CategoryImportant, []string{"infrastructure", "budget", "risk", "cloud"},
		"Northstar Cloud detected a usage change on Project Orion.\n\nThe compute cluster is 18 percent above forecast and the attached invoice highlights the services driving the budget risk.\n\nReview before Friday so the infrastructure owner can right-size the workload.",
		withAttachment("northstar-orion-invoice.pdf", "application/pdf", 184320))
	add(26, "Anton Golubtsov <demo@demo.local>", "Re: Next Steps with Anthropic!", "INBOX", 0, 8704, false, true, ai.CategoryImportant, []string{"reply", "scheduling", "interview"},
		"Hi Tyitana,\n\nThanks for the update - looking forward to it. I'll keep an eye out for Shea's message.\n\nBest regards,\nAnton Golubtsov")
	add(27, "Tyitana Horton <tytiana@anthropic.example>", "Next Steps with Anthropic!", "INBOX", 1, 9216, true, false, ai.CategoryImportant, []string{"scheduling", "interview", "follow-up"},
		"Hi Anton,\n\nThank you for taking the time to speak with me. For next steps, we'd like to invite you to complete our technical assessment. Shea will reach out separately with a scheduling link and more details on what to expect.\n\nPlease don't hesitate to reach out if you have any questions.\n\nCheers,\nTyitana")
	add(28, "Anton Golubtsov <demo@demo.local>", "Re: Next Steps with Anthropic!", "Drafts", 0, 6144, true, false, ai.CategoryImportant, []string{"draft", "scheduling", "interview"},
		"Hi Tyitana,\n\nThanks for the details and the scheduling link. I'll use it to select a time shortly.\n\nLooking forward to the next step.\n\nBest regards,\nAnton Golubtsov",
		withDraft("tytiana@anthropic.example, shea@anthropic.example", "", ""))
	add(2, "Mara Vale <mara@forgepoint.example>", "Storage policy migration review", "INBOX", 1, 9216, false, true, ai.CategoryImportant, []string{"code", "infrastructure", "migration"},
		"Mara shared the storage policy migration plan.\n\nThe risky bit is the cache backfill window. She needs a review on the rollback checklist and the release candidate notes before standup.")
	add(3, "Mara Vale <mara@forgepoint.example>", "Re: Storage policy migration review", "INBOX", 2, 8704, true, false, ai.CategoryImportant, []string{"code", "infrastructure", "migration"},
		"Thanks for the first pass.\n\nI added the missing owner for the cold-storage bucket and clarified how we drain queue workers during the migration.")
	add(4, "Packet Press <newsletter@packetpress.example>", "Weekly systems digest: queues, caches, latency", "INBOX", 3, 7168, true, false, ai.CategoryNewsletter, []string{"newsletter", "infrastructure", "systems"},
		"# Weekly systems digest\n\n- Queue observability without dashboard sprawl\n- Cache invalidation drills that fit into a team retro\n- Latency budgets for small product teams\n\nRead the full issue at https://packetpress.example/issues/42",
		withHTML, withUnsub("https://packetpress.example/unsubscribe/demo"))
	add(5, "Trailpost Travel <offers@trailpost.example>", "Weekend fares for mountain towns", "INBOX", 4, 6656, true, false, ai.CategorySubscription, []string{"travel", "promotion"},
		"Trailpost found open seats to three mountain towns this weekend.\n\nThese are promotional fares and expire tonight. No action is required unless you want to book travel.",
		withUnsub("https://trailpost.example/unsubscribe/demo"))
	add(6, "Harbor Ledger <alerts@harborledger.example>", "Security notice: new device sign-in", "INBOX", 5, 8192, false, true, ai.CategoryImportant, []string{"security", "finance", "risk"},
		"Harbor Ledger noticed a new device sign-in from Portland.\n\nIf this was you, no action is needed. If not, reset your password and review account activity.")
	add(7, "Greenhouse Clinic <care@greenhouse-clinic.example>", "Appointment reminder for Thursday", "INBOX", 6, 6144, true, false, ai.CategoryImportant, []string{"health", "appointment"},
		"Reminder: your appointment is Thursday at 10:30 AM.\n\nPlease bring your insurance card and arrive ten minutes early for check-in.")
	add(8, "Market Lane <receipts@marketlane.example>", "Receipt for keyboard tray", "Receipts", 7, 5120, true, false, ai.CategoryTransactional, []string{"shopping", "receipt"},
		"Thanks for your Market Lane order.\n\nYour receipt for the adjustable keyboard tray is attached for your records.",
		withAttachment("marketlane-receipt.txt", "text/plain", 2048))
	add(9, "Market Lane <orders@marketlane.example>", "Your order is out for delivery", "Receipts", 8, 5632, false, false, ai.CategoryTransactional, []string{"shopping", "delivery"},
		"Your order is out for delivery today.\n\nTracking shows the package should arrive before 7 PM.")
	add(10, "PulseNet <notify@pulsenet.example>", "Mention roundup from your network", "Social", 9, 4608, true, false, ai.CategorySocial, []string{"social", "mention"},
		"Three people mentioned your migration checklist this week.\n\nOpen PulseNet to review the discussion and mute the thread if it is no longer useful.")
	add(11, "Northstar Cloud <billing@northstar-cloud.example>", "Budget forecast changed for compute cluster", "INBOX", 10, 9728, false, false, ai.CategoryImportant, []string{"infrastructure", "budget", "cloud"},
		"The compute cluster forecast changed after new storage replication jobs started.\n\nNorthstar recommends checking reserved capacity before the end of the billing period.")
	add(12, "Lumen School <billing@lumenschool.example>", "Tuition statement available", "INBOX", 11, 10496, true, false, ai.CategoryImportant, []string{"finance", "statement"},
		"Your tuition statement is available in the Lumen School portal.\n\nPayment is due next month. A PDF copy is attached.",
		withAttachment("lumen-statement.pdf", "application/pdf", 98304))
	add(13, "Field Notes Review <editors@fieldnotes-review.example>", "Your spring field guide issue", "INBOX", 12, 7424, true, false, ai.CategoryNewsletter, []string{"newsletter", "guide"},
		"# Spring field guide\n\nThis issue covers tiny city gardens, resilient herbs, and a practical checklist for weekend planting.",
		withHTML, withUnsub("https://fieldnotes-review.example/unsubscribe/demo"))
	add(14, "City Arts Hall <tickets@cityarts.example>", "Tickets for Friday evening", "Receipts", 13, 12288, true, false, ai.CategoryTransactional, []string{"travel", "ticket"},
		"Your tickets for Friday evening are ready.\n\nShow the attached confirmation at the door.",
		withAttachment("city-arts-tickets.pdf", "application/pdf", 122880))
	add(15, "Trailpost Travel <offers@trailpost.example>", "Last call: coast cabins under 120", "INBOX", 14, 6400, true, false, ai.CategorySubscription, []string{"travel", "promotion"},
		"Coast cabins under 120 are available for a limited time.\n\nThis is a promotional message from Trailpost Travel.",
		withUnsub("https://trailpost.example/unsubscribe/demo"))
	add(16, "Mara Vale <mara@forgepoint.example>", "Build failed on release candidate", "INBOX", 15, 9984, false, true, ai.CategoryImportant, []string{"code", "risk", "release"},
		"The release candidate failed on the fixture import step.\n\nMara included the failing package name and asked for a second set of eyes before retrying the deploy.")
	add(17, "Ledgerly <exports@ledgerly.example>", "Monthly books export is ready", "Receipts", 16, 15360, true, false, ai.CategoryTransactional, []string{"finance", "books"},
		"Your monthly books export is ready.\n\nDownload the CSV from Ledgerly or use the attached summary file.",
		withAttachment("ledgerly-april-export.csv", "text/csv", 32768))
	add(18, "Harbor Ledger <statements@harborledger.example>", "April statement is ready", "INBOX", 17, 8192, true, false, ai.CategoryTransactional, []string{"finance", "statement"},
		"Your April statement is ready.\n\nLog in to Harbor Ledger to review balances and recent transactions.")
	add(19, "Packet Press <newsletter@packetpress.example>", "Containers without the churn", "INBOX", 18, 6912, true, false, ai.CategoryNewsletter, []string{"newsletter", "containers"},
		"# Containers without the churn\n\nA short guide to keeping build images small, predictable, and boring.",
		withHTML, withUnsub("https://packetpress.example/unsubscribe/demo"))
	add(20, "Studio West <billing@studiowest.example>", "Invoice paid: April retainer", "Receipts", 19, 5632, true, false, ai.CategoryTransactional, []string{"finance", "invoice"},
		"Studio West received payment for the April retainer.\n\nNo action is needed.")
	add(21, "Nimbus Health <portal@nimbus-health.example>", "Lab results available in portal", "INBOX", 20, 7168, false, false, ai.CategoryImportant, []string{"health", "portal"},
		"New lab results are available in your Nimbus Health portal.\n\nPlease review the note from your care team.")
	add(22, "Northstar Cloud <billing@northstar-cloud.example>", "Re: Budget forecast changed for compute cluster", "INBOX", 21, 8960, true, false, ai.CategoryImportant, []string{"infrastructure", "budget", "cloud"},
		"Following up on the compute cluster forecast.\n\nThe largest cost change is storage replication in the west region.")
	add(23, "Civic Water <notices@civicwater.example>", "Service interruption planned for Tuesday", "INBOX", 22, 5888, true, false, ai.CategoryImportant, []string{"notice", "risk"},
		"Civic Water plans a service interruption Tuesday from 1 AM to 4 AM.\n\nNo action is needed unless you manage equipment that depends on water service.")
	add(24, "Market Lane <offers@marketlane.example>", "Gift card bonus weekend", "INBOX", 23, 5248, true, false, ai.CategorySubscription, []string{"shopping", "promotion"},
		"Market Lane is running a gift card bonus this weekend.\n\nThis promotional email can be ignored unless you are shopping soon.",
		withUnsub("https://marketlane.example/unsubscribe/demo"))
	add(25, "Taskpad Teams <teams@taskpad.example>", "Link rendering stress preview", "INBOX", 24, 52224, true, false, ai.CategoryNewsletter, []string{"newsletter", "links", "rendering"},
		"# Free team trial inside\n\nSign in on your computer to unlock the team features.\n\n[Display in your browser](https://taskpad.mail.example/en/emails/team/onboarding/day0/creator-mobile?o=eyJmaXJzdF9uYW1lIjoiQW50b24iLCJ3b3Jrc3BhY2VfaW52aXRlX2NvZGUiOiJrczRBQ1hDUDJTQmxPV0l3TkRka1lqVTROak14WldReVpEQmpOemhtTnpnek5tTXhOekJrT0EiLCJ1bnN1YnNjcmliZV9saW5rIjoiZXhhbXBsZSJ9&s=-DM3t6fB_3TyPkavY9d1vRxPgY_VQR6z9k1KfuJjjFY)\n\n![Taskpad logo](https://taskpad.mail.example/_next/static/media/taskpad-logo.0-dsvhpw__1x7.png)\n\nOpen the workspace directly: https://app.taskpad.example/app/team/welcome/path/that/is/long/enough/to/prove/wrapping?utm_source=email&utm_medium=trial&token=abcdefghijklmnopqrstuvwxyz0123456789",
		withHTML,
		withInlineImage("taskpad-inline-logo", "image/png", demoPNG()))

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
