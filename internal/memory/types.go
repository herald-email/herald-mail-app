package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	DefaultDirectory = "~/.herald/memories"

	KindLastContact         = "last_contact"
	KindLastUserReply       = "last_user_reply"
	KindOpenQuestion        = "open_question"
	KindCommitment          = "commitment"
	KindDeadline            = "deadline"
	KindRelationshipContext = "relationship_context"
	KindTrackStatus         = "track_status"
	KindResearchNote        = "research_note"

	StatusActive        = "active"
	StatusWaiting       = "waiting"
	StatusResolved      = "resolved"
	StatusStale         = "stale"
	StatusConflict      = "conflict"
	StatusSourceMissing = "source_missing"

	FreshnessFresh = "fresh"
	FreshnessStale = "stale"

	SourceEmail     = "email"
	SourceSentEmail = "sent_email"
	SourceObsidian  = "obsidian"
	SourceResearch  = "research_url"
	SourceCalendar  = "calendar"

	PromptVersionHeuristicV1 = "memory-heuristic-v1"
)

// Evidence is a normalized source pointer. It intentionally stores only a
// bounded snippet so immutable memory records do not become a second raw-mail
// archive.
type Evidence struct {
	SourceType string    `json:"source_type" yaml:"source_type"`
	SourceID   string    `json:"source_id,omitempty" yaml:"source_id,omitempty"`
	AccountID  string    `json:"account_id,omitempty" yaml:"account_id,omitempty"`
	ID         string    `json:"id,omitempty" yaml:"id,omitempty"`
	MessageID  string    `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	LocalID    string    `json:"local_id,omitempty" yaml:"local_id,omitempty"`
	Folder     string    `json:"folder,omitempty" yaml:"folder,omitempty"`
	Date       time.Time `json:"date,omitempty" yaml:"date,omitempty"`
	Snippet    string    `json:"snippet,omitempty" yaml:"snippet,omitempty"`
	Path       string    `json:"path,omitempty" yaml:"path,omitempty"`
	URL        string    `json:"url,omitempty" yaml:"url,omitempty"`
}

type MemoryDetails struct {
	GeneratedSummary string    `json:"generated_summary,omitempty" yaml:"generated_summary,omitempty"`
	SourceQuote      string    `json:"source_quote,omitempty" yaml:"source_quote,omitempty"`
	SourceCount      int       `json:"source_count,omitempty" yaml:"source_count,omitempty"`
	ExtractionPrompt string    `json:"extraction_prompt,omitempty" yaml:"extraction_prompt,omitempty"`
	LastValidatedAt  time.Time `json:"last_validated_at,omitempty" yaml:"last_validated_at,omitempty"`
	ReviewReason     string    `json:"review_reason,omitempty" yaml:"review_reason,omitempty"`
}

type Memory struct {
	ID             string        `json:"id" yaml:"id"`
	Kind           string        `json:"kind" yaml:"kind"`
	Claim          string        `json:"claim" yaml:"claim"`
	Summary        string        `json:"summary,omitempty" yaml:"summary,omitempty"`
	Topic          string        `json:"topic,omitempty" yaml:"topic,omitempty"`
	People         []string      `json:"people,omitempty" yaml:"people,omitempty"`
	Company        string        `json:"company,omitempty" yaml:"company,omitempty"`
	Domain         string        `json:"domain,omitempty" yaml:"domain,omitempty"`
	Status         string        `json:"status,omitempty" yaml:"status,omitempty"`
	Freshness      string        `json:"freshness,omitempty" yaml:"freshness,omitempty"`
	Confidence     float64       `json:"confidence" yaml:"confidence"`
	CreatedAt      time.Time     `json:"created_at" yaml:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at" yaml:"updated_at"`
	LastActivityAt time.Time     `json:"last_activity_at,omitempty" yaml:"last_activity_at,omitempty"`
	PromptVersion  string        `json:"prompt_version,omitempty" yaml:"prompt_version,omitempty"`
	Evidence       []Evidence    `json:"evidence" yaml:"evidence"`
	ObsidianTarget string        `json:"obsidian_target,omitempty" yaml:"obsidian_target,omitempty"`
	Tags           []string      `json:"tags,omitempty" yaml:"tags,omitempty"`
	Links          []string      `json:"links,omitempty" yaml:"links,omitempty"`
	Supersedes     []string      `json:"supersedes,omitempty" yaml:"supersedes,omitempty"`
	Related        []string      `json:"related,omitempty" yaml:"related,omitempty"`
	Details        MemoryDetails `json:"details,omitempty" yaml:"details,omitempty"`
}

type Track struct {
	ID             string     `json:"id" yaml:"id"`
	Topic          string     `json:"topic" yaml:"topic"`
	People         []string   `json:"people,omitempty" yaml:"people,omitempty"`
	Company        string     `json:"company,omitempty" yaml:"company,omitempty"`
	Domain         string     `json:"domain,omitempty" yaml:"domain,omitempty"`
	Status         string     `json:"status" yaml:"status"`
	OpenLoops      []string   `json:"open_loops,omitempty" yaml:"open_loops,omitempty"`
	Commitments    []string   `json:"commitments,omitempty" yaml:"commitments,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at,omitempty" yaml:"last_activity_at,omitempty"`
	MemoryIDs      []string   `json:"memory_ids,omitempty" yaml:"memory_ids,omitempty"`
	Evidence       []Evidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

type Dossier struct {
	ID                  string     `json:"id" yaml:"id"`
	Subject             string     `json:"subject" yaml:"subject"`
	Kind                string     `json:"kind" yaml:"kind"`
	RelationshipSummary string     `json:"relationship_summary,omitempty" yaml:"relationship_summary,omitempty"`
	RecentInteractions  []Memory   `json:"recent_interactions,omitempty" yaml:"recent_interactions,omitempty"`
	ActiveTracks        []Track    `json:"active_tracks,omitempty" yaml:"active_tracks,omitempty"`
	OpenLoops           []Memory   `json:"open_loops,omitempty" yaml:"open_loops,omitempty"`
	VaultLinks          []string   `json:"vault_links,omitempty" yaml:"vault_links,omitempty"`
	ResearchNotes       []Memory   `json:"research_notes,omitempty" yaml:"research_notes,omitempty"`
	Freshness           string     `json:"freshness,omitempty" yaml:"freshness,omitempty"`
	GeneratedAt         time.Time  `json:"generated_at,omitempty" yaml:"generated_at,omitempty"`
	Evidence            []Evidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

type Nudge struct {
	ID          string     `json:"id" yaml:"id"`
	Type        string     `json:"type" yaml:"type"`
	Message     string     `json:"message" yaml:"message"`
	Why         string     `json:"why,omitempty" yaml:"why,omitempty"`
	Confidence  float64    `json:"confidence" yaml:"confidence"`
	MemoryIDs   []string   `json:"memory_ids,omitempty" yaml:"memory_ids,omitempty"`
	Evidence    []Evidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	ActionState string     `json:"action_state,omitempty" yaml:"action_state,omitempty"`
}

type EmailSnapshot struct {
	Email     *models.EmailData
	BodyText  string
	Direction string
}

type Query struct {
	Text                 string   `json:"text,omitempty" yaml:"text,omitempty"`
	People               []string `json:"people,omitempty" yaml:"people,omitempty"`
	Company              string   `json:"company,omitempty" yaml:"company,omitempty"`
	Domain               string   `json:"domain,omitempty" yaml:"domain,omitempty"`
	Topic                string   `json:"topic,omitempty" yaml:"topic,omitempty"`
	Kind                 string   `json:"kind,omitempty" yaml:"kind,omitempty"`
	Status               string   `json:"status,omitempty" yaml:"status,omitempty"`
	MinConfidence        float64  `json:"min_confidence,omitempty" yaml:"min_confidence,omitempty"`
	Limit                int      `json:"limit,omitempty" yaml:"limit,omitempty"`
	IncludeLowConfidence bool     `json:"include_low_confidence,omitempty" yaml:"include_low_confidence,omitempty"`
}

type ReplyPrepQuery struct {
	Recipient     string  `json:"recipient,omitempty" yaml:"recipient,omitempty"`
	Subject       string  `json:"subject,omitempty" yaml:"subject,omitempty"`
	DraftExcerpt  string  `json:"draft_excerpt,omitempty" yaml:"draft_excerpt,omitempty"`
	MessageID     string  `json:"message_id,omitempty" yaml:"message_id,omitempty"`
	Limit         int     `json:"limit,omitempty" yaml:"limit,omitempty"`
	MinConfidence float64 `json:"min_confidence,omitempty" yaml:"min_confidence,omitempty"`
}

type ReplyPrep struct {
	Query       ReplyPrepQuery `json:"query" yaml:"query"`
	Memories    []Memory       `json:"memories,omitempty" yaml:"memories,omitempty"`
	Nudges      []Nudge        `json:"nudges,omitempty" yaml:"nudges,omitempty"`
	Sources     []Evidence     `json:"sources,omitempty" yaml:"sources,omitempty"`
	GeneratedAt time.Time      `json:"generated_at" yaml:"generated_at"`
}

func PrepareMemoryForAppend(m Memory, now time.Time) Memory {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.CreatedAt
	}
	if m.LastActivityAt.IsZero() {
		m.LastActivityAt = latestEvidenceDate(m.Evidence, m.CreatedAt)
	}
	if strings.TrimSpace(m.PromptVersion) == "" {
		m.PromptVersion = PromptVersionHeuristicV1
	}
	if strings.TrimSpace(m.Status) == "" {
		m.Status = StatusActive
	}
	if strings.TrimSpace(m.Freshness) == "" {
		m.Freshness = FreshnessFresh
	}
	if m.Details.SourceCount == 0 {
		m.Details.SourceCount = len(m.Evidence)
	}
	m.People = CompactStrings(m.People)
	m.Tags = CompactStrings(m.Tags)
	m.Links = CompactStrings(m.Links)
	if strings.TrimSpace(m.ID) == "" {
		m.ID = DeterministicID(m)
	}
	return m
}

func DeterministicID(m Memory) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(m.Kind)),
		normalizeComparable(m.Claim),
		normalizeComparable(m.Topic),
		normalizeComparable(m.Company),
		normalizeComparable(m.Domain),
	}
	people := append([]string(nil), m.People...)
	sort.Strings(people)
	for _, person := range people {
		parts = append(parts, normalizeComparable(person))
	}
	for _, evidence := range m.Evidence {
		parts = append(parts,
			strings.ToLower(strings.TrimSpace(evidence.SourceType)),
			normalizeComparable(firstNonEmpty(evidence.MessageID, evidence.ID, evidence.Path, evidence.URL)),
			evidence.Date.UTC().Format(time.RFC3339),
		)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return "mem_" + hex.EncodeToString(sum[:])[:20]
}

func MemoryMatches(m Memory, q Query) bool {
	if q.MinConfidence > 0 && m.Confidence < q.MinConfidence && !q.IncludeLowConfidence {
		return false
	}
	if q.Kind != "" && !strings.EqualFold(strings.TrimSpace(m.Kind), strings.TrimSpace(q.Kind)) {
		return false
	}
	if q.Status != "" && !strings.EqualFold(strings.TrimSpace(m.Status), strings.TrimSpace(q.Status)) {
		return false
	}
	if q.Company != "" && !containsFold(m.Company, q.Company) {
		return false
	}
	if q.Domain != "" && !containsFold(m.Domain, q.Domain) {
		return false
	}
	if q.Topic != "" && !containsFold(m.Topic, q.Topic) && !containsFold(q.Topic, m.Topic) && !containsFold(m.Claim, q.Topic) {
		return false
	}
	if len(q.People) > 0 {
		matchedPerson := false
		for _, person := range q.People {
			if person = strings.TrimSpace(person); person != "" && memoryHasPerson(m, person) {
				matchedPerson = true
				break
			}
		}
		if !matchedPerson {
			return false
		}
	}
	text := strings.TrimSpace(q.Text)
	if text != "" && !memoryContainsText(m, text) {
		return false
	}
	return true
}

func SortMemoriesNewestFirst(memories []Memory) {
	sort.SliceStable(memories, func(i, j int) bool {
		a := memories[i].LastActivityAt
		if a.IsZero() {
			a = memories[i].CreatedAt
		}
		b := memories[j].LastActivityAt
		if b.IsZero() {
			b = memories[j].CreatedAt
		}
		if a.Equal(b) {
			return memories[i].ID < memories[j].ID
		}
		return a.After(b)
	})
}

func CompactStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func bounded(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func latestEvidenceDate(evidence []Evidence, fallback time.Time) time.Time {
	latest := fallback
	for _, item := range evidence {
		if !item.Date.IsZero() && (latest.IsZero() || item.Date.After(latest)) {
			latest = item.Date
		}
	}
	return latest
}

func memoryHasPerson(m Memory, person string) bool {
	if containsFold(m.Claim, person) || containsFold(m.Summary, person) {
		return true
	}
	for _, candidate := range m.People {
		if containsFold(candidate, person) || containsFold(person, candidate) {
			return true
		}
	}
	for _, evidence := range m.Evidence {
		if containsFold(evidence.Snippet, person) {
			return true
		}
	}
	return false
}

func memoryContainsText(m Memory, text string) bool {
	haystack := strings.Join([]string{
		m.ID,
		m.Kind,
		m.Claim,
		m.Summary,
		m.Topic,
		m.Company,
		m.Domain,
		m.Status,
		strings.Join(m.People, " "),
		strings.Join(m.Tags, " "),
		strings.Join(m.Links, " "),
	}, " ")
	if containsFold(haystack, text) {
		return true
	}
	for _, evidence := range m.Evidence {
		if containsFold(evidence.MessageID, text) || containsFold(evidence.Path, text) ||
			containsFold(evidence.URL, text) || containsFold(evidence.Snippet, text) {
			return true
		}
	}
	return false
}

func containsFold(haystack, needle string) bool {
	haystack = strings.ToLower(strings.TrimSpace(haystack))
	needle = strings.ToLower(strings.TrimSpace(needle))
	return needle == "" || strings.Contains(haystack, needle)
}

func normalizeComparable(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func displayEvidenceLabel(e Evidence) string {
	switch e.SourceType {
	case SourceEmail, SourceSentEmail:
		return strings.TrimSpace(firstNonEmpty(e.MessageID, e.LocalID, e.ID))
	case SourceObsidian:
		return strings.TrimSpace(e.Path)
	case SourceResearch:
		return strings.TrimSpace(e.URL)
	default:
		return strings.TrimSpace(firstNonEmpty(e.ID, e.MessageID, e.Path, e.URL))
	}
}

func nudgeID(memoryID, nudgeType string) string {
	if memoryID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", memoryID, nudgeType)))
	return "nudge_" + hex.EncodeToString(sum[:])[:16]
}
