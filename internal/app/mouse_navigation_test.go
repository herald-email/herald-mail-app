package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func mousePress(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func mouseWheelDown(x, y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown}
}

func mouseWheelUp(x, y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelUp}
}

func makeMouseTimelineModel(t *testing.T) *Model {
	t.Helper()
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.showSidebar = false
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)
	return m
}

func makeMouseCalendarModel(t *testing.T) (*Model, string) {
	t.Helper()
	events := testCalendarEvents()
	collections := []models.CalendarCollection{
		{
			Ref: models.CollectionRef{
				SourceID:     "demo-calendar",
				AccountID:    "default",
				Kind:         models.SourceKindCalendar,
				CollectionID: "work",
				DisplayName:  "Work",
			},
			Color: "#3367d6",
		},
		{
			Ref: models.CollectionRef{
				SourceID:     "demo-calendar",
				AccountID:    "default",
				Kind:         models.SourceKindCalendar,
				CollectionID: "family",
				DisplayName:  "Family",
			},
			Color: "#0b8043",
		},
	}
	for i := range events {
		if i >= 2 {
			events[i].Ref.CalendarID = "family"
			events[i].Ref = events[i].Ref.WithDefaults()
		}
	}
	cfg := &config.Config{}
	cfg.Vendor = "gmail"
	cfg.Server.Host = "imap.gmail.com"
	cfg.Server.Port = 993
	cfg.Gmail.RefreshToken = "rt-token"
	cfg.Gmail.Email = "user@gmail.com"
	cfg.Sources = []config.SourceConfig{
		{
			ID:          "default-mail",
			Kind:        "mail",
			Provider:    "gmail",
			AccountID:   "default",
			Credentials: config.CredentialsConfig{Username: "user@gmail.com", Password: "bridge-pass"},
			IMAP:        config.ServerConfig{Host: "imap.gmail.com", Port: 993},
			SMTP:        config.ServerConfig{Host: "smtp.gmail.com", Port: 587},
		},
		{ID: "demo-calendar", Kind: "calendar", Provider: "google_calendar", AccountID: "default"},
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "herald.yaml")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	b := &calendarAgendaStubBackend{available: true, events: events, collections: collections}
	m := New(b, nil, "", nil, false)
	m.SetConfig(cfg)
	m.SetConfigPath(configPath)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 42})
	m = updated.(*Model)
	m.loading = false
	m.activeTab = tabCalendar
	m.calendarAvailable = true
	m.setCalendarEventsForDisplay(events)
	m.setCalendarCollections(collections)
	m.calendarView = calendarViewAgenda
	m.calendarAgendaShowPast = true
	m.calendarAgendaStart, m.calendarAgendaEnd = calendarAgendaWindowFor(events[0].Start)
	m.ensureCalendarSelectionVisible()
	return m, configPath
}

func makeMouseThreadTimelineModel(t *testing.T) (*Model, *models.EmailData, *models.EmailData) {
	t.Helper()
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.showSidebar = false
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	root := &models.EmailData{
		MessageID: "thread-root",
		Sender:    "Rowan Finch <demo@demo.local>",
		Subject:   "Re: Next Steps with Cobalt Works!",
		Date:      now,
		Size:      8704,
		Folder:    "INBOX",
		UID:       26,
	}
	child := &models.EmailData{
		MessageID: "thread-child",
		Sender:    "Mina Park <mina@cobalt-works.example>",
		Subject:   "Next Steps with Cobalt Works!",
		Date:      root.Date.Add(-3 * time.Minute),
		Size:      9216,
		Folder:    "INBOX",
		UID:       27,
	}
	m.timeline.emails = []*models.EmailData{root, child}
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)
	return m, root, child
}

func TestMouseClickTabSwitchesWithoutTypingIntoCompose(t *testing.T) {
	m := makeMouseTimelineModel(t)

	contactsTabX := visibleWidth(" Herald  ") + tabMouseWidth(topLevelTabNavigation[0]) + 1
	model, _ := m.Update(mousePress(contactsTabX, 0))
	updated := model.(*Model)

	if updated.activeTab != tabContacts {
		t.Fatalf("expected mouse click on title-row tab to switch to Contacts, got tab %d", updated.activeTab)
	}
	if got := updated.composeTo.Value(); got != "" {
		t.Fatalf("expected tab mouse click not to type into compose field, got %q", got)
	}
}

func TestMouseClickTimelineRowOpensPreview(t *testing.T) {
	m := makeMouseTimelineModel(t)

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected row click to open a timeline preview")
	}
	if updated.timeline.selectedEmail.MessageID != "msg-001" {
		t.Fatalf("expected first email selected, got %s", updated.timeline.selectedEmail.MessageID)
	}
	if cmd == nil {
		t.Fatal("expected row click to request body loading")
	}
}

func TestMouseClickCollapsedThreadRootFirstSelectsPreviewWithoutExpanding(t *testing.T) {
	m, root, _ := makeMouseThreadTimelineModel(t)

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected collapsed thread-root click to select the top email")
	}
	if updated.timeline.selectedEmail.MessageID != root.MessageID {
		t.Fatalf("selected email = %q, want %q", updated.timeline.selectedEmail.MessageID, root.MessageID)
	}
	if updated.timeline.expandedThreads[normalizeSubject(root.Subject)] {
		t.Fatal("expected first click on unselected collapsed root to keep thread collapsed")
	}
	if len(updated.timeline.threadRowMap) != 1 {
		t.Fatalf("expected collapsed thread to remain one visible row, got %d", len(updated.timeline.threadRowMap))
	}
	if cmd == nil {
		t.Fatal("expected first collapsed root click to request body loading")
	}
}

func TestMouseClickSelectedCollapsedThreadRootExpandsWithoutRefetch(t *testing.T) {
	m, root, _ := makeMouseThreadTimelineModel(t)
	m.timeline.selectedEmail = root

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if !updated.timeline.expandedThreads[normalizeSubject(root.Subject)] {
		t.Fatal("expected second click on selected collapsed root to expand the thread")
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != root.MessageID {
		t.Fatal("expected selected root email to remain selected after expand")
	}
	if len(updated.timeline.threadRowMap) != 2 {
		t.Fatalf("expected expanded thread rows, got %d", len(updated.timeline.threadRowMap))
	}
	if cmd != nil {
		t.Fatal("expected expand click not to refetch the already selected preview")
	}
}

func TestMouseClickExpandedThreadRootFirstSelectsPreviewWithoutFolding(t *testing.T) {
	m, root, _ := makeMouseThreadTimelineModel(t)
	m.timeline.expandedThreads[normalizeSubject(root.Subject)] = true
	m.updateTimelineTable()

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if updated.timeline.selectedEmail == nil {
		t.Fatal("expected expanded thread-root click to select the top email")
	}
	if updated.timeline.selectedEmail.MessageID != root.MessageID {
		t.Fatalf("selected email = %q, want %q", updated.timeline.selectedEmail.MessageID, root.MessageID)
	}
	if !updated.timeline.expandedThreads[normalizeSubject(root.Subject)] {
		t.Fatal("expected first click on unselected expanded root to keep thread expanded")
	}
	if len(updated.timeline.threadRowMap) != 2 {
		t.Fatalf("expected expanded thread to remain two visible rows, got %d", len(updated.timeline.threadRowMap))
	}
	if cmd == nil {
		t.Fatal("expected first expanded root click to request body loading")
	}
}

func TestMouseClickSelectedExpandedThreadRootFoldsWithoutClearingPreview(t *testing.T) {
	m, root, _ := makeMouseThreadTimelineModel(t)
	m.timeline.expandedThreads[normalizeSubject(root.Subject)] = true
	m.timeline.selectedEmail = root
	m.updateTimelineTable()

	model, cmd := m.Update(mousePress(5, 3))
	updated := model.(*Model)

	if updated.timeline.expandedThreads[normalizeSubject(root.Subject)] {
		t.Fatal("expected second click on selected expanded root to fold the thread")
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != root.MessageID {
		t.Fatal("expected selected root email to remain selected after fold")
	}
	if len(updated.timeline.threadRowMap) != 1 {
		t.Fatalf("expected folded thread to become one visible row, got %d", len(updated.timeline.threadRowMap))
	}
	if cmd != nil {
		t.Fatal("expected fold click not to refetch the already selected preview")
	}
}

func TestMouseWheelTimelinePreviewScrollsBody(t *testing.T) {
	m := makeMouseTimelineModel(t)
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.body = &models.EmailBody{TextPlain: strings.Repeat("line\n", 80)}
	m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
	m.timeline.bodyLoading = false
	m.timeline.bodyWrappedLines = strings.Split(strings.Repeat("line\n", 80), "\n")
	m.setFocusedPanel(panelPreview)
	m.updateTableDimensions(120, 40)

	previewX := m.windowWidth - 3
	model, _ := m.Update(mouseWheelDown(previewX, 10))
	updated := model.(*Model)

	if updated.timeline.bodyScrollOffset != 3 {
		t.Fatalf("expected preview wheel to scroll body by 3 lines, got %d", updated.timeline.bodyScrollOffset)
	}

	model, _ = updated.Update(mouseWheelUp(previewX, 10))
	updated = model.(*Model)
	if updated.timeline.bodyScrollOffset != 0 {
		t.Fatalf("expected preview wheel up to scroll back to top, got %d", updated.timeline.bodyScrollOffset)
	}
}

func TestMouseModeToggleReleasesAndRestoresCapture(t *testing.T) {
	m := makeMouseTimelineModel(t)

	model, cmd := m.Update(keyRunes("m"))
	updated := model.(*Model)
	if !updated.timeline.mouseMode {
		t.Fatal("expected m to enter terminal mouse-selection mode")
	}
	if cmd != nil {
		t.Fatal("expected m to update mouse capture through the next Bubble Tea v2 view")
	}
	if got := updated.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("MouseMode=%v, want MouseModeNone", got)
	}

	model, cmd = updated.Update(keyRunes("m"))
	updated = model.(*Model)
	if updated.timeline.mouseMode {
		t.Fatal("expected second m to restore TUI mouse capture mode")
	}
	if cmd != nil {
		t.Fatal("expected second m to update mouse capture through the next Bubble Tea v2 view")
	}
	if got := updated.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("MouseMode=%v, want MouseModeCellMotion", got)
	}
}

func TestCalendarMouseModeToggleReleasesAndRestoresCapture(t *testing.T) {
	m, _ := makeMouseCalendarModel(t)

	model, cmd := m.Update(keyRunes("m"))
	updated := model.(*Model)
	if !updated.mouseSelectionMode {
		t.Fatal("expected m in Calendar to enter terminal mouse-selection mode")
	}
	if cmd != nil {
		t.Fatal("expected m to update mouse capture through the next Bubble Tea v2 view")
	}
	if got := updated.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("MouseMode=%v, want MouseModeNone", got)
	}

	model, cmd = updated.Update(keyRunes("m"))
	updated = model.(*Model)
	if updated.mouseSelectionMode {
		t.Fatal("expected second m in Calendar to restore TUI mouse capture mode")
	}
	if cmd != nil {
		t.Fatal("expected second m to update mouse capture through the next Bubble Tea v2 view")
	}
	if got := updated.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("MouseMode=%v, want MouseModeCellMotion", got)
	}
}

func TestMouseModeShortcutDoesNotStealTextEntry(t *testing.T) {
	m, _ := makeMouseCalendarModel(t)

	m.activeTab = tabCompose
	m.composeField = composeFieldBody
	m.composeBody.Focus()
	model, _ := m.Update(keyRunes("m"))
	updated := model.(*Model)
	if got := updated.composeBody.Value(); got != "m" {
		t.Fatalf("compose body = %q, want literal m", got)
	}
	if updated.mouseSelectionMode {
		t.Fatal("literal m in Compose toggled mouse selection mode")
	}

	m, _ = makeMouseCalendarModel(t)
	m.openCalendarSearch()
	model, _ = m.Update(keyRunes("m"))
	updated = model.(*Model)
	if got := updated.calendarSearchQuery; got != "m" {
		t.Fatalf("calendar search query = %q, want literal m", got)
	}
	if updated.mouseSelectionMode {
		t.Fatal("literal m in Calendar Search toggled mouse selection mode")
	}

	m, _ = makeMouseCalendarModel(t)
	m.calendarDetail = &m.calendarEvents[0]
	m.openCalendarEdit()
	model, _ = m.Update(keyRunes("m"))
	updated = model.(*Model)
	if got := updated.calendarEdit.Draft.Title; !strings.HasSuffix(got, "m") {
		t.Fatalf("calendar edit title = %q, want literal m appended", got)
	}
	if updated.mouseSelectionMode {
		t.Fatal("literal m in Calendar Edit toggled mouse selection mode")
	}
}

func TestMouseClickCalendarMiniMonthSelectsDay(t *testing.T) {
	m, _ := makeMouseCalendarModel(t)
	m.calendarView = calendarViewDay
	m.calendarDay = calendarDayStartFor(m.calendarEvents[0].Start)
	target := calendarDayStartFor(m.calendarEvents[2].Start)
	x, y := calendarMiniMonthDayPointForTest(t, m, target)

	model, _ := m.Update(mousePress(x, y))
	updated := model.(*Model)

	if !sameCalendarDate(updated.calendarDay, target) {
		t.Fatalf("calendarDay = %s, want clicked day %s", updated.calendarDay, target)
	}
	if updated.calendarFocus != calendarFocusRail {
		t.Fatalf("calendarFocus = %v, want rail after mini-month click", updated.calendarFocus)
	}
}

func TestMouseClickCalendarMiniMonthAnchorsWeekRangeOnClickedDay(t *testing.T) {
	m, _ := makeMouseCalendarModel(t)
	m.calendarView = calendarViewWeek
	m.calendarWeekStart = m.calendarWeekStartFor(m.calendarEvents[0].Start)
	target := time.Date(2026, 6, 2, 12, 0, 0, 0, time.Local)
	x, y := calendarMiniMonthDayPointForTest(t, m, target)

	model, _ := m.Update(mousePress(x, y))
	updated := model.(*Model)

	wantStart := updated.calendarWeekStartFor(target)
	if !sameCalendarDate(updated.calendarWeekStart, wantStart) {
		t.Fatalf("calendarWeekStart = %s, want clicked week %s", updated.calendarWeekStart, wantStart)
	}
	if !sameCalendarDate(updated.selectedCalendarDay(), target) {
		t.Fatalf("selectedCalendarDay = %s, want clicked day %s", updated.selectedCalendarDay(), target)
	}
}

func TestMouseClickCalendarMiniMonthAnchorsThreeDayRangeOnClickedDay(t *testing.T) {
	m, _ := makeMouseCalendarModel(t)
	m.calendarView = calendarViewThreeDay
	m.calendarThreeDayStart = calendarDayStartFor(m.calendarEvents[0].Start)
	target := time.Date(2026, 6, 4, 12, 0, 0, 0, time.Local)
	x, y := calendarMiniMonthDayPointForTest(t, m, target)

	model, _ := m.Update(mousePress(x, y))
	updated := model.(*Model)

	if !sameCalendarDate(updated.calendarThreeDayStart, target) {
		t.Fatalf("calendarThreeDayStart = %s, want clicked day %s", updated.calendarThreeDayStart, target)
	}
	if !sameCalendarDate(updated.selectedCalendarDay(), target) {
		t.Fatalf("selectedCalendarDay = %s, want clicked day %s", updated.selectedCalendarDay(), target)
	}
}

func TestMouseClickCalendarMiniMonthAnchorsAgendaMonthOnClickedDay(t *testing.T) {
	m, _ := makeMouseCalendarModel(t)
	m.calendarView = calendarViewAgenda
	target := time.Date(2026, 6, 2, 12, 0, 0, 0, time.Local)
	x, y := calendarMiniMonthDayPointForTest(t, m, target)

	model, _ := m.Update(mousePress(x, y))
	updated := model.(*Model)

	wantStart, wantEnd := calendarAgendaWindowFor(target)
	if !sameCalendarDate(updated.calendarAgendaStart, wantStart) || !sameCalendarDate(updated.calendarAgendaEnd, wantEnd) {
		t.Fatalf("agenda range = %s - %s, want %s - %s", updated.calendarAgendaStart, updated.calendarAgendaEnd, wantStart, wantEnd)
	}
	if !sameCalendarDate(updated.selectedCalendarDay(), target) {
		t.Fatalf("selectedCalendarDay = %s, want clicked day %s", updated.selectedCalendarDay(), target)
	}
}

func TestMouseClickCalendarEventSelectsWithoutOpeningDetail(t *testing.T) {
	m, _ := makeMouseCalendarModel(t)
	x, y := calendarAgendaEventPointForTest(t, m, 1)

	model, cmd := m.Update(mousePress(x, y))
	updated := model.(*Model)

	if cmd != nil {
		t.Fatal("expected single event click to select without opening detail")
	}
	if updated.calendarDetailOpen {
		t.Fatal("single event click opened full calendar detail")
	}
	if updated.calendarDetail == nil || updated.calendarDetail.Title != "Daily standup" {
		t.Fatalf("calendarDetail = %#v, want Daily standup", updated.calendarDetail)
	}
}

func TestMouseDoubleClickCalendarEventOpensDetail(t *testing.T) {
	m, _ := makeMouseCalendarModel(t)
	x, y := calendarAgendaEventPointForTest(t, m, 1)

	model, cmd := m.Update(mousePress(x, y))
	updated := model.(*Model)
	if cmd != nil {
		t.Fatal("expected first event click to select only")
	}
	model, cmd = updated.Update(mousePress(x, y))
	updated = model.(*Model)
	for _, msg := range calendarImmediateMessagesForTest(cmd) {
		model, _ = updated.Update(msg)
		updated = model.(*Model)
	}

	if !updated.calendarDetailOpen {
		t.Fatal("expected double click on selected event to open full calendar detail")
	}
	if updated.calendarDetail == nil || updated.calendarDetail.Title != "Daily standup" {
		t.Fatalf("calendarDetail = %#v, want Daily standup", updated.calendarDetail)
	}
}

func TestMouseClickCalendarRailTogglePersistsSelection(t *testing.T) {
	m, configPath := makeMouseCalendarModel(t)
	x, y := calendarCollectionPointForTest(t, m, 1)

	model, _ := m.Update(mousePress(x, y))
	updated := model.(*Model)

	familyKey := calendarCollectionRefKey(updated.calendarCollections[1].Ref)
	if !updated.calendarHiddenCollections[familyKey] {
		t.Fatalf("expected family calendar to be hidden after checkbox click")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile config: %v", err)
	}
	if strings.Contains(string(data), familyKey) {
		t.Fatalf("hidden calendar key %q was persisted as selected:\n%s", familyKey, string(data))
	}
	if !strings.Contains(string(data), "selected_calendars:") || !strings.Contains(string(data), calendarCollectionRefKey(updated.calendarCollections[0].Ref)) {
		t.Fatalf("config did not persist visible selected calendar:\n%s", string(data))
	}

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}
	next, _ := makeMouseCalendarModel(t)
	next.SetConfig(reloaded)
	next.setCalendarCollections(updated.calendarCollections)
	next.pruneCalendarCollectionState()
	if !next.calendarHiddenCollections[familyKey] {
		t.Fatalf("expected hidden calendar selection to restore from YAML")
	}
}

func calendarMiniMonthDayPointForTest(t *testing.T, m *Model, day time.Time) (int, int) {
	t.Helper()
	start, _, _ := m.calendarActiveRange()
	month := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
	gridStart := m.calendarWeekStartFor(month)
	day = calendarDayStartFor(day)
	offset := int(day.Sub(gridStart).Hours() / 24)
	if offset < 0 || offset >= 42 {
		t.Fatalf("day %s not in mini-month grid starting %s", day, gridStart)
	}
	top := m.mouseContentTop()
	return 2 + (offset%7)*3, top + 1 + 4 + (offset / 7)
}

func calendarCollectionPointForTest(t *testing.T, m *Model, collectionIndex int) (int, int) {
	t.Helper()
	if collectionIndex < 0 || collectionIndex >= len(m.calendarCollections) {
		t.Fatalf("collection index %d out of range", collectionIndex)
	}
	top := m.mouseContentTop()
	return 3, top + 1 + 14 + collectionIndex
}

func calendarAgendaEventPointForTest(t *testing.T, m *Model, visibleEventOffset int) (int, int) {
	t.Helper()
	visible := m.indexedVisibleCalendarEvents()
	if visibleEventOffset < 0 || visibleEventOffset >= len(visible) {
		t.Fatalf("event offset %d out of range", visibleEventOffset)
	}
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	railW, mainW, _ := m.calendarMousePanelWidths(plan)
	x := railW + panelGapWidth + 3
	if railW == 0 {
		x = 3
	}
	if mainW <= 0 {
		t.Fatal("calendar main panel width is not positive")
	}
	contentY := 0
	if status := m.visibleCalendarStatus(); status != "" {
		_ = status
		contentY++
	}
	if hiddenPast := m.calendarAgendaHiddenPastCount(); hiddenPast > 0 {
		contentY += len(m.calendarAgendaPastNoticeLines(hiddenPast))
	}
	var lastDay time.Time
	for i, item := range visible {
		day := calendarDayStartFor(item.event.Start)
		if lastDay.IsZero() || !sameCalendarDate(day, lastDay) {
			contentY++
			lastDay = day
		}
		if i == visibleEventOffset {
			break
		}
		contentY++
	}
	return x, m.mouseContentTop() + 1 + contentY
}
