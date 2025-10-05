package cache

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"mail-processor/internal/models"
)

// Cache manages the SQLite email cache
type Cache struct {
	db *sql.DB
}

// New creates a new cache instance
func New(dbPath string) (*Cache, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	cache := &Cache{db: db}
	if err := cache.initDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return cache, nil
}

// Close closes the database connection
func (c *Cache) Close() error {
	return c.db.Close()
}

// initDB creates the cache table if it doesn't exist
func (c *Cache) initDB() error {
	query := `
		CREATE TABLE IF NOT EXISTS emails (
			message_id TEXT PRIMARY KEY,
			uid INTEGER,
			sender TEXT,
			subject TEXT,
			date DATETIME,
			size INTEGER,
			has_attachments INTEGER,
			folder TEXT,
			last_updated DATETIME
		)
	`
	_, err := c.db.Exec(query)
	if err != nil {
		return err
	}
	
	// Add UID column to existing table if it doesn't exist
	_, err = c.db.Exec(`ALTER TABLE emails ADD COLUMN uid INTEGER`)
	if err != nil {
		// Column might already exist, that's okay
		logger.Debug("UID column might already exist: %v", err)
	}
	
	return nil
}

// GetCachedIDs returns all cached message IDs for a folder
func (c *Cache) GetCachedIDs(folder string) (map[string]bool, error) {
	query := "SELECT message_id FROM emails WHERE folder = ?"
	rows, err := c.db.Query(query, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}

	return ids, rows.Err()
}

// CacheEmail stores email data in cache
func (c *Cache) CacheEmail(email *models.EmailData) error {
	query := `
		INSERT OR REPLACE INTO emails 
		(message_id, sender, subject, date, size, has_attachments, folder, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	
	hasAttachments := 0
	if email.HasAttachments {
		hasAttachments = 1
	}

	_, err := c.db.Exec(query,
		email.MessageID,
		email.Sender,
		email.Subject,
		email.Date.Format(time.RFC3339),
		email.Size,
		hasAttachments,
		email.Folder,
		time.Now().Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to cache email %s: %w", email.MessageID, err)
	}

	return nil
}

// GetAllEmails retrieves all cached emails for a folder
func (c *Cache) GetAllEmails(folder string, groupByDomain bool) (map[string][]*models.EmailData, error) {
	query := `
		SELECT sender, subject, date, size, has_attachments 
		FROM emails WHERE folder = ?
	`
	rows, err := c.db.Query(query, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	emailsBySender := make(map[string][]*models.EmailData)
	
	for rows.Next() {
		var sender, subject string
		var date time.Time
		var size, hasAttachments int

		if err := rows.Scan(&sender, &subject, &date, &size, &hasAttachments); err != nil {
			return nil, err
		}

		email := &models.EmailData{
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAttachments == 1,
		}

		// Group by domain if requested
		key := sender
		if groupByDomain {
			key = extractDomain(sender)
		}

		emailsBySender[key] = append(emailsBySender[key], email)
	}

	return emailsBySender, rows.Err()
}

// DeleteSenderEmails removes all emails from a specific sender
func (c *Cache) DeleteSenderEmails(sender, folder string) error {
	query := "DELETE FROM emails WHERE sender = ? AND folder = ?"
	_, err := c.db.Exec(query, sender, folder)
	return err
}

// DeleteEmail removes a specific email by message ID
func (c *Cache) DeleteEmail(messageID string) error {
	query := "DELETE FROM emails WHERE message_id = ?"
	_, err := c.db.Exec(query, messageID)
	return err
}

// extractDomain extracts the second-level domain from an email address
func extractDomain(emailAddress string) string {
	// Simple domain extraction - can be enhanced with more sophisticated logic
	if emailAddress == "" {
		return emailAddress
	}

	// Find the @ symbol
	atIndex := -1
	for i, char := range emailAddress {
		if char == '@' {
			atIndex = i
			break
		}
	}

	if atIndex == -1 {
		return emailAddress
	}

	// Extract domain part
	domain := emailAddress[atIndex+1:]
	
	// Split domain into parts
	parts := []string{}
	current := ""
	for _, char := range domain {
		if char == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	// Handle special cases like co.uk, com.au, etc.
	if len(parts) > 2 {
		secondLevel := parts[len(parts)-2]
		if secondLevel == "co" || secondLevel == "com" || secondLevel == "org" || 
		   secondLevel == "gov" || secondLevel == "edu" || secondLevel == "net" {
			if len(parts) >= 3 {
				return parts[len(parts)-3] + "." + parts[len(parts)-2] + "." + parts[len(parts)-1]
			}
		}
	}

	if len(parts) >= 2 {
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}

	return domain
}

// GetNewestCachedDate returns the date of the newest cached email in a folder
func (c *Cache) GetNewestCachedDate(folder string) (time.Time, error) {
	query := `SELECT MAX(date) FROM emails WHERE folder = ?`
	
	var dateStr string
	err := c.db.QueryRow(query, folder).Scan(&dateStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil // No cached emails
		}
		return time.Time{}, err
	}
	
	if dateStr == "" {
		return time.Time{}, nil
	}
	
	// Parse the RFC3339 formatted date
	date, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse cached date: %w", err)
	}
	
	return date, nil
}

// GetCachedUIDs returns all cached UIDs for a folder
func (c *Cache) GetCachedUIDs(folder string) (map[uint32]bool, error) {
	query := `SELECT uid FROM emails WHERE folder = ? AND uid IS NOT NULL`
	
	rows, err := c.db.Query(query, folder)
	if err != nil {
		return nil, fmt.Errorf("failed to query cached UIDs: %w", err)
	}
	defer rows.Close()

	uids := make(map[uint32]bool)
	for rows.Next() {
		var uid uint32
		if err := rows.Scan(&uid); err != nil {
			continue // Skip invalid entries
		}
		uids[uid] = true
	}

	return uids, rows.Err()
}