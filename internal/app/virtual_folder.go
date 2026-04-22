package app

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/models"
)

const virtualFolderAllMailOnly = "__virtual__/all-mail-only"

func isVirtualAllMailOnlyFolder(folder string) bool {
	return folder == virtualFolderAllMailOnly
}

func displayFolderName(folder string) string {
	if isVirtualAllMailOnlyFolder(folder) {
		return "All Mail only"
	}
	return folder
}

func (m *Model) timelineIsReadOnlyDiagnostic() bool {
	return m.activeTab == tabTimeline && isVirtualAllMailOnlyFolder(m.currentFolder)
}

func (m *Model) selectedSidebarFolderPath() string {
	items := flattenTree(m.folderTree)
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return ""
	}
	return items[m.sidebarCursor].node.fullPath
}

func (m *Model) activateCurrentFolder() tea.Cmd {
	if isVirtualAllMailOnlyFolder(m.currentFolder) {
		m.loading = true
		m.startupFallbackIssued = false
		m.startTime = time.Now()
		m.backend.StopIDLE()
		m.backend.StopPolling()
		m.syncStatusMode = ""
		m.syncCountdown = 0
		return m.loadTimelineEmails()
	}
	return tea.Batch(m.startLoading(), m.tickSpinner(), m.listenForProgress())
}

func filterVirtualFolderEmails(emails []*models.EmailData, query string) []*models.EmailData {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return []*models.EmailData{}
	}
	out := make([]*models.EmailData, 0, len(emails))
	for _, email := range emails {
		if email == nil {
			continue
		}
		haystack := strings.ToLower(email.Sender + "\n" + email.Subject)
		if strings.Contains(haystack, query) {
			out = append(out, email)
		}
	}
	return out
}
