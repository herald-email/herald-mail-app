package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	buildversion "github.com/herald-email/herald-mail-app/internal/version"
)

const problemReportLogLimit = 100
const problemReportSupportAddress = "support@herald-mail.app"
const problemReportFeedbackURL = "https://herald-mail.app/feedback/"
const problemReportShortcutHint = "ctrl+g: report"

type problemReportSnapshot struct {
	GeneratedAt   time.Time
	ConfigPath    string
	StatusMessage string
	LogPath       string
	DebugEnabled  bool
	Selected      models.EmailData
	HasSelected   bool
	PreviewLoad   previewLoadTelemetry
	Logs          []LogEntry
}

func previewFailureBodyText(msg EmailBodyMsg, email *models.EmailData) string {
	errText := previewLoadErrString(msg.Err)
	ref := previewFailureMessageRef(msg, email)
	hint := previewFailureHint(errText)

	var b strings.Builder
	b.WriteString("(Failed to load body)\n\n")
	b.WriteString("Reason: " + hint + "\n")
	if errText != "" {
		b.WriteString("Details: " + singleLineReportValue(errText) + "\n")
	}
	b.WriteString("Message: " + previewFailureRefLine(ref) + "\n\n")
	b.WriteString("Press Ctrl+G to choose how to email, copy, or save a problem report with the last Herald events. For the next run, start Herald with -debug and attach the log file shown in the report.")
	return b.String()
}

func isProblemReportShortcut(msg tea.KeyPressMsg) bool {
	switch shortcutKey(msg) {
	case "ctrl+g":
		return true
	default:
		return false
	}
}

func (m *Model) problemReportShortcutTextEntryActive() bool {
	if m.activeTab == tabCompose {
		return true
	}
	if m.showChat && m.focusedPanel == panelChat {
		return true
	}
	if m.timeline.attachmentSavePrompt {
		return true
	}
	if m.timeline.searchMode && m.activeTab == tabTimeline && m.timeline.searchFocus == timelineSearchFocusInput {
		return true
	}
	if m.activeTab == tabContacts && m.contactSearchMode != "" {
		return true
	}
	if m.activeTab == tabCalendar {
		if m.calendarEdit.Active {
			return true
		}
		if !m.calendarDetailOpen && (m.calendarView == calendarViewSearch || m.calendarView == calendarViewCrossSearch) {
			return true
		}
	}
	return false
}

func (m *Model) shouldHandleProblemReportShortcut(msg tea.KeyPressMsg) bool {
	if !isProblemReportShortcut(msg) {
		return false
	}
	if m.showProblemReport || m.problemReportShortcutTextEntryActive() {
		return false
	}
	return true
}

func previewFailureMessageRef(msg EmailBodyMsg, email *models.EmailData) models.MessageRef {
	ref := msg.MessageRef
	if email != nil {
		emailRef := email.MessageRef()
		if ref.SourceID == "" {
			ref.SourceID = emailRef.SourceID
		}
		if ref.AccountID == "" {
			ref.AccountID = emailRef.AccountID
		}
		if ref.LocalID == "" {
			ref.LocalID = emailRef.LocalID
		}
		if ref.UIDValidity == 0 {
			ref.UIDValidity = emailRef.UIDValidity
		}
		if ref.MessageID == "" {
			ref.MessageID = emailRef.MessageID
		}
		if ref.Folder == "" {
			ref.Folder = emailRef.Folder
		}
		if ref.UID == 0 {
			ref.UID = emailRef.UID
		}
	}
	if ref.MessageID == "" {
		ref.MessageID = msg.MessageID
	}
	if ref.Folder == "" {
		ref.Folder = msg.Folder
	}
	if ref.UID == 0 {
		ref.UID = msg.UID
	}
	return ref.WithDefaults()
}

func previewFailureHint(errText string) string {
	lower := strings.ToLower(errText)
	switch {
	case strings.Contains(lower, "auth") ||
		strings.Contains(lower, "oauth") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "invalid_grant") ||
		strings.Contains(lower, "401") ||
		strings.Contains(lower, "403"):
		return "Re-authenticate this account or refresh its IMAP/OAuth credentials."
	case strings.Contains(lower, "context deadline exceeded") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "timed out"):
		return "The provider did not return body data before Herald's preview timeout."
	case strings.Contains(lower, "not connected") ||
		strings.Contains(lower, "broken pipe") ||
		strings.Contains(lower, "connection") ||
		strings.Contains(lower, "eof") ||
		strings.Contains(lower, "network"):
		return "The IMAP/provider connection dropped while Herald was fetching the body."
	case strings.Contains(lower, "not found") ||
		strings.Contains(lower, "no such message") ||
		strings.Contains(lower, "uid"):
		return "The cached UID may be stale because the message moved, was deleted, or the folder UID validity changed. Refresh this folder."
	case strings.Contains(lower, "select folder") ||
		strings.Contains(lower, "mailbox") ||
		strings.Contains(lower, "folder"):
		return "Herald could not open the provider folder. The folder name, label permissions, or account routing may be wrong."
	case strings.Contains(lower, "mime") ||
		strings.Contains(lower, "parse"):
		return "Herald fetched the message but could not parse its MIME body."
	default:
		return "The provider returned an error while Herald was fetching the message body."
	}
}

func previewFailureRefLine(ref models.MessageRef) string {
	parts := []string{
		"source_id=" + string(ref.SourceID),
		"account_id=" + string(ref.AccountID),
		"folder=" + singleLineReportValue(ref.Folder),
		fmt.Sprintf("uid=%d", ref.UID),
	}
	if ref.UIDValidity != 0 {
		parts = append(parts, fmt.Sprintf("uid_validity=%d", ref.UIDValidity))
	}
	if ref.LocalID != "" {
		parts = append(parts, "local_id="+singleLineReportValue(ref.LocalID))
	}
	if ref.MessageID != "" {
		parts = append(parts, "message_id="+singleLineReportValue(ref.MessageID))
	}
	return strings.Join(parts, " ")
}

func (m *Model) writeProblemReportCmd() tea.Cmd {
	snapshot := m.problemReportSnapshot(time.Now())
	return func() tea.Msg {
		path, err := writeProblemReport(snapshot)
		return ProblemReportMsg{Path: path, Err: err}
	}
}

func (m *Model) openProblemReportSupportCompose() {
	snapshot := m.problemReportSnapshot(time.Now())
	email := m.problemReportEmail()
	m.rememberComposeReturn()
	m.activeTab = tabCompose
	m.composeTo.SetValue(problemReportSupportAddress)
	m.composeCC.SetValue("")
	m.composeBCC.SetValue("")
	m.composeSubject.SetValue("Herald problem report")
	m.composeBody.SetValue(problemReportSupportBody(snapshot))
	m.composeAttachments = nil
	if email != nil {
		m.setComposeSourceForEmail(email)
	}
	m.composeStatus = "Review this report before sending. Herald support will reply to the From address used for this email."
	m.statusMessage = ""
	m.replyContextEmail = nil
	m.composePreserved = nil
	m.composePreview = false
	m.composeAIThread = false
	m.resetComposeAIBar()
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeSubject.Blur()
	m.composeBody.Focus()
	m.resetFieldKeyMode()
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
}

func problemReportSupportBody(snapshot problemReportSnapshot) string {
	return strings.Join([]string{
		"Please describe what happened here:",
		"",
		"",
		"Support will reply to the From address used to send this email.",
		"",
		"---",
		"",
		formatProblemReport(snapshot),
	}, "\n")
}

func (m *Model) problemReportSnapshot(now time.Time) problemReportSnapshot {
	s := problemReportSnapshot{
		GeneratedAt:   now,
		ConfigPath:    m.configPath,
		StatusMessage: m.statusMessage,
		LogPath:       logger.Path(),
		DebugEnabled:  logger.IsDebugMode(),
		PreviewLoad:   m.timeline.previewLoad,
	}
	if email := m.problemReportEmail(); email != nil {
		s.Selected = *email
		s.HasSelected = true
	}
	if m.logViewer != nil {
		s.Logs = m.logViewer.LastEntries(problemReportLogLimit)
	}
	return s
}

func (m *Model) problemReportEmail() *models.EmailData {
	if m.activeTab == tabContacts && m.contactPreviewEmail != nil {
		return m.contactPreviewEmail
	}
	if m.timeline.selectedEmail != nil {
		return m.timeline.selectedEmail
	}
	if m.contactPreviewEmail != nil {
		return m.contactPreviewEmail
	}
	return nil
}

func writeProblemReport(snapshot problemReportSnapshot) (string, error) {
	dir := problemReportDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create report directory: %w", err)
	}
	name := fmt.Sprintf("PROBLEM_REPORT_%s.md", snapshot.GeneratedAt.Format("20060102_150405"))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(formatProblemReport(snapshot)), 0o600); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return path, nil
}

func problemReportDir() string {
	if dir := strings.TrimSpace(os.Getenv("HERALD_REPORT_DIR")); dir != "" {
		return expandTilde(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".", ".herald", "reports")
	}
	return filepath.Join(home, ".herald", "reports")
}

func (m *Model) renderProblemReportOverlayView() string {
	w, h := m.compactOverlayViewportSize()
	layout := newCompactOverlayLayoutWithMax(w, h, 76, 20)
	return m.renderCompactOverlayView(m.renderProblemReportPanel(layout))
}

func (m *Model) renderProblemReportPanel(layout compactOverlayLayout) string {
	var lines []string
	title := m.theme.Text.Primary.Style().Bold(true).Render("Report Problem")
	lines = append(lines, title, "")
	lines = append(lines, "Create a support bundle for the selected message.")
	lines = append(lines, "Nothing is written, sent, or saved until you choose.")
	lines = append(lines, "")
	lines = append(lines, "[e] Email Support")
	lines = append(lines, "    Open an editable draft to "+problemReportSupportAddress+".")
	lines = append(lines, "    Herald support will reply to the From address used")
	lines = append(lines, "    for that email.")
	lines = append(lines, "[c] Copy report/logs")
	lines = append(lines, "    Copy the diagnostic report to the clipboard.")
	lines = append(lines, "[s] Save to ~/.herald/reports")
	lines = append(lines, "    Write the report locally for manual sharing.")
	lines = append(lines, "[f] "+wizardHyperlink("feedback form", problemReportFeedbackURL))
	lines = append(lines, "    Copy the feedback form link.")
	lines = append(lines, "")
	lines = append(lines, "Esc closes this dialog.")

	return renderCompactOverlayBox(strings.Join(lines, "\n"), layout)
}

func formatProblemReport(snapshot problemReportSnapshot) string {
	var b strings.Builder
	b.WriteString("# Herald Problem Report\n\n")
	b.WriteString("Generated: " + snapshot.GeneratedAt.Format(time.RFC3339) + "\n")
	b.WriteString("Version: " + buildversion.String("herald") + "\n")
	b.WriteString("Runtime: " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + "\n")
	b.WriteString(fmt.Sprintf("Debug enabled: %t\n", snapshot.DebugEnabled))
	b.WriteString("Log file: " + valueOrNone(snapshot.LogPath) + "\n")
	b.WriteString("Config path: " + valueOrNone(snapshot.ConfigPath) + "\n")
	b.WriteString("Status: " + valueOrNone(snapshot.StatusMessage) + "\n")

	b.WriteString("\n## Selected Message\n\n")
	if snapshot.HasSelected {
		ref := snapshot.Selected.MessageRef()
		writeReportField(&b, "source_id", string(ref.SourceID))
		writeReportField(&b, "account_id", string(ref.AccountID))
		writeReportField(&b, "folder", ref.Folder)
		writeReportField(&b, "uid", fmt.Sprint(ref.UID))
		writeReportField(&b, "uid_validity", fmt.Sprint(ref.UIDValidity))
		writeReportField(&b, "local_id", ref.LocalID)
		writeReportField(&b, "message_id", ref.MessageID)
		b.WriteString("sender: redacted\n")
		b.WriteString("subject: redacted\n")
	} else {
		b.WriteString("No message was selected when the report was generated.\n")
	}

	b.WriteString("\n## Preview Load\n\n")
	load := snapshot.PreviewLoad
	ref := load.MessageRef
	if ref.MessageID == "" && snapshot.HasSelected {
		ref = snapshot.Selected.MessageRef()
	}
	if ref.MessageID == "" {
		b.WriteString("No preview load telemetry was available.\n")
	} else {
		ref = ref.WithDefaults()
		writeReportField(&b, "source_id", string(ref.SourceID))
		writeReportField(&b, "account_id", string(ref.AccountID))
		writeReportField(&b, "folder", firstNonEmptyString(load.Folder, ref.Folder))
		writeReportField(&b, "uid", fmt.Sprint(firstNonZeroUint32(load.UID, ref.UID)))
		writeReportField(&b, "uid_validity", fmt.Sprint(ref.UIDValidity))
		writeReportField(&b, "local_id", ref.LocalID)
		writeReportField(&b, "message_id", firstNonEmptyString(load.MessageID, ref.MessageID))
		writeReportField(&b, "load_source", load.Source)
		writeReportField(&b, "duration", load.Duration.String())
		if load.Err != "" {
			writeReportField(&b, "error", load.Err)
			writeReportField(&b, "hint", previewFailureHint(load.Err))
		}
	}

	b.WriteString(fmt.Sprintf("\n## Last %d events\n\n", problemReportLogLimit))
	if len(snapshot.Logs) == 0 {
		b.WriteString("No in-app log events were captured.\n")
		return b.String()
	}
	for _, entry := range snapshot.Logs {
		fmt.Fprintf(&b, "- %s %s %s\n",
			entry.Timestamp.Format(time.RFC3339),
			entry.Level,
			singleLineReportValue(entry.Message),
		)
	}
	return b.String()
}

func writeReportField(b *strings.Builder, name, value string) {
	b.WriteString(name + ": " + valueOrNone(value) + "\n")
}

func valueOrNone(value string) string {
	value = singleLineReportValue(value)
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}

func singleLineReportValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}

func firstNonZeroUint32(values ...uint32) uint32 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
