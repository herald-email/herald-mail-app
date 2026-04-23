package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"mail-processor/internal/ai"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
	"mail-processor/internal/render"
	"mail-processor/internal/rules"
)

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// openBrowserFn launches the system browser for the given URL. It is a
// package-level variable so tests can substitute a no-op to avoid spawning a
// real browser process.
var openBrowserFn = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

type tickMsg struct{}

func describeImagesCmd(classifier ai.AIClient, images []models.InlineImage) []tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(images))
	visionAI := ai.WithTaskKind(classifier, ai.TaskKindImageDescription)
	for _, img := range images {
		img := img // capture loop variable
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			desc, err := visionAI.DescribeImage(ctx, img.Data, img.MIMEType)
			return ImageDescMsg{ContentID: img.ContentID, Description: desc, Err: err}
		})
	}
	return cmds
}

// tickSpinner returns a command to tick the spinner
func (m *Model) tickSpinner() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// listenForProgress listens for progress updates from the IMAP client
func (m *Model) listenForProgress() tea.Cmd {
	if m.progressCh == nil {
		return nil
	}
	return func() tea.Msg {
		info := <-m.progressCh
		logger.Debug("listenForProgress: phase=%s current=%d total=%d message=%q", info.Phase, info.Current, info.Total, strings.TrimSpace(info.Message))
		return LoadingMsg{Info: info}
	}
}

func (m *Model) listenForSyncEvents() tea.Cmd {
	ch := m.syncEventsCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			logger.Debug("listenForSyncEvents: channel closed")
			return nil
		}
		logger.Debug("listenForSyncEvents: received folder=%s generation=%d phase=%s current=%d total=%d delta=%d message=%q", event.Folder, event.Generation, event.Phase, event.Current, event.Total, event.EventCount, strings.TrimSpace(event.Message))
		return SyncEventMsg{Event: event}
	}
}

// listenForValidIDs waits for the background reconciliation to send the live
// valid-ID set, then delivers it as ValidIDsMsg so all views can re-filter.
func (m *Model) listenForValidIDs() tea.Cmd {
	ch := m.validIDsCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ids, ok := <-ch
		if !ok {
			return nil
		}
		return ValidIDsMsg{ValidIDs: ids}
	}
}

// listenForDeletionResults listens for deletion results from the worker
func (m *Model) listenForDeletionResults() tea.Cmd {
	return func() tea.Msg {
		return <-m.deletionResultCh
	}
}

// ruleWorker processes emails through the rule engine serially.
func (m *Model) ruleWorker() {
	engine := rules.New(m.backend, m.backend, m.classifier)
	engine.DryRun = m.dryRun
	for req := range m.ruleRequestCh {
		fired, err := engine.EvaluateEmail(req.Email, req.Category)
		select {
		case m.ruleResultCh <- models.RuleResult{
			MessageID:  req.Email.MessageID,
			FiredCount: fired,
			Err:        err,
		}:
		default:
			// result dropped if channel full — rule fired but UI won't see the count
		}
	}
}

// listenForRuleResult waits for a single result from the rule engine worker.
func (m *Model) listenForRuleResult() tea.Cmd {
	return func() tea.Msg {
		result := <-m.ruleResultCh
		return RuleResultMsg{Result: result}
	}
}

// startLoading kicks off the backend's load sequence for the current folder.
func (m *Model) startLoading() tea.Cmd {
	m.loading = true
	m.startTime = time.Now()
	m.syncCountsSettled = false
	m.syncingFolder = m.currentFolder
	logger.Debug("startLoading: folder=%s visibleData=%t syncGeneration=%d", m.currentFolder, m.hasVisibleStartupData(), m.syncGeneration)
	loadCmd := func() tea.Msg {
		logger.Debug("startLoading: dispatching backend.Load for folder=%s", m.currentFolder)
		m.backend.Load(m.currentFolder)
		return nil
	}
	if isVirtualAllMailOnlyFolder(m.currentFolder) {
		return loadCmd
	}
	return tea.Batch(
		loadCmd,
		m.loadFolderStatusCmd([]string{m.currentFolder}, 0),
		m.loadFoldersCmd(500*time.Millisecond),
	)
}

func (m *Model) loadCachedStartupCmd() tea.Cmd {
	folder := m.currentFolder
	return func() tea.Msg {
		stats, statsErr := m.backend.GetSenderStatistics(folder)
		if statsErr != nil {
			return StartupHydratedMsg{Err: statsErr}
		}
		emails, emailsErr := m.backend.GetTimelineEmails(folder)
		if emailsErr != nil {
			return StartupHydratedMsg{Err: emailsErr}
		}
		return StartupHydratedMsg{Stats: stats, Emails: emails}
	}
}

func (m *Model) loadSyncSnapshotCmd(folder string, generation int64, finishLoading bool, status string) tea.Cmd {
	return func() tea.Msg {
		logger.Debug("loadSyncSnapshotCmd: folder=%s generation=%d finish=%v status=%q", folder, generation, finishLoading, strings.TrimSpace(status))
		stats, statsErr := m.backend.GetSenderStatistics(folder)
		if statsErr != nil {
			return SyncHydratedMsg{Folder: folder, Generation: generation, Err: statsErr, FinishLoading: finishLoading, StatusMessage: status}
		}
		emails, emailsErr := m.backend.GetTimelineEmails(folder)
		if emailsErr != nil {
			return SyncHydratedMsg{Folder: folder, Generation: generation, Err: emailsErr, FinishLoading: finishLoading, StatusMessage: status}
		}
		logger.Debug("loadSyncSnapshotCmd: hydrated folder=%s generation=%d stats=%d emails=%d", folder, generation, len(stats), len(emails))
		return SyncHydratedMsg{
			Folder:        folder,
			Generation:    generation,
			Stats:         stats,
			Emails:        emails,
			FinishLoading: finishLoading,
			StatusMessage: status,
		}
	}
}

func (m *Model) loadCachedStartupFinalCmd(status string) tea.Cmd {
	folder := m.currentFolder
	return func() tea.Msg {
		stats, statsErr := m.backend.GetSenderStatistics(folder)
		if statsErr != nil {
			return StartupHydratedMsg{Err: statsErr, FinishLoading: true}
		}
		emails, emailsErr := m.backend.GetTimelineEmails(folder)
		if emailsErr != nil {
			return StartupHydratedMsg{Err: emailsErr, FinishLoading: true}
		}
		return StartupHydratedMsg{
			Stats:         stats,
			Emails:        emails,
			FinishLoading: true,
			StatusMessage: status,
		}
	}
}

func (m *Model) hasVisibleStartupData() bool {
	if len(m.timeline.emails) > 0 {
		return true
	}
	if len(m.stats) > 0 {
		return true
	}
	return false
}

func (m *Model) canInteractWithVisibleData() bool {
	return !m.loading || m.hasVisibleStartupData()
}

func (m *Model) loadFoldersCmd(delay time.Duration) tea.Cmd {
	logger.Debug("loadFoldersCmd: scheduled delay=%s", delay)
	load := func() tea.Msg {
		logger.Debug("loadFoldersCmd: requesting folders")
		folders, err := m.backend.ListFolders()
		if err != nil {
			logger.Warn("Failed to list folders: %v", err)
			return FoldersLoadedMsg{}
		}
		logger.Info("Loaded %d folders", len(folders))
		logger.Debug("loadFoldersCmd: loaded %d folders", len(folders))
		return FoldersLoadedMsg{Folders: folders}
	}
	if delay <= 0 {
		return load
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return load()
	})
}

func (m *Model) loadFolderStatusCmd(folders []string, delay time.Duration) tea.Cmd {
	logger.Debug("loadFolderStatusCmd: scheduled delay=%s folders=%d", delay, len(folders))
	load := func() tea.Msg {
		logger.Debug("loadFolderStatusCmd: requesting status for %d folders", len(folders))
		status, err := m.backend.GetFolderStatus(folders)
		if err != nil {
			logger.Warn("Failed to get folder status: %v", err)
			return FolderStatusMsg{Status: map[string]models.FolderStatus{}}
		}
		logger.Debug("loadFolderStatusCmd: loaded status for %d folders", len(status))
		return FolderStatusMsg{Status: status}
	}
	if delay <= 0 {
		return load
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return load()
	})
}

func (m *Model) summaryKeyAtCursor() (string, bool) {
	cursor := m.summaryTable.Cursor()
	key, ok := m.rowToSender[cursor]
	if !ok || key == "" {
		return "", false
	}
	return key, true
}

func (m *Model) selectedSummaryCount() int {
	return len(m.selectedSummaryKeys)
}

func (m *Model) resetCleanupSelection() {
	m.selectedSummaryKeys = make(map[string]bool)
	m.selectedMessages = make(map[string]bool)
}

// updateSummaryTable updates the summary table with current data
func (m *Model) updateSummaryTable() {
	if m.stats == nil {
		return
	}

	// Sort senders by total emails
	type senderStat struct {
		sender string
		stats  *models.SenderStats
	}

	var sortedStats []senderStat
	for sender, stats := range m.stats {
		sortedStats = append(sortedStats, senderStat{sender, stats})
	}

	// Sort by email count (descending), then by sender name (ascending) for stable order
	sort.Slice(sortedStats, func(i, j int) bool {
		if sortedStats[i].stats.TotalEmails == sortedStats[j].stats.TotalEmails {
			return sortedStats[i].sender < sortedStats[j].sender
		}
		return sortedStats[i].stats.TotalEmails > sortedStats[j].stats.TotalEmails
	})

	oldCursor := m.summaryTable.Cursor()
	preservedKey := m.selectedSender
	if preservedKey == "" {
		if key, ok := m.rowToSender[oldCursor]; ok {
			preservedKey = key
		}
	}

	// Build table rows and mapping
	var rows []table.Row
	m.rowToSender = make(map[int]string) // Clear and rebuild mapping
	keyToRow := make(map[string]int)
	for i, item := range sortedStats {
		// Store original sender for deletion
		m.rowToSender[i] = item.sender
		keyToRow[item.sender] = i

		senderColW := m.summaryTable.Columns()[1].Width
		if senderColW <= 0 {
			senderColW = 46
		}
		sender := styledSender(item.sender, senderColW)
		stats := item.stats

		// Format date range
		dateRange := "N/A"
		if !stats.FirstEmail.IsZero() && !stats.LastEmail.IsZero() {
			if stats.FirstEmail.Year() == stats.LastEmail.Year() {
				dateRange = fmt.Sprintf("%s - %s",
					stats.FirstEmail.Format("Jan"),
					stats.LastEmail.Format("Jan 2006"))
			} else {
				dateRange = fmt.Sprintf("%s - %s",
					stats.FirstEmail.Format("Jan 2006"),
					stats.LastEmail.Format("Jan 2006"))
			}
		}

		// Add selection indicator in first column
		checkmark := " "
		if m.selectedSummaryKeys[item.sender] {
			checkmark = "✓"
		}

		row := table.Row{
			checkmark,
			sender,
			fmt.Sprintf("%d", stats.TotalEmails),
			dateRange,
		}
		rows = append(rows, row)
	}

	m.summaryTable.SetRows(rows)
	if len(rows) == 0 {
		m.summaryTable.SetCursor(0)
		m.selectedSender = ""
		return
	}
	if preservedKey != "" {
		if row, ok := keyToRow[preservedKey]; ok {
			m.summaryTable.SetCursor(row)
		} else if oldCursor >= 0 && oldCursor < len(rows) {
			m.summaryTable.SetCursor(oldCursor)
		} else {
			m.summaryTable.SetCursor(0)
		}
	} else if oldCursor >= 0 && oldCursor < len(rows) {
		m.summaryTable.SetCursor(oldCursor)
	} else {
		m.summaryTable.SetCursor(0)
	}
}

// updateDetailsTable updates the details table for the selected sender
func (m *Model) updateDetailsTable() {
	cursor := m.summaryTable.Cursor()
	sender := m.selectedSender
	if key, ok := m.rowToSender[cursor]; ok && key != "" {
		sender = key
	}
	ok := sender != ""
	if !ok || sender == "" {
		sender, ok = m.rowToSender[0]
	}
	if !ok || sender == "" {
		m.selectedSender = ""
		m.detailsTable.SetRows([]table.Row{})
		return
	}

	m.selectedSender = sender

	emails, err := m.backend.GetEmailsBySender(m.currentFolder)
	if err != nil {
		logger.Warn("Failed to get emails for sender %s: %v", sender, err)
		m.detailsTable.SetRows([]table.Row{})
		return
	}

	senderEmails := emails[sender]
	if len(senderEmails) == 0 {
		logger.Debug("No emails found for sender: %s", sender)
		m.detailsTable.SetRows([]table.Row{})
		return
	}

	sort.Slice(senderEmails, func(i, j int) bool {
		return senderEmails[i].Date.After(senderEmails[j].Date)
	})

	m.detailsEmails = senderEmails

	logger.Debug("updateDetailsTable: %d messages shown, %d selected globally", len(senderEmails), len(m.selectedMessages))

	m.rebuildDetailsRows()
}

func (m *Model) rebuildDetailsRows() {
	oldCursor := m.detailsTable.Cursor()
	if len(m.detailsEmails) == 0 {
		m.detailsTable.SetRows([]table.Row{})
		m.detailsTable.SetCursor(0)
		return
	}

	var rows []table.Row
	for _, email := range m.detailsEmails {
		dateStr := "N/A"
		if !email.Date.IsZero() {
			dateStr = email.Date.Format("06-01-02 15:04")
		}

		subject := sanitizeText(email.Subject)
		if subject == "" {
			subject = "No Subject"
		}
		maxLen := m.subjectColWidth
		if maxLen <= 0 {
			maxLen = 32
		}
		subject = truncate(subject, maxLen)

		attachments := "N"
		if email.HasAttachments {
			attachments = "Y"
		}

		checkmark := " "
		if email.MessageID != "" && m.selectedMessages[email.MessageID] {
			checkmark = "✓"
		}

		row := table.Row{
			checkmark,
			dateStr,
			subject,
			fmt.Sprintf("%.1f", float64(email.Size)/1024),
			attachments,
		}
		rows = append(rows, row)
	}

	m.detailsTable.SetRows(rows)
	if len(rows) == 0 {
		m.detailsTable.SetCursor(0)
		return
	}
	if oldCursor < 0 {
		oldCursor = 0
	}
	if oldCursor >= len(rows) {
		oldCursor = len(rows) - 1
	}
	m.detailsTable.SetCursor(oldCursor)
}

func (m *Model) reflowVisibleTableRows() {
	if m.stats != nil {
		m.updateSummaryTable()
	}
	m.rebuildDetailsRows()

	cursor := m.timelineTable.Cursor()
	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) == 0 {
		m.timelineTable.SetCursor(0)
		return
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(rows) {
		cursor = len(rows) - 1
	}
	m.timelineTable.SetCursor(cursor)
}

// toggleDomainMode switches between domain and email grouping
func (m *Model) toggleDomainMode() {
	m.groupByDomain = !m.groupByDomain
	m.backend.SetGroupByDomain(m.groupByDomain)

	logger.Info("Toggling domain mode to: %v", m.groupByDomain)

	stats, err := m.backend.GetSenderStatistics(m.currentFolder)
	if err != nil {
		logger.Error("Failed to reload statistics after toggling domain mode: %v", err)
		return
	}

	m.stats = stats
	m.selectedSender = ""
	m.resetCleanupSelection()
	m.updateSummaryTable()
	m.updateDetailsTable()
}

// getPhaseIcon returns an icon for the current phase
func getPhaseIcon(phase string) string {
	switch phase {
	case "scanning":
		return "🔍"
	case "processing":
		return "📧"
	case "complete":
		return "✅"
	default:
		return "⚙️"
	}
}

// calculateTextWidth estimates the visual width of text with emojis
func calculateTextWidth(text string) int {
	return render.CalculateTextWidth(text)
}

// renderProgressBar creates a visual progress bar
func (m *Model) renderProgressBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := (percent * width) / 100
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	progressBarStyle := lipgloss.NewStyle().
		Foreground(defaultTheme.InfoFg).
		Background(defaultTheme.HeaderBg).
		Padding(0, 1).
		Margin(0, 2)

	return progressBarStyle.Render(fmt.Sprintf("[%s] %d%%", bar, percent))
}

// sanitizeText removes emoji and symbols while preserving all language text
func sanitizeText(text string) string {
	return render.SanitizeText(text)
}

// styledSender renders a sender string with the display name in bright white and
// the <email> part in dim gray, making the two visually distinct in table columns.
func styledSender(raw string, maxWidth int) string {
	nameStyle := lipgloss.NewStyle().Foreground(defaultTheme.TextFg)
	emailStyle := lipgloss.NewStyle().Foreground(defaultTheme.MutedFg)

	if lt := strings.Index(raw, " <"); lt > 0 && strings.HasSuffix(raw, ">") {
		name := sanitizeText(raw[:lt])
		email := raw[lt+1:]
		combined := name + " " + email
		combined = ansi.Truncate(combined, maxWidth, "…")
		if lt2 := strings.Index(combined, " <"); lt2 > 0 {
			return nameStyle.Render(combined[:lt2]) + " " + emailStyle.Render(combined[lt2+1:])
		}
		return nameStyle.Render(combined)
	}

	plain := sanitizeText(raw)
	plain = ansi.Truncate(plain, maxWidth, "…")
	return nameStyle.Render(plain)
}

// updateTableDimensions recalculates table and column sizes based on terminal dimensions
func (m *Model) updateTableDimensions(width, height int) {
	if width == 0 {
		return
	}
	m.windowWidth = width
	m.windowHeight = height

	extraChromeRows := 0
	if m.hasTopSyncStrip() {
		extraChromeRows = 1
	}

	// renderMainView chrome: header(1) + tab bar(1) + optional sync strip(1) +
	// status bar(1) + key hints(1). Each table panel adds 2 border lines
	// (top + bottom), so total deduction = 7 plus any extra sync chrome.
	tableHeight := height - 7 - extraChromeRows
	if tableHeight < 5 {
		tableHeight = 5
	}
	plan := m.buildLayoutPlan(width, height)
	m.normalizeFocusForLayout(plan)
	m.sidebarTooWide = m.showSidebar &&
		(m.activeTab == tabTimeline || m.activeTab == tabCleanup) &&
		!m.showCleanupPreview &&
		!(m.activeTab == tabTimeline && m.timeline.selectedEmail != nil) &&
		!plan.SidebarVisible

	const summaryFixedCols = 7
	const summaryNumCols = 4
	const detailsFixedCols = 28
	const detailsNumCols = 5

	if m.showCleanupPreview && m.cleanupFullScreen {
		m.cleanupPreviewWidth = width
	} else if m.showCleanupPreview {
		m.cleanupPreviewWidth = plan.Cleanup.PreviewWidth + 2
		cpDetAttW := 3
		sumF := summaryFixedCols
		sumN := summaryNumCols
		detF := detailsFixedCols
		detN := detailsNumCols

		const cpMinSender = 6
		const cpMinDateRange = 8

		summaryAvailable := plan.Cleanup.SummaryWidth
		detailsAvailable := plan.Cleanup.DetailsWidth
		if summaryAvailable == 0 {
			summaryAvailable = 12
		}
		remaining := summaryAvailable - (sumF + sumN*2)
		if remaining < 0 {
			remaining = 0
		}
		cpDateRangeW := clampInt(remaining/3, cpMinDateRange, 20)
		if remaining-cpDateRangeW < cpMinSender {
			cpDateRangeW = remaining - cpMinSender
		}
		if cpDateRangeW < cpMinDateRange {
			cpDateRangeW = cpMinDateRange
		}
		senderW := remaining - cpDateRangeW
		if senderW < cpMinSender {
			senderW = cpMinSender
		}

		if detailsAvailable-(detF+detN*2) < 8 && cpDetAttW > 0 {
			detF -= cpDetAttW
			detN--
			cpDetAttW = 0
		}
		subjectW := detailsAvailable - (detF + detN*2)
		if subjectW < 4 {
			subjectW = 4
		}

		m.summaryTable.SetColumns([]table.Column{
			{Title: "✓", Width: 1},
			{Title: "Sender/Domain", Width: senderW},
			{Title: "Count", Width: 6},
			{Title: "Dates", Width: cpDateRangeW},
		})
		m.summaryTable.SetWidth(sumF + senderW + sumN*2)

		m.subjectColWidth = subjectW
		m.detailsTable.SetColumns([]table.Column{
			{Title: "✓", Width: 1},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: subjectW},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: cpDetAttW},
		})
		m.detailsTable.SetWidth(detF + subjectW + detN*2)
	} else {
		m.cleanupPreviewWidth = 0

		const minSender = 8
		const minDateRange = 8
		detailsAttW := 3

		sumFixed := summaryFixedCols
		detFixed := detailsFixedCols
		sumNCols := summaryNumCols
		detNCols := detailsNumCols

		if plan.Cleanup.DetailsWidth-(detFixed+detNCols*2) < 8 && detailsAttW > 0 {
			detFixed -= 3
			detNCols--
			detailsAttW = 0
		}

		summaryAvailable := plan.Cleanup.SummaryWidth
		remaining := summaryAvailable - sumFixed - sumNCols*2
		if remaining < 0 {
			remaining = 0
		}
		dateRangeW := clampInt(remaining/3, minDateRange, 20)
		if remaining-dateRangeW < minSender {
			dateRangeW = remaining - minSender
		}
		if dateRangeW < minDateRange {
			dateRangeW = minDateRange
		}
		senderWidth := remaining - dateRangeW
		if senderWidth < minSender {
			senderWidth = minSender
		}
		subjectWidth := plan.Cleanup.DetailsWidth - detFixed - detNCols*2
		if senderWidth < 8 {
			senderWidth = 8
		}
		if subjectWidth < 8 {
			subjectWidth = 8
		}

		m.summaryTable.SetColumns([]table.Column{
			{Title: "✓", Width: 1},
			{Title: "Sender/Domain", Width: senderWidth},
			{Title: "Count", Width: 6},
			{Title: "Dates", Width: dateRangeW},
		})
		m.summaryTable.SetWidth(sumFixed + senderWidth + sumNCols*2)

		m.subjectColWidth = subjectWidth
		m.detailsTable.SetColumns([]table.Column{
			{Title: "✓", Width: 1},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: subjectWidth},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: detailsAttW},
		})
		m.detailsTable.SetWidth(detFixed + subjectWidth + detNCols*2)
	}

	// SetHeight controls visible data rows (excludes header row).
	// renderStyledTableView renders height+1 lines (header + data); baseStyle adds 2
	// border lines → total = height+3. Other panels have Height(tableHeight)+2 border = tableHeight+2.
	// Subtract 1 so table total (height+2) matches panel total (tableHeight+2).
	m.summaryTable.SetHeight(tableHeight - 1)
	m.detailsTable.SetHeight(tableHeight - 1)

	const timelineFixedCols = 30
	const timelineNumCols = 6

	const timelineTableFixedOverhead = timelineFixedCols + timelineNumCols*2 + 2
	const minPreviewWidth = 25
	previewWidth := 0
	if m.timeline.selectedEmail != nil {
		maxPreview := plan.Timeline.PreviewWidth
		if maxPreview >= minPreviewWidth {
			previewWidth = maxPreview
		}
	}
	m.timeline.previewWidth = previewWidth

	// Progressive timeline column hiding: when preview is open and space is tight,
	// drop low-priority fixed columns to preserve Sender + Subject visibility.
	tTagW := 4
	tAttW := 3
	tSizeW := 7
	tDateW := 16
	tFixed := timelineFixedCols    // 30 = Date(16) + Size(7) + Att(3) + Tag(4)
	tNCols := timelineNumCols      // 6
	const minTimelineVariable = 15 // minimum for usable Sender + Subject

	timelineAvailable := plan.Timeline.TableWidth
	calcTimelineVar := func() int {
		v := timelineAvailable - tFixed - tNCols*2
		if v < 0 {
			return 0
		}
		return v
	}

	if previewWidth > 0 {
		if calcTimelineVar() < minTimelineVariable && tTagW > 0 {
			tFixed -= tTagW
			tNCols--
			tTagW = 0
		}
		if calcTimelineVar() < minTimelineVariable && tAttW > 0 {
			tFixed -= tAttW
			tNCols--
			tAttW = 0
		}
		if calcTimelineVar() < minTimelineVariable && tSizeW > 0 {
			tFixed -= tSizeW
			tNCols--
			tSizeW = 0
		}
		if calcTimelineVar() < minTimelineVariable && tDateW > 0 {
			tFixed -= tDateW
			tNCols--
			tDateW = 0
		}
	}

	timelineVariable := calcTimelineVar()
	tSenderWidth := timelineVariable * 30 / 100
	tSubjectWidth := timelineVariable - tSenderWidth
	if timelineVariable >= 24 {
		if tSenderWidth < 10 {
			tSenderWidth = 10
		}
		if tSubjectWidth < 14 {
			tSubjectWidth = 14
		}
	}
	m.timeline.senderWidth = tSenderWidth
	m.timeline.subjectWidth = tSubjectWidth
	m.timelineTable.SetColumns([]table.Column{
		{Title: "Sender", Width: tSenderWidth},
		{Title: "Subject", Width: tSubjectWidth},
		{Title: "Date", Width: tDateW},
		{Title: "Size KB", Width: tSizeW},
		{Title: "Att", Width: tAttW},
		{Title: "Tag", Width: tTagW},
	})
	m.timelineTable.SetWidth(tFixed + tSenderWidth + tSubjectWidth + tNCols*2)
	m.timelineTable.SetHeight(tableHeight - 1)

	logWidth := width - 4
	if logWidth < 20 {
		logWidth = 20
	}
	m.logViewer.SetSize(logWidth, tableHeight)

	m.composeTo.Width = plan.Compose.FieldInnerWidth
	m.composeCC.Width = plan.Compose.FieldInnerWidth
	m.composeBCC.Width = plan.Compose.FieldInnerWidth
	m.composeSubject.Width = plan.Compose.FieldInnerWidth
	composeBodyWidth := plan.Compose.BodyInnerWidth
	composeExtraRows := m.composeAdditionalRows(tableHeight)
	composeBodyHeight := tableHeight - 16 - composeExtraRows
	minComposeBodyHeight := 3
	if composeExtraRows > 0 {
		minComposeBodyHeight = 1
	}
	if composeBodyHeight < minComposeBodyHeight {
		composeBodyHeight = minComposeBodyHeight
	}
	m.composeBody.SetWidth(composeBodyWidth)
	m.composeBody.SetHeight(composeBodyHeight)

	m.reflowVisibleTableRows()
}

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	return render.Truncate(s, n)
}

// --- Thin wrappers around render package for backward compatibility ---

// wrapLines delegates to render.WrapLines.
func wrapLines(text string, width int) []string {
	return render.WrapLines(text, width)
}

// stripInvisibleChars delegates to render.StripInvisibleChars.
func stripInvisibleChars(s string) string {
	return render.StripInvisibleChars(s)
}

// urlRe matches http/https URLs.
var urlRe = render.URLRe

// linkifyWrappedLines delegates to render.LinkifyWrappedLines.
func linkifyWrappedLines(lines []string) []string {
	return render.LinkifyWrappedLines(lines)
}

// linkifyURLs delegates to render.LinkifyURLs.
func linkifyURLs(text string) string {
	return render.LinkifyURLs(text)
}

// shortenURL delegates to render.ShortenURL.
func shortenURL(raw string) string {
	return render.ShortenURL(raw)
}

// wrapText delegates to render.WrapText.
func wrapText(text string, width int) []string {
	return render.WrapText(text, width)
}

// skipEscapeSeq delegates to render.SkipEscapeSeq.
func skipEscapeSeq(runes []rune, pos int) int {
	return render.SkipEscapeSeq(runes, pos)
}
