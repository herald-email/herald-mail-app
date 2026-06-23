package imap

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// DeleteSenderEmails deletes all emails from a sender in both IMAP and cache.
// On connection errors it reconnects once and retries.
func (c *Client) DeleteSenderEmails(sender, folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.deleteSenderEmailsLocked(sender, folder)
	if err != nil && isConnectionError(err) {
		logger.Info("DeleteSenderEmails: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		err = c.deleteSenderEmailsLocked(sender, folder)
	}
	return err
}

// deleteSenderEmailsLocked performs the actual deletion. Must be called with c.mu held.
func (c *Client) deleteSenderEmailsLocked(sender, folder string) error {
	logger.Info("Starting deletion of all emails from '%s' (len=%d) in folder %s", sender, len(sender), folder)

	mbox, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	logger.Info("Folder '%s' has %d total messages", folder, mbox.Messages)

	var seqNums []uint32

	if mbox.Messages > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddRange(1, mbox.Messages)

		messages := make(chan *imap.Message, 10)
		done := make(chan error, 1)

		go func() {
			done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
		}()

		matchCount := 0
		for msg := range messages {
			if msg.Envelope != nil && len(msg.Envelope.From) > 0 && msg.Envelope.From[0] != nil {
				addr := msg.Envelope.From[0]
				fromAddr := ""
				if addr.MailboxName != "" && addr.HostName != "" {
					fromAddr = addr.MailboxName + "@" + addr.HostName
				}

				if matchCount < 5 {
					logger.Debug("Message %d: from='%s' (len=%d) vs target='%s' (len=%d) match=%v",
						msg.SeqNum, fromAddr, len(fromAddr), sender, len(sender), fromAddr == sender)
				}

				if fromAddr == sender {
					seqNums = append(seqNums, msg.SeqNum)
					matchCount++
				}
			}
		}

		if err := <-done; err != nil {
			return fmt.Errorf("failed to fetch messages: %w", err)
		}
	}

	logger.Info("Found %d messages from '%s' to move to Trash", len(seqNums), sender)

	if len(seqNums) > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddNum(seqNums...)

		trashFolders := []string{"Trash", "Deleted Items", "[Gmail]/Trash", "INBOX.Trash"}
		moved := false

		for _, trashFolder := range trashFolders {
			if err := c.client.Copy(seqset, trashFolder); err == nil {
				logger.Info("Copied %d messages to %s", len(seqNums), trashFolder)
				moved = true
				break
			}
		}

		if !moved {
			logger.Warn("Could not copy to any Trash folder, marking as deleted instead")
		}

		store := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
			return fmt.Errorf("failed to mark messages as deleted: %w", err)
		}

		if err := c.client.Expunge(nil); err != nil {
			return fmt.Errorf("failed to expunge: %w", err)
		}

		logger.Info("Successfully moved %d messages from %s to Trash", len(seqNums), sender)
	}

	if err := c.cache.DeleteSenderEmails(sender, folder); err != nil {
		return fmt.Errorf("failed to delete from cache: %w", err)
	}

	logger.Info("Successfully completed deletion for %s", sender)
	return nil
}

// DeleteDomainEmails deletes all emails from a domain in both IMAP and cache.
// On connection errors it reconnects once and retries.
func (c *Client) DeleteDomainEmails(domain, folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.deleteDomainEmailsLocked(domain, folder)
	if err != nil && isConnectionError(err) {
		logger.Info("DeleteDomainEmails: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		err = c.deleteDomainEmailsLocked(domain, folder)
	}
	return err
}

// deleteDomainEmailsLocked performs the actual deletion. Must be called with c.mu held.
func (c *Client) deleteDomainEmailsLocked(domain, folder string) error {
	logger.Info("Starting deletion of all emails from domain '%s' in folder %s", domain, folder)

	mbox, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	logger.Info("Folder '%s' has %d total messages", folder, mbox.Messages)

	var seqNums []uint32

	if mbox.Messages > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddRange(1, mbox.Messages)

		messages := make(chan *imap.Message, 10)
		done := make(chan error, 1)

		go func() {
			done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
		}()

		matchCount := 0
		for msg := range messages {
			if msg.Envelope != nil && len(msg.Envelope.From) > 0 && msg.Envelope.From[0] != nil {
				addr := msg.Envelope.From[0]
				fromAddr := ""
				if addr.MailboxName != "" && addr.HostName != "" {
					fromAddr = addr.MailboxName + "@" + addr.HostName
				}

				if addr.HostName == domain || strings.HasSuffix(addr.HostName, "."+domain) {
					seqNums = append(seqNums, msg.SeqNum)
					matchCount++
					if matchCount <= 5 {
						logger.Debug("Match %d: from='%s' domain='%s'", matchCount, fromAddr, addr.HostName)
					}
				}
			}
		}

		if err := <-done; err != nil {
			return fmt.Errorf("failed to fetch messages: %w", err)
		}
	}

	logger.Info("Found %d messages from domain '%s' to move to Trash", len(seqNums), domain)

	if len(seqNums) > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddNum(seqNums...)

		trashFolders := []string{"Trash", "Deleted Items", "[Gmail]/Trash", "INBOX.Trash"}
		moved := false

		for _, trashFolder := range trashFolders {
			if err := c.client.Copy(seqset, trashFolder); err == nil {
				logger.Info("Copied %d messages to %s", len(seqNums), trashFolder)
				moved = true
				break
			}
		}

		if !moved {
			logger.Warn("Could not copy to any Trash folder, marking as deleted instead")
		}

		store := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
			return fmt.Errorf("failed to mark messages as deleted: %w", err)
		}

		if err := c.client.Expunge(nil); err != nil {
			return fmt.Errorf("failed to expunge: %w", err)
		}

		logger.Info("Successfully moved %d messages from domain %s to Trash", len(seqNums), domain)
	}

	if err := c.cache.DeleteDomainEmails(domain, folder); err != nil {
		return fmt.Errorf("failed to delete from cache: %w", err)
	}

	logger.Info("Successfully completed deletion for domain %s", domain)
	return nil
}

// DeleteEmail moves a specific email to Trash by MessageID.
// On connection errors it reconnects once and retries.
func (c *Client) DeleteEmail(messageID string, folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.deleteEmailLocked(messageID, folder)
	if err != nil && isConnectionError(err) {
		logger.Info("DeleteEmail: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		err = c.deleteEmailLocked(messageID, folder)
	}
	return err
}

// deleteEmailLocked performs the actual deletion. Must be called with c.mu held.
func (c *Client) deleteEmailLocked(messageID string, folder string) error {
	logger.Info("Moving message to Trash: %s", messageID)

	_, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("Message-ID", messageID)

	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search for message: %w", err)
	}

	logger.Info("Found %d messages with Message-ID %s", len(seqNums), messageID)

	if len(seqNums) > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddNum(seqNums[0])

		trashFolders := []string{"Trash", "Deleted Items", "[Gmail]/Trash", "INBOX.Trash"}
		moved := false

		for _, trashFolder := range trashFolders {
			if err := c.client.Copy(seqset, trashFolder); err == nil {
				logger.Info("Copied message to %s", trashFolder)
				moved = true
				break
			}
		}

		if !moved {
			logger.Warn("Could not copy to any Trash folder, marking as deleted instead")
			store := imap.FormatFlagsOp(imap.AddFlags, true)
			if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
				return fmt.Errorf("failed to mark message as deleted: %w", err)
			}
		} else {
			store := imap.FormatFlagsOp(imap.AddFlags, true)
			if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
				logger.Warn("Failed to mark original as deleted: %v", err)
			}
		}

		if err := c.client.Expunge(nil); err != nil {
			return fmt.Errorf("failed to expunge: %w", err)
		}

		logger.Info("Successfully moved message with ID: %s to Trash", messageID)
	} else {
		logger.Warn("Message not found in IMAP with ID: %s (likely already deleted)", messageID)
	}

	if err := c.cache.DeleteEmail(messageID); err != nil {
		logger.Warn("Failed to delete from cache: %v", err)
	} else {
		logger.Info("Removed message from cache: %s", messageID)
	}

	return nil
}

// DeleteEmailsByRef deletes a batch of source-scoped messages. Fresh UID refs in
// the same folder use one UID-set provider mutation; stale or incomplete refs
// fall back to the legacy Message-ID search path.
func (c *Client) DeleteEmailsByRef(refs []models.MessageRef) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.deleteEmailsByRefLocked(refs)
	if err != nil && isConnectionError(err) {
		logger.Info("DeleteEmailsByRef: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		err = c.deleteEmailsByRefLocked(refs)
	}
	return err
}

func (c *Client) deleteEmailsByRefLocked(refs []models.MessageRef) error {
	byFolder := make(map[string][]models.MessageRef)
	var folderOrder []string
	for _, ref := range refs {
		if parsed, ok := models.MessageRefFromLocalID(ref.LocalID); ok {
			if ref.Folder == "" {
				ref.Folder = parsed.Folder
			}
			if ref.MessageID == "" {
				ref.MessageID = parsed.MessageID
			}
		}
		ref = ref.WithDefaults()
		if strings.TrimSpace(ref.MessageID) == "" || strings.TrimSpace(ref.Folder) == "" {
			continue
		}
		if _, ok := byFolder[ref.Folder]; !ok {
			folderOrder = append(folderOrder, ref.Folder)
		}
		byFolder[ref.Folder] = append(byFolder[ref.Folder], ref)
	}

	var firstErr error
	for _, folder := range folderOrder {
		group := byFolder[folder]
		mbox, err := c.client.Select(folder, false)
		if err != nil {
			return fmt.Errorf("failed to select folder %s: %w", folder, err)
		}
		fresh, fallback := splitFreshUIDRefs(group, mbox.UidValidity)
		if len(fresh) > 0 {
			seqset := new(imap.SeqSet)
			uids := make([]uint32, 0, len(fresh))
			messageIDs := make([]string, 0, len(fresh))
			for _, ref := range fresh {
				uids = append(uids, ref.UID)
				messageIDs = append(messageIDs, ref.MessageID)
			}
			seqset.AddNum(uids...)
			if err := c.uidMoveToTrashLocked(seqset); err != nil {
				logger.Warn("Batch UID delete failed for %d messages in %s, falling back per message: %v", len(fresh), folder, err)
				fallback = append(fallback, fresh...)
			} else if err := c.cache.DeleteEmailsByMessageIDs(folder, messageIDs); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("failed to delete batch from cache: %w", err)
				}
			}
		}
		for _, ref := range fallback {
			if err := c.deleteEmailLocked(ref.MessageID, folder); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func splitFreshUIDRefs(refs []models.MessageRef, uidValidity uint32) (fresh, fallback []models.MessageRef) {
	for _, ref := range refs {
		ref = ref.WithDefaults()
		if ref.UID != 0 && ref.UIDValidity != 0 && ref.UIDValidity == uidValidity {
			fresh = append(fresh, ref)
			continue
		}
		fallback = append(fallback, ref)
	}
	return fresh, fallback
}

func trashFoldersForDelete(cached string) []string {
	base := []string{"Trash", "Deleted Items", "[Gmail]/Trash", "INBOX.Trash"}
	cached = strings.TrimSpace(cached)
	if cached == "" {
		return base
	}
	out := []string{cached}
	for _, folder := range base {
		if folder != cached {
			out = append(out, folder)
		}
	}
	return out
}

func (c *Client) uidMoveToTrashLocked(seqset *imap.SeqSet) error {
	var lastErr error
	for _, trashFolder := range trashFoldersForDelete(c.trashFolder) {
		if err := c.client.UidMove(seqset, trashFolder); err == nil {
			c.trashFolder = trashFolder
			logger.Info("Moved batch to %s", trashFolder)
			return nil
		} else {
			lastErr = err
			if trashFolder == c.trashFolder {
				c.trashFolder = ""
			}
		}

		copied, err := c.uidCopyStoreExpungeToTrashLocked(seqset, trashFolder)
		if err == nil {
			c.trashFolder = trashFolder
			logger.Info("Copied and expunged batch to %s", trashFolder)
			return nil
		}
		lastErr = err
		if copied {
			return err
		}
	}

	logger.Warn("Could not move batch to any Trash folder, marking as deleted instead: %v", lastErr)
	store := imap.FormatFlagsOp(imap.AddFlags, true)
	if err := c.client.UidStore(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
		return fmt.Errorf("failed to mark batch as deleted: %w", err)
	}
	if err := c.client.Expunge(nil); err != nil {
		return fmt.Errorf("failed to expunge batch: %w", err)
	}
	return nil
}

func (c *Client) uidCopyStoreExpungeToTrashLocked(seqset *imap.SeqSet, trashFolder string) (bool, error) {
	if err := c.client.UidCopy(seqset, trashFolder); err != nil {
		return false, err
	}

	store := imap.FormatFlagsOp(imap.AddFlags, true)
	if err := c.client.UidStore(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
		return true, fmt.Errorf("failed to mark copied batch as deleted: %w", err)
	}
	if err := c.client.Expunge(nil); err != nil {
		return true, fmt.Errorf("failed to expunge copied batch: %w", err)
	}
	return true, nil
}

// MoveEmail copies the message identified by messageID from fromFolder to toFolder,
// then expunges it from fromFolder and removes it from the SQLite cache.
// On connection errors it reconnects once and retries.
func (c *Client) MoveEmail(messageID, fromFolder, toFolder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.moveEmailLocked(messageID, fromFolder, toFolder)
	if err != nil && isConnectionError(err) {
		logger.Info("MoveEmail: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		err = c.moveEmailLocked(messageID, fromFolder, toFolder)
	}
	return err
}

// moveEmailLocked performs the actual move. Must be called with c.mu held.
func (c *Client) moveEmailLocked(messageID, fromFolder, toFolder string) error {
	logger.Info("Moving message %s from %s to %s", messageID, fromFolder, toFolder)

	_, err := c.client.Select(fromFolder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", fromFolder, err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("Message-ID", messageID)
	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search for message: %w", err)
	}

	if len(seqNums) > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddNum(seqNums[0])

		if err := c.client.Copy(seqset, toFolder); err != nil {
			return fmt.Errorf("failed to copy message to %s: %w", toFolder, err)
		}
		logger.Info("Copied message %s to %s", messageID, toFolder)

		store := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
			logger.Warn("Failed to mark original as deleted: %v", err)
		}
		if err := c.client.Expunge(nil); err != nil {
			return fmt.Errorf("failed to expunge: %w", err)
		}
	} else {
		logger.Warn("Message not found in IMAP with ID: %s (likely already moved)", messageID)
	}

	if err := c.cache.DeleteEmail(messageID); err != nil {
		logger.Warn("Failed to delete from cache: %v", err)
	}
	return nil
}

func archiveFoldersForVendor(vendor string) []string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "gmail":
		return []string{"[Gmail]/All Mail", "Archive", "Archives"}
	default:
		return []string{"Archive", "Archives"}
	}
}

// ArchiveEmail moves a specific email to an archive folder by MessageID.
// On connection errors it reconnects once and retries.
func (c *Client) ArchiveEmail(messageID string, folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.archiveEmailLocked(messageID, folder)
	if err != nil && isConnectionError(err) {
		logger.Info("ArchiveEmail: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		err = c.archiveEmailLocked(messageID, folder)
	}
	return err
}

// archiveEmailLocked performs the actual archive. Must be called with c.mu held.
func (c *Client) archiveEmailLocked(messageID string, folder string) error {
	logger.Info("Archiving message: %s", messageID)

	_, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("Message-ID", messageID)
	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search for message: %w", err)
	}

	if len(seqNums) > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddNum(seqNums[0])

		archived := false
		for _, af := range archiveFoldersForVendor(c.cfg.Vendor) {
			if err := c.client.Copy(seqset, af); err == nil {
				logger.Info("Archived message to %s", af)
				archived = true
				break
			}
		}
		if !archived {
			logger.Warn("Could not copy to any archive folder for message %s", messageID)
		}

		store := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
			logger.Warn("Failed to mark original as deleted: %v", err)
		}
		if err := c.client.Expunge(nil); err != nil {
			return fmt.Errorf("failed to expunge: %w", err)
		}
	}

	if err := c.cache.DeleteEmail(messageID); err != nil {
		logger.Warn("Failed to delete from cache: %v", err)
	}
	return nil
}

// ArchiveSenderEmails moves all emails from a sender to an archive folder.
// On connection errors it reconnects once and retries.
func (c *Client) ArchiveSenderEmails(sender, folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.archiveSenderEmailsLocked(sender, folder)
	if err != nil && isConnectionError(err) {
		logger.Info("ArchiveSenderEmails: connection error, reconnecting: %v", err)
		if reconErr := c.Reconnect(); reconErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %v)", reconErr, err)
		}
		err = c.archiveSenderEmailsLocked(sender, folder)
	}
	return err
}

// archiveSenderEmailsLocked performs the actual archive. Must be called with c.mu held.
func (c *Client) archiveSenderEmailsLocked(sender, folder string) error {
	logger.Info("Archiving all emails from '%s' in folder %s", sender, folder)

	mbox, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	var seqNums []uint32
	if mbox.Messages > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddRange(1, mbox.Messages)
		messages := make(chan *imap.Message, 10)
		done := make(chan error, 1)
		go func() {
			done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
		}()
		for msg := range messages {
			if msg.Envelope != nil && len(msg.Envelope.From) > 0 && msg.Envelope.From[0] != nil {
				addr := msg.Envelope.From[0]
				fromAddr := ""
				if addr.MailboxName != "" && addr.HostName != "" {
					fromAddr = addr.MailboxName + "@" + addr.HostName
				}
				if fromAddr == sender {
					seqNums = append(seqNums, msg.SeqNum)
				}
			}
		}
		if err := <-done; err != nil {
			return fmt.Errorf("failed to fetch messages: %w", err)
		}
	}

	if len(seqNums) > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddNum(seqNums...)
		archived := false
		for _, af := range archiveFoldersForVendor(c.cfg.Vendor) {
			if err := c.client.Copy(seqset, af); err == nil {
				logger.Info("Archived %d messages to %s", len(seqNums), af)
				archived = true
				break
			}
		}
		if !archived {
			logger.Warn("Could not copy to any archive folder for sender %s", sender)
		}
		store := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
			return fmt.Errorf("failed to mark messages as deleted: %w", err)
		}
		if err := c.client.Expunge(nil); err != nil {
			return fmt.Errorf("failed to expunge: %w", err)
		}
	}

	if err := c.cache.DeleteSenderEmails(sender, folder); err != nil {
		return fmt.Errorf("failed to delete from cache: %w", err)
	}
	return nil
}

// CleanupCache syncs cache with IMAP server by removing deleted messages
func (c *Client) CleanupCache(folder string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	logger.Info("Starting cache cleanup for folder %s", folder)

	// Select folder
	mbox, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	// Get all cached message IDs
	cachedIDs, err := c.cache.GetCachedIDs(folder)
	if err != nil {
		return fmt.Errorf("failed to get cached IDs: %w", err)
	}

	logger.Info("Found %d messages in cache", len(cachedIDs))

	// Skip cleanup if cache is empty
	if len(cachedIDs) == 0 {
		logger.Info("Cache is empty, skipping cleanup")
		return nil
	}

	// Get all current IMAP message IDs
	currentIDs := make(map[string]bool)

	if mbox.Messages > 0 {
		seqset := new(imap.SeqSet)
		seqset.AddRange(1, mbox.Messages)

		messages := make(chan *imap.Message, 10)
		done := make(chan error, 1)

		go func() {
			done <- c.client.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope}, messages)
		}()

		for msg := range messages {
			messageID := extractMessageID(msg)
			if messageID != "" {
				currentIDs[messageID] = true
			}
		}

		if err := <-done; err != nil {
			return fmt.Errorf("failed to fetch message headers: %w", err)
		}
	}

	logger.Info("Found %d messages on IMAP server", len(currentIDs))

	// Find deleted messages
	deletedCount := 0
	for cachedID := range cachedIDs {
		if !currentIDs[cachedID] {
			if err := c.cache.DeleteEmail(cachedID); err != nil {
				logger.Warn("Error deleting cached email %s: %v", cachedID, err)
				continue
			}
			deletedCount++
		}
	}

	if deletedCount > 0 {
		logger.Info("Removed %d deleted messages from cache", deletedCount)
	} else {
		logger.Info("No deleted messages found in cache")
	}

	return nil
}
