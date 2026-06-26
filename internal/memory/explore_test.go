package memory

import (
	"strings"
	"testing"
	"time"
)

func exploreTestMemory(id, kind, claim, status string, confidence float64, activity time.Time, evidence Evidence) Memory {
	return PrepareMemoryForAppend(Memory{
		ID:             id,
		Kind:           kind,
		Claim:          claim,
		Summary:        claim,
		Topic:          "Senior engineer interview",
		People:         []string{"Alex Morgan", "alex@example.com"},
		Company:        "Cobalt Works",
		Domain:         "cobalt.example",
		Status:         status,
		Freshness:      FreshnessFresh,
		Confidence:     confidence,
		CreatedAt:      activity,
		UpdatedAt:      activity,
		LastActivityAt: activity,
		ObsidianTarget: "Job search/active/Cobalt Works/Memory.md",
		Evidence:       []Evidence{evidence},
	}, activity)
}

func TestBuildExploreResultFiltersAndFacets(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	settings := DefaultSettings()
	settings.UpdateRules.LowConfidenceDisposition = LowConfidenceReview
	settings.UpdateRules.MatchThreshold = 0.80
	memories := []Memory{
		exploreTestMemory("mem-open", KindOpenQuestion, "Cobalt Works asked for interview availability", StatusWaiting, 0.93, now.Add(-2*time.Hour), Evidence{
			SourceType: SourceEmail,
			MessageID:  "msg-open",
			Folder:     "INBOX",
			Date:       now.Add(-2 * time.Hour),
			Snippet:    "Does the interview schedule still work?",
		}),
		exploreTestMemory("mem-review", KindCommitment, "Cobalt Works needs a pricing follow-up", StatusActive, 0.55, now.Add(-24*time.Hour), Evidence{
			SourceType: SourceSentEmail,
			MessageID:  "msg-sent",
			Folder:     "Sent",
			Date:       now.Add(-24 * time.Hour),
			Snippet:    "I will send the pricing proposal.",
		}),
		exploreTestMemory("mem-conflict", KindTrackStatus, "Cobalt Works headcount changed", StatusConflict, 0.91, now.Add(-48*time.Hour), Evidence{
			SourceType: SourceObsidian,
			Path:       "Job search/active/Cobalt Works/Memory.md",
			Date:       now.Add(-48 * time.Hour),
			Snippet:    "Headcount is in conflict.",
		}),
	}

	result := BuildExploreResult(memories, ExploreQuery{
		Text:      "Cobalt",
		Filter:    ExploreFilterReview,
		DateRange: ExploreDate90d,
		Settings:  settings,
		Now:       now,
		Limit:     20,
	})

	if result.Total < 2 {
		t.Fatalf("review result total = %d, want at least low-confidence and conflict rows", result.Total)
	}
	for _, row := range result.Rows {
		if !strings.Contains(strings.ToLower(row.Title+" "+row.Summary+" "+row.Company), "cobalt") {
			t.Fatalf("row does not match text filter: %#v", row)
		}
		if row.RowType == ExploreRowMemory && row.ReviewReason == "" {
			t.Fatalf("review memory missing review reason: %#v", row)
		}
	}
	if got := facetCount(result.Facets.Filters, ExploreFilterReview); got < 2 {
		t.Fatalf("review facet = %d, want at least 2", got)
	}
	if got := facetCount(result.Facets.Sources, ExploreFilterSourceObsidian); got == 0 {
		t.Fatal("expected Obsidian source facet")
	}
	if got := facetCount(result.Facets.Dates, ExploreDate7d); got == 0 {
		t.Fatal("expected 7d date facet")
	}
}

func TestBuildExploreResultDeterministicTrackRows(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	memories := []Memory{
		exploreTestMemory("mem-a", KindOpenQuestion, "Cobalt Works asked for availability", StatusWaiting, 0.93, now.Add(-time.Hour), Evidence{
			SourceType: SourceEmail,
			MessageID:  "msg-a",
			Folder:     "INBOX",
			Date:       now.Add(-time.Hour),
		}),
		exploreTestMemory("mem-b", KindCommitment, "Cobalt Works follow-up is due", StatusWaiting, 0.90, now.Add(-30*time.Minute), Evidence{
			SourceType: SourceSentEmail,
			MessageID:  "msg-b",
			Folder:     "Sent",
			Date:       now.Add(-30 * time.Minute),
		}),
	}

	result := BuildExploreResult(memories, ExploreQuery{
		Filter:   ExploreFilterAll,
		Settings: DefaultSettings(),
		Now:      now,
		Limit:    10,
	})

	if len(result.Rows) < 3 {
		t.Fatalf("rows = %d, want track + memory rows", len(result.Rows))
	}
	var trackRows int
	for _, row := range result.Rows {
		if row.RowType == ExploreRowTrack {
			trackRows++
			if row.Track == nil || len(row.Track.MemoryIDs) != 2 {
				t.Fatalf("track row did not aggregate memories: %#v", row)
			}
		}
		if row.ID == "" || row.Title == "" {
			t.Fatalf("row missing stable ID/title: %#v", row)
		}
	}
	if trackRows != 1 {
		t.Fatalf("track rows = %d, want 1", trackRows)
	}
}

func facetCount(facets []FacetCount, id string) int {
	for _, facet := range facets {
		if facet.ID == id {
			return facet.Count
		}
	}
	return 0
}
