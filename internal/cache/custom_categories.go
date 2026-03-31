package cache

import (
	"database/sql"
	"fmt"
	"time"
)

// SaveCustomCategory inserts or replaces a custom prompt result for a message.
func (c *Cache) SaveCustomCategory(messageID string, promptID int64, result string) error {
	_, err := c.db.Exec(
		`INSERT INTO custom_categories (message_id, prompt_id, result, classified_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(message_id, prompt_id) DO UPDATE SET result=excluded.result, classified_at=excluded.classified_at`,
		messageID,
		promptID,
		result,
		time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("save custom category: %w", err)
	}
	return nil
}

// GetCustomCategory returns the stored result for a message/prompt pair.
// Returns ("", sql.ErrNoRows) if no result exists.
func (c *Cache) GetCustomCategory(messageID string, promptID int64) (string, error) {
	var result string
	err := c.db.QueryRow(
		`SELECT result FROM custom_categories WHERE message_id = ? AND prompt_id = ?`,
		messageID, promptID,
	).Scan(&result)
	if err == sql.ErrNoRows {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("get custom category: %w", err)
	}
	return result, nil
}

// GetCustomCategoriesForEmail returns all prompt results for a message keyed by prompt ID.
func (c *Cache) GetCustomCategoriesForEmail(messageID string) (map[int64]string, error) {
	rows, err := c.db.Query(
		`SELECT prompt_id, result FROM custom_categories WHERE message_id = ?`,
		messageID,
	)
	if err != nil {
		return nil, fmt.Errorf("get custom categories for email: %w", err)
	}
	defer rows.Close()

	results := make(map[int64]string)
	for rows.Next() {
		var promptID int64
		var result string
		if err := rows.Scan(&promptID, &result); err != nil {
			return nil, fmt.Errorf("scan custom category: %w", err)
		}
		results[promptID] = result
	}
	return results, rows.Err()
}
