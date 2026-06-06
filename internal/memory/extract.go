package memory

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	directionInbound = "inbound"
	directionSent    = "sent"
)

var emailAddrPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

type Extractor struct {
	Now           func() time.Time
	UserAddresses []string
	Settings      Settings
}

func (e Extractor) Extract(emails []EmailSnapshot) []Memory {
	now := e.now()
	settings := e.Settings
	settings.ApplyDefaults()
	normalized := normalizeSourceEmails(emails)
	memories := make([]Memory, 0, len(normalized)*2)
	for i := range normalized {
		if normalized[i].Email == nil {
			continue
		}
		normalized[i].Direction = e.directionFor(normalized[i])
		memories = append(memories, e.extractEmailMemories(normalized[i], settings)...)
	}
	memories = append(memories, e.extractTrackMemories(normalized, settings)...)
	for i := range memories {
		memories[i] = PrepareMemoryForAppend(memories[i], now)
	}
	return dedupeMemories(memories)
}

func (e Extractor) extractEmailMemories(item EmailSnapshot, settings Settings) []Memory {
	email := item.Email
	body := strings.TrimSpace(item.BodyText)
	snippet := bounded(firstUsefulSentence(body, email.Subject), 280)
	person, people := peopleForSnapshot(item)
	domain := domainFromSender(email.Sender)
	company := companyForSnapshot(item)
	topic := normalizeSubject(email.Subject)
	evidence := emailEvidence(email, item.Direction, snippet)
	target := defaultTargetFor(settings, person, company, topic)

	var memories []Memory
	switch item.Direction {
	case directionSent:
		claim := fmt.Sprintf("You last replied about %q on %s.", topic, email.Date.Format("2006-01-02"))
		details := detailsForSnapshot(item, claim, snippet)
		memories = append(memories, Memory{
			Kind:           KindLastUserReply,
			Claim:          claim,
			Summary:        claim,
			Topic:          topic,
			People:         people,
			Company:        company,
			Domain:         domain,
			Status:         StatusResolved,
			Confidence:     confidenceFromSnapshot(0.86, item),
			LastActivityAt: email.Date,
			Evidence:       []Evidence{evidence},
			ObsidianTarget: target,
			Tags:           tagsForSnapshot(KindLastUserReply, settings, item),
			Details:        details,
		})
	default:
		claim := fmt.Sprintf("%s last contacted you about %q on %s.", person, topic, email.Date.Format("2006-01-02"))
		details := detailsForSnapshot(item, claim, snippet)
		memories = append(memories, Memory{
			Kind:           KindLastContact,
			Claim:          claim,
			Summary:        claim,
			Topic:          topic,
			People:         people,
			Company:        company,
			Domain:         domain,
			Status:         StatusActive,
			Confidence:     confidenceFromSnapshot(0.82, item),
			LastActivityAt: email.Date,
			Evidence:       []Evidence{evidence},
			ObsidianTarget: target,
			Tags:           tagsForSnapshot(KindLastContact, settings, item),
			Details:        details,
		})
	}

	for _, question := range questionsFromText(body) {
		claim := fmt.Sprintf("Open question in %q: %s", topic, question)
		details := detailsForSnapshot(item, question, question)
		memories = append(memories, Memory{
			Kind:           KindOpenQuestion,
			Claim:          claim,
			Summary:        question,
			Topic:          topic,
			People:         people,
			Company:        company,
			Domain:         domain,
			Status:         StatusWaiting,
			Confidence:     confidenceFromSnapshot(0.78, item),
			LastActivityAt: email.Date,
			Evidence:       []Evidence{emailEvidence(email, item.Direction, question)},
			ObsidianTarget: target,
			Tags:           tagsForSnapshot(KindOpenQuestion, settings, item),
			Details:        details,
		})
		break
	}

	if commitment := commitmentSentence(body); commitment != "" {
		kind := KindCommitment
		confidence := 0.74
		if looksLikeDeadline(commitment) {
			kind = KindDeadline
			confidence = 0.77
		}
		claim := fmt.Sprintf("%s in %q: %s", memoryKindTitle(kind), topic, commitment)
		details := detailsForSnapshot(item, commitment, commitment)
		memories = append(memories, Memory{
			Kind:           kind,
			Claim:          claim,
			Summary:        commitment,
			Topic:          topic,
			People:         people,
			Company:        company,
			Domain:         domain,
			Status:         StatusActive,
			Confidence:     confidenceFromSnapshot(confidence, item),
			LastActivityAt: email.Date,
			Evidence:       []Evidence{emailEvidence(email, item.Direction, commitment)},
			ObsidianTarget: target,
			Tags:           tagsForSnapshot(kind, settings, item),
			Details:        details,
		})
	}
	return memories
}

func (e Extractor) extractTrackMemories(emails []EmailSnapshot, settings Settings) []Memory {
	byThread := make(map[string][]EmailSnapshot)
	for _, item := range emails {
		if item.Email == nil {
			continue
		}
		key := threadKey(item.Email)
		if key == "" {
			continue
		}
		byThread[key] = append(byThread[key], item)
	}
	var memories []Memory
	for _, thread := range byThread {
		sort.SliceStable(thread, func(i, j int) bool {
			return thread[i].Email.Date.Before(thread[j].Email.Date)
		})
		if len(thread) == 0 || !isMemoryWorthyThread(thread) {
			continue
		}
		latest := thread[len(thread)-1]
		direction := e.directionFor(latest)
		status := StatusActive
		summary := "Latest thread activity is available."
		if direction == directionSent {
			status = StatusWaiting
			summary = "You replied last; this may be awaiting a response."
		} else if len(questionsFromText(latest.BodyText)) > 0 || containsAnyFold(latest.BodyText, "please", "could you", "can you") {
			status = StatusWaiting
			summary = "Latest inbound message may need your response."
		}
		email := latest.Email
		person, _ := peopleForSnapshot(latest)
		domain := domainFromSender(email.Sender)
		company := companyForSnapshot(latest)
		topic := normalizeSubject(email.Subject)
		claim := fmt.Sprintf("Track %q is %s: %s", topic, status, summary)
		evidence := evidenceFromThread(thread, 4)
		memories = append(memories, Memory{
			Kind:           KindTrackStatus,
			Claim:          claim,
			Summary:        summary,
			Topic:          topic,
			People:         peopleFromThread(thread),
			Company:        company,
			Domain:         domain,
			Status:         status,
			Confidence:     confidenceFromThread(0.72, thread),
			LastActivityAt: email.Date,
			Evidence:       evidence,
			ObsidianTarget: defaultTargetFor(settings, person, company, topic),
			Tags:           tagsForSnapshot(KindTrackStatus, settings, latest),
			Details: MemoryDetails{
				GeneratedSummary: summary,
				SourceCount:      len(evidence),
				ExtractionPrompt: PromptVersionHeuristicV1,
				Classification:   dominantClassification(thread),
				ContactCompany:   company,
				ContactTopics:    topicsFromThread(thread),
				SourceSignals:    signalsFromThread(thread),
			},
		})
	}
	return memories
}

func (e Extractor) now() time.Time {
	if e.Now != nil {
		if now := e.Now(); !now.IsZero() {
			return now
		}
	}
	return time.Now()
}

func (e Extractor) directionFor(item EmailSnapshot) string {
	if strings.TrimSpace(item.Direction) != "" {
		return item.Direction
	}
	if item.Email == nil {
		return directionInbound
	}
	folder := strings.ToLower(item.Email.Folder)
	if strings.Contains(folder, "sent") || strings.Contains(folder, "draft") || strings.Contains(folder, "outbox") {
		return directionSent
	}
	sender := strings.ToLower(item.Email.Sender)
	for _, address := range e.UserAddresses {
		address = strings.ToLower(strings.TrimSpace(address))
		if address != "" && strings.Contains(sender, address) {
			return directionSent
		}
	}
	return directionInbound
}

func normalizeSourceEmails(emails []EmailSnapshot) []EmailSnapshot {
	out := make([]EmailSnapshot, 0, len(emails))
	for _, item := range emails {
		if item.Email == nil {
			continue
		}
		item.BodyText = BoundSnapshotBodyText(item.BodyText)
		item.ContactTopics = CompactStrings(item.ContactTopics)
		if item.BodyText != "" {
			item.HasBodyCache = true
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Email.Date.Before(out[j].Email.Date)
	})
	return out
}

func emailEvidence(email *models.EmailData, direction, snippet string) Evidence {
	sourceType := SourceEmail
	if direction == directionSent {
		sourceType = SourceSentEmail
	}
	ref := email.MessageRef().WithDefaults()
	return Evidence{
		SourceType: sourceType,
		SourceID:   string(ref.SourceID),
		AccountID:  string(ref.AccountID),
		ID:         firstNonEmpty(ref.LocalID, email.MessageID),
		MessageID:  email.MessageID,
		LocalID:    ref.LocalID,
		Folder:     email.Folder,
		Date:       email.Date,
		Snippet:    bounded(snippet, 300),
	}
}

func firstUsefulSentence(body, fallback string) string {
	for _, sentence := range splitSentences(body) {
		if len([]rune(sentence)) >= 12 {
			return sentence
		}
	}
	return fallback
}

func questionsFromText(text string) []string {
	var questions []string
	for _, sentence := range splitSentences(text) {
		if strings.Contains(sentence, "?") {
			questions = append(questions, bounded(sentence, 220))
		}
	}
	return CompactStrings(questions)
}

func commitmentSentence(text string) string {
	for _, sentence := range splitSentences(text) {
		lower := strings.ToLower(sentence)
		if containsAnyFold(lower, "i will", "we will", "i can", "we can", "i'll", "we'll", "please send", "please share", "follow up", "circle back", "deadline", "by ") {
			return bounded(sentence, 240)
		}
	}
	return ""
}

func looksLikeDeadline(sentence string) bool {
	return containsAnyFold(sentence, "deadline", " by ", "before ", "due ", "tomorrow", "friday", "monday", "tuesday", "wednesday", "thursday")
}

func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	raw := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '.'
	})
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func senderIdentity(sender string) string {
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return "Unknown contact"
	}
	if match := emailAddrPattern.FindString(sender); match != "" {
		return match
	}
	return sender
}

func peopleForSnapshot(item EmailSnapshot) (string, []string) {
	if item.Email == nil {
		return "Unknown contact", []string{"Unknown contact"}
	}
	raw := senderIdentity(item.Email.Sender)
	display := strings.TrimSpace(item.ContactDisplayName)
	if display == "" || strings.EqualFold(display, raw) {
		return raw, []string{raw}
	}
	return display, CompactStrings([]string{display, raw})
}

func domainFromSender(sender string) string {
	if match := emailAddrPattern.FindString(sender); match != "" {
		parts := strings.Split(match, "@")
		if len(parts) == 2 {
			return strings.ToLower(parts[1])
		}
	}
	if strings.Contains(sender, "@") {
		parts := strings.Split(sender, "@")
		return strings.ToLower(strings.Trim(parts[len(parts)-1], " <>"))
	}
	return ""
}

func companyForSnapshot(item EmailSnapshot) string {
	if company := strings.TrimSpace(item.ContactCompany); company != "" {
		return company
	}
	if item.Email == nil {
		return ""
	}
	return companyFromDomain(domainFromSender(item.Email.Sender))
}

func companyFromDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return ""
	}
	name := strings.Split(domain, ".")[0]
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func normalizeSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	for {
		lower := strings.ToLower(subject)
		matched := false
		for _, prefix := range []string{"re:", "fw:", "fwd:", "aw:", "tr:"} {
			if strings.HasPrefix(lower, prefix) {
				subject = strings.TrimSpace(subject[len(prefix):])
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	if subject == "" {
		return "untitled thread"
	}
	return subject
}

func detailsForSnapshot(item EmailSnapshot, generated, quote string) MemoryDetails {
	details := MemoryDetails{
		GeneratedSummary: generated,
		SourceQuote:      bounded(quote, 300),
		ExtractionPrompt: PromptVersionHeuristicV1,
		Classification:   strings.TrimSpace(item.Classification),
		ContactCompany:   strings.TrimSpace(item.ContactCompany),
		ContactTopics:    CompactStrings(item.ContactTopics),
		SourceSignals:    sourceSignalsForSnapshot(item),
	}
	if !item.HasBodyCache {
		details.ReviewReason = "No cached body text was available; this memory uses headers and source metadata."
	}
	if containsAnyFold(item.Classification, "newsletter", "subscription", "promotion", "unnecessary", "spam") {
		details.ReviewReason = "Classification suggests lower relationship value; keep below warning surfaces unless other evidence strengthens it."
	}
	return details
}

func confidenceFromSnapshot(base float64, item EmailSnapshot) float64 {
	confidence := base
	if item.HasBodyCache {
		confidence += 0.03
	} else {
		confidence -= 0.06
	}
	if item.HasEmbedding {
		confidence += 0.02
	}
	if strings.TrimSpace(item.ContactDisplayName) != "" || strings.TrimSpace(item.ContactCompany) != "" || len(item.ContactTopics) > 0 {
		confidence += 0.03
	}
	switch {
	case containsAnyFold(item.Classification, "important", "job", "work", "interview", "personal"):
		confidence += 0.03
	case containsAnyFold(item.Classification, "newsletter", "subscription", "promotion", "unnecessary", "spam"):
		confidence -= 0.08
	}
	return clampConfidence(confidence)
}

func confidenceFromThread(base float64, thread []EmailSnapshot) float64 {
	if len(thread) == 0 {
		return clampConfidence(base)
	}
	total := 0.0
	for _, item := range thread {
		total += confidenceFromSnapshot(base, item)
	}
	return clampConfidence(total / float64(len(thread)))
}

func clampConfidence(value float64) float64 {
	if value < 0.10 {
		return 0.10
	}
	if value > 0.99 {
		return 0.99
	}
	return value
}

func sourceSignalsForSnapshot(item EmailSnapshot) []string {
	signals := []string{}
	if item.HasBodyCache {
		signals = append(signals, "cached_body")
	}
	if item.HasEmbedding {
		signals = append(signals, "semantic_embedding")
	}
	if classification := strings.TrimSpace(item.Classification); classification != "" {
		signals = append(signals, "classification:"+classification)
	}
	if strings.TrimSpace(item.ContactDisplayName) != "" || strings.TrimSpace(item.ContactCompany) != "" || len(item.ContactTopics) > 0 {
		signals = append(signals, "contact_enrichment")
	}
	if item.Email != nil && strings.TrimSpace(threadKey(item.Email)) != "" {
		signals = append(signals, "thread_headers")
	}
	return CompactStrings(signals)
}

func signalsFromThread(thread []EmailSnapshot) []string {
	var signals []string
	for _, item := range thread {
		signals = append(signals, sourceSignalsForSnapshot(item)...)
	}
	return CompactStrings(signals)
}

func dominantClassification(thread []EmailSnapshot) string {
	counts := make(map[string]int)
	var best string
	for _, item := range thread {
		classification := strings.TrimSpace(item.Classification)
		if classification == "" {
			continue
		}
		counts[classification]++
		if best == "" || counts[classification] > counts[best] {
			best = classification
		}
	}
	return best
}

func topicsFromThread(thread []EmailSnapshot) []string {
	var topics []string
	for _, item := range thread {
		topics = append(topics, item.ContactTopics...)
	}
	return CompactStrings(topics)
}

func threadKey(email *models.EmailData) string {
	if email == nil {
		return ""
	}
	if strings.TrimSpace(email.ProviderThreadID) != "" {
		return "provider:" + strings.TrimSpace(email.ProviderThreadID)
	}
	return "subject:" + strings.ToLower(normalizeSubject(email.Subject))
}

func isMemoryWorthyThread(thread []EmailSnapshot) bool {
	for _, item := range thread {
		if item.Email == nil {
			continue
		}
		text := strings.Join([]string{
			item.Email.Subject,
			item.BodyText,
			item.Email.Sender,
			item.Classification,
			item.ContactCompany,
			strings.Join(item.ContactTopics, " "),
		}, " ")
		if containsAnyFold(text, "job", "interview", "recruiter", "offer", "application", "resume", "cv", "follow up", "deadline", "project", "proposal", "contract", "intro", "sergey") {
			return true
		}
	}
	return len(thread) >= 3
}

func peopleFromThread(thread []EmailSnapshot) []string {
	people := make([]string, 0, len(thread))
	for _, item := range thread {
		if item.Email != nil {
			_, itemPeople := peopleForSnapshot(item)
			people = append(people, itemPeople...)
		}
	}
	return CompactStrings(people)
}

func evidenceFromThread(thread []EmailSnapshot, limit int) []Evidence {
	if limit <= 0 {
		limit = len(thread)
	}
	start := len(thread) - limit
	if start < 0 {
		start = 0
	}
	out := make([]Evidence, 0, len(thread)-start)
	for _, item := range thread[start:] {
		if item.Email == nil {
			continue
		}
		out = append(out, emailEvidence(item.Email, item.Direction, firstUsefulSentence(item.BodyText, item.Email.Subject)))
	}
	return out
}

func defaultTargetFor(settings Settings, person, company, topic string) string {
	combined := strings.ToLower(strings.Join([]string{topic, company, person}, " "))
	if containsAnyFold(combined, "job", "interview", "recruiter", "application", "offer", "resume", "cv") {
		if company != "" {
			return strings.TrimRight(settings.Destinations.Companies, "/") + "/" + safeNoteName(company) + "/Memory.md"
		}
		return strings.TrimRight(settings.Destinations.JobSearch, "/") + "/Memory.md"
	}
	if person != "" && person != "Unknown contact" {
		return strings.TrimRight(settings.Destinations.People, "/") + "/" + safeNoteName(person) + ".md"
	}
	if company != "" {
		return strings.TrimRight(settings.Destinations.Companies, "/") + "/" + safeNoteName(company) + ".md"
	}
	return strings.TrimRight(settings.Destinations.Inbox, "/") + "/Memory.md"
}

func safeNoteName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "@", " at ")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "Unknown"
	}
	return value
}

func tagsForMemory(kind string, settings Settings) []string {
	switch NormalizeTagMode(settings.Obsidian.TagMode) {
	case TagModeNone:
		return nil
	case TagModeWorkflow:
		return []string{"#herald/memory", "#herald/" + strings.ReplaceAll(kind, "_", "-")}
	case TagModeCustom:
		if len(settings.Obsidian.CustomTags) > 0 {
			return settings.Obsidian.CustomTags
		}
		return []string{"#herald/memory"}
	default:
		switch kind {
		case KindTrackStatus:
			return []string{"#herald/track"}
		case KindOpenQuestion, KindCommitment, KindDeadline:
			return []string{"#herald/memory"}
		default:
			return nil
		}
	}
}

func tagsForSnapshot(kind string, settings Settings, item EmailSnapshot) []string {
	tags := tagsForMemory(kind, settings)
	if tag := classificationTag(settings, item.Classification); tag != "" {
		tags = append(tags, tag)
	}
	if item.HasEmbedding && NormalizeTagMode(settings.Obsidian.TagMode) == TagModeWorkflow {
		tags = append(tags, "#herald/semantic")
	}
	return CompactStrings(tags)
}

func classificationTag(settings Settings, classification string) string {
	classification = strings.TrimSpace(classification)
	if classification == "" || NormalizeTagMode(settings.Obsidian.TagMode) == TagModeNone {
		return ""
	}
	return "#herald/classification-" + classificationSlug(classification)
}

func classificationSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func memoryKindTitle(kind string) string {
	switch kind {
	case KindDeadline:
		return "Deadline"
	case KindCommitment:
		return "Commitment"
	default:
		return "Memory"
	}
}

func dedupeMemories(memories []Memory) []Memory {
	seen := make(map[string]bool, len(memories))
	out := make([]Memory, 0, len(memories))
	for _, memory := range memories {
		id := memory.ID
		if id == "" {
			id = DeterministicID(memory)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, memory)
	}
	SortMemoriesNewestFirst(out)
	return out
}

func containsAnyFold(value string, needles ...string) bool {
	value = strings.ToLower(value)
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
