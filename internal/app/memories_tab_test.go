package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/memory"
)

type memoriesExploreStubBackend struct {
	stubBackend
	memories []memory.Memory
	queries  []memory.ExploreQuery
}

func (b *memoriesExploreStubBackend) ExploreMemories(ctx context.Context, query memory.ExploreQuery) (memory.ExploreResult, error) {
	if err := ctx.Err(); err != nil {
		return memory.ExploreResult{}, err
	}
	b.queries = append(b.queries, query)
	return memory.BuildExploreResult(b.memories, query), nil
}

func newMemoriesTestModel(t *testing.T, width, height int) (*Model, *memoriesExploreStubBackend) {
	t.Helper()
	backend := &memoriesExploreStubBackend{memories: testExploreMemories()}
	m := New(backend, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabMemories
	return m, backend
}

func testExploreMemories() []memory.Memory {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.Local)
	return []memory.Memory{
		memory.PrepareMemoryForAppend(memory.Memory{
			ID:             "test-memory-cobalt-open",
			Kind:           memory.KindOpenQuestion,
			Claim:          "Mina asked whether the Cobalt Works interview schedule still works.",
			Summary:        "Cobalt Works is waiting on interview availability.",
			Topic:          "Cobalt Works interview",
			People:         []string{"Mina Park", "mina@cobalt-works.example"},
			Company:        "Cobalt Works",
			Domain:         "cobalt-works.example",
			Status:         memory.StatusWaiting,
			Confidence:     0.94,
			LastActivityAt: now.Add(-2 * time.Hour),
			ObsidianTarget: "Job search/active/Cobalt Works/Memory.md",
			Evidence: []memory.Evidence{{
				SourceType: memory.SourceEmail,
				MessageID:  "msg-cobalt",
				Folder:     "INBOX",
				Date:       now.Add(-2 * time.Hour),
				Snippet:    "Does the interview schedule still work?",
			}},
		}, now),
		memory.PrepareMemoryForAppend(memory.Memory{
			ID:             "test-memory-sergey-reply",
			Kind:           memory.KindLastUserReply,
			Claim:          "You told Sergey you would send availability by Friday.",
			Summary:        "Sergey is waiting on your availability.",
			Topic:          "Senior engineer interview",
			People:         []string{"Sergey", "sergey@example.com"},
			Company:        "Example",
			Domain:         "example.com",
			Status:         memory.StatusWaiting,
			Confidence:     0.89,
			LastActivityAt: now.Add(-24 * time.Hour),
			ObsidianTarget: "People/Sergey.md",
			Evidence: []memory.Evidence{{
				SourceType: memory.SourceSentEmail,
				MessageID:  "msg-sergey",
				Folder:     "Sent",
				Date:       now.Add(-24 * time.Hour),
				Snippet:    "I will send availability by Friday.",
			}},
		}, now),
		memory.PrepareMemoryForAppend(memory.Memory{
			ID:             "test-memory-cobalt-conflict",
			Kind:           memory.KindTrackStatus,
			Claim:          "Cobalt Works is both waiting and canceled in conflicting sources.",
			Summary:        "Conflicting source evidence needs review.",
			Topic:          "Cobalt Works interview state",
			Company:        "Cobalt Works",
			Domain:         "cobalt-works.example",
			Status:         memory.StatusConflict,
			Confidence:     0.91,
			LastActivityAt: now.Add(-6 * time.Hour),
			ObsidianTarget: "Job search/active/Cobalt Works/Memory.md",
			Details:        memory.MemoryDetails{ReviewReason: "email and note state disagree"},
			Evidence: []memory.Evidence{{
				SourceType: memory.SourceEmail,
				MessageID:  "msg-conflict",
				Folder:     "INBOX",
				Date:       now.Add(-6 * time.Hour),
				Snippet:    "Looking forward to the next conversation.",
			}, {
				SourceType: memory.SourceObsidian,
				Path:       "Job search/active/Cobalt Works/Memory.md",
				Date:       now.Add(-5 * time.Hour),
				Snippet:    "Marked canceled after budget review.",
			}},
		}, now),
	}
}

func applyMemoriesCmd(t *testing.T, m *Model, cmd tea.Cmd) *Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	model, _ := m.Update(cmd())
	return model.(*Model)
}

func TestMemoriesTabLoadsAndRendersFourPaneExplorer(t *testing.T) {
	m, _ := newMemoriesTestModel(t, 220, 50)
	m = applyMemoriesCmd(t, m, m.loadMemoriesExplore())

	rendered := stripANSI(m.renderMainView())
	for _, want := range []string{"4  Memories", "Filters", "Memory Explorer", "Track / Memory", "Sources + Actions", "Cobalt Works", "SOURCE LINKS", "OBSIDIAN", "read-only"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("memories render missing %q:\n%s", want, rendered)
		}
	}
	firstLine := strings.Split(rendered, "\n")[1]
	if !strings.Contains(firstLine, "Cobalt Works interview") {
		t.Fatalf("expected selected memory title in detail frame border, got %q\n%s", firstLine, rendered)
	}
	assertFitsWidth(t, 220, rendered)
}

func TestMemoriesTabSearchFilterAndLocalDismissAreReadOnly(t *testing.T) {
	m, backend := newMemoriesTestModel(t, 120, 36)
	m = applyMemoriesCmd(t, m, m.loadMemoriesExplore())

	model, _ := m.handleMemoriesKey(keyRunes("/"))
	m = model.(*Model)
	model, cmd := m.handleMemoriesKey(keyRunes("Sergey"))
	m = applyMemoriesCmd(t, model.(*Model), cmd)
	if len(backend.queries) == 0 || backend.queries[len(backend.queries)-1].Text != "Sergey" {
		t.Fatalf("last query = %#v, want Sergey search", backend.queries)
	}
	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "Sergey") || strings.Contains(rendered, "Mina asked") {
		t.Fatalf("search render did not narrow to Sergey:\n%s", rendered)
	}
	model, cmd = m.handleMemoriesKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = applyMemoriesCmd(t, model.(*Model), cmd)

	m.memories.focusPanel = memoriesFocusRail
	m.memories.railIdx = 0
	m.applyMemoryRailChoice(m.memoryRailChoices()[1])
	m = applyMemoriesCmd(t, m, m.loadMemoriesExplore())
	if m.memories.filter != memory.ExploreFilterOpenLoops {
		t.Fatalf("filter = %q, want open loops", m.memories.filter)
	}

	before := len(m.memoryVisibleRows())
	model, _ = m.handleMemoriesKey(keyRunes("x"))
	m = model.(*Model)
	if got := len(m.memoryVisibleRows()); got != before-1 {
		t.Fatalf("visible rows after local dismiss = %d, want %d", got, before-1)
	}
	if len(backend.memories) != 3 {
		t.Fatalf("backend memories mutated by local dismiss: %d", len(backend.memories))
	}
}

func TestMemoriesTabEmptyUnavailableAndWidthFit(t *testing.T) {
	m := New(&stubBackend{}, nil, "", nil, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabMemories
	m = applyMemoriesCmd(t, m, m.loadMemoriesExplore())

	rendered := stripANSI(m.renderMainView())
	if !strings.Contains(rendered, "No memory rows match") {
		t.Fatalf("empty memories render missing empty state:\n%s", rendered)
	}
	assertFitsWidth(t, 80, rendered)
}

func TestMemoriesShortcutHelpAdvertisesReadOnlyActions(t *testing.T) {
	m, _ := newMemoriesTestModel(t, 120, 36)
	m.showHelp = true
	help := stripANSI(m.renderShortcutHelpPanel())
	for _, want := range []string{"Memories", "cycle filter, list, dossier, and source panes", "refresh memory exploration", "dismiss the selected row locally"} {
		if !strings.Contains(help, want) {
			t.Fatalf("memories shortcut help missing %q:\n%s", want, help)
		}
	}
}

func TestMemoriesPanelFocusUsesArrowsAndTabHints(t *testing.T) {
	m, _ := newMemoriesTestModel(t, 120, 36)
	m = applyMemoriesCmd(t, m, m.loadMemoriesExplore())
	if m.memories.focusPanel != memoriesFocusRail {
		t.Fatalf("initial memories focus = %d, want rail", m.memories.focusPanel)
	}

	model, _ := m.handleMemoriesKey(tea.KeyPressMsg{Code: tea.KeyRight})
	m = model.(*Model)
	if m.memories.focusPanel != memoriesFocusList {
		t.Fatalf("right arrow focus = %d, want list", m.memories.focusPanel)
	}

	model, _ = m.handleMemoriesKey(tea.KeyPressMsg{Code: tea.KeyRight})
	m = model.(*Model)
	if m.memories.focusPanel != memoriesFocusDetail {
		t.Fatalf("second right arrow focus = %d, want detail", m.memories.focusPanel)
	}

	model, _ = m.handleMemoriesKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = model.(*Model)
	if m.memories.focusPanel != memoriesFocusList {
		t.Fatalf("left arrow focus = %d, want list", m.memories.focusPanel)
	}

	model, _ = m.handleMemoriesKey(tea.KeyPressMsg{Code: tea.KeyTab})
	m = model.(*Model)
	if m.memories.focusPanel != memoriesFocusDetail {
		t.Fatalf("tab focus = %d, want detail", m.memories.focusPanel)
	}

	hints := stripANSI(m.renderKeyHints())
	if !strings.Contains(hints, "←/→ or tab/shift+tab: panels") {
		t.Fatalf("memories status hints did not advertise arrow/tab panel switching:\n%s", hints)
	}
}

func TestMemoryConfidenceSegmentCountsUseSeverityZones(t *testing.T) {
	low, mid, high, empty := memoryConfidenceSegmentCounts(0.25, 12)
	if low == 0 || mid != 0 || high != 0 || empty == 0 {
		t.Fatalf("low confidence segments = low:%d mid:%d high:%d empty:%d, want only error zone filled", low, mid, high, empty)
	}

	low, mid, high, empty = memoryConfidenceSegmentCounts(0.60, 12)
	if low == 0 || mid == 0 || high != 0 || empty == 0 {
		t.Fatalf("mid confidence segments = low:%d mid:%d high:%d empty:%d, want error+warning zones filled", low, mid, high, empty)
	}

	low, mid, high, empty = memoryConfidenceSegmentCounts(0.93, 12)
	if low == 0 || mid == 0 || high == 0 || empty == 0 {
		t.Fatalf("high confidence segments = low:%d mid:%d high:%d empty:%d, want error+warning+success with tail remaining", low, mid, high, empty)
	}
}
