package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
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

func countLabel(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func (m *Model) toggleSelection() {
	if m.summaryTable.Focused() {
		key, ok := m.summaryKeyAtCursor()
		if !ok {
			return
		}
		if m.selectedSummaryKeys[key] {
			delete(m.selectedSummaryKeys, key)
		} else {
			m.selectedSummaryKeys[key] = true
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

func (m *Model) toggleTimelineSelection() {
	if m.activeTab != tabTimeline || m.timelineIsReadOnlyDiagnostic() {
		return
	}
	if m.focusedPanel != panelTimeline {
		return
	}
	targets := m.currentTimelineRowEmails()
	if len(targets) == 0 {
		return
	}
	m.ensureTimelineSelection()
	allSelected := true
	selectable := 0
	for _, email := range targets {
		if email == nil || email.MessageID == "" {
			continue
		}
		selectable++
		if !m.timeline.selectedMessageIDs[email.MessageID] {
			allSelected = false
		}
	}
	if selectable == 0 {
		return
	}
	for _, email := range targets {
		if email == nil || email.MessageID == "" {
			continue
		}
		if allSelected {
			delete(m.timeline.selectedMessageIDs, email.MessageID)
		} else {
			m.timeline.selectedMessageIDs[email.MessageID] = true
		}
	}
	m.updateTimelineTable()
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
		messageID          string
		sender             string
		isDomain           bool
		folder             string
		affectedMessageIDs []string
	}

	folder := m.currentFolder
	virtualCleanup := m.activeTab == tabCleanup && isVirtualAllMailOnlyFolder(folder)
	var targets []deleteTarget
	seenMessageIDs := make(map[string]bool)

	affectedIDsForEmails := func(emails []*models.EmailData) []string {
		ids := make([]string, 0, len(emails))
		seen := make(map[string]bool, len(emails))
		for _, email := range emails {
			if email == nil {
				continue
			}
			id := strings.TrimSpace(email.MessageID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		return ids
	}

	findCleanupEmail := func(messageID string) *models.EmailData {
		for _, emails := range m.emailsBySender {
			for _, email := range emails {
				if email != nil && email.MessageID == messageID {
					return email
				}
			}
		}
		return nil
	}

	appendMessageTarget := func(email *models.EmailData) {
		if email == nil || strings.TrimSpace(email.MessageID) == "" || seenMessageIDs[email.MessageID] {
			return
		}
		targetFolder := strings.TrimSpace(email.Folder)
		if targetFolder == "" {
			targetFolder = folder
		}
		if virtualCleanup && isVirtualAllMailOnlyFolder(targetFolder) {
			logger.Warn("Skipping virtual-folder cleanup delete for %q with unresolved real folder", email.MessageID)
			return
		}
		seenMessageIDs[email.MessageID] = true
		targets = append(targets, deleteTarget{
			messageID:          email.MessageID,
			folder:             targetFolder,
			affectedMessageIDs: []string{email.MessageID},
		})
	}

	// Timeline tab: delete/archive current email
	if m.activeTab == tabTimeline {
		selectedTargets := m.selectedTimelineEmails(true)
		if len(selectedTargets) > 0 {
			for _, email := range selectedTargets {
				if isArchive && email.IsDraft {
					continue
				}
				appendMessageTarget(email)
			}
			if len(targets) == 0 {
				m.statusMessage = "Selected drafts cannot be archived"
				return nil
			}
		} else if !isArchive {
			if draft := m.currentTimelineFocusedDraftEmail(); draft != nil {
				appendMessageTarget(draft)
			}
		}
		if len(targets) == 0 {
			for _, email := range m.currentTimelineRowEmails() {
				if isArchive && email != nil && email.IsDraft {
					continue
				}
				appendMessageTarget(email)
			}
		}
		if len(targets) == 0 {
			return nil
		}
		ch := m.deletionRequestCh
		go func() {
			for _, t := range targets {
				ch <- models.DeletionRequest{
					MessageID:          t.messageID,
					Folder:             t.folder,
					IsArchive:          isArchive,
					AffectedMessageIDs: t.affectedMessageIDs,
				}
			}
		}()
		m.deletionsPending = len(targets)
		m.deletionsTotal = len(targets)
		logger.Info("Queued %d timeline deletion(s) isArchive=%v", len(targets), isArchive)
		return m.listenForDeletionResults()
	}

	if m.detailsTable.Focused() {
		if len(m.selectedMessages) > 0 {
			// Delete all selected messages (across all senders)
			for messageID := range m.selectedMessages {
				if email := findCleanupEmail(messageID); email != nil {
					appendMessageTarget(email)
					continue
				}
				targets = append(targets, deleteTarget{
					messageID:          messageID,
					folder:             folder,
					affectedMessageIDs: []string{messageID},
				})
			}
		} else {
			// Delete current message
			cursor := m.detailsTable.Cursor()
			if cursor < len(m.detailsEmails) {
				email := m.detailsEmails[cursor]
				appendMessageTarget(email)
			}
		}
	} else if len(m.selectedSummaryKeys) > 0 {
		// Delete multiple selected senders (or domains in domain mode)
		for key := range m.selectedSummaryKeys {
			if key == "" {
				continue
			}
			if virtualCleanup {
				for _, email := range m.emailsBySender[key] {
					appendMessageTarget(email)
				}
				continue
			}
			targets = append(targets, deleteTarget{
				sender:             key,
				isDomain:           m.groupByDomain,
				folder:             folder,
				affectedMessageIDs: affectedIDsForEmails(m.emailsBySender[key]),
			})
		}
	} else {
		// Delete current sender using row mapping (or domain in domain mode)
		sender, ok := m.summaryKeyAtCursor()
		if ok && sender != "" {
			if virtualCleanup {
				for _, email := range m.emailsBySender[sender] {
					appendMessageTarget(email)
				}
			} else {
				targets = append(targets, deleteTarget{
					sender:             sender,
					isDomain:           m.groupByDomain,
					folder:             folder,
					affectedMessageIDs: affectedIDsForEmails(m.emailsBySender[sender]),
				})
			}
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
				MessageID:          t.messageID,
				Sender:             t.sender,
				IsDomain:           t.isDomain,
				Folder:             t.folder,
				IsArchive:          isArchive,
				AffectedMessageIDs: t.affectedMessageIDs,
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
func (m *Model) deletionWorker(requestCh <-chan models.DeletionRequest, resultCh chan<- models.DeletionResult) {
	backoff := deletionRetryBackoff

	for req := range requestCh {
		result := models.DeletionResult{
			MessageID:          req.MessageID,
			Sender:             req.Sender,
			Folder:             req.Folder,
			IsDomain:           req.IsDomain,
			IsArchive:          req.IsArchive,
			AffectedMessageIDs: append([]string(nil), req.AffectedMessageIDs...),
		}
		if result.MessageID != "" && len(result.AffectedMessageIDs) == 0 {
			result.AffectedMessageIDs = []string{result.MessageID}
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
		resultCh <- result

		// Throttle between operations to let Proton Bridge / upstream API
		// release sockets and sync state.
		time.Sleep(deletionThrottle)
	}
}

func affectedDeletionMessageIDSet(result models.DeletionResult) map[string]bool {
	ids := make(map[string]bool, len(result.AffectedMessageIDs)+1)
	if id := strings.TrimSpace(result.MessageID); id != "" {
		ids[id] = true
	}
	for _, id := range result.AffectedMessageIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids[id] = true
		}
	}
	return ids
}

func pruneEmailSliceByMessageID(emails []*models.EmailData, ids map[string]bool) ([]*models.EmailData, bool) {
	if len(emails) == 0 || len(ids) == 0 {
		return emails, false
	}
	filtered := emails[:0]
	pruned := false
	for _, email := range emails {
		if email != nil && ids[email.MessageID] {
			pruned = true
			continue
		}
		filtered = append(filtered, email)
	}
	return filtered, pruned
}

func (m *Model) clearTimelinePreviewIfDeleted(ids map[string]bool) {
	if len(ids) == 0 {
		return
	}
	if origin := m.timeline.searchOrigin; origin != nil && origin.selectedEmail != nil && ids[origin.selectedEmail.MessageID] {
		origin.selectedEmail = nil
		origin.body = nil
		origin.bodyMessageID = ""
		origin.bodyLoading = false
		origin.bodyScrollOffset = 0
	}
	if m.timeline.selectedEmail == nil || !ids[m.timeline.selectedEmail.MessageID] {
		return
	}
	if m.timeline.bodyFetchCancel != nil {
		m.timeline.bodyFetchCancel()
		m.timeline.bodyFetchCancel = nil
	}
	m.revokeImagePreviews()
	m.timeline.selectedEmail = nil
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyLoading = false
	m.timeline.inlineImageDescs = nil
	m.timeline.fullScreen = false
	m.timeline.bodyWrappedLines = nil
	m.timeline.bodyWrappedWidth = 0
	m.timeline.bodyScrollOffset = 0
	m.timeline.selectedAttachment = 0
	m.timeline.attachmentSavePrompt = false
	m.timeline.attachmentSaveWarning = ""
	m.timeline.quickReplies = nil
	m.timeline.quickRepliesReady = false
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyPending = false
	m.timeline.quickReplyIdx = 0
	m.timeline.quickRepliesAIFetched = false
	m.timeline.visualMode = false
	m.timeline.visualStart = 0
	m.timeline.visualEnd = 0
	m.timeline.pendingY = false
}

func (m *Model) pruneCleanupStateAfterDeletion(ids map[string]bool) bool {
	if len(ids) == 0 {
		return false
	}
	pruned := false
	for id := range ids {
		delete(m.selectedMessages, id)
	}
	if filtered, ok := pruneEmailSliceByMessageID(m.detailsEmails, ids); ok {
		m.detailsEmails = filtered
		pruned = true
	}
	for key, emails := range m.emailsBySender {
		filtered, ok := pruneEmailSliceByMessageID(emails, ids)
		if !ok {
			continue
		}
		pruned = true
		if len(filtered) == 0 {
			delete(m.emailsBySender, key)
			delete(m.selectedSummaryKeys, key)
			if m.selectedSender == key {
				m.selectedSender = ""
			}
			continue
		}
		m.emailsBySender[key] = filtered
	}
	return pruned
}

func (m *Model) pruneTimelineStateAfterDeletion(result models.DeletionResult) bool {
	ids := affectedDeletionMessageIDSet(result)
	if len(ids) == 0 {
		return false
	}
	pruned := false
	for id := range ids {
		if m.timeline.selectedMessageIDs != nil && m.timeline.selectedMessageIDs[id] {
			delete(m.timeline.selectedMessageIDs, id)
			pruned = true
		}
	}
	if filtered, ok := pruneEmailSliceByMessageID(m.timeline.emails, ids); ok {
		m.timeline.emails = filtered
		pruned = true
	}
	if filtered, ok := pruneEmailSliceByMessageID(m.timeline.searchResults, ids); ok {
		m.timeline.searchResults = filtered
		pruned = true
	}
	if filtered, ok := pruneEmailSliceByMessageID(m.timeline.chatFilteredEmails, ids); ok {
		m.timeline.chatFilteredEmails = filtered
		pruned = true
	}
	m.clearTimelinePreviewIfDeleted(ids)
	if m.pruneCleanupStateAfterDeletion(ids) {
		pruned = true
	}
	if pruned {
		m.updateTimelineTable()
		m.rebuildDetailsRows()
	}
	return pruned
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
	if m.imagePreviewLinks != nil {
		m.imagePreviewLinks.Close()
	}
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
		if selected := m.selectedTimelineEmails(true); len(selected) > 0 {
			drafts := 0
			for _, email := range selected {
				if email != nil && email.IsDraft {
					drafts++
				}
			}
			desc := "Delete " + countLabel(len(selected), "selected message", "selected messages")
			if drafts > 0 {
				desc += " (discard " + countLabel(drafts, "draft", "drafts") + ")"
			}
			return desc + "?"
		}
		if draft := m.currentTimelineFocusedDraftEmail(); draft != nil {
			subj := draft.Subject
			if len(subj) > 50 {
				subj = subj[:47] + "..."
			}
			return fmt.Sprintf("Discard draft \"%s\"?", subj)
		}
		targets := m.currentTimelineRowEmails()
		if len(targets) > 0 {
			subj := targets[0].Subject
			if len(subj) > 50 {
				subj = subj[:47] + "..."
			}
			if len(targets) > 1 {
				return fmt.Sprintf("Delete thread \"%s\" (%d messages)?", subj, len(targets))
			}
			return fmt.Sprintf("Delete \"%s\"?", subj)
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
	if len(m.selectedSummaryKeys) > 0 {
		if m.groupByDomain {
			return fmt.Sprintf("Delete emails from %d selected domain(s)?", len(m.selectedSummaryKeys))
		}
		return fmt.Sprintf("Delete emails from %d selected sender(s)?", len(m.selectedSummaryKeys))
	}
	if sender, ok := m.summaryKeyAtCursor(); ok && sender != "" {
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
		if selected := m.selectedTimelineEmails(true); len(selected) > 0 {
			eligible := 0
			drafts := 0
			for _, email := range selected {
				if email != nil && email.IsDraft {
					drafts++
				} else if email != nil {
					eligible++
				}
			}
			if eligible == 0 {
				return ""
			}
			desc := "Archive " + countLabel(eligible, "selected message", "selected messages")
			if drafts > 0 {
				desc += " (skipping " + countLabel(drafts, "draft", "drafts") + ")"
			}
			return desc + "?"
		}
		targets := m.currentTimelineRowEmails()
		var eligible []*models.EmailData
		drafts := 0
		for _, email := range targets {
			if email == nil {
				continue
			}
			if email.IsDraft {
				drafts++
			} else {
				eligible = append(eligible, email)
			}
		}
		if len(eligible) > 0 {
			subj := eligible[0].Subject
			if len(subj) > 50 {
				subj = subj[:47] + "..."
			}
			if len(targets) > 1 {
				desc := fmt.Sprintf("Archive thread \"%s\" (%s", subj, countLabel(len(eligible), "message", "messages"))
				if drafts > 0 {
					desc += ", skipping " + countLabel(drafts, "draft", "drafts")
				}
				return desc + ")?"
			}
			return fmt.Sprintf("Archive \"%s\"?", subj)
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
	if len(m.selectedSummaryKeys) > 0 {
		if m.groupByDomain {
			return fmt.Sprintf("Archive emails from %d selected domain(s)?", len(m.selectedSummaryKeys))
		}
		return fmt.Sprintf("Archive emails from %d selected sender(s)?", len(m.selectedSummaryKeys))
	}
	if sender, ok := m.summaryKeyAtCursor(); ok && sender != "" {
		if m.groupByDomain {
			return fmt.Sprintf("Archive all emails from domain %s?", sender)
		}
		return fmt.Sprintf("Archive all emails from %s?", sender)
	}
	return ""
}

// --- Search helpers ---

// performSearch runs a local or semantic search and returns the result as a tea.Cmd.
