package memory

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	ResearchActionPerson         = "research_person"
	ResearchActionCompany        = "research_company"
	ResearchActionRefreshDossier = "refresh_dossier"
	ResearchActionBeforeReply    = "research_before_reply"
)

var ErrResearchURLRequired = errors.New("research source URL is required")

type ResearchModeRequest struct {
	Action              string   `json:"action" yaml:"action"`
	PersonName          string   `json:"person_name,omitempty" yaml:"person_name,omitempty"`
	Company             string   `json:"company,omitempty" yaml:"company,omitempty"`
	Domain              string   `json:"domain,omitempty" yaml:"domain,omitempty"`
	Role                string   `json:"role,omitempty" yaml:"role,omitempty"`
	URL                 string   `json:"url,omitempty" yaml:"url,omitempty"`
	PublicIdentifiers   []string `json:"public_identifiers,omitempty" yaml:"public_identifiers,omitempty"`
	PrivateBodyText     string   `json:"private_body_text,omitempty" yaml:"private_body_text,omitempty"`
	PrivateNoteText     string   `json:"private_note_text,omitempty" yaml:"private_note_text,omitempty"`
	AttachmentSummary   string   `json:"attachment_summary,omitempty" yaml:"attachment_summary,omitempty"`
	FullThreadSummary   string   `json:"full_thread_summary,omitempty" yaml:"full_thread_summary,omitempty"`
	AllowPrivateContext bool     `json:"allow_private_context,omitempty" yaml:"allow_private_context,omitempty"`
}

type ResearchQuery struct {
	Query             string   `json:"query" yaml:"query"`
	Purpose           string   `json:"purpose" yaml:"purpose"`
	PublicIdentifiers []string `json:"public_identifiers" yaml:"public_identifiers"`
}

type ResearchModePlan struct {
	Action                 string          `json:"action" yaml:"action"`
	ExternalOptIn          bool            `json:"external_opt_in" yaml:"external_opt_in"`
	PrivateContextAllowed  bool            `json:"private_context_allowed" yaml:"private_context_allowed"`
	Ready                  bool            `json:"ready" yaml:"ready"`
	Reason                 string          `json:"reason,omitempty" yaml:"reason,omitempty"`
	StaleAfterDays         int             `json:"stale_after_days" yaml:"stale_after_days"`
	Queries                []ResearchQuery `json:"queries,omitempty" yaml:"queries,omitempty"`
	PublicIdentifiers      []string        `json:"public_identifiers,omitempty" yaml:"public_identifiers,omitempty"`
	BlockedPrivateContext  []string        `json:"blocked_private_context,omitempty" yaml:"blocked_private_context,omitempty"`
	ApprovedPrivateContext []string        `json:"approved_private_context,omitempty" yaml:"approved_private_context,omitempty"`
}

type ResearchNoteInput struct {
	Action         string    `json:"action,omitempty" yaml:"action,omitempty"`
	PersonName     string    `json:"person_name,omitempty" yaml:"person_name,omitempty"`
	Company        string    `json:"company,omitempty" yaml:"company,omitempty"`
	Domain         string    `json:"domain,omitempty" yaml:"domain,omitempty"`
	Title          string    `json:"title,omitempty" yaml:"title,omitempty"`
	Summary        string    `json:"summary" yaml:"summary"`
	WhatChanged    string    `json:"what_changed,omitempty" yaml:"what_changed,omitempty"`
	URL            string    `json:"url" yaml:"url"`
	Query          string    `json:"query,omitempty" yaml:"query,omitempty"`
	RetrievedAt    time.Time `json:"retrieved_at,omitempty" yaml:"retrieved_at,omitempty"`
	Confidence     float64   `json:"confidence,omitempty" yaml:"confidence,omitempty"`
	StaleAfterDays int       `json:"stale_after_days,omitempty" yaml:"stale_after_days,omitempty"`
}

func BuildResearchModePlan(request ResearchModeRequest, settings Settings) ResearchModePlan {
	settings.ApplyDefaults()
	action := normalizeResearchAction(request.Action, request)
	publicIDs := researchPublicIdentifiers(request)
	privateAllowed := settings.Research.PrivateBodiesAllowed && request.AllowPrivateContext
	blocked, approvedPrivate := researchPrivateContext(request, privateAllowed)
	plan := ResearchModePlan{
		Action:                 action,
		ExternalOptIn:          settings.Research.ExternalOptIn,
		PrivateContextAllowed:  privateAllowed,
		StaleAfterDays:         settings.Research.StaleAfterDays,
		PublicIdentifiers:      publicIDs,
		BlockedPrivateContext:  blocked,
		ApprovedPrivateContext: approvedPrivate,
	}
	if len(publicIDs) > 0 {
		plan.Queries = researchQueries(action, publicIDs)
	}
	plan.Ready = settings.Research.Enabled && settings.Research.ExternalOptIn && len(plan.Queries) > 0
	if len(plan.Queries) == 0 {
		plan.Reason = "no public identifiers available"
	} else if !settings.Research.Enabled {
		plan.Reason = "research mode is disabled"
	} else if !settings.Research.ExternalOptIn {
		plan.Reason = "external research requires explicit opt-in"
	}
	return plan
}

func BuildResearchMemory(input ResearchNoteInput, settings Settings) (Memory, error) {
	settings.ApplyDefaults()
	url := strings.TrimSpace(input.URL)
	if url == "" {
		return Memory{}, ErrResearchURLRequired
	}
	retrievedAt := input.RetrievedAt
	if retrievedAt.IsZero() {
		retrievedAt = time.Now()
	}
	confidence := input.Confidence
	if confidence <= 0 {
		confidence = settings.Thresholds.Dossier
	}
	title := firstNonEmpty(input.Title, input.Summary, input.WhatChanged, url)
	summary := firstNonEmpty(input.Summary, input.WhatChanged, title)
	claim := summary
	if strings.TrimSpace(input.WhatChanged) != "" {
		claim = summary + " What changed since last contact: " + strings.TrimSpace(input.WhatChanged)
	}
	staleAfterDays := input.StaleAfterDays
	if staleAfterDays <= 0 {
		staleAfterDays = settings.Research.StaleAfterDays
	}
	mem := Memory{
		Kind:           KindResearchNote,
		Claim:          bounded(claim, 600),
		Summary:        bounded(summary, 300),
		Topic:          firstNonEmpty(input.Company, input.Domain, input.PersonName, "Research"),
		People:         CompactStrings([]string{input.PersonName}),
		Company:        strings.TrimSpace(input.Company),
		Domain:         strings.TrimSpace(input.Domain),
		Status:         StatusActive,
		Freshness:      ResearchFreshness(retrievedAt, time.Now(), staleAfterDays),
		Confidence:     confidence,
		LastActivityAt: retrievedAt,
		Tags:           []string{"research"},
		Links:          []string{url},
		ObsidianTarget: researchObsidianTarget(input, settings),
		Evidence: []Evidence{{
			SourceType: SourceResearch,
			URL:        url,
			Date:       retrievedAt,
			Snippet:    bounded(summary, 280),
		}},
		Details: MemoryDetails{
			GeneratedSummary: bounded(summary, 300),
			SourceQuote:      bounded(input.WhatChanged, 300),
			SourceCount:      1,
			ExtractionPrompt: "research-note-summarization",
			LastValidatedAt:  retrievedAt,
			SourceSignals: CompactStrings([]string{
				"from public research",
				"query: " + strings.TrimSpace(input.Query),
			}),
		},
	}
	return mem, nil
}

func (s *FileStore) AppendResearchNote(ctx context.Context, input ResearchNoteInput, settings Settings) (Memory, string, error) {
	if s == nil {
		return Memory{}, "", fmt.Errorf("memory store is nil")
	}
	mem, err := BuildResearchMemory(input, settings)
	if err != nil {
		return Memory{}, "", err
	}
	return s.Append(ctx, mem)
}

func ResearchFreshness(retrievedAt, now time.Time, staleAfterDays int) string {
	if staleAfterDays <= 0 {
		staleAfterDays = 30
	}
	if retrievedAt.IsZero() || now.IsZero() {
		return FreshnessFresh
	}
	if now.Sub(retrievedAt) > time.Duration(staleAfterDays)*24*time.Hour {
		return FreshnessStale
	}
	return FreshnessFresh
}

func normalizeResearchAction(action string, request ResearchModeRequest) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case ResearchActionCompany, "company":
		return ResearchActionCompany
	case ResearchActionRefreshDossier, "refresh":
		return ResearchActionRefreshDossier
	case ResearchActionBeforeReply, "before_reply", "reply":
		return ResearchActionBeforeReply
	case ResearchActionPerson, "person":
		return ResearchActionPerson
	default:
		if strings.TrimSpace(request.Company) != "" || strings.TrimSpace(request.Domain) != "" {
			return ResearchActionCompany
		}
		return ResearchActionPerson
	}
}

func researchPublicIdentifiers(request ResearchModeRequest) []string {
	values := append([]string{}, request.PublicIdentifiers...)
	values = append(values, request.PersonName, request.Role, request.Company, request.Domain, request.URL)
	return CompactStrings(oneLineStrings(values))
}

func researchPrivateContext(request ResearchModeRequest, allowed bool) ([]string, []string) {
	candidates := []struct {
		name  string
		value string
	}{
		{"private_body_text", request.PrivateBodyText},
		{"private_note_text", request.PrivateNoteText},
		{"attachment_summary", request.AttachmentSummary},
		{"full_thread_summary", request.FullThreadSummary},
	}
	var blocked []string
	var approved []string
	for _, candidate := range candidates {
		value := strings.TrimSpace(candidate.value)
		if value == "" {
			continue
		}
		if allowed {
			approved = append(approved, candidate.name+": "+bounded(value, 240))
		} else {
			blocked = append(blocked, candidate.name)
		}
	}
	return blocked, approved
}

func researchQueries(action string, publicIDs []string) []ResearchQuery {
	publicIDs = CompactStrings(publicIDs)
	if len(publicIDs) == 0 {
		return nil
	}
	primary := strings.Join(publicIDs, " ")
	purpose := "research public updates"
	switch action {
	case ResearchActionPerson:
		purpose = "research this person"
	case ResearchActionCompany:
		purpose = "research this company"
	case ResearchActionBeforeReply:
		purpose = "research before reply"
	case ResearchActionRefreshDossier:
		purpose = "refresh dossier research"
	}
	queries := []ResearchQuery{{
		Query:             primary,
		Purpose:           purpose,
		PublicIdentifiers: publicIDs,
	}}
	if len(publicIDs) > 1 {
		queries = append(queries, ResearchQuery{
			Query:             strings.Join(publicIDs[:min(len(publicIDs), 3)], " ") + " latest",
			Purpose:           "freshness check",
			PublicIdentifiers: publicIDs,
		})
	}
	return queries
}

func researchObsidianTarget(input ResearchNoteInput, settings Settings) string {
	name := firstNonEmpty(input.Company, input.Domain, input.PersonName, "Research")
	return filepath.ToSlash(filepath.Join(settings.Destinations.Research, safeNoteName(name)+".md"))
}

func oneLineStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	}
	return out
}
