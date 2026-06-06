package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/work"
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
		key := timelineSelectionKey(email)
		if key == "" {
			continue
		}
		selectable++
		if !m.timeline.selectedMessageIDs[key] {
			allSelected = false
		}
	}
	if selectable == 0 {
		return
	}
	for _, email := range targets {
		key := timelineSelectionKey(email)
		if key == "" {
			continue
		}
		if allSelected {
			delete(m.timeline.selectedMessageIDs, key)
		} else {
			m.timeline.selectedMessageIDs[key] = true
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

func (m *Model) confirmDeleteSelected() tea.Cmd {
	if m.activeTab == tabTimeline {
		m.finishTimelineRangeSelection()
	}
	if m.timelineIsReadOnlyDiagnostic() {
		return nil
	}
	if m.loading || m.deleting || m.pendingDeleteConfirm {
		return nil
	}
	desc := m.buildDeleteDesc()
	if desc == "" {
		return nil
	}
	m.pendingDeleteConfirm = true
	m.pendingDeleteDesc = desc
	m.pendingArchive = false
	m.pendingDeleteAction = func() tea.Cmd {
		m.deleting = true
		return m.deleteSelected()
	}
	return nil
}

func (m *Model) deleteSelectedImmediately() tea.Cmd {
	if m.activeTab == tabTimeline {
		m.finishTimelineRangeSelection()
	}
	if m.timelineIsReadOnlyDiagnostic() {
		return nil
	}
	if m.loading || m.deleting {
		return nil
	}
	if m.buildDeleteDesc() == "" {
		return nil
	}
	m.pendingDeleteConfirm = false
	m.pendingDeleteAction = nil
	m.pendingDeleteDesc = ""
	m.pendingArchive = false
	m.deleting = true
	return m.deleteSelected()
}

// archiveSelected archives the selected senders or individual messages via queue.
func (m *Model) archiveSelected() tea.Cmd {
	return m.queueRequests(true)
}

// queueRequests builds deletion/archive requests and sends them to the worker.
func (m *Model) queueRequests(isArchive bool) tea.Cmd {
	type deleteTarget struct {
		ref                models.MessageRef
		messageID          string
		sender             string
		isDomain           bool
		folder             string
		affectedMessageIDs []string
	}

	folder := m.currentFolder
	var targets []deleteTarget
	seenMessageIDs := make(map[string]bool)

	appendMessageTarget := func(email *models.EmailData) {
		key := timelineSelectionKey(email)
		if email == nil || strings.TrimSpace(email.MessageID) == "" || key == "" || seenMessageIDs[key] {
			return
		}
		targetFolder := strings.TrimSpace(email.Folder)
		if targetFolder == "" {
			targetFolder = folder
		}
		seenMessageIDs[key] = true
		targets = append(targets, deleteTarget{
			ref:                email.MessageRef(),
			messageID:          email.MessageID,
			folder:             targetFolder,
			affectedMessageIDs: []string{email.MessageID, key},
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
					MessageRef:         t.ref,
					SourceID:           t.ref.SourceID,
					AccountID:          t.ref.AccountID,
					LocalID:            t.ref.LocalID,
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

	return nil
}

// executeDeletion runs a single deletion/archive request and returns the error.
func (m *Model) executeDeletion(req models.DeletionRequest) (int, error) {
	if req.MessageID != "" {
		ref := deletionRequestMessageRef(req)
		if req.IsArchive {
			logger.Info("Archiving message: %s", req.MessageID)
			if scoped, ok := m.backend.(interface{ ArchiveEmailByRef(models.MessageRef) error }); ok && ref.SourceID != "" {
				return 1, scoped.ArchiveEmailByRef(ref)
			}
			return 1, m.backend.ArchiveEmail(req.MessageID, req.Folder)
		}
		logger.Info("Deleting message: %s", req.MessageID)
		if scoped, ok := m.backend.(interface{ DeleteEmailByRef(models.MessageRef) error }); ok && ref.SourceID != "" {
			return 1, scoped.DeleteEmailByRef(ref)
		}
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
	coordinator := work.NewCoordinator()
	var wg sync.WaitGroup

	for req := range requestCh {
		req := req
		result := coordinator.Submit(context.Background(), work.Spec{
			SourceID:    deletionWorkSourceID(req),
			ResourceKey: deletionWorkResourceKey(req),
			Policy:      work.PolicySerialBySource,
			Priority:    work.PriorityUserAction,
			Run: func(context.Context) (any, error) {
				result := m.runDeletionRequest(req)
				if req.Response != nil {
					req.Response <- result
				}
				resultCh <- result
				// Throttle between operations on the same source to let Proton
				// Bridge / upstream APIs release sockets and sync state.
				time.Sleep(deletionThrottle)
				return result, nil
			},
		})
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = result.Await(context.Background())
		}()
	}
	wg.Wait()
	coordinator.Close()
}

func (m *Model) runDeletionRequest(req models.DeletionRequest) models.DeletionResult {
	result := deletionResultFromRequest(req)
	backoff := deletionRetryBackoff

	// Try the deletion, retrying on connection errors with backoff. The IMAP
	// layer already reconnects once per call; retries here handle the case
	// where reconnect itself fails, such as port exhaustion that needs time to
	// clear.
	for attempt := 0; attempt <= deletionMaxRetries; attempt++ {
		count, err := m.executeDeletion(req)
		result.DeletedCount = count
		result.Error = err

		if err == nil {
			break
		}

		// Non-connection errors are not retryable.
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
	return result
}

func deletionResultFromRequest(req models.DeletionRequest) models.DeletionResult {
	ref := deletionRequestMessageRef(req)
	result := models.DeletionResult{
		MessageRef:         ref,
		SourceID:           ref.SourceID,
		AccountID:          ref.AccountID,
		LocalID:            ref.LocalID,
		MessageID:          req.MessageID,
		Sender:             req.Sender,
		Folder:             req.Folder,
		IsDomain:           req.IsDomain,
		IsArchive:          req.IsArchive,
		AffectedMessageIDs: append([]string(nil), req.AffectedMessageIDs...),
	}
	if result.MessageID == "" {
		result.MessageID = ref.MessageID
	}
	if result.Folder == "" {
		result.Folder = ref.Folder
	}
	if result.MessageID != "" && len(result.AffectedMessageIDs) == 0 {
		result.AffectedMessageIDs = []string{result.MessageID}
	}
	return result
}

func deletionRequestMessageRef(req models.DeletionRequest) models.MessageRef {
	ref := req.MessageRef
	if ref.MessageID == "" {
		ref.MessageID = req.MessageID
	}
	if ref.Folder == "" {
		ref.Folder = req.Folder
	}
	if ref.SourceID == "" {
		ref.SourceID = req.SourceID
	}
	if ref.AccountID == "" {
		ref.AccountID = req.AccountID
	}
	if ref.LocalID == "" {
		ref.LocalID = req.LocalID
	}
	if ref.MessageID == "" && ref.LocalID == "" {
		return ref
	}
	return ref.WithDefaults()
}

func deletionWorkSourceID(req models.DeletionRequest) work.SourceID {
	ref := deletionRequestMessageRef(req)
	if ref.SourceID != "" {
		return work.SourceID(ref.SourceID)
	}
	if req.Sender != "" {
		return work.SourceID(models.DefaultMailSourceID)
	}
	return work.SourceID(models.DefaultMailSourceID)
}

func deletionWorkResourceKey(req models.DeletionRequest) work.ResourceKey {
	ref := deletionRequestMessageRef(req)
	operation := "delete"
	if req.IsArchive {
		operation = "archive"
	}
	itemID := ref.LocalID
	if itemID == "" {
		itemID = ref.MessageID
	}
	if itemID == "" {
		itemID = req.Sender
	}
	return work.ResourceKey{
		SourceID:     string(deletionWorkSourceID(req)),
		AccountID:    string(ref.AccountID),
		CollectionID: ref.Folder,
		ItemID:       itemID,
		Operation:    operation,
	}
}

func affectedDeletionMessageIDSet(result models.DeletionResult) map[string]bool {
	ids := make(map[string]bool, len(result.AffectedMessageIDs)+1)
	if id := strings.TrimSpace(result.MessageID); id != "" {
		ids[id] = true
	}
	if id := strings.TrimSpace(result.LocalID); id != "" {
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
		if email != nil && (ids[email.MessageID] || ids[timelineSelectionKey(email)]) {
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
	if origin := m.timeline.searchOrigin; origin != nil && origin.selectedEmail != nil && (ids[origin.selectedEmail.MessageID] || ids[timelineSelectionKey(origin.selectedEmail)]) {
		origin.selectedEmail = nil
		origin.body = nil
		origin.bodyMessageID = ""
		origin.bodyLoading = false
		origin.inlineImageDescs = nil
		origin.remoteImageLoads = nil
		origin.remoteImageRevision++
		origin.bodyScrollOffset = 0
	}
	if m.timeline.selectedEmail == nil || !(ids[m.timeline.selectedEmail.MessageID] || ids[timelineSelectionKey(m.timeline.selectedEmail)]) {
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
	m.resetTimelineThreadMemoryDossier()
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
	m.clearPreviewSelection()
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
	if pruned {
		m.updateTimelineTable()
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
	m.cancelBackgroundWork()
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
			if len(targets) > 1 {
				if kind, label, ok := m.currentTimelineCleanupGroupLabel(); ok {
					return fmt.Sprintf("Delete %s \"%s\" (%s)?", kind, label, countLabel(len(targets), "message", "messages"))
				}
			}
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
	return ""
}

func (m *Model) currentTimelineCleanupGroupLabel() (string, string, bool) {
	ref, ok := m.currentTimelineRowRef()
	if !ok || ref.group == nil {
		return "", "", false
	}
	switch ref.group.groupingMode {
	case timelineGroupingSender:
		label := strings.TrimSpace(ref.group.label)
		if label == "" {
			label = "(unknown)"
		}
		return "sender group", label, true
	case timelineGroupingDomain:
		label := strings.TrimSpace(ref.group.label)
		if label == "" {
			label = "(unknown)"
		}
		return "domain group", label, true
	default:
		return "", "", false
	}
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
			if len(targets) > 1 {
				if kind, label, ok := m.currentTimelineCleanupGroupLabel(); ok {
					desc := fmt.Sprintf("Archive %s \"%s\" (%s", kind, label, countLabel(len(eligible), "message", "messages"))
					if drafts > 0 {
						desc += ", skipping " + countLabel(drafts, "draft", "drafts")
					}
					return desc + ")?"
				}
			}
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
	return ""
}

// --- Search helpers ---

// performSearch runs a local or semantic search and returns the result as a tea.Cmd.
