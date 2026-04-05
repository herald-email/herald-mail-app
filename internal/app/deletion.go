package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

func (m *Model) toggleSelection() {
	if m.summaryTable.Focused() {
		cursor := m.summaryTable.Cursor()
		if m.selectedRows[cursor] {
			delete(m.selectedRows, cursor)
		} else {
			m.selectedRows[cursor] = true
		}
		// Refresh the table to show/hide checkmarks
		m.updateSummaryTable()
	} else if m.detailsTable.Focused() {
		cursor := m.detailsTable.Cursor()
		if cursor < len(m.detailsEmails) {
			messageID := m.detailsEmails[cursor].MessageID
			if messageID == "" {
				logger.Warn("Cannot select message with empty MessageID")
				return
			}
			if m.selectedMessages[messageID] {
				logger.Debug("Deselecting message: %s", messageID)
				delete(m.selectedMessages, messageID)
			} else {
				logger.Debug("Selecting message: %s", messageID)
				m.selectedMessages[messageID] = true
			}
			logger.Debug("Total selected messages: %d", len(m.selectedMessages))
			// Refresh the table to show/hide checkmarks
			m.updateDetailsTable()
		}
	}
}

// deleteSelected deletes the selected senders or individual messages via queue.
// All model state is read and mutations performed here (on the Update goroutine)
// before a background goroutine is launched, avoiding data races.
func (m *Model) deleteSelected() tea.Cmd {
	return m.queueRequests(false)
}

// archiveSelected archives the selected senders or individual messages via queue.
func (m *Model) archiveSelected() tea.Cmd {
	return m.queueRequests(true)
}

// queueRequests builds deletion/archive requests and sends them to the worker.
func (m *Model) queueRequests(isArchive bool) tea.Cmd {
	type deleteTarget struct {
		messageID string
		sender    string
		isDomain  bool
		folder    string
	}

	folder := m.currentFolder
	var targets []deleteTarget

	// Timeline tab: delete/archive current email
	if m.activeTab == tabTimeline {
		cursor := m.timelineTable.Cursor()
		if cursor < len(m.threadRowMap) {
			ref := m.threadRowMap[cursor]
			var email *models.EmailData
			if ref.kind == rowKindThread {
				email = ref.group.emails[0]
			} else {
				email = ref.group.emails[ref.emailIdx]
			}
			if email != nil {
				targets = append(targets, deleteTarget{messageID: email.MessageID, folder: email.Folder})
			}
		}
		if len(targets) == 0 {
			return nil
		}
		ch := m.deletionRequestCh
		go func() {
			for _, t := range targets {
				ch <- models.DeletionRequest{
					MessageID: t.messageID,
					Folder:    t.folder,
					IsArchive: isArchive,
				}
			}
		}()
		m.deletionsPending = len(targets)
		m.deletionsTotal = len(targets)
		return m.listenForDeletionResults()
	}

	if m.detailsTable.Focused() {
		if len(m.selectedMessages) > 0 {
			// Delete all selected messages (across all senders)
			for messageID := range m.selectedMessages {
				targets = append(targets, deleteTarget{messageID: messageID, folder: folder})
			}
			m.selectedMessages = make(map[string]bool) // safe: still on Update goroutine
		} else {
			// Delete current message
			cursor := m.detailsTable.Cursor()
			if cursor < len(m.detailsEmails) {
				email := m.detailsEmails[cursor]
				targets = append(targets, deleteTarget{messageID: email.MessageID, folder: folder})
			}
		}
	} else if len(m.selectedRows) > 0 {
		// Delete multiple selected senders (or domains in domain mode)
		for cursor := range m.selectedRows {
			sender, ok := m.rowToSender[cursor]
			if !ok || sender == "" {
				logger.Warn("No sender mapping found for row %d", cursor)
				continue
			}
			targets = append(targets, deleteTarget{sender: sender, isDomain: m.groupByDomain, folder: folder})
		}
		m.selectedRows = make(map[int]bool) // safe: still on Update goroutine
	} else {
		// Delete current sender using row mapping (or domain in domain mode)
		cursor := m.summaryTable.Cursor()
		sender, ok := m.rowToSender[cursor]
		if ok && sender != "" {
			targets = append(targets, deleteTarget{sender: sender, isDomain: m.groupByDomain, folder: folder})
		}
	}

	if len(targets) == 0 {
		return nil
	}

	// Send deletion requests to the queue from a goroutine so we don't block
	// the Update loop. targets is a local copy; no model state is accessed.
	ch := m.deletionRequestCh
	go func() {
		for _, t := range targets {
			ch <- models.DeletionRequest{
				MessageID: t.messageID,
				Sender:    t.sender,
				IsDomain:  t.isDomain,
				Folder:    t.folder,
				IsArchive: isArchive,
			}
		}
	}()

	// Set pending counters
	m.deletionsPending = len(targets)
	m.deletionsTotal = len(targets)
	logger.Info("Queued %d deletion(s) isArchive=%v", len(targets), isArchive)

	// Start listening for deletion results
	return m.listenForDeletionResults()
}

// deletionWorker processes deletion requests from the queue
func (m *Model) deletionWorker() {
	for req := range m.deletionRequestCh {
		result := models.DeletionResult{
			MessageID: req.MessageID,
			Sender:    req.Sender,
			Folder:    req.Folder,
		}

		// Perform deletion or archive based on what's provided
		if req.MessageID != "" {
			if req.IsArchive {
				logger.Info("Archiving message: %s", req.MessageID)
				result.Error = m.backend.ArchiveEmail(req.MessageID, req.Folder)
			} else {
				logger.Info("Deleting message: %s", req.MessageID)
				result.Error = m.backend.DeleteEmail(req.MessageID, req.Folder)
			}
			result.DeletedCount = 1
		} else if req.Sender != "" {
			if req.IsArchive {
				logger.Info("Archiving all messages from sender: %s", req.Sender)
				result.Error = m.backend.ArchiveSenderEmails(req.Sender, req.Folder)
			} else if req.IsDomain {
				logger.Info("Deleting all messages from domain: %s", req.Sender)
				result.Error = m.backend.DeleteDomainEmails(req.Sender, req.Folder)
			} else {
				logger.Info("Deleting all messages from sender: %s", req.Sender)
				result.Error = m.backend.DeleteSenderEmails(req.Sender, req.Folder)
			}
		}

		// Send result back
		if req.Response != nil {
			req.Response <- result
		}

		// Also send to result channel for UI updates
		m.deletionResultCh <- result
	}
}

// cleanup cleans up resources
func (m *Model) cleanup() {
	// Do not close deletionRequestCh: the goroutine spawned by deleteSelected
	// may still be sending to it, and closing a channel while a sender is
	// active causes a panic. The deletion worker goroutine will be terminated
	// when the process exits.
	if m.backend != nil {
		m.backend.Close()
	}
}

// --- Deletion/archive confirmation description builders ---

// buildDeleteDesc builds a human-readable description for the deletion confirmation prompt.
func (m *Model) buildDeleteDesc() string {
	if m.activeTab == tabTimeline {
		cursor := m.timelineTable.Cursor()
		if cursor < len(m.threadRowMap) {
			ref := m.threadRowMap[cursor]
			var email *models.EmailData
			if ref.kind == rowKindThread {
				email = ref.group.emails[0]
			} else {
				email = ref.group.emails[ref.emailIdx]
			}
			if email != nil {
				subj := email.Subject
				if len(subj) > 50 {
					subj = subj[:47] + "..."
				}
				return fmt.Sprintf("Delete \"%s\"?", subj)
			}
		}
		return ""
	}
	if m.detailsTable.Focused() {
		if len(m.selectedMessages) > 0 {
			return fmt.Sprintf("Delete %d selected message(s)?", len(m.selectedMessages))
		}
		cursor := m.detailsTable.Cursor()
		if cursor < len(m.detailsEmails) {
			return fmt.Sprintf("Delete message from %s?", m.detailsEmails[cursor].Sender)
		}
		return ""
	}
	if len(m.selectedRows) > 0 {
		return fmt.Sprintf("Delete emails from %d selected sender(s)?", len(m.selectedRows))
	}
	cursor := m.summaryTable.Cursor()
	if sender, ok := m.rowToSender[cursor]; ok && sender != "" {
		if m.groupByDomain {
			return fmt.Sprintf("Delete all emails from domain %s?", sender)
		}
		return fmt.Sprintf("Delete all emails from %s?", sender)
	}
	return ""
}

// buildArchiveDesc builds a human-readable description for the archive confirmation prompt.
func (m *Model) buildArchiveDesc() string {
	if m.activeTab == tabTimeline {
		cursor := m.timelineTable.Cursor()
		if cursor < len(m.threadRowMap) {
			ref := m.threadRowMap[cursor]
			var email *models.EmailData
			if ref.kind == rowKindThread {
				email = ref.group.emails[0]
			} else {
				email = ref.group.emails[ref.emailIdx]
			}
			if email != nil {
				subj := email.Subject
				if len(subj) > 50 {
					subj = subj[:47] + "..."
				}
				return fmt.Sprintf("Archive \"%s\"?", subj)
			}
		}
		return ""
	}
	if m.detailsTable.Focused() {
		if len(m.selectedMessages) > 0 {
			return fmt.Sprintf("Archive %d selected message(s)?", len(m.selectedMessages))
		}
		cursor := m.detailsTable.Cursor()
		if cursor < len(m.detailsEmails) {
			return fmt.Sprintf("Archive message from %s?", m.detailsEmails[cursor].Sender)
		}
		return ""
	}
	if len(m.selectedRows) > 0 {
		return fmt.Sprintf("Archive emails from %d selected sender(s)?", len(m.selectedRows))
	}
	cursor := m.summaryTable.Cursor()
	if sender, ok := m.rowToSender[cursor]; ok && sender != "" {
		return fmt.Sprintf("Archive all emails from %s?", sender)
	}
	return ""
}

// --- Search helpers ---

// performSearch runs a local or semantic search and returns the result as a tea.Cmd.
