package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

type EmailSource interface {
	MemoryEmails(folder string, limit int) ([]EmailSnapshot, error)
}

type CalendarSource interface {
	MemoryCalendarEvents(start, end time.Time, limit int) ([]models.CalendarEvent, error)
}

type SourceBundle struct {
	Email    EmailSource
	Calendar CalendarSource
}

type RefreshResult struct {
	ScannedFolders        int      `json:"scanned_folders"`
	ScannedEmails         int      `json:"scanned_emails"`
	ScannedCalendarEvents int      `json:"scanned_calendar_events"`
	ScannedObsidianNotes  int      `json:"scanned_obsidian_notes"`
	ScannedResearchNotes  int      `json:"scanned_research_notes"`
	Extracted             int      `json:"extracted"`
	Written               int      `json:"written"`
	Skipped               int      `json:"skipped"`
	Errors                []string `json:"errors,omitempty"`
}

type Service struct {
	settings Settings
	store    *FileStore
	source   EmailSource
	calendar CalendarSource
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
	return NewServiceWithSourceBundle(settings, store, SourceBundle{Email: source})
}

func NewServiceWithSourceBundle(settings Settings, store *FileStore, sources SourceBundle) *Service {
	settings.ApplyDefaults()
	now := time.Now
	return &Service{
		settings: settings,
		store:    store,
		source:   sources.Email,
		calendar: sources.Calendar,
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
	var all []EmailSnapshot
	var result RefreshResult
	limitPerFolder := 250
	if s.source == nil {
		result.Errors = append(result.Errors, "memory email source is not configured")
	} else {
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
	}
	memories := s.extract.Extract(all)
	if s.settings.Sources.Calendar {
		calendarMemories, count, err := s.refreshCalendarMemories(ctx)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
		} else {
			result.ScannedCalendarEvents = count
			memories = append(memories, calendarMemories...)
		}
	}
	if s.settings.Sources.Obsidian {
		notes, err := LoadConfiguredObsidianNotes(ctx, s.settings)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Obsidian notes: %v", err))
		} else {
			result.ScannedObsidianNotes = len(notes)
			memories = append(memories, s.extract.ExtractObsidianNotes(notes)...)
		}
	}
	if s.settings.Sources.ResearchNotes {
		notes, err := LoadConfiguredResearchNotes(ctx, s.settings)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("research notes: %v", err))
		} else {
			result.ScannedResearchNotes = len(notes)
			memories = append(memories, s.extract.ExtractResearchNotes(notes)...)
		}
	}
	memories = dedupeMemories(memories)
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

func (s *Service) refreshCalendarMemories(ctx context.Context) ([]Memory, int, error) {
	if s.calendar == nil {
		return nil, 0, fmt.Errorf("calendar memory source is not configured")
	}
	start, end := s.calendarMemoryWindow()
	events, err := s.calendar.MemoryCalendarEvents(start, end, 250)
	if err != nil {
		return nil, 0, fmt.Errorf("calendar events: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, len(events), err
	}
	return s.extract.ExtractCalendarEvents(events), len(events), nil
}

func (s *Service) calendarMemoryWindow() (time.Time, time.Time) {
	now := s.now()
	if now.IsZero() {
		now = time.Now()
	}
	lookback := s.settings.Sources.CalendarLookbackDays
	if lookback <= 0 {
		lookback = 30
	}
	lookahead := s.settings.Sources.CalendarLookaheadDays
	if lookahead <= 0 {
		lookahead = 90
	}
	return now.AddDate(0, 0, -lookback), now.AddDate(0, 0, lookahead)
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
	controls, err := s.store.ControlState(ctx, s.now())
	if err != nil {
		return ReplyPrep{}, err
	}
	prep := BuildReplyPrepFromMemoriesWithControls(q, memories, s.settings, controls)
	prep.GeneratedAt = s.now()
	return prep, nil
}

func BuildReplyPrepFromMemories(q ReplyPrepQuery, memories []Memory, settings Settings) ReplyPrep {
	return BuildReplyPrepFromMemoriesWithControls(q, memories, settings, ControlState{})
}

func BuildReplyPrepFromMemoriesWithControls(q ReplyPrepQuery, memories []Memory, settings Settings, controls ControlState) ReplyPrep {
	settings.ApplyDefaults()
	memories = ApplyControlState(memories, controls, settings)
	if q.Limit <= 0 {
		q.Limit = 8
	}
	if len(memories) > q.Limit {
		memories = memories[:q.Limit]
	}
	nudges := nudgesFromMemories(memories, settings)
	if len(controls.Dismissals) > 0 {
		nudges = filterDismissedNudges(nudges, controls, q)
	}
	return ReplyPrep{
		Query:       q,
		Memories:    memories,
		Nudges:      nudges,
		Sources:     compactEvidence(memories),
		GeneratedAt: time.Now(),
	}
}

func (s *Service) DismissNudge(ctx context.Context, req NudgeDismissalRequest) (ControlEvent, error) {
	if s == nil || s.store == nil {
		return ControlEvent{}, fmt.Errorf("memory service is not configured")
	}
	if req.RetentionDays == 0 {
		req.RetentionDays = s.settings.UpdateRules.RetentionDays
	}
	if req.Now.IsZero() {
		req.Now = s.now()
	}
	return s.store.AppendControlEvent(ctx, NewNudgeDismissalEvent(req))
}

func (s *Service) MarkSourceMissing(ctx context.Context, evidence Evidence, reason string) (ControlEvent, error) {
	if s == nil || s.store == nil {
		return ControlEvent{}, fmt.Errorf("memory service is not configured")
	}
	return s.store.AppendControlEvent(ctx, NewSourceMissingEvent(evidence, reason, s.now()))
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

func filterDismissedNudges(nudges []Nudge, controls ControlState, q ReplyPrepQuery) []Nudge {
	out := nudges[:0:0]
	person := strings.TrimSpace(q.Recipient)
	threadID := firstNonEmpty(q.MessageID, q.Subject)
	for _, nudge := range nudges {
		if NudgeDismissed(nudge, controls, threadID, person) {
			continue
		}
		out = append(out, nudge)
	}
	return out
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
