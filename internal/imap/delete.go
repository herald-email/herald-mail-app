package imap

import (
	"fmt"
	"time"

	"github.com/emersion/go-imap"
	"mail-processor/internal/logger"
)

// DeleteSenderEmails deletes all emails from a sender in both IMAP and cache
func (c *Client) DeleteSenderEmails(sender, folder string) error {
	logger.Info("Starting deletion of all emails from %s", sender)

	// Select folder
	_, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	// Search for emails from sender
	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("From", sender)

	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search for emails from %s: %w", sender, err)
	}

	logger.Info("Found %d messages from %s to delete", len(seqNums), sender)

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

// DeleteEmail deletes a specific email
func (c *Client) DeleteEmail(sender string, emailDate time.Time, folder string) error {
	logger.Info("Deleting specific email from %s dated %v", sender, emailDate)

	// Select folder
	_, err := c.client.Select(folder, false)
	if err != nil {
		return fmt.Errorf("failed to select folder %s: %w", folder, err)
	}

	// Search for specific email
	criteria := imap.NewSearchCriteria()
	criteria.Header.Add("From", sender)
	
	// Add date criteria if available
	if !emailDate.IsZero() {
		criteria.SentSince = emailDate.Truncate(24 * time.Hour)
		criteria.SentBefore = emailDate.Add(24 * time.Hour).Truncate(24 * time.Hour)
	}

	seqNums, err := c.client.Search(criteria)
	if err != nil {
		return fmt.Errorf("failed to search for email: %w", err)
	}

	if len(seqNums) > 0 {
		// Delete first matching email
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

		logger.Info("Successfully deleted email from %s", sender)
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