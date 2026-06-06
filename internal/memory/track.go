package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type trackAccumulator struct {
	track    Track
	memories []Memory
}

func BuildTracksFromMemories(memories []Memory, settings Settings, now time.Time) []Track {
	settings.ApplyDefaults()
	if now.IsZero() {
		now = time.Now()
	}
	threshold := settings.Thresholds.Dossier
	groups := make(map[string]*trackAccumulator)
	order := make([]string, 0)
	for _, candidate := range memories {
		if !sourceBackedTrackMemory(candidate, threshold) {
			continue
		}
		key := trackGroupKey(candidate)
		acc := groups[key]
		if acc == nil {
			acc = &trackAccumulator{
				track: Track{
					ID:      deterministicTrackID(key),
					Topic:   firstNonEmpty(candidate.Topic, candidate.Company, candidate.Domain, "Relationship"),
					Company: candidate.Company,
					Domain:  candidate.Domain,
					Status:  StatusActive,
				},
			}
			groups[key] = acc
			order = append(order, key)
		}
		mergeMemoryIntoTrack(acc, candidate)
	}
	tracks := make([]Track, 0, len(order))
	for _, key := range order {
		acc := groups[key]
		track := acc.track
		track.People = CompactStrings(track.People)
		track.OpenLoops = CompactStrings(track.OpenLoops)
		track.Claims = CompactStrings(track.Claims)
		track.Commitments = CompactStrings(track.Commitments)
		track.MemoryIDs = CompactStrings(track.MemoryIDs)
		track.Evidence = compactTrackEvidence(track.Evidence)
		track.Status = deriveTrackStatus(acc.memories, settings, now)
		tracks = append(tracks, track)
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		if tracks[i].LastActivityAt.Equal(tracks[j].LastActivityAt) {
			return tracks[i].ID < tracks[j].ID
		}
		return tracks[i].LastActivityAt.After(tracks[j].LastActivityAt)
	})
	return tracks
}

func sourceBackedTrackMemory(memory Memory, threshold float64) bool {
	if len(memory.Evidence) == 0 || strings.EqualFold(memory.Status, StatusSourceMissing) {
		return false
	}
	if threshold > 0 && memory.Confidence > 0 && memory.Confidence < threshold {
		return false
	}
	return true
}

func trackGroupKey(memory Memory) string {
	parts := []string{
		normalizeComparable(firstNonEmpty(memory.Topic, memory.Company, memory.Domain, memory.Claim)),
		normalizeComparable(memory.Company),
		normalizeComparable(memory.Domain),
	}
	key := strings.Join(parts, "|")
	if strings.Trim(key, "|") == "" {
		return normalizeComparable(firstNonEmpty(memory.ID, DeterministicID(memory)))
	}
	return key
}

func mergeMemoryIntoTrack(acc *trackAccumulator, memory Memory) {
	acc.memories = append(acc.memories, memory)
	track := &acc.track
	if track.Topic == "" {
		track.Topic = firstNonEmpty(memory.Topic, memory.Company, memory.Domain, "Relationship")
	}
	if track.Company == "" {
		track.Company = memory.Company
	}
	if track.Domain == "" {
		track.Domain = memory.Domain
	}
	track.People = append(track.People, memory.People...)
	track.Claims = append(track.Claims, memorySummary(memory))
	track.MemoryIDs = append(track.MemoryIDs, memory.ID)
	track.Evidence = append(track.Evidence, memory.Evidence...)
	if activity := activityTime(memory); !activity.IsZero() && (track.LastActivityAt.IsZero() || activity.After(track.LastActivityAt)) {
		track.LastActivityAt = activity
	}
	switch memory.Kind {
	case KindOpenQuestion, KindDeadline:
		track.OpenLoops = append(track.OpenLoops, memorySummary(memory))
	case KindCommitment:
		track.Commitments = append(track.Commitments, memorySummary(memory))
	}
}

func deriveTrackStatus(memories []Memory, settings Settings, now time.Time) string {
	if len(memories) == 0 {
		return StatusActive
	}
	ordered := append([]Memory(nil), memories...)
	SortMemoriesNewestFirst(ordered)
	latest := ordered[0]
	latestStatus := lifecycleStatusForMemory(latest)
	switch latestStatus {
	case StatusDone, StatusBacklog, StatusResolved, StatusConflict, StatusSourceMissing:
		return latestStatus
	}
	if memoryIsStale(latest) || activeTrackIsStale(latest, settings, now) {
		return StatusStale
	}
	if latestStatus == StatusWaiting || hasUnresolvedTrackLoop(ordered) {
		return StatusWaiting
	}
	if latestStatus == StatusStale {
		return StatusStale
	}
	if latestStatus == "" {
		return StatusActive
	}
	return latestStatus
}

func lifecycleStatusForMemory(memory Memory) string {
	if status := lifecycleStatusFromTarget(memory.ObsidianTarget); status != "" {
		return status
	}
	switch strings.ToLower(strings.TrimSpace(memory.Status)) {
	case StatusActive, StatusWaiting, StatusResolved, StatusStale, StatusBacklog, StatusDone, StatusConflict, StatusSourceMissing:
		return strings.ToLower(strings.TrimSpace(memory.Status))
	default:
		return ""
	}
}

func lifecycleStatusFromTarget(target string) string {
	normalized := strings.ToLower(filepath.ToSlash(strings.TrimSpace(target)))
	normalized = strings.Trim(normalized, "/")
	switch {
	case normalized == "job search/done" || strings.HasPrefix(normalized, "job search/done/") || strings.Contains(normalized, "/job search/done/"):
		return StatusDone
	case normalized == "job search/backlog" || strings.HasPrefix(normalized, "job search/backlog/") || strings.Contains(normalized, "/job search/backlog/"):
		return StatusBacklog
	default:
		return ""
	}
}

func activeTrackIsStale(memory Memory, settings Settings, now time.Time) bool {
	status := strings.TrimSpace(lifecycleStatusForMemory(memory))
	if status != "" && status != StatusActive && status != StatusWaiting {
		return false
	}
	days := settings.UpdateRules.StaleAfterDays
	if days <= 0 || now.IsZero() {
		return false
	}
	activity := activityTime(memory)
	if activity.IsZero() {
		return false
	}
	return now.Sub(activity) > time.Duration(days)*24*time.Hour
}

func hasUnresolvedTrackLoop(memories []Memory) bool {
	for _, memory := range memories {
		status := lifecycleStatusForMemory(memory)
		if status == StatusDone || status == StatusBacklog || status == StatusResolved || status == StatusSourceMissing {
			continue
		}
		switch memory.Kind {
		case KindOpenQuestion, KindDeadline, KindCommitment:
			return true
		}
		if status == StatusWaiting {
			return true
		}
	}
	return false
}

func activityTime(memory Memory) time.Time {
	if !memory.LastActivityAt.IsZero() {
		return memory.LastActivityAt
	}
	return latestEvidenceDate(memory.Evidence, memory.CreatedAt)
}

func deterministicTrackID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "track_" + hex.EncodeToString(sum[:])[:20]
}
