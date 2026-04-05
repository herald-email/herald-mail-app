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
	for _, img := range images {
		img := img // capture loop variable
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			desc, err := classifier.DescribeImage(ctx, img.Data, img.MIMEType)
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
	return func() tea.Msg {
		info := <-m.progressCh
		return LoadingMsg{Info: info}
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
	return func() tea.Msg {
		m.backend.Load(m.currentFolder)
		return nil
	}
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

	// Build table rows and mapping
	var rows []table.Row
	m.rowToSender = make(map[int]string) // Clear and rebuild mapping
	for i, item := range sortedStats {
		// Store original sender for deletion
		m.rowToSender[i] = item.sender

		senderColW := m.summaryTable.Columns()[1].Width
		if senderColW <= 0 {
			senderColW = 33
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
		if m.selectedRows[i] {
			checkmark = "✓"
		}

		row := table.Row{
			checkmark,
			sender,
			fmt.Sprintf("%d", stats.TotalEmails),
			fmt.Sprintf("%.1f", stats.AvgSize/1024),
			fmt.Sprintf("%d", stats.WithAttachments),
			dateRange,
		}
		rows = append(rows, row)
	}

	m.summaryTable.SetRows(rows)
}

// updateDetailsTable updates the details table for the selected sender
func (m *Model) updateDetailsTable() {
	cursor := m.summaryTable.Cursor()
	sender, ok := m.rowToSender[cursor]
	if !ok || sender == "" {
		sender, ok = m.rowToSender[0]
	}
	if !ok || sender == "" {
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

	var rows []table.Row
	for _, email := range senderEmails {
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
	m.selectedRows = make(map[int]bool)
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
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("235")).
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
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	emailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	if lt := strings.Index(raw, " <"); lt > 0 && strings.HasSuffix(raw, ">") {
		name := sanitizeText(raw[:lt])
		email := raw[lt+1:]
		combined := name + " " + email
		if len([]rune(combined)) > maxWidth {
			combined = string([]rune(combined)[:maxWidth-1]) + "…"
		}
		if lt2 := strings.Index(combined, " <"); lt2 > 0 {
			return nameStyle.Render(combined[:lt2]) + " " + emailStyle.Render(combined[lt2+1:])
		}
		return nameStyle.Render(combined)
	}

	plain := sanitizeText(raw)
	if len([]rune(plain)) > maxWidth {
		plain = string([]rune(plain)[:maxWidth-1]) + "…"
	}
	return nameStyle.Render(plain)
}

// updateTableDimensions recalculates table and column sizes based on terminal dimensions
func (m *Model) updateTableDimensions(width, height int) {
	if width == 0 {
		return
	}
	m.windowWidth = width
	m.windowHeight = height

	// renderMainView chrome: header(1) + tab bar(1) + blank(1) + status bar(1) + key hints(1) = 5 rows.
	// Each table panel adds 2 border lines (top + bottom), so total deduction = 7.
	tableHeight := height - 7
	if tableHeight < 5 {
		tableHeight = 5
	}

	sidebarWouldConsume := sidebarContentWidth + 2 + 2
	const minTimelineVariableCols = 16
	m.sidebarTooWide = m.showSidebar &&
		(width-sidebarWouldConsume) < (minTermWidth+minTimelineVariableCols)

	sidebarExtra := 0
	if m.showSidebar && !m.sidebarTooWide {
		sidebarExtra = sidebarWouldConsume
	}
	chatExtra := 0
	if m.showChat {
		chatExtra = chatPanelWidth + 2 + 2
	}

	const summaryFixedCols = 41
	const summaryNumCols = 6
	const detailsFixedCols = 29
	const detailsNumCols = 5

	availForCleanup := width - chatExtra
	if m.showCleanupPreview && m.cleanupFullScreen {
		m.cleanupPreviewWidth = width
	} else if m.showCleanupPreview {
		availForCleanup = width - chatExtra
		cleanupPreviewW := availForCleanup / 2
		if cleanupPreviewW < 25 {
			cleanupPreviewW = 25
		}
		m.cleanupPreviewWidth = cleanupPreviewW

		tablesAvail := availForCleanup - cleanupPreviewW - 3
		if tablesAvail < 0 {
			tablesAvail = 0
		}
		tableHalf := tablesAvail / 2
		perTableOverhead := summaryFixedCols + summaryNumCols*2
		senderW := tableHalf - perTableOverhead
		if senderW < 4 {
			senderW = 4
		}
		perTableOverhead2 := detailsFixedCols + detailsNumCols*2
		subjectW := tablesAvail - tableHalf - perTableOverhead2
		if subjectW < 4 {
			subjectW = 4
		}

		m.summaryTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Sender/Domain", Width: senderW},
			{Title: "Count", Width: 6},
			{Title: "Avg KB", Width: 7},
			{Title: "Attach", Width: 6},
			{Title: "Date Range", Width: 20},
		})
		m.summaryTable.SetWidth(summaryFixedCols + senderW + summaryNumCols*2)

		m.subjectColWidth = subjectW
		m.detailsTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: subjectW},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: 3},
		})
		m.detailsTable.SetWidth(detailsFixedCols + subjectW + detailsNumCols*2)
	} else {
		m.cleanupPreviewWidth = 0

		const minVariable = 24

		attachW := 6
		dateRangeW := 20
		avgKBW := 7
		detailsAttW := 3

		sumFixed := summaryFixedCols
		detFixed := detailsFixedCols
		sumNCols := summaryNumCols
		detNCols := detailsNumCols

		calcVariable := func() int {
			overhead := sumNCols*2 + detNCols*2 + 4 + 2 + sidebarExtra + chatExtra
			v := width - overhead - sumFixed - detFixed
			if v < 0 {
				return 0
			}
			return v
		}

		if calcVariable() < minVariable && attachW > 0 {
			sumFixed -= 6
			sumNCols--
			attachW = 0
		}
		if calcVariable() < minVariable && detailsAttW > 0 {
			detFixed -= 3
			detNCols--
			detailsAttW = 0
		}
		if calcVariable() < minVariable && dateRangeW > 0 {
			sumFixed -= 20
			sumNCols--
			dateRangeW = 0
		}
		if calcVariable() < minVariable && avgKBW > 0 {
			sumFixed -= 7
			sumNCols--
			avgKBW = 0
		}

		cleanupVariable := calcVariable()
		senderWidth := cleanupVariable * 40 / 100
		subjectWidth := cleanupVariable - senderWidth
		if cleanupVariable >= minVariable {
			if senderWidth < 12 {
				senderWidth = 12
			}
			if subjectWidth < 12 {
				subjectWidth = 12
			}
		}

		m.summaryTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Sender/Domain", Width: senderWidth},
			{Title: "Count", Width: 6},
			{Title: "Avg KB", Width: avgKBW},
			{Title: "Attach", Width: attachW},
			{Title: "Date Range", Width: dateRangeW},
		})
		m.summaryTable.SetWidth(sumFixed + senderWidth + sumNCols*2)

		m.subjectColWidth = subjectWidth
		m.detailsTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: subjectWidth},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: detailsAttW},
		})
		m.detailsTable.SetWidth(detFixed + subjectWidth + detNCols*2)
	}

	m.summaryTable.SetHeight(tableHeight)
	m.detailsTable.SetHeight(tableHeight)

	const timelineFixedCols = 30
	const timelineNumCols = 6

	const timelineTableFixedOverhead = timelineFixedCols + timelineNumCols*2 + 2
	const minPreviewWidth = 25
	availableForTimeline := width - sidebarExtra - chatExtra
	previewWidth := 0
	if m.selectedTimelineEmail != nil {
		maxPreview := availableForTimeline - timelineTableFixedOverhead - 1
		if maxPreview >= minPreviewWidth {
			previewWidth = availableForTimeline / 2
			if previewWidth < minPreviewWidth {
				previewWidth = minPreviewWidth
			}
			if previewWidth > maxPreview {
				previewWidth = maxPreview
			}
			// Ensure enough space remains for Sender + Subject columns.
			const minSenderSubjectCols = 20
			maxForColumns := availableForTimeline - timelineTableFixedOverhead - 1 - minSenderSubjectCols
			if previewWidth > maxForColumns && maxForColumns >= minPreviewWidth {
				previewWidth = maxForColumns
			}
		}
	}
	m.emailPreviewWidth = previewWidth

	// With full Border() on the preview panel, both borders are inside Width(w-2)+2=w,
	// so no extra border column needed.
	previewBorder := 0
	timelineOverhead := timelineTableFixedOverhead + sidebarExtra + chatExtra + previewWidth + previewBorder
	timelineVariable := width - timelineOverhead
	if timelineVariable < 0 {
		timelineVariable = 0
	}
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
	m.timelineSenderWidth = tSenderWidth
	m.timelineSubjectWidth = tSubjectWidth
	m.timelineTable.SetColumns([]table.Column{
		{Title: "Sender", Width: tSenderWidth},
		{Title: "Subject", Width: tSubjectWidth},
		{Title: "Date", Width: 16},
		{Title: "Size KB", Width: 7},
		{Title: "Att", Width: 3},
		{Title: "Tag", Width: 4},
	})
	m.timelineTable.SetWidth(timelineFixedCols + tSenderWidth + tSubjectWidth + timelineNumCols*2)
	m.timelineTable.SetHeight(tableHeight)

	logWidth := width - 4
	if logWidth < 20 {
		logWidth = 20
	}
	m.logViewer.SetSize(logWidth, tableHeight)

	composeInputWidth := width - chatExtra - 12
	if composeInputWidth < 10 {
		composeInputWidth = 10
	}
	m.composeTo.Width = composeInputWidth
	m.composeCC.Width = composeInputWidth
	m.composeBCC.Width = composeInputWidth
	m.composeSubject.Width = composeInputWidth
	composeBodyWidth := width - chatExtra - 2
	if composeBodyWidth < 10 {
		composeBodyWidth = 10
	}
	composeBodyHeight := tableHeight - 16
	if composeBodyHeight < 3 {
		composeBodyHeight = 3
	}
	m.composeBody.SetWidth(composeBodyWidth)
	m.composeBody.SetHeight(composeBodyHeight)
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
