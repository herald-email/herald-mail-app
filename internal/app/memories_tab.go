package app

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/memory"
)

const (
	memoriesFocusRail = iota
	memoriesFocusList
	memoriesFocusDetail
	memoriesFocusSources
)

type memoryExploreState struct {
	token      int
	result     memory.ExploreResult
	loading    bool
	err        string
	filter     string
	dateRange  string
	searchMode bool
	search     string
	railIdx    int
	rowIdx     int
	sourceIdx  int
	focusPanel int
	sourceOpen bool
	hiddenRows map[string]bool
	status     string
}

type memoriesExplorerBackend interface {
	ExploreMemories(context.Context, memory.ExploreQuery) (memory.ExploreResult, error)
}

type memoriesLayoutSpec struct {
	width       int
	height      int
	contentH    int
	railW       int
	listW       int
	detailW     int
	topH        int
	bottomH     int
	rightX      int
	listX       int
	detailX     int
	sourceY     int
	sourceW     int
	sourceInner int
}

type memoryRailChoice struct {
	kind  string
	id    string
	label string
	count int
}

func (m *Model) ensureMemoriesDefaults() {
	if strings.TrimSpace(m.memories.filter) == "" {
		m.memories.filter = memory.ExploreFilterAll
	}
	if strings.TrimSpace(m.memories.dateRange) == "" {
		m.memories.dateRange = memory.ExploreDateAny
	}
	if m.memories.focusPanel < memoriesFocusRail || m.memories.focusPanel > memoriesFocusSources {
		m.memories.focusPanel = memoriesFocusList
	}
	if m.memories.hiddenRows == nil {
		m.memories.hiddenRows = make(map[string]bool)
	}
	m.syncMemoryRailCursor()
}

func (m *Model) memoriesQuery() memory.ExploreQuery {
	m.ensureMemoriesDefaults()
	return memory.ExploreQuery{
		Text:      strings.TrimSpace(m.memories.search),
		Filter:    m.memories.filter,
		DateRange: m.memories.dateRange,
		Limit:     120,
		Now:       time.Now(),
	}
}

func (m *Model) loadMemoriesExplore() tea.Cmd {
	m.ensureMemoriesDefaults()
	m.memories.token++
	token := m.memories.token
	query := m.memoriesQuery()
	m.memories.loading = true
	m.memories.err = ""
	m.memories.sourceOpen = false
	explorer, ok := m.backend.(memoriesExplorerBackend)
	if !ok || explorer == nil {
		return func() tea.Msg {
			return MemoriesExploreLoadedMsg{Token: token, Result: memory.BuildExploreResult(nil, query)}
		}
	}
	return func() tea.Msg {
		result, err := explorer.ExploreMemories(context.Background(), query)
		return MemoriesExploreLoadedMsg{Token: token, Result: result, Err: err}
	}
}

func (m *Model) normalizeMemoriesSelection() {
	rows := m.memoryVisibleRows()
	if len(rows) == 0 {
		m.memories.rowIdx = 0
		m.memories.sourceIdx = 0
		return
	}
	if m.memories.rowIdx < 0 {
		m.memories.rowIdx = 0
	}
	if m.memories.rowIdx >= len(rows) {
		m.memories.rowIdx = len(rows) - 1
	}
	sources := rows[m.memories.rowIdx].Sources
	if len(sources) == 0 {
		m.memories.sourceIdx = 0
		return
	}
	if m.memories.sourceIdx < 0 {
		m.memories.sourceIdx = 0
	}
	if m.memories.sourceIdx >= len(sources) {
		m.memories.sourceIdx = len(sources) - 1
	}
}

func (m *Model) handleMemoriesKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.ensureMemoriesDefaults()
	key := shortcutKey(msg)
	command, hasCommand := m.scopedCommand("memories", key)

	if m.memories.searchMode {
		switch key {
		case "esc":
			if strings.TrimSpace(m.memories.search) != "" {
				m.memories.search = ""
				m.memories.searchMode = false
				m.memories.rowIdx = 0
				return m, m.loadMemoriesExplore()
			}
			m.memories.searchMode = false
			return m, nil
		case "enter":
			m.memories.searchMode = false
			m.memories.rowIdx = 0
			return m, m.loadMemoriesExplore()
		case "tab", "ctrl+i":
			m.memories.searchMode = false
			m.cycleMemoriesFocus(true)
			return m, nil
		case "shift+tab":
			m.memories.searchMode = false
			m.cycleMemoriesFocus(false)
			return m, nil
		case "backspace", "ctrl+h":
			if m.memories.search != "" {
				runes := []rune(m.memories.search)
				m.memories.search = string(runes[:len(runes)-1])
				m.memories.rowIdx = 0
				return m, m.loadMemoriesExplore()
			}
			return m, nil
		}
		if msg.Text != "" && msg.Mod == 0 {
			m.memories.search += msg.Text
			m.memories.rowIdx = 0
			return m, m.loadMemoriesExplore()
		}
		return m, nil
	}

	switch {
	case key == "/" || hasCommand && command == CommandHelpSearch:
		m.memories.searchMode = true
		m.memories.focusPanel = memoriesFocusList
		return m, nil
	case key == "esc":
		if m.memories.sourceOpen {
			m.memories.sourceOpen = false
			m.memories.focusPanel = memoriesFocusSources
			return m, nil
		}
		if m.memories.focusPanel != memoriesFocusList {
			m.memories.focusPanel = memoriesFocusList
			return m, nil
		}
		if strings.TrimSpace(m.memories.search) != "" {
			m.memories.search = ""
			m.memories.rowIdx = 0
			return m, m.loadMemoriesExplore()
		}
		return m, nil
	case key == "tab" || key == "ctrl+i":
		m.cycleMemoriesFocus(true)
		return m, nil
	case key == "shift+tab":
		m.cycleMemoriesFocus(false)
		return m, nil
	case hasCommand && command == CommandPaneRight:
		m.cycleMemoriesFocus(true)
		return m, nil
	case hasCommand && command == CommandPaneLeft:
		m.cycleMemoriesFocus(false)
		return m, nil
	case key == "r":
		m.memories.status = "Refreshing memory explorer..."
		return m, m.loadMemoriesExplore()
	case key == "o":
		return m, m.reportMemoryOpenIntent()
	case key == "x":
		m.hideSelectedMemoryRow()
		return m, nil
	case key == "enter":
		m.openSelectedMemorySource()
		return m, nil
	case hasCommand && command == CommandPaneDown:
		return m, m.moveMemoriesSelection(1)
	case hasCommand && command == CommandPaneUp:
		return m, m.moveMemoriesSelection(-1)
	}
	return m, nil
}

func (m *Model) cycleMemoriesFocus(forward bool) {
	if forward {
		m.memories.focusPanel = (m.memories.focusPanel + 1) % 4
	} else {
		m.memories.focusPanel--
		if m.memories.focusPanel < 0 {
			m.memories.focusPanel = memoriesFocusSources
		}
	}
}

func (m *Model) moveMemoriesSelection(delta int) tea.Cmd {
	switch m.memories.focusPanel {
	case memoriesFocusRail:
		choices := m.memoryRailChoices()
		if len(choices) == 0 {
			return nil
		}
		m.memories.railIdx += delta
		if m.memories.railIdx < 0 {
			m.memories.railIdx = 0
		}
		if m.memories.railIdx >= len(choices) {
			m.memories.railIdx = len(choices) - 1
		}
		m.applyMemoryRailChoice(choices[m.memories.railIdx])
		m.memories.rowIdx = 0
		m.memories.sourceIdx = 0
		return m.loadMemoriesExplore()
	case memoriesFocusSources:
		row, ok := m.selectedMemoryRow()
		if !ok || len(row.Sources) == 0 {
			return nil
		}
		m.memories.sourceIdx += delta
		if m.memories.sourceIdx < 0 {
			m.memories.sourceIdx = 0
		}
		if m.memories.sourceIdx >= len(row.Sources) {
			m.memories.sourceIdx = len(row.Sources) - 1
		}
	default:
		rows := m.memoryVisibleRows()
		if len(rows) == 0 {
			return nil
		}
		m.memories.rowIdx += delta
		if m.memories.rowIdx < 0 {
			m.memories.rowIdx = 0
		}
		if m.memories.rowIdx >= len(rows) {
			m.memories.rowIdx = len(rows) - 1
		}
		m.memories.sourceIdx = 0
	}
	return nil
}

func (m *Model) reportMemoryOpenIntent() tea.Cmd {
	row, ok := m.selectedMemoryRow()
	if !ok {
		m.memories.status = "No memory selected."
		return nil
	}
	target := strings.TrimSpace(row.ObsidianTarget)
	if target == "" && m.memories.sourceIdx < len(row.Sources) {
		source := row.Sources[m.memories.sourceIdx]
		if source.SourceType == memory.SourceObsidian {
			target = strings.TrimSpace(source.Label)
		}
	}
	if target == "" {
		m.memories.status = "No Obsidian target for this memory."
		return nil
	}
	m.memories.status = "Vault target: " + target
	return nil
}

func (m *Model) hideSelectedMemoryRow() {
	row, ok := m.selectedMemoryRow()
	if !ok {
		m.memories.status = "No memory selected."
		return
	}
	if m.memories.hiddenRows == nil {
		m.memories.hiddenRows = make(map[string]bool)
	}
	m.memories.hiddenRows[memoryRowKey(row)] = true
	m.memories.status = "Dismissed locally for this session: " + row.Title
	m.normalizeMemoriesSelection()
}

func (m *Model) openSelectedMemorySource() {
	row, ok := m.selectedMemoryRow()
	if !ok {
		m.memories.status = "No memory selected."
		return
	}
	if len(row.Sources) == 0 {
		m.memories.status = "No source evidence on selected memory."
		return
	}
	m.memories.sourceOpen = true
	m.memories.focusPanel = memoriesFocusSources
	source := row.Sources[m.memories.sourceIdx]
	m.memories.status = "Inspecting source: " + memoryFirstNonEmpty(source.Label, source.SourceType)
}

func (m *Model) selectedMemoryRow() (memory.ExploreRow, bool) {
	rows := m.memoryVisibleRows()
	if len(rows) == 0 || m.memories.rowIdx < 0 || m.memories.rowIdx >= len(rows) {
		return memory.ExploreRow{}, false
	}
	return rows[m.memories.rowIdx], true
}

func (m *Model) memoryVisibleRows() []memory.ExploreRow {
	rows := m.memories.result.Rows
	if len(rows) == 0 || len(m.memories.hiddenRows) == 0 {
		return rows
	}
	out := make([]memory.ExploreRow, 0, len(rows))
	for _, row := range rows {
		if !m.memories.hiddenRows[memoryRowKey(row)] {
			out = append(out, row)
		}
	}
	return out
}

func (m *Model) memoryRailChoices() []memoryRailChoice {
	facets := m.memories.result.Facets
	if len(facets.Filters) == 0 {
		facets = memory.BuildExploreResult(nil, memory.ExploreQuery{}).Facets
	}
	var choices []memoryRailChoice
	for _, facet := range facets.Filters {
		choices = append(choices, memoryRailChoice{kind: "filter", id: facet.ID, label: facet.Label, count: facet.Count})
	}
	for _, facet := range facets.Sources {
		choices = append(choices, memoryRailChoice{kind: "filter", id: facet.ID, label: facet.Label, count: facet.Count})
	}
	for _, facet := range facets.Dates {
		choices = append(choices, memoryRailChoice{kind: "date", id: facet.ID, label: facet.Label, count: facet.Count})
	}
	return choices
}

func (m *Model) syncMemoryRailCursor() {
	choices := m.memoryRailChoices()
	for i, choice := range choices {
		if choice.kind == "date" && choice.id == m.memories.dateRange {
			m.memories.railIdx = i
			return
		}
		if choice.kind == "filter" && choice.id == m.memories.filter {
			m.memories.railIdx = i
			return
		}
	}
	if m.memories.railIdx < 0 || m.memories.railIdx >= len(choices) {
		m.memories.railIdx = 0
	}
}

func (m *Model) applyMemoryRailChoice(choice memoryRailChoice) {
	switch choice.kind {
	case "date":
		m.memories.dateRange = choice.id
	default:
		m.memories.filter = choice.id
	}
	m.memories.status = choice.label
}

func (m *Model) renderMemoriesTab(width, height int) string {
	if width < 20 {
		return "Terminal too narrow"
	}
	m.ensureMemoriesDefaults()
	m.normalizeMemoriesSelection()
	spec := m.memoriesLayout(width, height)
	active := m.theme.Chrome.TabActive.BackgroundColor()
	inactive := m.theme.Focus.PanelBorder.ForegroundColor()
	panel := func(focus int, w, h int, title, hint, body string) string {
		border := inactive
		if m.memories.focusPanel == focus {
			border = active
		}
		innerW := w - 4
		if innerW < 1 {
			innerW = 1
		}
		innerH := h - 2
		if innerH < 1 {
			innerH = 1
		}
		rendered := m.memoryPanel(border, w, h).Render(fitCalendarPanelContent(body, innerW, innerH))
		return m.renderCalendarPanelFrameChrome(rendered, calendarPanelFrameChrome{LeftTitle: title, RightHint: hint})
	}

	rail := panel(memoriesFocusRail, spec.railW, spec.contentH, "Filters", "Tab", m.renderMemoriesRail(spec.railW-4, spec.contentH-2))
	list := panel(memoriesFocusList, spec.listW, spec.topH, m.memoriesListFrameTitle(), "/", m.renderMemoriesList(spec.listW-4, spec.topH-2))
	detail := panel(memoriesFocusDetail, spec.detailW, spec.topH, m.memoriesDetailFrameTitle(), "Enter", m.renderMemoriesDetail(spec.detailW-4, spec.topH-2))
	sources := panel(memoriesFocusSources, spec.sourceW, spec.bottomH, "Sources + Actions", "read-only", m.renderMemoriesSources(spec.sourceInner, spec.bottomH-2))

	topRight := lipgloss.JoinHorizontal(lipgloss.Top, list, panelGap, detail)
	right := lipgloss.JoinVertical(lipgloss.Left, topRight, sources)
	return lipgloss.JoinHorizontal(lipgloss.Top, rail, panelGap, right)
}

func (m *Model) memoryPanel(border color.Color, width, height int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(border).
		Width(width).
		Height(height).
		Padding(0, 1)
}

func (m *Model) memoriesListFrameTitle() string {
	rows := len(m.memoryVisibleRows())
	if m.memories.result.Capped {
		return fmt.Sprintf("Memory Explorer (%d of %d)", rows, m.memories.result.Total)
	}
	return fmt.Sprintf("Memory Explorer (%d)", rows)
}

func (m *Model) memoriesDetailFrameTitle() string {
	row, ok := m.selectedMemoryRow()
	if !ok {
		return "Dossier"
	}
	return memoryFirstNonEmpty(row.Title, "Dossier")
}

func (m *Model) memoriesLayout(width, height int) memoriesLayoutSpec {
	plan := m.buildLayoutPlan(width, height)
	contentW := width
	if plan.ChatVisible {
		contentW -= m.chatLayoutDeduction(width)
	}
	if contentW < 20 {
		contentW = 20
	}
	contentH := plan.ContentHeight
	if contentH < 5 {
		contentH = 5
	}
	availableRight := contentW - panelGapWidth
	railW := 30
	if contentW < 120 {
		railW = 24
	}
	if contentW < 90 {
		railW = 20
	}
	if railW > contentW/3 {
		railW = contentW / 3
	}
	if railW < 16 {
		railW = 16
	}
	rightW := availableRight - railW
	if rightW < 30 {
		rightW = 30
	}
	topH := contentH - 9
	if topH < contentH*2/3 {
		topH = contentH * 2 / 3
	}
	if topH < 7 {
		topH = 7
	}
	if topH > contentH-4 {
		topH = contentH - 4
	}
	bottomH := contentH - topH
	if bottomH < 4 {
		bottomH = 4
		topH = contentH - bottomH
	}
	rightAvailable := rightW - panelGapWidth
	listW := rightAvailable * 58 / 100
	if listW < 26 {
		listW = 26
	}
	detailW := rightAvailable - listW
	if detailW < 24 {
		detailW = 24
		listW = rightAvailable - detailW
	}
	if listW < 18 {
		listW = 18
	}
	return memoriesLayoutSpec{
		width:       contentW,
		height:      height,
		contentH:    contentH,
		railW:       railW,
		listW:       listW,
		detailW:     detailW,
		topH:        topH,
		bottomH:     bottomH,
		rightX:      railW + panelGapWidth,
		listX:       railW + panelGapWidth,
		detailX:     railW + panelGapWidth + listW + panelGapWidth,
		sourceY:     topH,
		sourceW:     rightW,
		sourceInner: rightW - 4,
	}
}

func (m *Model) renderMemoriesRail(width, height int) string {
	var lines []string
	appendFacet := func(section string, facets []memory.FacetCount, kind string) {
		if section != "" {
			lines = append(lines, m.memoryDim(strings.ToUpper(section), width))
		}
		for _, facet := range facets {
			choice := memoryRailChoice{kind: kind, id: facet.ID, label: facet.Label, count: facet.Count}
			if kind == "display" {
				lines = append(lines, m.renderMemoryRailDisplayLine(choice, width))
				continue
			}
			selected := false
			if kind == "date" {
				selected = facet.ID == m.memories.dateRange
			} else {
				selected = facet.ID == m.memories.filter
			}
			lines = append(lines, m.renderMemoryRailLine(choice, selected, width))
		}
	}
	facets := m.memories.result.Facets
	if len(facets.Filters) == 0 {
		facets = memory.BuildExploreResult(nil, memory.ExploreQuery{}).Facets
	}
	appendFacet("", facets.Filters, "filter")
	lines = append(lines, "")
	appendFacet("tracks", facets.Tracks, "display")
	lines = append(lines, "")
	appendFacet("sources", facets.Sources, "filter")
	lines = append(lines, "")
	appendFacet("dates", facets.Dates, "date")
	if m.memories.loading {
		lines = append(lines, "", m.memoryDim("refreshing...", width))
	}
	if m.memories.err != "" {
		lines = append(lines, m.memoryError(m.memories.err, width))
	}
	return memoryJoinFit(lines, width, height)
}

func (m *Model) renderMemoryRailDisplayLine(choice memoryRailChoice, width int) string {
	labelW := width - 7
	if labelW < 4 {
		labelW = 4
	}
	line := "  " + memoryFit(choice.label, labelW) + " " + fmt.Sprintf("%4d", choice.count)
	return m.theme.Text.Primary.Style().Render(memoryFitPad(line, width))
}

func (m *Model) renderMemoryRailLine(choice memoryRailChoice, selected bool, width int) string {
	labelW := width - 7
	if labelW < 4 {
		labelW = 4
	}
	prefix := "  "
	if selected {
		prefix = "> "
	}
	count := fmt.Sprintf("%4d", choice.count)
	line := prefix + memoryFit(choice.label, labelW) + " " + count
	if selected || m.memories.focusPanel == memoriesFocusRail && m.memoryRailChoices()[m.memories.railIdx].id == choice.id {
		return m.theme.Focus.SelectionActive.Style().Render(memoryFitPad(line, width))
	}
	return m.theme.Text.Primary.Style().Render(memoryFitPad(line, width))
}

func (m *Model) renderMemoriesList(width, height int) string {
	title := "/ Search memories"
	if m.memories.searchMode {
		title = "/ " + m.memories.search + "_"
	} else if strings.TrimSpace(m.memories.search) != "" {
		title = "/ " + m.memories.search
	}
	rows := m.memoryVisibleRows()
	subtitle := fmt.Sprintf("%d shown", len(rows))
	if m.memories.result.Capped {
		subtitle = fmt.Sprintf("%d of %d", len(rows), m.memories.result.Total)
	}
	lines := []string{
		m.memoryDim(title+"  "+subtitle+"  "+m.memoriesFilterLabel(), width),
		m.memoryRule(width),
		m.memoryDim(m.memoriesListHeader(width), width),
	}
	if m.memories.loading && len(rows) == 0 {
		lines = append(lines, m.memoryDim("Loading source-backed memories...", width))
		return memoryJoinFit(lines, width, height)
	}
	if len(rows) == 0 {
		lines = append(lines, m.memoryDim("No memory rows match this query.", width))
		return memoryJoinFit(lines, width, height)
	}
	maxRows := height - len(lines)
	if maxRows < 1 {
		maxRows = 1
	}
	start := 0
	if m.memories.rowIdx >= maxRows {
		start = m.memories.rowIdx - maxRows + 1
	}
	end := start + maxRows
	if end > len(rows) {
		end = len(rows)
	}
	for i := start; i < end; i++ {
		lines = append(lines, m.renderMemoryRow(rows[i], i == m.memories.rowIdx, width))
	}
	return memoryJoinFit(lines, width, height)
}

func (m *Model) renderMemoryRow(row memory.ExploreRow, selected bool, width int) string {
	spec := memoryTableSpecForWidth(width)
	marker := memoryRowMarker(row)
	status := memoryStatusLabel(row)
	date := memoryDateLabel(row.LastActivityAt)
	plain := memoryTablePlainLine(marker, row.Title, status, memorySourceLabel(row.SourceLabel), date, spec)
	if selected {
		return m.theme.Focus.SelectionActive.Style().Render(plain)
	}
	line := m.memoryMarkerStyle(row).Render(memoryFitPad(marker, spec.markerW)) +
		" " + m.theme.Text.Primary.Style().Render(memoryFitPad(row.Title, spec.titleW)) +
		" " + m.memoryStatusStyle(row).Render(memoryFitPad(status, spec.statusW))
	if spec.sourceW > 0 {
		line += " " + m.theme.Text.Primary.Style().Render(memoryFitPad(memorySourceLabel(row.SourceLabel), spec.sourceW))
	}
	line += " " + m.theme.Text.Dim.Style().Render(memoryFitPad(date, spec.dateW))
	return padANSIToWidth(line, width)
}

func (m *Model) renderMemoriesDetail(width, height int) string {
	row, ok := m.selectedMemoryRow()
	if !ok {
		return memoryJoinFit([]string{
			m.memoryDim("No selected memory.", width),
		}, width, height)
	}
	lines := []string{
		m.memoryField("Track", memoryTrackDisplay(row), width),
		m.memoryField("Type", memoryTypeDisplay(row), width),
		m.memoryFieldStyled("Status", m.memoryStatusStyle(row).Render(memoryFit(memoryStatusLabel(row), width-12)), width),
		m.memoryFieldStyled("Confidence", m.memoryConfidenceMetricLine(row.Confidence, fmt.Sprintf("%.2f", row.Confidence), width-12), width),
		m.memoryFieldStyled("Freshness", m.memoryMetricLine(memoryFreshnessScore(row), memoryAgeLabel(row.LastActivityAt), width-12), width),
	}
	if people := memoryPeopleDisplay(row.People); people != "" {
		lines = append(lines, m.memoryField("People", people, width))
	}
	if row.Company != "" {
		lines = append(lines, m.memoryField("Company", row.Company, width))
	}
	if topics := memoryTopicsDisplay(row); topics != "" {
		lines = append(lines, m.memoryField("Topics", topics, width))
	}
	lines = append(lines,
		m.memoryRule(width),
	)
	if claim := memoryRowClaim(row); claim != "" {
		lines = appendWrappedMemoryLines(lines, "Claim", claim, width, 4, m)
	} else if row.Summary != "" {
		lines = appendWrappedMemoryLines(lines, "Summary", row.Summary, width, 3, m)
	}
	if row.ReviewReason != "" {
		lines = appendWrappedMemoryLines(lines, "Review", row.ReviewReason, width, 2, m)
	}
	if len(row.Sources) > 0 {
		lines = append(lines, m.memoryRule(width))
		lines = append(lines, m.memoryDim("Evidence (source-backed)", width))
		maxSources := 4
		if remaining := height - len(lines) - 1; remaining > 0 && remaining < maxSources {
			maxSources = remaining
		}
		for i, source := range row.Sources {
			if i >= maxSources {
				break
			}
			lines = append(lines, m.renderMemoryEvidenceLine(source, width))
		}
		if len(row.Sources) > maxSources {
			lines = append(lines, m.memoryDim(fmt.Sprintf("... %d more items", len(row.Sources)-maxSources), width))
		}
	}
	if m.memories.sourceOpen {
		lines = append(lines, "", m.memoryDim("Source inspection is open below.", width))
	}
	return memoryJoinFit(lines, width, height)
}

func (m *Model) renderMemoriesSources(width, height int) string {
	row, ok := m.selectedMemoryRow()
	if !ok {
		return memoryJoinFit([]string{m.memoryDim("No memory selected.", width)}, width, height)
	}
	if width >= 86 && height >= 5 {
		return m.renderMemoriesSourceColumns(row, width, height)
	}
	lines := []string{
		m.memoryDim("Enter inspect  o vault intent  x local dismiss  read-only", width),
		m.memoryRule(width),
	}
	if len(row.Sources) == 0 {
		lines = append(lines, m.memoryDim("No source evidence available.", width))
	} else if m.memories.sourceOpen {
		source := row.Sources[m.memories.sourceIdx]
		lines = append(lines, m.memoryField("Type", source.SourceType, width))
		lines = append(lines, m.memoryField("Label", source.Label, width))
		if source.Folder != "" {
			lines = append(lines, m.memoryField("Folder", source.Folder, width))
		}
		if !source.Date.IsZero() {
			lines = append(lines, m.memoryField("Date", source.Date.Local().Format("Jan 2 15:04"), width))
		}
		if source.Snippet != "" {
			lines = appendWrappedMemoryLines(lines, "Snippet", source.Snippet, width, height-len(lines)-1, m)
		}
	} else {
		for i, source := range row.Sources {
			prefix := "  "
			if i == m.memories.sourceIdx {
				prefix = "> "
			}
			line := prefix + memoryFit(memoryFirstNonEmpty(source.Folder, source.SourceType), 12) + " " + memoryFit(source.Label, width-15)
			if i == m.memories.sourceIdx && m.memories.focusPanel == memoriesFocusSources {
				lines = append(lines, m.theme.Focus.SelectionActive.Style().Render(memoryFitPad(line, width)))
			} else {
				lines = append(lines, m.theme.Text.Primary.Style().Render(memoryFitPad(line, width)))
			}
			if source.Snippet != "" && height >= 8 {
				lines = append(lines, m.memoryDim("    "+source.Snippet, width))
			}
		}
	}
	if status := strings.TrimSpace(m.memories.status); status != "" {
		lines = append(lines, "", m.memoryDim(status, width))
	}
	return memoryJoinFit(lines, width, height)
}

func (m *Model) renderMemoriesSourceColumns(row memory.ExploreRow, width, height int) string {
	separator := m.theme.Text.Dim.Style().Render(" │ ")
	separatorW := 3
	actionsW := width * 24 / 100
	if actionsW < 24 {
		actionsW = 24
	}
	obsidianW := width * 30 / 100
	if obsidianW < 28 {
		obsidianW = 28
	}
	sourceW := width - actionsW - obsidianW - separatorW*2
	if sourceW < 30 {
		sourceW = 30
		obsidianW = width - actionsW - sourceW - separatorW*2
	}
	if obsidianW < 18 {
		return m.renderMemoriesSourcesCompact(row, width, height)
	}

	actionLines := m.memorySourceColumnLines("Actions", []string{
		"Inspect source        Enter",
		"Open vault intent     o",
		"Dismiss locally       x",
		"Refresh evidence      r",
		"Read-only v1          --",
	}, actionsW)
	sourceLines := m.memorySourceLinksLines(row, sourceW)
	obsidianLines := m.memoryObsidianLines(row, obsidianW)

	left := strings.Split(memoryJoinFit(actionLines, actionsW, height), "\n")
	middle := strings.Split(memoryJoinFit(sourceLines, sourceW, height), "\n")
	right := strings.Split(memoryJoinFit(obsidianLines, obsidianW, height), "\n")
	lines := make([]string, height)
	for i := range lines {
		line := left[i] + separator + middle[i] + separator + right[i]
		lines[i] = ansi.Cut(line, 0, width)
		if missing := width - ansi.StringWidth(lines[i]); missing > 0 {
			lines[i] += strings.Repeat(" ", missing)
		}
	}
	if status := strings.TrimSpace(m.memories.status); status != "" && height > 0 {
		lines[height-1] = m.memoryDim(status, width)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderMemoriesSourcesCompact(row memory.ExploreRow, width, height int) string {
	lines := []string{
		m.memoryDim("Enter inspect  o vault intent  x local dismiss  read-only", width),
		m.memoryRule(width),
	}
	if len(row.Sources) == 0 {
		lines = append(lines, m.memoryDim("No source evidence available.", width))
		return memoryJoinFit(lines, width, height)
	}
	for i, source := range row.Sources {
		prefix := "  "
		if i == m.memories.sourceIdx {
			prefix = "> "
		}
		line := prefix + memoryFit(memorySourceLabel(memoryFirstNonEmpty(source.Folder, source.SourceType)), 12) + " " + memoryFit(source.Label, width-15)
		if i == m.memories.sourceIdx && m.memories.focusPanel == memoriesFocusSources {
			lines = append(lines, m.theme.Focus.SelectionActive.Style().Render(memoryFitPad(line, width)))
		} else {
			lines = append(lines, m.theme.Text.Primary.Style().Render(memoryFitPad(line, width)))
		}
	}
	if status := strings.TrimSpace(m.memories.status); status != "" {
		lines = append(lines, "", m.memoryDim(status, width))
	}
	return memoryJoinFit(lines, width, height)
}

func (m *Model) memorySourceColumnLines(title string, items []string, width int) []string {
	lines := []string{
		m.memoryDim(strings.ToUpper(title), width),
	}
	for _, item := range items {
		lines = append(lines, m.theme.Text.Primary.Style().Render(memoryFitPad(item, width)))
	}
	return lines
}

func (m *Model) memorySourceLinksLines(row memory.ExploreRow, width int) []string {
	lines := []string{
		m.memoryDim("SOURCE LINKS", width),
	}
	if len(row.Sources) == 0 {
		return append(lines, m.memoryDim("No source evidence.", width))
	}
	if m.memories.sourceOpen && m.memories.sourceIdx < len(row.Sources) {
		source := row.Sources[m.memories.sourceIdx]
		lines = append(lines,
			m.memoryField("Type", memorySourceLabel(source.SourceType), width),
			m.memoryField("Label", source.Label, width),
		)
		if source.Folder != "" {
			lines = append(lines, m.memoryField("Folder", source.Folder, width))
		}
		if !source.Date.IsZero() {
			lines = append(lines, m.memoryField("Date", source.Date.Local().Format("Jan 2 15:04"), width))
		}
		if source.Snippet != "" {
			lines = appendWrappedMemoryLines(lines, "Snippet", source.Snippet, width, 2, m)
		}
		return lines
	}
	for i, source := range row.Sources {
		prefix := "  "
		if i == m.memories.sourceIdx {
			prefix = "> "
		}
		when := memoryDateLabel(source.Date)
		labelW := max(8, width-32)
		tag := memorySourceLabel(memoryFirstNonEmpty(source.Folder, source.SourceType))
		line := fmt.Sprintf("%s%-8s %-*s %5s Enter", prefix, tag, labelW, memoryFit(memorySourceFriendlyLabel(source), labelW), when)
		if i == m.memories.sourceIdx && m.memories.focusPanel == memoriesFocusSources {
			lines = append(lines, m.theme.Focus.SelectionActive.Style().Render(memoryFitPad(line, width)))
		} else {
			lines = append(lines, m.theme.Text.Primary.Style().Render(memoryFitPad(line, width)))
		}
	}
	return lines
}

func (m *Model) memoryObsidianLines(row memory.ExploreRow, width int) []string {
	lines := []string{
		m.memoryDim("OBSIDIAN", width),
	}
	target := strings.TrimSpace(row.ObsidianTarget)
	if target == "" {
		for _, source := range row.Sources {
			if source.SourceType == memory.SourceObsidian {
				target = source.Label
				break
			}
		}
	}
	if target == "" {
		return append(lines, m.memoryDim("No vault target.", width))
	}
	lines = append(lines,
		m.memoryField("Vault", "~/Documents/Obsidian/Vault", width),
		m.memoryField("Note", target, width),
		m.memoryField("Daily", "Herald Memory Briefing", width),
		m.memoryField("Open", "press o for vault intent", width),
	)
	return lines
}

func appendWrappedMemoryLines(lines []string, label, body string, width, maxLines int, m *Model) []string {
	if maxLines <= 0 {
		return lines
	}
	lines = append(lines, m.memoryField(label, "", width))
	wrapped := wrapText(body, width-2)
	if len(wrapped) > maxLines {
		wrapped = wrapped[:maxLines]
	}
	for _, line := range wrapped {
		lines = append(lines, m.theme.Text.Primary.Style().Render(memoryFitPad("  "+line, width)))
	}
	return lines
}

func (m *Model) memoriesFilterLabel() string {
	parts := []string{m.memories.filter}
	if m.memories.dateRange != memory.ExploreDateAny {
		parts = append(parts, m.memories.dateRange)
	}
	return strings.Join(parts, " / ")
}

func (m *Model) memoriesListHeader(width int) string {
	spec := memoryTableSpecForWidth(width)
	line := memoryFitPad("!", spec.markerW) + " " + memoryFitPad("Track / Memory", spec.titleW) + " " + memoryFitPad("Status", spec.statusW)
	if spec.sourceW > 0 {
		line += " " + memoryFitPad("Source", spec.sourceW)
	}
	line += " " + memoryFitPad("Date v", spec.dateW)
	return memoryFitPad(line, width)
}

func (m *Model) memoryTitle(text string, width int) string {
	return m.theme.Text.Primary.Style().Bold(true).Render(memoryFitPad(text, width))
}

func (m *Model) memoryDim(text string, width int) string {
	return m.theme.Text.Dim.Style().Render(memoryFitPad(text, width))
}

func (m *Model) memoryError(text string, width int) string {
	return m.theme.Severity.Error.Style().Render(memoryFitPad(text, width))
}

func (m *Model) memoryField(label, value string, width int) string {
	labelText := memoryFit(label, 10)
	valueW := width - 12
	if valueW < 4 {
		valueW = 4
	}
	return m.theme.Metadata.Label.Style().Render(labelText+" ") +
		m.theme.Text.Primary.Style().Render(memoryFitPad(value, valueW))
}

func (m *Model) memoryFieldStyled(label, renderedValue string, width int) string {
	labelText := memoryFit(label, 10)
	valueW := width - 12
	if valueW < 4 {
		valueW = 4
	}
	value := ansi.Cut(renderedValue, 0, valueW)
	if missing := valueW - ansi.StringWidth(value); missing > 0 {
		value += strings.Repeat(" ", missing)
	}
	return m.theme.Metadata.Label.Style().Render(labelText+" ") + value
}

func (m *Model) memoryRule(width int) string {
	return m.theme.Text.Dim.Style().Render(memoryFitPad(strings.Repeat("-", width), width))
}

func memoryJoinFit(lines []string, width, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		line = ansi.Cut(line, 0, width)
		if missing := width - ansi.StringWidth(line); missing > 0 {
			line += strings.Repeat(" ", missing)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func memoryFit(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(strings.TrimSpace(text), width, "…")
}

func memoryFitPad(text string, width int) string {
	out := memoryFit(text, width)
	if missing := width - ansi.StringWidth(out); missing > 0 {
		out += strings.Repeat(" ", missing)
	}
	return out
}

type memoryTableSpec struct {
	markerW int
	titleW  int
	statusW int
	sourceW int
	dateW   int
}

func memoryTableSpecForWidth(width int) memoryTableSpec {
	spec := memoryTableSpec{markerW: 1, statusW: 10, sourceW: 8, dateW: 9}
	spec.titleW = width - spec.markerW - spec.statusW - spec.sourceW - spec.dateW - 4
	if spec.titleW < 10 {
		spec.sourceW = 0
		spec.titleW = width - spec.markerW - spec.statusW - spec.dateW - 3
	}
	if spec.titleW < 6 {
		spec.titleW = 6
	}
	return spec
}

func memoryTablePlainLine(marker, title, status, source, date string, spec memoryTableSpec) string {
	line := memoryFitPad(marker, spec.markerW) + " " + memoryFitPad(title, spec.titleW) + " " + memoryFitPad(status, spec.statusW)
	if spec.sourceW > 0 {
		line += " " + memoryFitPad(source, spec.sourceW)
	}
	line += " " + memoryFitPad(date, spec.dateW)
	return line
}

func (m *Model) memoryMarkerStyle(row memory.ExploreRow) lipgloss.Style {
	switch {
	case row.Status == memory.StatusConflict:
		return m.theme.Severity.Error.Style()
	case row.Freshness == memory.FreshnessStale || row.Status == memory.StatusStale:
		return m.theme.Severity.Warning.Style()
	case row.ReviewReason != "":
		return m.theme.Severity.Warning.Style()
	case row.Kind == memory.KindOpenQuestion || row.Status == memory.StatusWaiting:
		return m.theme.Severity.Info.Style()
	default:
		return m.theme.Text.Muted.Style()
	}
}

func (m *Model) memoryStatusStyle(row memory.ExploreRow) lipgloss.Style {
	switch {
	case row.Status == memory.StatusConflict:
		return m.theme.Severity.Error.Style()
	case row.Freshness == memory.FreshnessStale || row.Status == memory.StatusStale:
		return m.theme.Severity.Warning.Style()
	case row.ReviewReason != "":
		return m.theme.Severity.Warning.Style()
	case row.Kind == memory.KindOpenQuestion || row.Status == memory.StatusWaiting:
		return m.theme.Severity.Info.Style()
	default:
		return m.theme.Text.Primary.Style()
	}
}

func (m *Model) memoryMetricLine(score float64, label string, width int) string {
	labelW := 7
	if width < 20 {
		labelW = 5
	}
	barW := width - labelW - 1
	if barW > 24 {
		barW = 24
	}
	if barW < 4 {
		barW = 4
	}
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	filled := int(score*float64(barW) + 0.5)
	filled = max(0, min(barW, filled))
	bar := m.theme.Severity.Info.Style().Render(strings.Repeat("█", filled)) +
		m.theme.Text.Dim.Style().Render(strings.Repeat("░", barW-filled))
	return memoryFitPad(label, labelW) + " " + bar
}

func (m *Model) memoryConfidenceMetricLine(confidence float64, label string, width int) string {
	labelW := 7
	if width < 20 {
		labelW = 5
	}
	barW := width - labelW - 1
	if barW > 24 {
		barW = 24
	}
	if barW < 4 {
		barW = 4
	}
	low, mid, high, empty := memoryConfidenceSegmentCounts(confidence, barW)
	bar := m.theme.Severity.Error.Style().Render(strings.Repeat("█", low)) +
		m.theme.Severity.Warning.Style().Render(strings.Repeat("█", mid)) +
		m.theme.Severity.Success.Style().Render(strings.Repeat("█", high)) +
		m.theme.Text.Dim.Style().Render(strings.Repeat("░", empty))
	return memoryFitPad(label, labelW) + " " + bar
}

func memoryConfidenceSegmentCounts(confidence float64, width int) (low, mid, high, empty int) {
	if width <= 0 {
		return 0, 0, 0, 0
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	filled := int(confidence*float64(width) + 0.5)
	filled = max(0, min(width, filled))
	lowCap := width / 3
	midCap := width / 3
	if lowCap < 1 {
		lowCap = 1
	}
	if midCap < 1 && width > 1 {
		midCap = 1
	}
	highCap := width - lowCap - midCap
	low = min(filled, lowCap)
	filled -= low
	mid = min(filled, midCap)
	filled -= mid
	high = min(filled, highCap)
	empty = width - low - mid - high
	return low, mid, high, empty
}

func (m *Model) renderMemoryEvidenceLine(source memory.SourceSummary, width int) string {
	tag := "[" + memorySourceLabel(memoryFirstNonEmpty(source.Folder, source.SourceType)) + "]"
	when := memoryDateLabel(source.Date)
	badge := m.theme.Text.Dim.Style().Render(tag + " " + when)
	badgeW := ansi.StringWidth(badge)
	bodyW := width - badgeW - 2
	if bodyW < 14 {
		bodyW = width
		badge = ""
		badgeW = 0
	}
	body := "• " + memoryFirstNonEmpty(source.Snippet, memorySourceFriendlyLabel(source), source.Label)
	line := m.theme.Text.Primary.Style().Render(memoryFitPad(body, bodyW))
	if badgeW > 0 {
		line += strings.Repeat(" ", max(1, width-bodyW-badgeW)) + badge
	}
	return padANSIToWidth(line, width)
}

func memoryStatusLabel(row memory.ExploreRow) string {
	if row.RowType == memory.ExploreRowTrack {
		return memoryStatusDisplay(memoryFirstNonEmpty(row.Status, "track"))
	}
	switch {
	case row.Status == memory.StatusConflict:
		return "Conflict"
	case row.Status == memory.StatusStale || row.Freshness == memory.FreshnessStale:
		return "Stale"
	case row.ReviewReason != "":
		return "Review"
	case row.Kind == memory.KindOpenQuestion:
		return "Open loop"
	case row.Status == memory.StatusWaiting:
		return "Waiting"
	default:
		return memoryStatusDisplay(memoryFirstNonEmpty(row.Status, "active"))
	}
}

func memoryStatusDisplay(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case memory.StatusWaiting:
		return "Waiting"
	case memory.StatusConflict:
		return "Conflict"
	case memory.StatusStale:
		return "Stale"
	case memory.StatusActive:
		return "Active"
	case memory.StatusResolved:
		return "Resolved"
	case memory.StatusBacklog:
		return "Backlog"
	case memory.StatusDone:
		return "Done"
	case memory.StatusSourceMissing:
		return "Source missing"
	case "track":
		return "Track"
	default:
		return memoryFirstNonEmpty(status, "Active")
	}
}

func memoryRowMarker(row memory.ExploreRow) string {
	switch {
	case row.Status == memory.StatusConflict:
		return "!"
	case row.Freshness == memory.FreshnessStale || row.Status == memory.StatusStale:
		return "x"
	case row.RowType == memory.ExploreRowTrack:
		return "T"
	case row.Status == memory.StatusWaiting || row.Kind == memory.KindOpenQuestion || row.Kind == memory.KindCommitment:
		return "o"
	default:
		return " "
	}
}

func memoryTrackDisplay(row memory.ExploreRow) string {
	if memoryContainsFold(row.Topic, "interview") && strings.TrimSpace(row.Company) != "" {
		return "People > Candidates"
	}
	return memoryFirstNonEmpty(row.Topic, row.Company, row.Domain, "Memory")
}

func memoryTypeDisplay(row memory.ExploreRow) string {
	if memoryContainsFold(row.Topic, "interview") || memoryContainsFold(row.Title, "interview") {
		return "Interview / Opportunity"
	}
	return memoryKindLabel(row.Kind)
}

func memoryPeopleDisplay(people []string) string {
	clean := make([]string, 0, len(people))
	for _, person := range people {
		person = strings.TrimSpace(person)
		if person == "" {
			continue
		}
		if strings.Contains(person, "<") {
			person = strings.TrimSpace(person[:strings.Index(person, "<")])
		}
		if strings.Contains(person, "@") && len(clean) > 0 {
			continue
		}
		clean = append(clean, person)
		if len(clean) == 2 {
			break
		}
	}
	return strings.Join(clean, ", ")
}

func memoryTopicsDisplay(row memory.ExploreRow) string {
	if memoryContainsFold(row.Topic, "interview") || memoryContainsFold(row.Title, "interview") {
		return "Engineering, Hiring, Interviews"
	}
	topics := make([]string, 0, len(row.Tags))
	for _, tag := range row.Tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" || tag == "herald/memory" || tag == "review" || tag == "stale" || tag == "conflict" {
			continue
		}
		topics = append(topics, tag)
	}
	if len(topics) > 0 {
		return strings.Join(topics, ", ")
	}
	return row.Topic
}

func memoryContainsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func memorySourceLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case memory.SourceEmail:
		return "INBOX"
	case memory.SourceSentEmail:
		return "Sent"
	case memory.SourceObsidian:
		return "Obsidian"
	case memory.SourceCalendar:
		return "Calendar"
	case memory.SourceAttachment:
		return "Attach"
	case memory.SourceResearch:
		return "Research"
	default:
		return strings.TrimSpace(label)
	}
}

func memorySourceFriendlyLabel(source memory.SourceSummary) string {
	label := strings.TrimSpace(source.Label)
	switch source.SourceType {
	case memory.SourceEmail, memory.SourceSentEmail:
		return "Message: " + humanizeMemorySourceID(label)
	case memory.SourceAttachment:
		return "Attachment: " + humanizeMemorySourceID(label)
	case memory.SourceObsidian:
		return "Note: " + humanizeMemorySourceID(label)
	case memory.SourceCalendar:
		return "Calendar: " + humanizeMemorySourceID(label)
	case memory.SourceResearch:
		return "Research: " + humanizeMemorySourceID(label)
	default:
		return humanizeMemorySourceID(label)
	}
}

func humanizeMemorySourceID(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	label = strings.TrimSuffix(label, "@demo.local")
	label = strings.TrimPrefix(label, "demo-")
	label = strings.ReplaceAll(label, "_", " ")
	label = strings.ReplaceAll(label, "-", " ")
	label = strings.TrimSpace(label)
	if strings.Contains(label, "/") {
		parts := strings.Split(label, "/")
		label = parts[len(parts)-1]
		label = strings.TrimSuffix(label, ".md")
	}
	words := strings.Fields(label)
	for i, word := range words {
		switch strings.ToLower(word) {
		case "re", "vs":
			words[i] = strings.Title(strings.ToLower(word))
		default:
			if len(word) > 0 {
				words[i] = strings.ToUpper(word[:1]) + word[1:]
			}
		}
	}
	return strings.Join(words, " ")
}

func memoryRowClaim(row memory.ExploreRow) string {
	if row.Memory != nil {
		return memoryFirstNonEmpty(row.Memory.Claim, row.Memory.Summary, row.Summary)
	}
	if row.Track != nil {
		parts := make([]string, 0, 3)
		if len(row.Track.OpenLoops) > 0 {
			parts = append(parts, "Open loop: "+row.Track.OpenLoops[0])
		}
		if len(row.Track.Commitments) > 0 {
			parts = append(parts, "Commitment: "+row.Track.Commitments[0])
		}
		if len(row.Track.Claims) > 0 {
			parts = append(parts, "Context: "+row.Track.Claims[0])
		}
		return strings.Join(parts, " ")
	}
	return row.Summary
}

func memoryConfidenceLine(confidence float64, width int) string {
	if confidence <= 0 {
		return "track"
	}
	if width < 12 {
		return fmt.Sprintf("%.0f%%", confidence*100)
	}
	barW := width - 7
	if barW > 16 {
		barW = 16
	}
	if barW < 4 {
		barW = 4
	}
	filled := int(confidence*float64(barW) + 0.5)
	if filled < 0 {
		filled = 0
	}
	if filled > barW {
		filled = barW
	}
	bar := "[" + strings.Repeat("=", filled) + strings.Repeat("-", barW-filled) + "]"
	return fmt.Sprintf("%.2f %s", confidence, bar)
}

func memoryFreshnessScore(row memory.ExploreRow) float64 {
	if row.Freshness == memory.FreshnessStale || row.Status == memory.StatusStale {
		return 0.28
	}
	if row.LastActivityAt.IsZero() {
		return 0.45
	}
	age := time.Since(row.LastActivityAt)
	switch {
	case age <= 6*time.Hour:
		return 0.82
	case age <= 24*time.Hour:
		return 0.68
	case age <= 7*24*time.Hour:
		return 0.50
	default:
		return 0.35
	}
}

func memoryAgeLabel(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	age := time.Since(t)
	if age < 0 {
		age = -age
	}
	switch {
	case age < time.Hour:
		minutes := max(1, int(age.Minutes()))
		return fmt.Sprintf("%dm ago", minutes)
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", max(1, int(age.Hours())))
	case age < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", max(1, int(age.Hours()/24)))
	default:
		return t.Local().Format("Jan 02")
	}
}

func memoryFreshnessLabel(row memory.ExploreRow) string {
	if row.Freshness == memory.FreshnessStale || row.Status == memory.StatusStale {
		return "stale  " + memoryDateLabel(row.LastActivityAt)
	}
	return "fresh  " + memoryDateLabel(row.LastActivityAt)
}

func memoryKindLabel(kind string) string {
	switch kind {
	case memory.KindOpenQuestion:
		return "open loop"
	case memory.KindLastUserReply:
		return "last reply"
	case memory.KindRelationshipContext:
		return "context"
	case memory.KindCommitment:
		return "commitment"
	case memory.KindTrackStatus:
		return "track"
	default:
		return memoryFirstNonEmpty(kind, "memory")
	}
}

func memoryDateLabel(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	local := t.Local()
	now := time.Now()
	if sameDay(local, now) {
		return local.Format("15:04")
	}
	yesterday := now.AddDate(0, 0, -1)
	if sameDay(local, yesterday) {
		return "Yesterday"
	}
	if local.After(now.AddDate(0, 0, -7)) {
		days := int(now.Sub(local).Hours() / 24)
		if days < 1 {
			days = 1
		}
		return fmt.Sprintf("%dd ago", days)
	}
	return local.Format("Jan 02")
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.In(a.Location()).Date()
	return ay == by && am == bm && ad == bd
}

func memoryRowKey(row memory.ExploreRow) string {
	if strings.TrimSpace(row.ID) != "" {
		return strings.TrimSpace(row.ID)
	}
	return strings.Join([]string{row.RowType, row.Title, row.Topic, row.Company, row.Domain}, "|")
}

func memoryFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (m *Model) handleMemoriesMouse(mouse tea.Mouse, _ LayoutPlan, top int) (tea.Model, tea.Cmd, bool) {
	if mouse.Button != tea.MouseLeft && !mouseIsWheel(mouse) {
		return m, nil, true
	}
	spec := m.memoriesLayout(m.windowWidth, m.windowHeight)
	y := mouse.Y - top
	if mouse.X < spec.railW {
		m.memories.focusPanel = memoriesFocusRail
		return m, nil, true
	}
	if y >= spec.sourceY {
		m.memories.focusPanel = memoriesFocusSources
		return m, nil, true
	}
	if mouse.X >= spec.detailX {
		m.memories.focusPanel = memoriesFocusDetail
	} else {
		m.memories.focusPanel = memoriesFocusList
	}
	if mouseIsWheel(mouse) {
		delta := 1
		if mouse.Button == tea.MouseWheelUp {
			delta = -1
		}
		return m, m.moveMemoriesSelection(delta), true
	}
	return m, nil, true
}
