package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type EmailSource interface {
	MemoryEmails(folder string, limit int) ([]EmailSnapshot, error)
}

type RefreshResult struct {
	ScannedFolders int      `json:"scanned_folders"`
	ScannedEmails  int      `json:"scanned_emails"`
	Extracted      int      `json:"extracted"`
	Written        int      `json:"written"`
	Skipped        int      `json:"skipped"`
	Errors         []string `json:"errors,omitempty"`
}

type Service struct {
	settings Settings
	store    *FileStore
	source   EmailSource
	extract  Extractor
	now      func() time.Time
}

func NewService(settings Settings, source EmailSource) (*Service, error) {
	settings.ApplyDefaults()
	store, err := NewFileStore(settings.Directory)
	if err != nil {
		return nil, err
	}
	return NewServiceWithStore(settings, store, source), nil
}

func NewServiceWithStore(settings Settings, store *FileStore, source EmailSource) *Service {
	settings.ApplyDefaults()
	now := time.Now
	return &Service{
		settings: settings,
		store:    store,
		source:   source,
		now:      now,
		extract: Extractor{
			Now:      now,
			Settings: settings,
		},
	}
}

func (s *Service) Refresh(ctx context.Context) (RefreshResult, error) {
	if err := ctx.Err(); err != nil {
		return RefreshResult{}, err
	}
	if s == nil || s.store == nil {
		return RefreshResult{}, fmt.Errorf("memory service is not configured")
	}
	if !s.settings.Enabled {
		return RefreshResult{}, nil
	}
	if s.source == nil {
		return RefreshResult{}, fmt.Errorf("memory email source is not configured")
	}
	var all []EmailSnapshot
	var result RefreshResult
	limitPerFolder := 250
	for _, folder := range s.settings.Sources.Folders {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			continue
		}
		result.ScannedFolders++
		emails, err := s.source.MemoryEmails(folder, limitPerFolder)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", folder, err))
			continue
		}
		result.ScannedEmails += len(emails)
		all = append(all, emails...)
	}
	memories := s.extract.Extract(all)
	result.Extracted = len(memories)
	for _, memory := range memories {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		_, _, err := s.store.Append(ctx, memory)
		if errors.Is(err, ErrMemoryExists) {
			result.Skipped++
			continue
		}
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		result.Written++
	}
	if len(result.Errors) > 0 && result.Written == 0 && result.Skipped == 0 {
		return result, fmt.Errorf("memory refresh failed: %s", strings.Join(result.Errors, "; "))
	}
	return result, nil
}

func (s *Service) Search(ctx context.Context, q Query) ([]Memory, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("memory service is not configured")
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	return s.store.Search(ctx, q)
}

func (s *Service) BuildReplyPrep(ctx context.Context, q ReplyPrepQuery) (ReplyPrep, error) {
	if s == nil || s.store == nil {
		return ReplyPrep{}, fmt.Errorf("memory service is not configured")
	}
	if q.Limit <= 0 {
		q.Limit = 8
	}
	if q.MinConfidence <= 0 {
		q.MinConfidence = s.settings.Thresholds.ChatRetrieval
	}
	search := Query{
		People:               peopleFromReplyQuery(q),
		Domain:               domainFromSender(q.Recipient),
		Topic:                q.Subject,
		MinConfidence:        q.MinConfidence,
		Limit:                q.Limit,
		IncludeLowConfidence: true,
	}
	memories, err := s.Search(ctx, search)
	if err != nil {
		return ReplyPrep{}, err
	}
	if strings.TrimSpace(q.Subject) != "" {
		byTopic, err := s.Search(ctx, Query{
			Topic:                q.Subject,
			MinConfidence:        q.MinConfidence,
			Limit:                q.Limit,
			IncludeLowConfidence: true,
		})
		if err != nil {
			return ReplyPrep{}, err
		}
		memories = mergeMemoryMatches(memories, byTopic, q.Limit)
	}
	prep := BuildReplyPrepFromMemories(q, memories, s.settings)
	prep.GeneratedAt = s.now()
	return prep, nil
}

func BuildReplyPrepFromMemories(q ReplyPrepQuery, memories []Memory, settings Settings) ReplyPrep {
	settings.ApplyDefaults()
	if q.Limit <= 0 {
		q.Limit = 8
	}
	if len(memories) > q.Limit {
		memories = memories[:q.Limit]
	}
	nudges := nudgesFromMemories(memories, settings)
	return ReplyPrep{
		Query:       q,
		Memories:    memories,
		Nudges:      nudges,
		Sources:     compactEvidence(memories),
		GeneratedAt: time.Now(),
	}
}

func nudgesFromMemories(memories []Memory, settings Settings) []Nudge {
	settings.ApplyDefaults()
	threshold := settings.Thresholds.ComposeRadar
	if threshold <= 0 {
		threshold = 0.75
	}
	dismissScope := normalizeNudgeDismissScope(settings.UpdateRules.DismissalScope)
	var nudges []Nudge
	for _, memory := range memories {
		if memory.Confidence < threshold || memory.Status == StatusSourceMissing {
			continue
		}
		nudgeType, message, why := nudgeText(memory)
		if message == "" {
			continue
		}
		nudges = append(nudges, Nudge{
			ID:             nudgeID(memory.ID, nudgeType),
			Type:           nudgeType,
			Message:        message,
			Why:            why,
			Confidence:     memory.Confidence,
			MemoryIDs:      []string{memory.ID},
			Evidence:       memory.Evidence,
			ActionState:    NudgeActionNew,
			DismissalScope: dismissScope,
		})
		if len(nudges) >= 3 {
			break
		}
	}
	return nudges
}

func nudgeText(memory Memory) (string, string, string) {
	summary := firstNonEmpty(memory.Summary, memory.Claim)
	if memory.Status == StatusConflict {
		return NudgeTypeConflict, "Conflict: " + summary, "Source-backed memories may disagree; clarify before sending."
	}
	switch memory.Kind {
	case KindOpenQuestion:
		return NudgeTypeOpenLoop, "Open question: " + summary, "This thread may still need a response."
	case KindDeadline, KindCommitment:
		return NudgeTypeDraftRisk, "Draft risk: " + summary, "A commitment or date-sensitive detail could affect the reply."
	case KindLastUserReply:
		return NudgeTypeCallback, "Already replied: " + summary, "Your previous reply may affect whether to follow up or wait."
	case KindTrackStatus:
		if memory.Status == StatusWaiting {
			return NudgeTypeCallback, summary, "The active track may be waiting on someone."
		}
		if memory.Status == StatusStale {
			return NudgeTypeCallback, summary, "This track may be stale enough to ask for an update."
		}
		return NudgeTypeRelationshipContext, summary, "Recent track status may matter."
	case KindRelationshipContext, KindLastContact:
		return NudgeTypeRelationshipContext, summary, "Recent relationship context may matter."
	case KindResearchNote:
		return NudgeTypeResearchUpdate, "Research update: " + summary, "Recent public research may change how you frame the reply."
	default:
		return "", "", ""
	}
}

func normalizeNudgeDismissScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case NudgeDismissDraft:
		return NudgeDismissDraft
	case NudgeDismissPerson:
		return NudgeDismissPerson
	case NudgeDismissPermanent:
		return NudgeDismissPermanent
	default:
		return NudgeDismissThread
	}
}

func compactEvidence(memories []Memory) []Evidence {
	seen := make(map[string]bool)
	var out []Evidence
	for _, memory := range memories {
		for _, evidence := range memory.Evidence {
			key := strings.Join([]string{evidence.SourceType, displayEvidenceLabel(evidence)}, ":")
			if key == ":" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, evidence)
		}
	}
	return out
}

func mergeMemoryMatches(primary, secondary []Memory, limit int) []Memory {
	if limit <= 0 {
		limit = 20
	}
	seen := make(map[string]bool, len(primary)+len(secondary))
	out := make([]Memory, 0, len(primary)+len(secondary))
	for _, memory := range append(primary, secondary...) {
		key := strings.TrimSpace(memory.ID)
		if key == "" {
			key = DeterministicID(memory)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, memory)
	}
	SortMemoriesNewestFirst(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func peopleFromReplyQuery(q ReplyPrepQuery) []string {
	var people []string
	if recipient := strings.TrimSpace(q.Recipient); recipient != "" {
		people = append(people, recipient)
		if match := emailAddrPattern.FindString(recipient); match != "" {
			people = append(people, match)
		}
	}
	return CompactStrings(people)
}
