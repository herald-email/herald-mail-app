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

	// renderMainView chrome: header(1) + tab bar(1) + blank(1) + status bar(1) + key hints(1) = 5 rows.
	// Each table panel adds 2 border lines (top + bottom), so total deduction = 7.
	tableHeight := height - 7
	if tableHeight < 5 {
		tableHeight = 5
	}
	plan := m.buildLayoutPlan(width, height)
	m.normalizeFocusForLayout(plan)
	m.sidebarTooWide = m.showSidebar &&
		(m.activeTab == tabTimeline || m.activeTab == tabCleanup) &&
		!m.showCleanupPreview &&
		!plan.SidebarVisible

	const summaryFixedCols = 41
	const summaryNumCols = 6
	const detailsFixedCols = 29
	const detailsNumCols = 5

	if m.showCleanupPreview && m.cleanupFullScreen {
		m.cleanupPreviewWidth = width
	} else if m.showCleanupPreview {
		m.cleanupPreviewWidth = plan.Cleanup.PreviewWidth + 2

		// Progressive column hiding for cleanup tables with preview open.
		cpAttachW := 6
		cpDateRangeW := 20
		cpAvgKBW := 7
		cpDetAttW := 3
		sumF := summaryFixedCols // 41
		sumN := summaryNumCols   // 6
		detF := detailsFixedCols // 29
		detN := detailsNumCols   // 5

		const cpMinSender = 8

		summaryAvailable := plan.Cleanup.SummaryWidth
		detailsAvailable := plan.Cleanup.DetailsWidth
		if summaryAvailable == 0 {
			summaryAvailable = 12
		}
		calcSender := func() int { return summaryAvailable - (sumF + sumN*2) }

		if calcSender() < cpMinSender && cpAttachW > 0 {
			sumF -= cpAttachW
			sumN--
			cpAttachW = 0
		}
		if calcSender() < cpMinSender && cpDetAttW > 0 {
			detF -= cpDetAttW
			detN--
			cpDetAttW = 0
		}
		if calcSender() < cpMinSender && cpDateRangeW > 0 {
			sumF -= cpDateRangeW
			sumN--
			cpDateRangeW = 0
		}
		if calcSender() < cpMinSender && cpAvgKBW > 0 {
			sumF -= cpAvgKBW
			sumN--
			cpAvgKBW = 0
		}

		senderW := summaryAvailable - (sumF + sumN*2)
		if senderW < 4 {
			senderW = 4
		}
		subjectW := detailsAvailable - (detF + detN*2)
		if subjectW < 4 {
			subjectW = 4
		}

		m.summaryTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Sender/Domain", Width: senderW},
			{Title: "Count", Width: 6},
			{Title: "Avg KB", Width: cpAvgKBW},
			{Title: "Attach", Width: cpAttachW},
			{Title: "Date Range", Width: cpDateRangeW},
		})
		m.summaryTable.SetWidth(sumF + senderW + sumN*2)

		m.subjectColWidth = subjectW
		m.detailsTable.SetColumns([]table.Column{
			{Title: "✓", Width: 2},
			{Title: "Date", Width: 16},
			{Title: "Subject", Width: subjectW},
			{Title: "Size", Width: 8},
			{Title: "Att", Width: cpDetAttW},
		})
		m.detailsTable.SetWidth(detF + subjectW + detN*2)
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

		cleanupAvailable := plan.Cleanup.SummaryWidth + plan.Cleanup.DetailsWidth
		calcVariable := func() int {
			v := cleanupAvailable - sumFixed - detFixed - sumNCols*2 - detNCols*2
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

		senderWidth := plan.Cleanup.SummaryWidth - sumFixed - sumNCols*2
		subjectWidth := plan.Cleanup.DetailsWidth - detFixed - detNCols*2
		if senderWidth < 8 {
			senderWidth = 8
		}
		if subjectWidth < 8 {
			subjectWidth = 8
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
