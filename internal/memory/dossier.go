package memory

import (
	"strings"
	"time"
)

const (
	DossierKindPerson = "person"

	defaultDossierMemoryLimit = 12
	defaultDossierTrackLimit  = 4
	defaultDossierItemLimit   = 3
	defaultDossierLinkLimit   = 5
)

type DossierBuildOptions struct {
	ID      string
	Subject string
	Kind    string
	Now     time.Time
	Limit   int
}

func BuildPersonDossier(subject string, memories []Memory, settings Settings, now time.Time) Dossier {
	return BuildDossierFromMemories(DossierBuildOptions{
		Subject: subject,
		Kind:    DossierKindPerson,
		Now:     now,
	}, memories, settings)
}

func BuildDossierFromMemories(opts DossierBuildOptions, memories []Memory, settings Settings) Dossier {
	settings.ApplyDefaults()
	if opts.Kind == "" {
		opts.Kind = DossierKindPerson
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultDossierMemoryLimit
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	memories = filterDossierMemories(memories, settings, opts.Limit)
	dossier := Dossier{
		ID:          firstNonEmpty(opts.ID, deterministicDossierID(opts.Kind, opts.Subject)),
		Subject:     strings.TrimSpace(opts.Subject),
		Kind:        opts.Kind,
		Freshness:   FreshnessFresh,
		GeneratedAt: opts.Now,
		Evidence:    compactEvidence(memories),
	}
	if len(memories) == 0 {
		return dossier
	}
	dossier.RelationshipSummary = dossierRelationshipSummary(memories)
	dossier.RecentInteractions = dossierMemoriesByKind(memories, defaultDossierItemLimit, KindLastContact, KindLastUserReply, KindRelationshipContext)
	dossier.ActiveTracks = dossierTracks(memories, defaultDossierTrackLimit)
	dossier.OpenLoops = dossierMemoriesByKind(memories, defaultDossierItemLimit, KindOpenQuestion, KindDeadline, KindCommitment)
	dossier.ResearchNotes = dossierMemoriesByKind(memories, 2, KindResearchNote)
	dossier.VaultLinks = dossierVaultLinks(memories, defaultDossierLinkLimit)
	for _, memory := range memories {
		if memory.Freshness == FreshnessStale || memory.Status == StatusStale {
			dossier.Freshness = FreshnessStale
			break
		}
	}
	return dossier
}

func filterDossierMemories(memories []Memory, settings Settings, limit int) []Memory {
	threshold := settings.Thresholds.Dossier
	out := make([]Memory, 0, len(memories))
	for _, memory := range memories {
		if memory.Status == StatusSourceMissing {
			continue
		}
		if threshold > 0 && memory.Confidence < threshold {
			continue
		}
		out = append(out, memory)
	}
	SortMemoriesNewestFirst(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func dossierRelationshipSummary(memories []Memory) string {
	preferredKinds := []string{KindRelationshipContext, KindLastUserReply, KindLastContact, KindTrackStatus}
	for _, kind := range preferredKinds {
		for _, memory := range memories {
			if memory.Kind == kind {
				if summary := memorySummary(memory); summary != "" {
					return summary
				}
			}
		}
	}
	for _, memory := range memories {
		if summary := memorySummary(memory); summary != "" {
			return summary
		}
	}
	return ""
}

func dossierMemoriesByKind(memories []Memory, limit int, kinds ...string) []Memory {
	wanted := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		wanted[kind] = true
	}
	out := make([]Memory, 0, limit)
	for _, memory := range memories {
		if wanted[memory.Kind] {
			out = append(out, memory)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

func dossierTracks(memories []Memory, limit int) []Track {
	tracks := BuildTracksFromMemories(memories, Settings{}, time.Time{})
	out := make([]Track, 0, len(tracks))
	for _, track := range tracks {
		if track.Status == StatusResolved || track.Status == StatusDone || track.Status == StatusSourceMissing {
			continue
		}
		out = append(out, track)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func dossierVaultLinks(memories []Memory, limit int) []string {
	var links []string
	for _, memory := range memories {
		links = append(links, memory.ObsidianTarget)
		links = append(links, memory.Links...)
	}
	links = CompactStrings(links)
	if limit > 0 && len(links) > limit {
		return links[:limit]
	}
	return links
}

func compactTrackEvidence(evidence []Evidence) []Evidence {
	seen := make(map[string]bool, len(evidence))
	var out []Evidence
	for _, item := range evidence {
		label := strings.Join([]string{item.SourceType, displayEvidenceLabel(item)}, ":")
		if label == ":" || seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, item)
	}
	return out
}

func memorySummary(memory Memory) string {
	return strings.TrimSpace(firstNonEmpty(memory.Summary, memory.Claim, memory.Details.GeneratedSummary))
}

func deterministicDossierID(kind, subject string) string {
	id := normalizeComparable(kind + "-" + subject)
	id = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, id)
	id = strings.Trim(id, "-")
	if id == "" {
		return "dossier"
	}
	return "dossier_" + id
}
