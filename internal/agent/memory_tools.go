package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/core"
	"github.com/herald-email/herald-mail-app/internal/memory"
)

const defaultMemoryToolMaxResults = 12

type MemoryToolSource interface {
	SearchMemories(ctx context.Context, query memory.Query) ([]memory.Memory, error)
	BuildReplyMemoryContext(ctx context.Context, query memory.ReplyPrepQuery) (memory.ReplyPrep, error)
}

type MemoryToolOptions struct {
	MaxResults      int
	ChatMinScore    float64
	ComposeMinScore float64
}

type MemoryToolService struct {
	source MemoryToolSource
	opts   MemoryToolOptions
}

type SearchMemoriesParams struct {
	Query         string   `json:"query,omitempty" jsonschema:"description=Search text for memory claims, summaries, topics, source snippets, or source IDs"`
	Person        string   `json:"person,omitempty" jsonschema:"description=Person name or email to retrieve memories about"`
	People        []string `json:"people,omitempty" jsonschema:"description=People names or emails to retrieve memories about"`
	Company       string   `json:"company,omitempty" jsonschema:"description=Company name to retrieve memories about"`
	Domain        string   `json:"domain,omitempty" jsonschema:"description=Email or company domain to retrieve memories about"`
	Topic         string   `json:"topic,omitempty" jsonschema:"description=Thread, project, job-search, or relationship topic"`
	Kind          string   `json:"kind,omitempty" jsonschema:"description=Memory kind such as last_contact, last_user_reply, open_question, commitment, deadline, track_status"`
	Status        string   `json:"status,omitempty" jsonschema:"description=Memory state such as active, waiting, resolved, stale, conflict, source_missing"`
	MinConfidence float64  `json:"min_confidence,omitempty" jsonschema:"description=Minimum confidence from 0.0 to 1.0"`
	Limit         int      `json:"limit,omitempty" jsonschema:"description=Maximum memories to return"`
}

type MemorySearchResult struct {
	Tool     string          `json:"tool"`
	Query    memory.Query    `json:"query"`
	Total    int             `json:"total"`
	Capped   bool            `json:"capped"`
	Memories []memory.Memory `json:"memories"`
}

type ContactHistoryParams struct {
	Person        string  `json:"person" jsonschema:"description=Person name or email"`
	Limit         int     `json:"limit,omitempty" jsonschema:"description=Maximum memories to return"`
	MinConfidence float64 `json:"min_confidence,omitempty" jsonschema:"description=Minimum confidence from 0.0 to 1.0"`
}

type CompanyTracksParams struct {
	Company       string  `json:"company,omitempty" jsonschema:"description=Company name"`
	Domain        string  `json:"domain,omitempty" jsonschema:"description=Company email/domain"`
	Limit         int     `json:"limit,omitempty" jsonschema:"description=Maximum memories to return"`
	MinConfidence float64 `json:"min_confidence,omitempty" jsonschema:"description=Minimum confidence from 0.0 to 1.0"`
}

type OpenLoopsParams struct {
	Person        string  `json:"person,omitempty" jsonschema:"description=Person name or email"`
	Company       string  `json:"company,omitempty" jsonschema:"description=Company name"`
	Topic         string  `json:"topic,omitempty" jsonschema:"description=Thread, project, or job-search topic"`
	Limit         int     `json:"limit,omitempty" jsonschema:"description=Maximum memories to return"`
	MinConfidence float64 `json:"min_confidence,omitempty" jsonschema:"description=Minimum confidence from 0.0 to 1.0"`
}

type ReplyMemoryContextParams struct {
	Recipient     string  `json:"recipient,omitempty" jsonschema:"description=Recipient name or email from the active compose draft"`
	Subject       string  `json:"subject,omitempty" jsonschema:"description=Subject from the active compose draft"`
	DraftExcerpt  string  `json:"draft_excerpt,omitempty" jsonschema:"description=Bounded excerpt from the active draft body"`
	MessageID     string  `json:"message_id,omitempty" jsonschema:"description=Source message ID when replying"`
	Limit         int     `json:"limit,omitempty" jsonschema:"description=Maximum memories to consider"`
	MinConfidence float64 `json:"min_confidence,omitempty" jsonschema:"description=Minimum memory confidence"`
}

func NewMemoryToolService(source MemoryToolSource, opts MemoryToolOptions) *MemoryToolService {
	if opts.MaxResults <= 0 {
		opts.MaxResults = defaultMemoryToolMaxResults
	}
	if opts.ChatMinScore <= 0 {
		opts.ChatMinScore = 0.35
	}
	if opts.ComposeMinScore <= 0 {
		opts.ComposeMinScore = 0.75
	}
	return &MemoryToolService{source: source, opts: opts}
}

func (s *MemoryToolService) GollemTools() []core.Tool {
	if s == nil {
		return nil
	}
	return []core.Tool{
		core.FuncTool[SearchMemoriesParams](
			"search_memories",
			"Search immutable Herald Memories by person, company, topic, kind, status, confidence, or source text. Read-only and source-backed.",
			func(ctx context.Context, _ *core.RunContext, params SearchMemoriesParams) (MemorySearchResult, error) {
				return s.SearchMemories(ctx, params)
			},
		),
		core.FuncTool[ContactHistoryParams](
			"get_contact_history",
			"Return source-backed memories for a person or contact. Use for relationship history and callbacks.",
			func(ctx context.Context, _ *core.RunContext, params ContactHistoryParams) (MemorySearchResult, error) {
				return s.ContactHistory(ctx, params)
			},
		),
		core.FuncTool[CompanyTracksParams](
			"get_company_tracks",
			"Return source-backed company or job-search track memories.",
			func(ctx context.Context, _ *core.RunContext, params CompanyTracksParams) (MemorySearchResult, error) {
				return s.CompanyTracks(ctx, params)
			},
		),
		core.FuncTool[OpenLoopsParams](
			"get_open_loops",
			"Return waiting/open-question memories for a person, company, or topic.",
			func(ctx context.Context, _ *core.RunContext, params OpenLoopsParams) (MemorySearchResult, error) {
				return s.OpenLoops(ctx, params)
			},
		),
		core.FuncTool[ReplyMemoryContextParams](
			"get_reply_memory_context",
			"Return at most three high-confidence Compose Radar nudges and supporting memories for an active reply draft.",
			func(ctx context.Context, rc *core.RunContext, params ReplyMemoryContextParams) (memory.ReplyPrep, error) {
				params = s.paramsFromRun(rc, params)
				return s.ReplyMemoryContext(ctx, params)
			},
		),
	}
}

func (s *MemoryToolService) SearchMemories(ctx context.Context, params SearchMemoriesParams) (MemorySearchResult, error) {
	if s == nil || s.source == nil {
		return MemorySearchResult{}, fmt.Errorf("memory tools are not configured")
	}
	query := memory.Query{
		Text:          strings.TrimSpace(params.Query),
		People:        memory.CompactStrings(append(params.People, params.Person)),
		Company:       strings.TrimSpace(params.Company),
		Domain:        strings.TrimSpace(params.Domain),
		Topic:         strings.TrimSpace(params.Topic),
		Kind:          strings.TrimSpace(params.Kind),
		Status:        strings.TrimSpace(params.Status),
		MinConfidence: minConfidence(params.MinConfidence, s.opts.ChatMinScore),
		Limit:         s.limit(params.Limit),
	}
	memories, err := s.source.SearchMemories(ctx, query)
	if err != nil {
		return MemorySearchResult{}, err
	}
	total := len(memories)
	if len(memories) > query.Limit {
		memories = memories[:query.Limit]
	}
	return MemorySearchResult{
		Tool:     "search_memories",
		Query:    query,
		Total:    total,
		Capped:   total > len(memories),
		Memories: memories,
	}, nil
}

func (s *MemoryToolService) ContactHistory(ctx context.Context, params ContactHistoryParams) (MemorySearchResult, error) {
	return s.SearchMemories(ctx, SearchMemoriesParams{
		Person:        params.Person,
		Limit:         params.Limit,
		MinConfidence: params.MinConfidence,
	})
}

func (s *MemoryToolService) CompanyTracks(ctx context.Context, params CompanyTracksParams) (MemorySearchResult, error) {
	return s.SearchMemories(ctx, SearchMemoriesParams{
		Company:       params.Company,
		Domain:        params.Domain,
		Kind:          memory.KindTrackStatus,
		Limit:         params.Limit,
		MinConfidence: params.MinConfidence,
	})
}

func (s *MemoryToolService) OpenLoops(ctx context.Context, params OpenLoopsParams) (MemorySearchResult, error) {
	status := memory.StatusWaiting
	kind := memory.KindOpenQuestion
	return s.SearchMemories(ctx, SearchMemoriesParams{
		Person:        params.Person,
		Company:       params.Company,
		Topic:         params.Topic,
		Kind:          kind,
		Status:        status,
		Limit:         params.Limit,
		MinConfidence: params.MinConfidence,
	})
}

func (s *MemoryToolService) ReplyMemoryContext(ctx context.Context, params ReplyMemoryContextParams) (memory.ReplyPrep, error) {
	if s == nil || s.source == nil {
		return memory.ReplyPrep{}, fmt.Errorf("memory tools are not configured")
	}
	query := memory.ReplyPrepQuery{
		Recipient:     strings.TrimSpace(params.Recipient),
		Subject:       strings.TrimSpace(params.Subject),
		DraftExcerpt:  strings.TrimSpace(params.DraftExcerpt),
		MessageID:     strings.TrimSpace(params.MessageID),
		Limit:         s.limit(params.Limit),
		MinConfidence: minConfidence(params.MinConfidence, s.opts.ComposeMinScore),
	}
	return s.source.BuildReplyMemoryContext(ctx, query)
}

func (s *MemoryToolService) paramsFromRun(rc *core.RunContext, params ReplyMemoryContextParams) ReplyMemoryContextParams {
	if input, ok := core.TryGetDeps[ChatInput](rc); ok && input.ComposeSnapshot != nil {
		if strings.TrimSpace(params.Recipient) == "" {
			params.Recipient = input.ComposeSnapshot.To
		}
		if strings.TrimSpace(params.Subject) == "" {
			params.Subject = input.ComposeSnapshot.Subject
		}
		if strings.TrimSpace(params.DraftExcerpt) == "" {
			params.DraftExcerpt = boundedText(input.ComposeSnapshot.Body, 1000)
		}
	}
	return params
}

func (s *MemoryToolService) limit(requested int) int {
	limit := s.opts.MaxResults
	if requested > 0 && requested < limit {
		limit = requested
	}
	if limit <= 0 {
		return defaultMemoryToolMaxResults
	}
	return limit
}

func minConfidence(requested, fallback float64) float64 {
	if requested > 0 {
		return requested
	}
	return fallback
}
