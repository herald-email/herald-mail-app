package imap

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap"
	"mail-processor/internal/logger"
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

// archiveFolders is the ordered list of common archive folder names to try
var archiveFolders = []string{"Archive", "[Gmail]/All Mail", "Archives", "All Mail"}

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
		for _, af := range archiveFolders {
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
		for _, af := range archiveFolders {
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