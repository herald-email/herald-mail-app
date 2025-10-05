package imap

import (
	"fmt"
	"time"

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

// DeleteEmail deletes a specific email using IMAP search
func (c *Client) DeleteEmail(sender string, emailDate time.Time, folder string) error {
	logger.Info("Deleting specific email from %s dated %v", sender, emailDate)

	// Select folder (connection is kept open, no need to reconnect)
	_, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	// Use IMAP search like Python version: FROM sender ON date
	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("From", sender)

	// Add date criteria - search for messages sent on this specific day
	if !emailDate.IsZero() {
		// SentSince is inclusive (>= date at 00:00)
		dayStart := time.Date(emailDate.Year(), emailDate.Month(), emailDate.Day(), 0, 0, 0, 0, emailDate.Location())
		criteria.SentSince = dayStart
		// SentBefore is exclusive (< date at 00:00 next day)
		criteria.SentBefore = dayStart.AddDate(0, 0, 1)
	}

	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search for email: %w", err)
	}

	logger.Info("Found %d messages matching sender %s on %s", len(seqNums), sender, emailDate.Format("2006-01-02"))

	if len(seqNums) > 0 {
		// Delete first matching message (like Python version)
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

		logger.Info("Successfully deleted 1 message from %s on %s", sender, emailDate.Format("2006-01-02"))
	} else {
		logger.Warn("No messages found matching sender=%s on %s", sender, emailDate.Format("2006-01-02"))
		return fmt.Errorf("message not found")
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