package memory

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	controlLogRelPath = "state/control_log.jsonl"

	ControlForget        = "forget"
	ControlPin           = "pin"
	ControlCorrect       = "correct"
	ControlSourceMissing = "source_missing"
	ControlDismissNudge  = "dismiss_nudge"
	ControlResolve       = "resolve"
)

type ControlEvent struct {
	ID              string     `json:"id" yaml:"id"`
	Action          string     `json:"action" yaml:"action"`
	MemoryID        string     `json:"memory_id,omitempty" yaml:"memory_id,omitempty"`
	NudgeID         string     `json:"nudge_id,omitempty" yaml:"nudge_id,omitempty"`
	DismissalScope  string     `json:"dismissal_scope,omitempty" yaml:"dismissal_scope,omitempty"`
	ThreadID        string     `json:"thread_id,omitempty" yaml:"thread_id,omitempty"`
	Person          string     `json:"person,omitempty" yaml:"person,omitempty"`
	CorrectedClaim  string     `json:"corrected_claim,omitempty" yaml:"corrected_claim,omitempty"`
	Note            string     `json:"note,omitempty" yaml:"note,omitempty"`
	Reason          string     `json:"reason,omitempty" yaml:"reason,omitempty"`
	EvidenceDigest  string     `json:"evidence_digest,omitempty" yaml:"evidence_digest,omitempty"`
	Evidence        []Evidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	CreatedAt       time.Time  `json:"created_at" yaml:"created_at"`
	ExpiresAt       time.Time  `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	RetentionDays   int        `json:"retention_days,omitempty" yaml:"retention_days,omitempty"`
	OriginalMemory  *Memory    `json:"original_memory,omitempty" yaml:"original_memory,omitempty"`
	SourceReference Evidence   `json:"source_reference,omitempty" yaml:"source_reference,omitempty"`
}

type ControlState struct {
	Events        []ControlEvent          `json:"events,omitempty" yaml:"events,omitempty"`
	Forgotten     map[string]ControlEvent `json:"forgotten,omitempty" yaml:"forgotten,omitempty"`
	Pinned        map[string]ControlEvent `json:"pinned,omitempty" yaml:"pinned,omitempty"`
	Corrections   map[string]ControlEvent `json:"corrections,omitempty" yaml:"corrections,omitempty"`
	SourceMissing []ControlEvent          `json:"source_missing,omitempty" yaml:"source_missing,omitempty"`
	Dismissals    []ControlEvent          `json:"dismissals,omitempty" yaml:"dismissals,omitempty"`
}

type UpdateDecision struct {
	Action      string  `json:"action" yaml:"action"`
	ExistingID  string  `json:"existing_id,omitempty" yaml:"existing_id,omitempty"`
	MatchScore  float64 `json:"match_score,omitempty" yaml:"match_score,omitempty"`
	Candidate   Memory  `json:"candidate" yaml:"candidate"`
	ReviewLabel string  `json:"review_label,omitempty" yaml:"review_label,omitempty"`
}

type UpdateRuleAudit struct {
	MatchThreshold           float64 `json:"match_threshold" yaml:"match_threshold"`
	ConflictCreatesState     bool    `json:"conflict_creates_state" yaml:"conflict_creates_state"`
	StaleAfterDays           int     `json:"stale_after_days" yaml:"stale_after_days"`
	RetentionDays            int     `json:"retention_days" yaml:"retention_days"`
	LowConfidenceDisposition string  `json:"low_confidence_disposition" yaml:"low_confidence_disposition"`
	DismissalScope           string  `json:"dismissal_scope" yaml:"dismissal_scope"`
}

type SourceAudit struct {
	MemoryID      string         `json:"memory_id" yaml:"memory_id"`
	Claim         string         `json:"claim" yaml:"claim"`
	Status        string         `json:"status" yaml:"status"`
	Freshness     string         `json:"freshness" yaml:"freshness"`
	Evidence      []Evidence     `json:"evidence" yaml:"evidence"`
	ControlEvents []ControlEvent `json:"control_events,omitempty" yaml:"control_events,omitempty"`
}

type NudgeDismissalRequest struct {
	Nudge          Nudge     `json:"nudge" yaml:"nudge"`
	Scope          string    `json:"scope,omitempty" yaml:"scope,omitempty"`
	ThreadID       string    `json:"thread_id,omitempty" yaml:"thread_id,omitempty"`
	Person         string    `json:"person,omitempty" yaml:"person,omitempty"`
	Now            time.Time `json:"now,omitempty" yaml:"now,omitempty"`
	RetentionDays  int       `json:"retention_days,omitempty" yaml:"retention_days,omitempty"`
	DismissalNote  string    `json:"dismissal_note,omitempty" yaml:"dismissal_note,omitempty"`
	OverrideDigest string    `json:"override_digest,omitempty" yaml:"override_digest,omitempty"`
}

func NewForgetEvent(memoryID, note string, now time.Time) ControlEvent {
	return prepareControlEvent(ControlEvent{Action: ControlForget, MemoryID: memoryID, Note: note, CreatedAt: now})
}

func NewPinEvent(memoryID, note string, now time.Time) ControlEvent {
	return prepareControlEvent(ControlEvent{Action: ControlPin, MemoryID: memoryID, Note: note, CreatedAt: now})
}

func NewCorrectionEvent(memoryID, correctedClaim, note string, now time.Time) ControlEvent {
	return prepareControlEvent(ControlEvent{Action: ControlCorrect, MemoryID: memoryID, CorrectedClaim: correctedClaim, Note: note, CreatedAt: now})
}

func NewSourceMissingEvent(reference Evidence, reason string, now time.Time) ControlEvent {
	return prepareControlEvent(ControlEvent{
		Action:          ControlSourceMissing,
		Reason:          firstNonEmpty(reason, "source missing"),
		SourceReference: NormalizeEvidence(reference),
		Evidence:        []Evidence{NormalizeEvidence(reference)},
		CreatedAt:       now,
	})
}

func NewResolvedEvent(memoryID string, evidence []Evidence, archiveNote string, now time.Time) ControlEvent {
	return prepareControlEvent(ControlEvent{
		Action:    ControlResolve,
		MemoryID:  memoryID,
		Note:      archiveNote,
		Evidence:  NormalizeEvidenceList(evidence),
		CreatedAt: now,
	})
}

func NewNudgeDismissalEvent(req NudgeDismissalRequest) ControlEvent {
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	scope := normalizeNudgeDismissScope(firstNonEmpty(req.Scope, req.Nudge.DismissalScope))
	digest := req.OverrideDigest
	if digest == "" {
		digest = EvidenceDigest(req.Nudge.Evidence)
	}
	event := ControlEvent{
		Action:         ControlDismissNudge,
		NudgeID:        req.Nudge.ID,
		DismissalScope: scope,
		ThreadID:       strings.TrimSpace(req.ThreadID),
		Person:         strings.TrimSpace(req.Person),
		Note:           req.DismissalNote,
		EvidenceDigest: digest,
		Evidence:       NormalizeEvidenceList(req.Nudge.Evidence),
		CreatedAt:      now,
		RetentionDays:  req.RetentionDays,
	}
	if len(req.Nudge.MemoryIDs) > 0 {
		event.MemoryID = req.Nudge.MemoryIDs[0]
	}
	if req.RetentionDays > 0 {
		event.ExpiresAt = now.Add(time.Duration(req.RetentionDays) * 24 * time.Hour)
	}
	return prepareControlEvent(event)
}

func prepareControlEvent(event ControlEvent) ControlEvent {
	event.Action = strings.ToLower(strings.TrimSpace(event.Action))
	event.MemoryID = strings.TrimSpace(event.MemoryID)
	event.NudgeID = strings.TrimSpace(event.NudgeID)
	event.DismissalScope = normalizeNudgeDismissScope(event.DismissalScope)
	if event.Action != ControlDismissNudge && event.DismissalScope == NudgeDismissThread {
		event.DismissalScope = ""
	}
	event.ThreadID = strings.TrimSpace(event.ThreadID)
	event.Person = strings.TrimSpace(event.Person)
	event.CorrectedClaim = bounded(event.CorrectedClaim, 500)
	event.Note = bounded(event.Note, 500)
	event.Reason = bounded(event.Reason, 240)
	event.Evidence = NormalizeEvidenceList(event.Evidence)
	event.SourceReference = NormalizeEvidence(event.SourceReference)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	if event.ID == "" {
		event.ID = deterministicControlID(event)
	}
	return event
}

func (s *FileStore) AppendControlEvent(ctx context.Context, event ControlEvent) (ControlEvent, error) {
	if err := ctx.Err(); err != nil {
		return ControlEvent{}, err
	}
	if s == nil {
		return ControlEvent{}, fmt.Errorf("memory store is nil")
	}
	event = prepareControlEvent(event)
	path := filepath.Join(s.root, controlLogRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return ControlEvent{}, err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return ControlEvent{}, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return ControlEvent{}, err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return ControlEvent{}, err
	}
	return event, nil
}

func (s *FileStore) ReadControlEvents(ctx context.Context) ([]ControlEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("memory store is nil")
	}
	path := filepath.Join(s.root, controlLogRelPath)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var events []ControlEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event ControlEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, err
		}
		events = append(events, prepareControlEvent(event))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *FileStore) ControlState(ctx context.Context, now time.Time) (ControlState, error) {
	events, err := s.ReadControlEvents(ctx)
	if err != nil {
		return ControlState{}, err
	}
	return BuildControlState(events, now), nil
}

func (s *FileStore) EffectiveList(ctx context.Context, settings Settings) ([]Memory, error) {
	memories, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.ControlState(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	return ApplyControlState(memories, state, settings), nil
}

func BuildControlState(events []ControlEvent, now time.Time) ControlState {
	if now.IsZero() {
		now = time.Now()
	}
	state := ControlState{
		Forgotten:   make(map[string]ControlEvent),
		Pinned:      make(map[string]ControlEvent),
		Corrections: make(map[string]ControlEvent),
	}
	for _, event := range events {
		event = prepareControlEvent(event)
		if controlExpired(event, now) {
			continue
		}
		state.Events = append(state.Events, event)
		switch event.Action {
		case ControlForget:
			if event.MemoryID != "" {
				state.Forgotten[event.MemoryID] = event
			}
		case ControlPin:
			if event.MemoryID != "" {
				state.Pinned[event.MemoryID] = event
			}
		case ControlCorrect:
			if event.MemoryID != "" {
				state.Corrections[event.MemoryID] = event
			}
		case ControlSourceMissing:
			state.SourceMissing = append(state.SourceMissing, event)
		case ControlDismissNudge:
			state.Dismissals = append(state.Dismissals, event)
		}
	}
	return state
}

func ApplyControlState(memories []Memory, state ControlState, settings Settings) []Memory {
	settings.ApplyDefaults()
	out := make([]Memory, 0, len(memories))
	for _, memory := range memories {
		if _, forgotten := state.Forgotten[memory.ID]; forgotten {
			continue
		}
		if event, corrected := state.Corrections[memory.ID]; corrected && strings.TrimSpace(event.CorrectedClaim) != "" {
			memory.Related = append(memory.Related, event.ID)
			memory.Summary = event.CorrectedClaim
			memory.Claim = event.CorrectedClaim
			memory.Details.ReviewReason = firstNonEmpty(memory.Details.ReviewReason, "user corrected generated memory text")
		}
		if event, pinned := state.Pinned[memory.ID]; pinned {
			memory.Related = append(memory.Related, event.ID)
			memory.Tags = append(memory.Tags, "#herald/pinned")
		}
		if event, missing := sourceMissingEventForMemory(memory, state); missing {
			memory.Related = append(memory.Related, event.ID)
			memory.Status = StatusSourceMissing
			memory.Freshness = FreshnessStale
			memory.Details.ReviewReason = firstNonEmpty(event.Reason, "source missing")
		}
		memory.Related = CompactStrings(memory.Related)
		memory.Tags = CompactStrings(memory.Tags)
		out = append(out, memory)
	}
	SortMemoriesNewestFirst(out)
	sort.SliceStable(out, func(i, j int) bool {
		_, pinnedI := state.Pinned[out[i].ID]
		_, pinnedJ := state.Pinned[out[j].ID]
		if pinnedI == pinnedJ {
			return false
		}
		return pinnedI
	})
	return out
}

func NudgeDismissed(nudge Nudge, state ControlState, threadID, person string) bool {
	digest := EvidenceDigest(nudge.Evidence)
	for _, event := range state.Dismissals {
		if event.NudgeID != "" && nudge.ID != "" && event.NudgeID != nudge.ID {
			continue
		}
		if event.MemoryID != "" && len(nudge.MemoryIDs) > 0 && event.MemoryID != nudge.MemoryIDs[0] {
			continue
		}
		if event.EvidenceDigest != "" && event.EvidenceDigest != digest {
			continue
		}
		switch normalizeNudgeDismissScope(event.DismissalScope) {
		case NudgeDismissDraft:
			if strings.EqualFold(event.ThreadID, strings.TrimSpace(threadID)) && strings.TrimSpace(threadID) != "" {
				return true
			}
		case NudgeDismissThread:
			if strings.EqualFold(event.ThreadID, strings.TrimSpace(threadID)) && strings.TrimSpace(threadID) != "" {
				return true
			}
		case NudgeDismissPerson:
			if strings.EqualFold(event.Person, strings.TrimSpace(person)) && strings.TrimSpace(person) != "" {
				return true
			}
		case NudgeDismissPermanent:
			return true
		}
	}
	return false
}

func EvidenceDigest(evidence []Evidence) string {
	normalized := NormalizeEvidenceList(evidence)
	var parts []string
	for _, item := range normalized {
		parts = append(parts, strings.Join([]string{
			item.SourceType,
			item.SourceID,
			item.ID,
			item.MessageID,
			item.LocalID,
			item.Path,
			item.URL,
			item.Date.UTC().Format(time.RFC3339),
		}, "\x1f"))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1e")))
	return hex.EncodeToString(sum[:])
}

func PlanMemoryUpdate(existing []Memory, candidate Memory, settings Settings, now time.Time) UpdateDecision {
	settings.ApplyDefaults()
	if now.IsZero() {
		now = time.Now()
	}
	candidate = PrepareMemoryForAppend(candidate, now)
	var best Memory
	var bestScore float64
	for _, memory := range existing {
		score := memoryMatchScore(memory, candidate)
		if score > bestScore {
			best = memory
			bestScore = score
		}
	}
	threshold := settings.UpdateRules.MatchThreshold
	if threshold <= 0 {
		threshold = settings.Thresholds.Match
	}
	decision := UpdateDecision{Action: "append_new", MatchScore: bestScore, Candidate: candidate}
	if best.ID == "" || bestScore < threshold {
		return decision
	}
	candidate.Supersedes = CompactStrings(append(candidate.Supersedes, best.ID))
	decision.Action = "append_update"
	decision.ExistingID = best.ID
	if settings.UpdateRules.ConflictCreatesState && memoriesConflict(best, candidate) {
		candidate.Status = StatusConflict
		candidate.Details.ReviewReason = "conflicting evidence requires review"
		decision.Action = "append_conflict"
		decision.ReviewLabel = "conflict"
	}
	decision.Candidate = candidate
	return decision
}

func ResolveOpenLoopMemory(original Memory, evidence []Evidence, archiveNote string, now time.Time) Memory {
	resolved := original
	resolved.ID = ""
	resolved.Status = StatusResolved
	resolved.Freshness = FreshnessFresh
	resolved.CreatedAt = now
	resolved.UpdatedAt = now
	resolved.LastActivityAt = now
	resolved.Supersedes = CompactStrings(append(resolved.Supersedes, original.ID))
	resolved.Related = CompactStrings(append(resolved.Related, original.ID))
	resolved.Evidence = NormalizeEvidenceList(append(append([]Evidence{}, original.Evidence...), evidence...))
	resolved.Details.ReviewReason = ""
	if archiveNote = strings.TrimSpace(archiveNote); archiveNote != "" {
		resolved.Summary = bounded(archiveNote, 500)
	}
	return PrepareMemoryForAppend(resolved, now)
}

func BuildUpdateRuleAudit(settings Settings) UpdateRuleAudit {
	settings.ApplyDefaults()
	return UpdateRuleAudit{
		MatchThreshold:           settings.UpdateRules.MatchThreshold,
		ConflictCreatesState:     settings.UpdateRules.ConflictCreatesState,
		StaleAfterDays:           settings.UpdateRules.StaleAfterDays,
		RetentionDays:            settings.UpdateRules.RetentionDays,
		LowConfidenceDisposition: settings.UpdateRules.LowConfidenceDisposition,
		DismissalScope:           normalizeNudgeDismissScope(settings.UpdateRules.DismissalScope),
	}
}

func BuildSourceAudit(memory Memory, state ControlState) SourceAudit {
	audit := SourceAudit{
		MemoryID:  memory.ID,
		Claim:     memory.Claim,
		Status:    memory.Status,
		Freshness: memory.Freshness,
		Evidence:  compactBriefingEvidence(memory.Evidence),
	}
	for _, event := range state.Events {
		if event.MemoryID == memory.ID || controlEvidenceMatchesMemory(event.SourceReference, memory) {
			audit.ControlEvents = append(audit.ControlEvents, event)
		}
	}
	return audit
}

func memoryMatchScore(a, b Memory) float64 {
	score := 0.0
	total := 0.0
	add := func(weight float64, matched bool) {
		total += weight
		if matched {
			score += weight
		}
	}
	add(0.20, strings.EqualFold(a.Kind, b.Kind))
	add(0.20, normalizeComparable(a.Topic) != "" && normalizeComparable(a.Topic) == normalizeComparable(b.Topic))
	add(0.15, normalizeComparable(a.Company) != "" && normalizeComparable(a.Company) == normalizeComparable(b.Company))
	add(0.10, normalizeComparable(a.Domain) != "" && normalizeComparable(a.Domain) == normalizeComparable(b.Domain))
	add(0.20, peopleOverlap(a.People, b.People))
	add(0.15, evidenceOverlap(a.Evidence, b.Evidence))
	if total == 0 {
		return 0
	}
	return score / total
}

func memoriesConflict(a, b Memory) bool {
	if lifecycleStatusForMemory(a) == StatusConflict || lifecycleStatusForMemory(b) == StatusConflict {
		return true
	}
	if lifecycleStatusForMemory(a) != "" && lifecycleStatusForMemory(b) != "" && lifecycleStatusForMemory(a) != lifecycleStatusForMemory(b) {
		return true
	}
	return normalizeComparable(a.Claim) != "" &&
		normalizeComparable(b.Claim) != "" &&
		normalizeComparable(a.Claim) != normalizeComparable(b.Claim) &&
		(a.Kind == b.Kind || normalizeComparable(a.Topic) == normalizeComparable(b.Topic))
}

func sourceMissingEventForMemory(memory Memory, state ControlState) (ControlEvent, bool) {
	for _, event := range state.SourceMissing {
		if event.MemoryID != "" && event.MemoryID == memory.ID {
			return event, true
		}
		if controlEvidenceMatchesMemory(event.SourceReference, memory) {
			return event, true
		}
		for _, evidence := range event.Evidence {
			if controlEvidenceMatchesMemory(evidence, memory) {
				return event, true
			}
		}
	}
	return ControlEvent{}, false
}

func controlEvidenceMatchesMemory(reference Evidence, memory Memory) bool {
	reference = NormalizeEvidence(reference)
	if displayEvidenceLabel(reference) == "" {
		return false
	}
	for _, evidence := range memory.Evidence {
		evidence = NormalizeEvidence(evidence)
		if reference.SourceType != "" && evidence.SourceType != reference.SourceType {
			continue
		}
		if firstNonEmpty(reference.MessageID, reference.LocalID, reference.ID, reference.Path, reference.URL) ==
			firstNonEmpty(evidence.MessageID, evidence.LocalID, evidence.ID, evidence.Path, evidence.URL) {
			return true
		}
	}
	return false
}

func controlExpired(event ControlEvent, now time.Time) bool {
	return !event.ExpiresAt.IsZero() && now.After(event.ExpiresAt)
}

func peopleOverlap(a, b []string) bool {
	for _, left := range a {
		for _, right := range b {
			if strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right)) && strings.TrimSpace(left) != "" {
				return true
			}
		}
	}
	return false
}

func evidenceOverlap(a, b []Evidence) bool {
	seen := make(map[string]bool, len(a))
	for _, evidence := range a {
		label := strings.Join([]string{evidence.SourceType, displayEvidenceLabel(evidence)}, ":")
		if label != ":" {
			seen[label] = true
		}
	}
	for _, evidence := range b {
		label := strings.Join([]string{evidence.SourceType, displayEvidenceLabel(evidence)}, ":")
		if seen[label] {
			return true
		}
	}
	return false
}

func deterministicControlID(event ControlEvent) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		event.Action,
		event.MemoryID,
		event.NudgeID,
		event.DismissalScope,
		event.ThreadID,
		event.Person,
		event.CorrectedClaim,
		event.Note,
		event.Reason,
		event.EvidenceDigest,
		displayEvidenceLabel(event.SourceReference),
		event.CreatedAt.UTC().Format(time.RFC3339Nano),
	}, "\x1f")))
	return "ctrl_" + hex.EncodeToString(sum[:])[:20]
}
