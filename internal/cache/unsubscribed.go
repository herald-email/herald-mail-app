package cache

import (
	"mail-processor/internal/models"
)

// RecordUnsubscribe inserts or replaces an unsubscribe record for a sender.
func (c *Cache) RecordUnsubscribe(sender, method, unsubscribeURL string) error {
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO unsubscribed_senders (sender, unsubbed_at, method, unsubscribe_url)
		 VALUES (?, datetime('now'), ?, ?)`,
		sender, method, unsubscribeURL,
	)
	return err
}

// IsUnsubscribedSender returns true if the sender has a record in the unsubscribed_senders table.
func (c *Cache) IsUnsubscribedSender(sender string) (bool, error) {
	var count int
	err := c.db.QueryRow(
		`SELECT COUNT(*) FROM unsubscribed_senders WHERE sender = ?`, sender,
	).Scan(&count)
	return count > 0, err
}

// GetUnsubscribedSenders returns all unsubscribed senders ordered by most recent first.
func (c *Cache) GetUnsubscribedSenders() ([]*models.UnsubscribedSender, error) {
	rows, err := c.db.Query(
		`SELECT id, sender, unsubbed_at, method, unsubscribe_url FROM unsubscribed_senders ORDER BY unsubbed_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*models.UnsubscribedSender
	for rows.Next() {
		u := &models.UnsubscribedSender{}
		if err := rows.Scan(&u.ID, &u.Sender, &u.UnsubbedAt, &u.Method, &u.UnsubscribeURL); err != nil {
			continue
		}
		results = append(results, u)
	}
	return results, rows.Err()
}
