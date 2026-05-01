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

func (m *Model) cleanupIsReadOnlyDiagnostic() bool {
	return m.activeTab == tabCleanup && m.currentFolderIsReadOnlyDiagnostic()
}

func (m *Model) readOnlyDiagnosticStatus() string {
	return displayFolderName(m.currentFolder) + " is read-only"
}

func (m *Model) blockCleanupReadOnlyMutation() bool {
	if !m.cleanupIsReadOnlyDiagnostic() {
		return false
	}
	m.statusMessage = m.readOnlyDiagnosticStatus()
	return true
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

func cleanupGroupKey(sender string, groupByDomain bool) string {
	if !groupByDomain {
		return sender
	}
	return cleanupExtractDomain(sender)
}

func cleanupExtractDomain(sender string) string {
	if sender == "" {
		return sender
	}

	atIndex := strings.LastIndex(sender, "@")
	if atIndex == -1 || atIndex+1 >= len(sender) {
		return sender
	}

	domain := strings.TrimRight(strings.TrimSpace(sender[atIndex+1:]), ">")
	parts := strings.Split(domain, ".")
	if len(parts) > 2 {
		secondLevel := parts[len(parts)-2]
		if secondLevel == "co" || secondLevel == "com" || secondLevel == "org" ||
			secondLevel == "gov" || secondLevel == "edu" || secondLevel == "net" {
			if len(parts) >= 3 {
				return parts[len(parts)-3] + "." + parts[len(parts)-2] + "." + parts[len(parts)-1]
			}
		}
	}
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
	return domain
}

func buildCleanupGroupsFromEmails(emails []*models.EmailData, groupByDomain bool) map[string][]*models.EmailData {
	grouped := make(map[string][]*models.EmailData)
	for _, email := range emails {
		if email == nil {
			continue
		}
		key := cleanupGroupKey(email.Sender, groupByDomain)
		if strings.TrimSpace(key) == "" {
			continue
		}
		grouped[key] = append(grouped[key], email)
	}
	return grouped
}

func buildCleanupStatsFromGroups(grouped map[string][]*models.EmailData) map[string]*models.SenderStats {
	stats := make(map[string]*models.SenderStats, len(grouped))
	for key, emails := range grouped {
		if len(emails) == 0 {
			continue
		}

		totalSize := 0
		withAttachments := 0
		firstEmail := emails[0].Date
		lastEmail := emails[0].Date

		for _, email := range emails {
			totalSize += email.Size
			if email.HasAttachments {
				withAttachments++
			}
			if email.Date.Before(firstEmail) {
				firstEmail = email.Date
			}
			if email.Date.After(lastEmail) {
				lastEmail = email.Date
			}
		}

		stats[key] = &models.SenderStats{
			TotalEmails:     len(emails),
			AvgSize:         float64(totalSize) / float64(len(emails)),
			WithAttachments: withAttachments,
			FirstEmail:      firstEmail,
			LastEmail:       lastEmail,
		}
	}
	return stats
}

func (m *Model) hydrateCleanupFromVirtualFolderEmails(emails []*models.EmailData) {
	m.emailsBySender = buildCleanupGroupsFromEmails(emails, m.groupByDomain)
	m.stats = buildCleanupStatsFromGroups(m.emailsBySender)
	m.updateSummaryTable()
	m.updateDetailsTable()
}
