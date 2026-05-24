package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/render"
	"github.com/herald-email/herald-mail-app/internal/rules"
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
	m.cancelBackgroundWork()
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
		emails, emailsErr := m.backend.GetTimelineEmails(folder)
		if emailsErr != nil {
			return StartupHydratedMsg{Err: emailsErr}
		}
		return StartupHydratedMsg{Emails: emails}
	}
}

func (m *Model) loadSyncSnapshotCmd(folder string, generation int64, finishLoading bool, status string) tea.Cmd {
	return func() tea.Msg {
		logger.Debug("loadSyncSnapshotCmd: folder=%s generation=%d finish=%v status=%q", folder, generation, finishLoading, strings.TrimSpace(status))
		emails, emailsErr := m.backend.GetTimelineEmails(folder)
		if emailsErr != nil {
			return SyncHydratedMsg{Folder: folder, Generation: generation, Err: emailsErr, FinishLoading: finishLoading, StatusMessage: status}
		}
		logger.Debug("loadSyncSnapshotCmd: hydrated folder=%s generation=%d emails=%d", folder, generation, len(emails))
		return SyncHydratedMsg{
			Folder:        folder,
			Generation:    generation,
			Emails:        emails,
			FinishLoading: finishLoading,
			StatusMessage: status,
		}
	}
}

func (m *Model) loadCachedStartupFinalCmd(status string) tea.Cmd {
	folder := m.currentFolder
	return func() tea.Msg {
		emails, emailsErr := m.backend.GetTimelineEmails(folder)
		if emailsErr != nil {
			return StartupHydratedMsg{Err: emailsErr, FinishLoading: true}
		}
		return StartupHydratedMsg{
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

func (m *Model) reflowVisibleTableRows() {
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
		Foreground(m.theme.Severity.Info.ForegroundColor()).
		Background(m.theme.Chrome.TitleBar.BackgroundColor()).
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
	return styledSenderWithTheme(defaultTheme, raw, maxWidth)
}

func styledSenderWithTheme(theme Theme, raw string, maxWidth int) string {
	nameStyle := lipgloss.NewStyle().Foreground(theme.Text.Primary.ForegroundColor())
	emailStyle := lipgloss.NewStyle().Foreground(theme.Text.Muted.ForegroundColor())

	name := senderDisplayLabel(raw)
	email := senderAddress(raw)
	if name == "" {
		name = sanitizeText(raw)
	}
	if email == "" || strings.EqualFold(name, email) || maxWidth < 34 {
		return nameStyle.Render(ansi.Truncate(name, maxWidth, "…"))
	}

	combined := name + " <" + email + ">"
	combined = ansi.Truncate(combined, maxWidth, "…")
	if lt := strings.Index(combined, " <"); lt > 0 {
		return nameStyle.Render(combined[:lt]) + " " + emailStyle.Render(combined[lt+1:])
	}

	return nameStyle.Render(combined)
}

// updateTableDimensions recalculates table and column sizes based on terminal dimensions
func (m *Model) updateTableDimensions(width, height int) {
	if width == 0 {
		return
	}
	m.windowWidth = width
	m.windowHeight = height

	plan := m.buildLayoutPlan(width, height)
	m.normalizeFocusForLayout(plan)
	tableHeight := plan.ContentHeight
	m.sidebarTooWide = m.showSidebar &&
		m.activeTab == tabTimeline &&
		!(m.activeTab == tabTimeline && m.timeline.selectedEmail != nil) &&
		!plan.SidebarVisible

	const minPreviewWidth = 25
	previewWidth := 0
	if m.timeline.selectedEmail != nil {
		maxPreview := plan.Timeline.PreviewWidth
		if maxPreview >= minPreviewWidth {
			previewWidth = maxPreview
		}
	}
	m.timeline.previewWidth = previewWidth

	// Progressive timeline column hiding: drop optional metadata before letting
	// the sender and subject stop feeling like the primary reading surface.
	tWhenW := 12
	tTagW := 8
	tFixed := 1 + tWhenW + tTagW // Select + When + Tag
	tNCols := 5                  // Select, Sender, Subject, When, Tag
	const minTimelineVariable = 30

	timelineAvailable := plan.Timeline.TableWidth
	calcTimelineVar := func() int {
		v := timelineAvailable - tFixed - tNCols*2
		if v < 0 {
			return 0
		}
		return v
	}

	if calcTimelineVar() < minTimelineVariable && tTagW > 0 {
		tFixed -= tTagW
		tNCols--
		tTagW = 0
	}
	if calcTimelineVar() < minTimelineVariable && tWhenW > 0 {
		tFixed -= tWhenW
		tNCols--
		tWhenW = 0
	}

	timelineVariable := calcTimelineVar()
	tSenderWidth := timelineVariable * 32 / 100
	if timelineVariable >= 24 && tSenderWidth < 10 {
		tSenderWidth = 10
	}
	if tSenderWidth > 36 {
		tSenderWidth = 36
	}
	tSubjectWidth := timelineVariable - tSenderWidth
	if timelineVariable >= 24 && tSubjectWidth < 14 {
		tSubjectWidth = 14
		tSenderWidth = timelineVariable - tSubjectWidth
	}
	if tSenderWidth < 1 {
		tSenderWidth = 1
	}
	if tSubjectWidth < 1 {
		tSubjectWidth = 1
		if timelineVariable > 1 {
			tSenderWidth = timelineVariable - 1
		}
	}
	m.timeline.senderWidth = tSenderWidth
	m.timeline.subjectWidth = tSubjectWidth
	m.timelineTable.SetColumns([]table.Column{
		{Title: "✓", Width: 1},
		{Title: "Sender", Width: tSenderWidth},
		{Title: "Subject", Width: tSubjectWidth},
		{Title: "When", Width: tWhenW},
		{Title: "Tag", Width: tTagW},
	})
	m.timelineTable.SetWidth(tFixed + tSenderWidth + tSubjectWidth + tNCols*2)
	m.timelineTable.SetHeight(tableHeight + 1)

	logWidth := width - 4
	if logWidth < 20 {
		logWidth = 20
	}
	m.logViewer.SetSize(logWidth, tableHeight)

	m.composeTo.SetWidth(plan.Compose.FieldInnerWidth)
	m.composeCC.SetWidth(plan.Compose.FieldInnerWidth)
	m.composeBCC.SetWidth(plan.Compose.FieldInnerWidth)
	m.composeSubject.SetWidth(plan.Compose.FieldInnerWidth)
	m.attachmentPathInput.SetWidth(plan.Compose.FieldInnerWidth)
	composeBodyWidth := plan.Compose.BodyInnerWidth
	composeExtraRows := m.composeAdditionalRows(tableHeight)
	// Compose renders directly in the main viewport, while tableHeight is the
	// bordered-panel inner budget used by table surfaces. Give Compose back
	// those two rows so the body editor absorbs spare vertical space.
	composeFixedRows := m.composeFixedRows()
	composeViewportRows := tableHeight + 2
	composeBodyHeight := composeViewportRows - composeFixedRows - composeExtraRows
	minComposeBodyHeight := 3
	if composeExtraRows > 0 {
		minComposeBodyHeight = 1
	}
	if composeBodyHeight < minComposeBodyHeight {
		composeBodyHeight = minComposeBodyHeight
	}
	m.composeBody.SetWidth(composeBodyWidth)
	m.composeBody.SetHeight(composeBodyHeight)
	m.composeAIResponse.SetWidth(composeBodyWidth)
	m.composeAIResponse.SetHeight(composeBodyHeight)

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

// renderEmailBodyLines delegates to render.RenderEmailBodyLines.
func renderEmailBodyLines(text string, width int) []string {
	return render.RenderEmailBodyLines(text, width)
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
