package imap

import (
	"fmt"

	"github.com/emersion/go-imap"
	"mail-processor/internal/logger"
)

// DeleteSenderEmails deletes all emails from a sender in both IMAP and cache
func (c *Client) DeleteSenderEmails(sender, folder string) error {
	logger.Info("Starting deletion of all emails from '%s' (len=%d) in folder %s", sender, len(sender), folder)

	// Select folder (connection is kept open, no need to reconnect)
	mbox, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	logger.Info("Folder '%s' has %d total messages", folder, mbox.Messages)

	// Get all messages and filter by sender manually
	// This is more reliable than IMAP search which can be inconsistent
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
				// Build sender the same way as in processMessage
				fromAddr := ""
				if addr.MailboxName != "" && addr.HostName != "" {
					fromAddr = addr.MailboxName + "@" + addr.HostName
				}

				// Log first 5 messages for debugging
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

	logger.Info("Found %d messages from '%s' to delete", len(seqNums), sender)

	if len(seqNums) > 0 {
		// Mark messages as deleted
		seqset := new(imap.SeqSet)
		seqset.AddNum(seqNums...)

		store := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
			return fmt.Errorf("failed to mark messages as deleted: %w", err)
		}

		// Expunge to permanently delete
		if err := c.client.Expunge(nil); err != nil {
			return fmt.Errorf("failed to expunge deleted messages: %w", err)
		}

		logger.Info("Successfully deleted %d messages from IMAP", len(seqNums))
	}

	// Delete from cache
	if err := c.cache.DeleteSenderEmails(sender, folder); err != nil {
		return fmt.Errorf("failed to delete from cache: %w", err)
	}

	logger.Info("Successfully completed deletion for %s", sender)
	return nil
}

// DeleteEmail deletes a specific email by MessageID
func (c *Client) DeleteEmail(messageID string, folder string) error {
	logger.Info("Deleting message with ID: %s", messageID)

	// Select folder (connection is kept open, no need to reconnect)
	_, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	// Search by Message-ID header
	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("Message-ID", messageID)

	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search for message: %w", err)
	}

	logger.Info("Found %d messages with Message-ID %s", len(seqNums), messageID)

	if len(seqNums) > 0 {
		// Delete from IMAP
		seqset := new(imap.SeqSet)
		seqset.AddNum(seqNums[0])

		store := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := c.client.Store(seqset, store, []interface{}{imap.DeletedFlag}, nil); err != nil {
			return fmt.Errorf("failed to mark message as deleted: %w", err)
		}

		// Expunge to permanently delete
		if err := c.client.Expunge(nil); err != nil {
			return fmt.Errorf("failed to expunge deleted message: %w", err)
		}

		logger.Info("Successfully deleted message with ID: %s from IMAP", messageID)
	} else {
		logger.Warn("Message not found in IMAP with ID: %s (likely already deleted)", messageID)
	}

	// Delete from cache regardless (message might be already deleted from IMAP)
	if err := c.cache.DeleteEmail(messageID); err != nil {
		logger.Warn("Failed to delete from cache: %v", err)
	} else {
		logger.Info("Removed message from cache: %s", messageID)
	}

	return nil
}

// CleanupCache syncs cache with IMAP server by removing deleted messages
func (c *Client) CleanupCache(folder string) error {
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