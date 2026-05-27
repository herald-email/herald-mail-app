package app

import (
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func timelineSortEmails() []*models.EmailData {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	return []*models.EmailData{
		{
			MessageID: "bob-new",
			Sender:    "Bob News <bob@updates.example.com>",
			Subject:   "Release notes",
			Date:      now,
			Folder:    "INBOX",
		},
		{
			MessageID: "alice-mid",
			Sender:    "Alice Alerts <alice@alerts.example.com>",
			Subject:   "Quarterly planning",
			Date:      now.Add(-1 * time.Hour),
			Folder:    "INBOX",
		},
		{
			MessageID: "mara-thread-new",
			Sender:    "Mara Vale <mara@forgepoint.example>",
			Subject:   "Storage review",
			Date:      now.Add(-2 * time.Hour),
			Folder:    "INBOX",
		},
		{
			MessageID: "mara-thread-old",
			Sender:    "Mara Vale <mara@forgepoint.example>",
			Subject:   "Re: Storage review",
			Date:      now.Add(-3 * time.Hour),
			Folder:    "INBOX",
		},
		{
			MessageID: "zed-old",
			Sender:    "Zed Systems <zed@systems.example>",
			Subject:   "Oldest notice",
			Date:      now.Add(-4 * time.Hour),
			Folder:    "INBOX",
		},
	}
}

func newTimelineSortModel(t *testing.T) *Model {
	t.Helper()
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.showSidebar = false
	m.timeline.emails = timelineSortEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)
	return m
}

func pressTimelineSortKey(t *testing.T, m *Model) *Model {
	t.Helper()
	return timelineKeyPress(t, m, keyRunes("O"))
}

func timelineColumnTitle(m *Model, prefix string) string {
	for _, col := range m.timelineTable.Columns() {
		title := stripANSI(col.Title)
		if strings.HasPrefix(title, prefix) {
			return title
		}
	}
	return ""
}

func timelineHeaderClickX(t *testing.T, m *Model, prefix string) int {
	t.Helper()
	offset := 0
	for _, col := range m.timelineTable.Columns() {
		if col.Width <= 0 {
			continue
		}
		cellWidth := col.Width + 2
		if strings.HasPrefix(stripANSI(col.Title), prefix) {
			return 1 + offset + cellWidth/2
		}
		offset += cellWidth
	}
	t.Fatalf("column %q not found in %#v", prefix, tableColumnTitles(m.timelineTable.Columns()))
	return 0
}

func TestTimelineSortDefaultWhenDescendingWithIndicator(t *testing.T) {
	m := newTimelineSortModel(t)

	if m.timeline.sortMode != timelineSortWhenDesc {
		t.Fatalf("default sort mode = %v, want When descending", m.timeline.sortMode)
	}
	if got := timelineColumnTitle(m, "When"); got != "When ↓" {
		t.Fatalf("When header = %q, want descending indicator", got)
	}
	rows := m.timelineTable.Rows()
	if got := stripANSI(rows[0][2]); !strings.Contains(got, "Release notes") {
		t.Fatalf("default sort should show newest group first, got subject %q", got)
	}
}

func TestTimelineSortKeyCyclesModesAndStatus(t *testing.T) {
	m := newTimelineSortModel(t)

	checks := []struct {
		mode        timelineSortMode
		header      string
		firstSubstr string
		status      string
	}{
		{timelineSortWhenAsc, "When ↑", "Oldest notice", "Sorted by when ascending"},
		{timelineSortSenderAsc, "Sender ↑", "Quarterly planning", "Sorted by sender ascending"},
		{timelineSortSenderDesc, "Sender ↓", "Oldest notice", "Sorted by sender descending"},
		{timelineSortCountDesc, "Subject ↓", "[2] Storage review", "Sorted by count descending"},
		{timelineSortCountAsc, "Subject ↑", "Release notes", "Sorted by count ascending"},
		{timelineSortWhenDesc, "When ↓", "Release notes", "Sorted by when descending"},
	}

	for _, check := range checks {
		m = pressTimelineSortKey(t, m)
		if m.timeline.sortMode != check.mode {
			t.Fatalf("after cycle sort mode = %v, want %v", m.timeline.sortMode, check.mode)
		}
		if !strings.Contains(stripANSI(m.statusMessage), check.status) {
			t.Fatalf("status = %q, want %q", stripANSI(m.statusMessage), check.status)
		}
		if got := timelineColumnTitle(m, strings.Fields(check.header)[0]); got != check.header {
			t.Fatalf("active header = %q, want %q", got, check.header)
		}
		if got := stripANSI(m.timelineTable.Rows()[0][2]); !strings.Contains(got, check.firstSubstr) {
			t.Fatalf("first subject = %q, want it to contain %q", got, check.firstSubstr)
		}
	}
}

func TestTimelineSortSenderAndCountInGroupedModes(t *testing.T) {
	m := newTimelineGroupingModel(t)

	m.timeline.groupingMode = timelineGroupingSender
	m.timeline.sortMode = timelineSortSenderDesc
	m.updateTimelineTable()
	if got := stripANSI(m.timelineTable.Rows()[0][1]); !strings.Contains(got, "Carol") {
		t.Fatalf("sender descending should put Carol first, got %q", got)
	}

	m.timeline.sortMode = timelineSortCountDesc
	m.updateTimelineTable()
	if got := stripANSI(m.timelineTable.Rows()[0][2]); !strings.Contains(got, "[2]") {
		t.Fatalf("sender count descending should put the two-message sender first, got %q", got)
	}

	m.timeline.groupingMode = timelineGroupingDomain
	m.timeline.sortMode = timelineSortCountAsc
	m.updateTimelineTable()
	if got := stripANSI(m.timelineTable.Rows()[0][2]); !strings.Contains(got, "Personal note") {
		t.Fatalf("domain count ascending should put the one-message domain first, got subject %q", got)
	}
}

func TestTimelineSortHeaderClicksSelectAndFlipCriterion(t *testing.T) {
	m := newTimelineSortModel(t)

	model, cmd := m.Update(mousePress(timelineHeaderClickX(t, m, "Sender"), 2))
	updated := model.(*Model)
	if cmd != nil {
		t.Fatal("sort header click should not load a preview")
	}
	if updated.timeline.sortMode != timelineSortSenderAsc {
		t.Fatalf("Sender header click sort mode = %v, want sender ascending", updated.timeline.sortMode)
	}
	if got := timelineColumnTitle(updated, "Sender"); got != "Sender ↑" {
		t.Fatalf("Sender header = %q, want ascending indicator", got)
	}
	if updated.timeline.selectedEmail != nil {
		t.Fatal("header click should not select or preview a row")
	}

	model, _ = updated.Update(mousePress(timelineHeaderClickX(t, updated, "Sender"), 2))
	updated = model.(*Model)
	if updated.timeline.sortMode != timelineSortSenderDesc {
		t.Fatalf("second Sender header click sort mode = %v, want sender descending", updated.timeline.sortMode)
	}

	model, _ = updated.Update(mousePress(timelineHeaderClickX(t, updated, "Subject"), 2))
	updated = model.(*Model)
	if updated.timeline.sortMode != timelineSortCountDesc {
		t.Fatalf("Subject header click sort mode = %v, want count descending", updated.timeline.sortMode)
	}
	if got := timelineColumnTitle(updated, "Subject"); got != "Subject ↓" {
		t.Fatalf("Subject header = %q, want count descending indicator", got)
	}

	model, _ = updated.Update(mousePress(timelineHeaderClickX(t, updated, "When"), 2))
	updated = model.(*Model)
	if updated.timeline.sortMode != timelineSortWhenDesc {
		t.Fatalf("When header click sort mode = %v, want when descending", updated.timeline.sortMode)
	}
	model, _ = updated.Update(mousePress(timelineHeaderClickX(t, updated, "When"), 2))
	updated = model.(*Model)
	if updated.timeline.sortMode != timelineSortWhenAsc {
		t.Fatalf("second When header click sort mode = %v, want when ascending", updated.timeline.sortMode)
	}
}

func TestTimelineSortPreservesStarredPinningWithinModes(t *testing.T) {
	m := newTimelineSortModel(t)
	m.timeline.emails[4].IsStarred = true
	m.timeline.sortMode = timelineSortSenderAsc
	m.updateTimelineTable()

	rows := m.timelineTable.Rows()
	if got := stripANSI(rows[0][2]); !strings.Contains(got, "Oldest notice") {
		t.Fatalf("starred row should remain pinned above sender-sorted rows, got %q", got)
	}
	if got := stripANSI(rows[1][2]); !strings.Contains(got, "Quarterly planning") {
		t.Fatalf("unstarred rows should still sort by sender after pinned group, got %q", got)
	}
}

func TestTimelineSortHintsAndHelpAdvertiseSortCycle(t *testing.T) {
	m := newTimelineSortModel(t)
	hints := stripANSI(m.rawKeyHintsForWidth(120, m.chromeState(m.buildLayoutPlan(120, 40))))
	if !strings.Contains(hints, "O: sort") {
		t.Fatalf("expected Timeline hints to advertise sorting, got %q", hints)
	}

	help := pressQuestion(m)
	rendered := stripANSI(help.View().Content)
	if !strings.Contains(rendered, "O") || !strings.Contains(rendered, "cycle Timeline sorting") {
		t.Fatalf("expected Timeline shortcut help to include sorting command, got:\n%s", rendered)
	}
}

func TestTimelineSortKeyStaysInsideTextEntrySurfaces(t *testing.T) {
	t.Run("compose", func(t *testing.T) {
		m := newTimelineSortModel(t)
		m.activeTab = tabCompose
		focusComposeTextField(m, composeFieldBody)

		model, cmd := m.handleKeyMsg(keyRunes("O"))
		if commandIsQuit(cmd) {
			t.Fatal("plain O from compose returned quit command")
		}
		updated := model.(*Model)

		if got := updated.composeBody.Value(); got != "O" {
			t.Fatalf("compose body value=%q, want literal O", got)
		}
		if updated.timeline.sortMode != timelineSortWhenDesc {
			t.Fatalf("compose O changed Timeline sort to %v", updated.timeline.sortMode)
		}
	})

	t.Run("timeline search prompt", func(t *testing.T) {
		m := newTimelineSortModel(t)
		m.openTimelineSearch()

		model, cmd := m.handleKeyMsg(keyRunes("O"))
		if commandIsQuit(cmd) {
			t.Fatal("plain O from Timeline search returned quit command")
		}
		updated := model.(*Model)

		if got := updated.timeline.searchInput.Value(); got != "O" {
			t.Fatalf("Timeline search value=%q, want literal O", got)
		}
		if updated.timeline.sortMode != timelineSortWhenDesc {
			t.Fatalf("Timeline search O changed sort to %v", updated.timeline.sortMode)
		}
	})

	t.Run("prompt editor", func(t *testing.T) {
		m := newTimelineSortModel(t)
		m.showPromptEditor = true
		m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
		_ = m.promptEditor.Init()

		model, cmd := m.Update(keyRunes("O"))
		if commandIsQuit(cmd) {
			t.Fatal("plain O from prompt editor returned quit command")
		}
		updated := model.(*Model)

		if got := updated.promptEditor.name; got != "O" {
			t.Fatalf("prompt editor name=%q, want literal O", got)
		}
		if updated.timeline.sortMode != timelineSortWhenDesc {
			t.Fatalf("prompt editor O changed sort to %v", updated.timeline.sortMode)
		}
	})
}
