package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
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

func (m *Model) currentFolderIsReadOnlyDiagnostic() bool {
	return isVirtualAllMailOnlyFolder(m.currentFolder)
}

func (m *Model) timelineIsReadOnlyDiagnostic() bool {
	return m.activeTab == tabTimeline && m.currentFolderIsReadOnlyDiagnostic()
}

func (m *Model) readOnlyDiagnosticStatus() string {
	return displayFolderName(m.currentFolder) + " is read-only"
}

func (m *Model) selectedSidebarFolderPath() string {
	items := m.visibleSidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return ""
	}
	if items[m.sidebarCursor].kind != sidebarItemFolder {
		return ""
	}
	return items[m.sidebarCursor].node.fullPath
}

func (m *Model) activateCurrentFolder() tea.Cmd {
	if isVirtualAllMailOnlyFolder(m.currentFolder) {
		m.cancelBackgroundWork()
		m.loading = true
		m.startTime = time.Now()
		m.backend.StopIDLE()
		m.backend.StopPolling()
		m.syncStatusMode = ""
		m.syncCountdown = 0
		return m.loadTimelineEmails()
	}
	return tea.Batch(m.startLoading(), m.tickSpinner(), m.listenForSyncEvents())
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
