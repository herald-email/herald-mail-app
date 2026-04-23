package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// deletionThrottle is the minimum pause between consecutive IMAP delete
// operations, giving Proton Bridge (and similar backends) time to sync
// with their upstream API and release sockets.
const deletionThrottle = 1 * time.Second

// deletionRetryBackoff is the initial wait before retrying a failed
// deletion due to a connection error. The backoff doubles on each
// consecutive failure, capped at deletionMaxBackoff.
const deletionRetryBackoff = 2 * time.Second
const deletionMaxBackoff = 30 * time.Second

// deletionMaxRetries is how many times the worker retries a single
// request before giving up and moving to the next one.
const deletionMaxRetries = 3

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
		if cursor < len(m.timeline.threadRowMap) {
			ref := m.timeline.threadRowMap[cursor]
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

// executeDeletion runs a single deletion/archive request and returns the error.
func (m *Model) executeDeletion(req models.DeletionRequest) (int, error) {
	if req.MessageID != "" {
		if req.IsArchive {
			logger.Info("Archiving message: %s", req.MessageID)
			return 1, m.backend.ArchiveEmail(req.MessageID, req.Folder)
		}
		logger.Info("Deleting message: %s", req.MessageID)
		return 1, m.backend.DeleteEmail(req.MessageID, req.Folder)
	}
	if req.Sender != "" {
		if req.IsArchive {
			logger.Info("Archiving all messages from sender: %s", req.Sender)
			return 0, m.backend.ArchiveSenderEmails(req.Sender, req.Folder)
		}
		if req.IsDomain {
			logger.Info("Deleting all messages from domain: %s", req.Sender)
			return 0, m.backend.DeleteDomainEmails(req.Sender, req.Folder)
		}
		logger.Info("Deleting all messages from sender: %s", req.Sender)
		return 0, m.backend.DeleteSenderEmails(req.Sender, req.Folder)
	}
	return 0, nil
}

// deletionWorker processes deletion requests from the queue.
// It throttles operations to avoid overwhelming the IMAP backend and
// retries with exponential backoff when the connection drops.
func (m *Model) deletionWorker() {
	backoff := deletionRetryBackoff

	for req := range m.deletionRequestCh {
		result := models.DeletionResult{
			MessageID: req.MessageID,
			Sender:    req.Sender,
			Folder:    req.Folder,
		}

		// Try the deletion, retrying on connection errors with backoff.
		// The IMAP layer already reconnects once per call; retries here
		// handle the case where reconnect itself fails (e.g. port exhaustion
		// that needs time to clear).
		for attempt := 0; attempt <= deletionMaxRetries; attempt++ {
			count, err := m.executeDeletion(req)
			result.DeletedCount = count
			result.Error = err

			if err == nil {
				backoff = deletionRetryBackoff // reset on success
				break
			}

			// Non-connection errors are not retryable
			if !isConnectionErrorStr(err.Error()) {
				break
			}

			result.ConnectionLost = true

			if attempt < deletionMaxRetries {
				logger.Warn("Deletion failed (attempt %d/%d), retrying in %v: %v",
					attempt+1, deletionMaxRetries+1, backoff, err)
				time.Sleep(backoff)
				backoff *= 2
				if backoff > deletionMaxBackoff {
					backoff = deletionMaxBackoff
				}
			} else {
				logger.Error("Deletion failed after %d attempts, moving to next: %v",
					deletionMaxRetries+1, err)
			}
		}

		if req.Response != nil {
			req.Response <- result
		}
		m.deletionResultCh <- result

		// Throttle between operations to let Proton Bridge / upstream API
		// release sockets and sync state.
		time.Sleep(deletionThrottle)
	}
}

// isConnectionErrorStr checks if an error message indicates a dead connection.
func isConnectionErrorStr(s string) bool {
	for _, substr := range []string{
		"broken pipe", "connection reset", "connection closed",
		"i/o timeout", "use of closed network connection", "EOF",
		"reconnect failed", "can't assign requested address",
	} {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// cleanup cleans up resources
func (m *Model) cleanup() {
	// Do not close deletionRequestCh: the goroutine spawned by deleteSelected
	// may still be sending to it, and closing a channel while a sender is
	// active causes a panic. The deletion worker goroutine will be terminated
	// when the process exits.
	if m.backend != nil {
		go func() {
			_ = m.backend.Close()
		}()
	}
}

// --- Deletion/archive confirmation description builders ---

// buildDeleteDesc builds a human-readable description for the deletion confirmation prompt.
func (m *Model) buildDeleteDesc() string {
	if m.activeTab == tabTimeline {
		cursor := m.timelineTable.Cursor()
		if cursor < len(m.timeline.threadRowMap) {
			ref := m.timeline.threadRowMap[cursor]
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
		if cursor < len(m.timeline.threadRowMap) {
			ref := m.timeline.threadRowMap[cursor]
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
