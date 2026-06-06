package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const defaultDailyBriefingLimit = 8

type DailyBriefingQuery struct {
	Since     time.Time         `json:"since,omitempty" yaml:"since,omitempty"`
	Now       time.Time         `json:"now,omitempty" yaml:"now,omitempty"`
	Limit     int               `json:"limit,omitempty" yaml:"limit,omitempty"`
	SyncState ObsidianSyncState `json:"sync_state,omitempty" yaml:"sync_state,omitempty"`
}

type DailyBriefingDiff struct {
	GeneratedAt     time.Time                 `json:"generated_at" yaml:"generated_at"`
	Since           time.Time                 `json:"since" yaml:"since"`
	DestinationPath string                    `json:"destination_path" yaml:"destination_path"`
	ChangedTracks   []DailyBriefingTrackItem  `json:"changed_tracks,omitempty" yaml:"changed_tracks,omitempty"`
	NewlyResolved   []DailyBriefingMemoryItem `json:"newly_resolved,omitempty" yaml:"newly_resolved,omitempty"`
	NewlyStale      []DailyBriefingTrackItem  `json:"newly_stale,omitempty" yaml:"newly_stale,omitempty"`
	FailedSyncs     []DailyBriefingSyncIssue  `json:"failed_syncs,omitempty" yaml:"failed_syncs,omitempty"`
	ReviewNeeded    []DailyBriefingMemoryItem `json:"review_needed,omitempty" yaml:"review_needed,omitempty"`
}

type DailyBriefingTrackItem struct {
	ID             string     `json:"id" yaml:"id"`
	Topic          string     `json:"topic" yaml:"topic"`
	Status         string     `json:"status" yaml:"status"`
	Summary        string     `json:"summary,omitempty" yaml:"summary,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at,omitempty" yaml:"last_activity_at,omitempty"`
	MemoryIDs      []string   `json:"memory_ids,omitempty" yaml:"memory_ids,omitempty"`
	Evidence       []Evidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	NoteLinks      []string   `json:"note_links,omitempty" yaml:"note_links,omitempty"`
}

type DailyBriefingMemoryItem struct {
	ID             string     `json:"id" yaml:"id"`
	Kind           string     `json:"kind" yaml:"kind"`
	Claim          string     `json:"claim" yaml:"claim"`
	Status         string     `json:"status,omitempty" yaml:"status,omitempty"`
	ReviewReason   string     `json:"review_reason,omitempty" yaml:"review_reason,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at,omitempty" yaml:"last_activity_at,omitempty"`
	Evidence       []Evidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	NoteLinks      []string   `json:"note_links,omitempty" yaml:"note_links,omitempty"`
}

type DailyBriefingSyncIssue struct {
	Type    string    `json:"type" yaml:"type"`
	Message string    `json:"message" yaml:"message"`
	Path    string    `json:"path,omitempty" yaml:"path,omitempty"`
	Count   int       `json:"count,omitempty" yaml:"count,omitempty"`
	LastRun time.Time `json:"last_run,omitempty" yaml:"last_run,omitempty"`
}

func (d DailyBriefingDiff) HasChanges() bool {
	return len(d.ChangedTracks) > 0 ||
		len(d.NewlyResolved) > 0 ||
		len(d.NewlyStale) > 0 ||
		len(d.FailedSyncs) > 0 ||
		len(d.ReviewNeeded) > 0
}

func BuildDailyBriefingDiff(memories []Memory, settings Settings, query DailyBriefingQuery) DailyBriefingDiff {
	settings.ApplyDefaults()
	now := query.Now
	if now.IsZero() {
		now = time.Now()
	}
	since := query.Since
	if since.IsZero() {
		since = now.Add(-24 * time.Hour)
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultDailyBriefingLimit
	}
	SortMemoriesNewestFirst(memories)
	byID := memoriesByID(memories)
	diff := DailyBriefingDiff{
		GeneratedAt:     now,
		Since:           since,
		DestinationPath: DailyBriefingDestinationPath(settings, now),
	}
	for _, track := range BuildTracksFromMemories(memories, settings, now) {
		if len(diff.ChangedTracks) < limit && trackChangedSince(track, byID, since) {
			diff.ChangedTracks = append(diff.ChangedTracks, dailyTrackItem(track, byID))
		}
		if len(diff.NewlyStale) < limit && trackNewlyStale(track, byID, settings, since, now) {
			diff.NewlyStale = append(diff.NewlyStale, dailyTrackItem(track, byID))
		}
	}
	for _, memory := range memories {
		if len(diff.NewlyResolved) < limit && memoryChangedSince(memory, since) && loopResolved(memory) {
			diff.NewlyResolved = append(diff.NewlyResolved, dailyMemoryItem(memory))
		}
		if len(diff.ReviewNeeded) < limit && memoryChangedSince(memory, since) && memoryNeedsReview(memory, settings) {
			diff.ReviewNeeded = append(diff.ReviewNeeded, dailyMemoryItem(memory))
		}
	}
	if issue, ok := failedSyncIssue(query.SyncState, since); ok {
		diff.FailedSyncs = append(diff.FailedSyncs, issue)
	}
	return diff
}

func DailyBriefingDestinationPath(settings Settings, now time.Time) string {
	settings.ApplyDefaults()
	if now.IsZero() {
		now = time.Now()
	}
	dir := strings.Trim(strings.TrimSpace(settings.Destinations.DailyBriefing), "/")
	if dir == "" {
		dir = "Scheduled Task Artifacts"
	}
	return filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("Herald Memory Briefing %s.md", now.Format("2006-01-02"))))
}

func (s *FileStore) BuildDailyBriefing(ctx context.Context, settings Settings, query DailyBriefingQuery) (DailyBriefingDiff, error) {
	if err := ctx.Err(); err != nil {
		return DailyBriefingDiff{}, err
	}
	if s == nil {
		return DailyBriefingDiff{}, fmt.Errorf("memory store is nil")
	}
	memories, err := s.List(ctx)
	if err != nil {
		return DailyBriefingDiff{}, err
	}
	if query.SyncState == (ObsidianSyncState{}) {
		if state, err := s.ReadObsidianSyncState(ctx); err == nil {
			query.SyncState = state
		}
	}
	return BuildDailyBriefingDiff(memories, settings, query), nil
}

func (s *Service) BuildDailyBriefing(ctx context.Context, query DailyBriefingQuery) (DailyBriefingDiff, error) {
	if s == nil || s.store == nil {
		return DailyBriefingDiff{}, fmt.Errorf("memory service is not configured")
	}
	return s.store.BuildDailyBriefing(ctx, s.settings, query)
}

func RenderDailyBriefingMarkdown(diff DailyBriefingDiff, settings Settings) string {
	settings.ApplyDefaults()
	var sb strings.Builder
	titleDate := diff.GeneratedAt
	if titleDate.IsZero() {
		titleDate = time.Now()
	}
	sb.WriteString("# Herald Memory Briefing - ")
	sb.WriteString(titleDate.Format("2006-01-02"))
	sb.WriteString("\n\n")
	sb.WriteString("Generated: ")
	sb.WriteString(titleDate.Format(time.RFC3339))
	sb.WriteString("\n")
	if !diff.Since.IsZero() {
		sb.WriteString("Since: ")
		sb.WriteString(diff.Since.Format(time.RFC3339))
		sb.WriteString("\n")
	}
	if diff.DestinationPath != "" {
		sb.WriteString("Target: ")
		sb.WriteString(escapeMarkdownLine(diff.DestinationPath))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	if !diff.HasChanges() {
		sb.WriteString("_No memory changes in this briefing window._\n")
		return sb.String()
	}
	renderTrackSection(&sb, "Changed Tracks", diff.ChangedTracks, settings)
	renderMemorySection(&sb, "Newly Resolved Loops", diff.NewlyResolved, settings)
	renderTrackSection(&sb, "Newly Stale Loops", diff.NewlyStale, settings)
	renderSyncSection(&sb, "Failed Syncs", diff.FailedSyncs)
	renderMemorySection(&sb, "Review Needed", diff.ReviewNeeded, settings)
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func renderTrackSection(sb *strings.Builder, title string, items []DailyBriefingTrackItem, settings Settings) {
	sb.WriteString("## ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	if len(items) == 0 {
		sb.WriteString("_None._\n\n")
		return
	}
	for _, item := range items {
		sb.WriteString("- **")
		sb.WriteString(escapeMarkdownLine(firstNonEmpty(item.Topic, "Untitled track")))
		sb.WriteString("**")
		if item.Status != "" {
			sb.WriteString(" [")
			sb.WriteString(escapeMarkdownLine(item.Status))
			sb.WriteString("]")
		}
		if item.Summary != "" {
			sb.WriteString(": ")
			sb.WriteString(escapeMarkdownLine(bounded(item.Summary, 180)))
		}
		renderBriefingMetadata(sb, item.NoteLinks, item.Evidence, settings)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

func renderMemorySection(sb *strings.Builder, title string, items []DailyBriefingMemoryItem, settings Settings) {
	sb.WriteString("## ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	if len(items) == 0 {
		sb.WriteString("_None._\n\n")
		return
	}
	for _, item := range items {
		sb.WriteString("- **")
		sb.WriteString(escapeMarkdownLine(strings.ReplaceAll(firstNonEmpty(item.Kind, "memory"), "_", " ")))
		sb.WriteString("**")
		if item.Status != "" {
			sb.WriteString(" [")
			sb.WriteString(escapeMarkdownLine(item.Status))
			sb.WriteString("]")
		}
		sb.WriteString(": ")
		sb.WriteString(escapeMarkdownLine(bounded(item.Claim, 180)))
		if item.ReviewReason != "" {
			sb.WriteString(" - ")
			sb.WriteString(escapeMarkdownLine(bounded(item.ReviewReason, 120)))
		}
		renderBriefingMetadata(sb, item.NoteLinks, item.Evidence, settings)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

func renderSyncSection(sb *strings.Builder, title string, issues []DailyBriefingSyncIssue) {
	sb.WriteString("## ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	if len(issues) == 0 {
		sb.WriteString("_None._\n\n")
		return
	}
	for _, issue := range issues {
		sb.WriteString("- **")
		sb.WriteString(escapeMarkdownLine(firstNonEmpty(issue.Type, "sync")))
		sb.WriteString("**: ")
		sb.WriteString(escapeMarkdownLine(bounded(issue.Message, 180)))
		if issue.Count > 0 {
			sb.WriteString(fmt.Sprintf(" (%d)", issue.Count))
		}
		if issue.Path != "" {
			sb.WriteString(" - ")
			sb.WriteString(escapeMarkdownLine(issue.Path))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

func renderBriefingMetadata(sb *strings.Builder, noteLinks []string, evidence []Evidence, settings Settings) {
	var parts []string
	if links := renderBriefingNoteLinks(noteLinks, settings); links != "" {
		parts = append(parts, links)
	}
	if sources := renderBriefingEvidence(evidence); sources != "" {
		parts = append(parts, sources)
	}
	if len(parts) == 0 {
		return
	}
	sb.WriteString(" - ")
	sb.WriteString(strings.Join(parts, " - "))
}

func renderBriefingNoteLinks(paths []string, settings Settings) string {
	mode := NormalizeLinkMode(settings.Obsidian.LinkMode)
	if mode == LinkModeNone {
		return ""
	}
	paths = CompactStrings(paths)
	if len(paths) == 0 {
		return ""
	}
	links := make([]string, 0, len(paths))
	for _, path := range paths {
		links = append(links, renderPathLink(path, path, mode))
	}
	return strings.Join(links, ", ")
}

func renderBriefingEvidence(evidence []Evidence) string {
	labels := make([]string, 0, len(evidence))
	for _, item := range evidence {
		label := displayEvidenceLabel(item)
		if label == "" {
			continue
		}
		labels = append(labels, strings.TrimSpace(item.SourceType)+" "+bounded(label, 80))
	}
	labels = CompactStrings(labels)
	if len(labels) == 0 {
		return ""
	}
	return "sources: " + strings.Join(labels, ", ")
}

func memoriesByID(memories []Memory) map[string]Memory {
	out := make(map[string]Memory, len(memories))
	for _, memory := range memories {
		if memory.ID != "" {
			out[memory.ID] = memory
		}
	}
	return out
}

func trackChangedSince(track Track, byID map[string]Memory, since time.Time) bool {
	if since.IsZero() {
		return true
	}
	for _, id := range track.MemoryIDs {
		if memory, ok := byID[id]; ok && memoryChangedSince(memory, since) {
			return true
		}
	}
	return !track.LastActivityAt.IsZero() && track.LastActivityAt.After(since)
}

func memoryChangedSince(memory Memory, since time.Time) bool {
	if since.IsZero() {
		return true
	}
	for _, candidate := range []time.Time{memory.UpdatedAt, memory.CreatedAt, memory.LastActivityAt, latestEvidenceDate(memory.Evidence, time.Time{})} {
		if !candidate.IsZero() && (candidate.Equal(since) || candidate.After(since)) {
			return true
		}
	}
	return false
}

func trackNewlyStale(track Track, byID map[string]Memory, settings Settings, since, now time.Time) bool {
	if track.Status != StatusStale {
		return false
	}
	for _, id := range track.MemoryIDs {
		memory, ok := byID[id]
		if !ok {
			continue
		}
		if memoryChangedSince(memory, since) && memoryIsStale(memory) {
			return true
		}
	}
	days := settings.UpdateRules.StaleAfterDays
	if days <= 0 || track.LastActivityAt.IsZero() || since.IsZero() || now.IsZero() {
		return false
	}
	transition := track.LastActivityAt.Add(time.Duration(days) * 24 * time.Hour)
	return (transition.Equal(since) || transition.After(since)) && transition.Before(now)
}

func loopResolved(memory Memory) bool {
	switch memory.Kind {
	case KindOpenQuestion, KindDeadline, KindCommitment:
	default:
		return false
	}
	return lifecycleStatusForMemory(memory) == StatusResolved
}

func dailyTrackItem(track Track, byID map[string]Memory) DailyBriefingTrackItem {
	return DailyBriefingTrackItem{
		ID:             track.ID,
		Topic:          track.Topic,
		Status:         track.Status,
		Summary:        firstNonEmpty(firstString(track.OpenLoops), firstString(track.Commitments), firstString(track.Claims)),
		LastActivityAt: track.LastActivityAt,
		MemoryIDs:      CompactStrings(track.MemoryIDs),
		Evidence:       compactBriefingEvidence(track.Evidence),
		NoteLinks:      noteLinksForTrack(track, byID),
	}
}

func dailyMemoryItem(memory Memory) DailyBriefingMemoryItem {
	return DailyBriefingMemoryItem{
		ID:             memory.ID,
		Kind:           memory.Kind,
		Claim:          firstNonEmpty(memory.Summary, memory.Claim),
		Status:         memory.Status,
		ReviewReason:   memory.Details.ReviewReason,
		LastActivityAt: activityTime(memory),
		Evidence:       compactBriefingEvidence(memory.Evidence),
		NoteLinks:      noteLinksForMemory(memory),
	}
}

func compactBriefingEvidence(evidence []Evidence) []Evidence {
	evidence = compactTrackEvidence(evidence)
	for i := range evidence {
		evidence[i].Snippet = ""
	}
	return evidence
}

func noteLinksForTrack(track Track, byID map[string]Memory) []string {
	var links []string
	for _, id := range track.MemoryIDs {
		if memory, ok := byID[id]; ok {
			links = append(links, noteLinksForMemory(memory)...)
		}
	}
	return CompactStrings(links)
}

func noteLinksForMemory(memory Memory) []string {
	links := append([]string{}, memory.ObsidianTarget)
	links = append(links, memory.Links...)
	return CompactStrings(links)
}

func failedSyncIssue(state ObsidianSyncState, since time.Time) (DailyBriefingSyncIssue, bool) {
	if state.FailedWrites <= 0 {
		return DailyBriefingSyncIssue{}, false
	}
	if !since.IsZero() && !state.LastRun.IsZero() && state.LastRun.Before(since) {
		return DailyBriefingSyncIssue{}, false
	}
	message := strings.TrimSpace(state.Error)
	if message == "" {
		message = "Obsidian sync reported failed writes."
	}
	return DailyBriefingSyncIssue{
		Type:    "obsidian_write_failed",
		Message: message,
		Path:    state.VaultPath,
		Count:   state.FailedWrites,
		LastRun: state.LastRun,
	}, true
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
