package cache

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"mail-processor/internal/logger"
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

// initDB creates the cache tables if they don't exist
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
	if _, err := c.db.Exec(query); err != nil {
		return err
	}

	// Add UID column to existing table if it doesn't exist
	if _, err := c.db.Exec(`ALTER TABLE emails ADD COLUMN uid INTEGER`); err != nil {
		logger.Debug("UID column might already exist: %v", err)
	}

	// Classifications table
	classQuery := `
		CREATE TABLE IF NOT EXISTS email_classifications (
			message_id   TEXT PRIMARY KEY,
			category     TEXT NOT NULL DEFAULT '',
			classified_at DATETIME NOT NULL
		)
	`
	if _, err := c.db.Exec(classQuery); err != nil {
		return err
	}

	return nil
}

// SetClassification stores or updates an AI classification for a message
func (c *Cache) SetClassification(messageID, category string) error {
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO email_classifications (message_id, category, classified_at) VALUES (?, ?, ?)`,
		messageID, category, time.Now().Format(time.RFC3339),
	)
	return err
}

// GetClassifications returns a map of message_id → category for a folder
func (c *Cache) GetClassifications(folder string) (map[string]string, error) {
	rows, err := c.db.Query(`
		SELECT ec.message_id, ec.category
		FROM email_classifications ec
		JOIN emails e ON e.message_id = ec.message_id
		WHERE e.folder = ?`, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var id, cat string
		if err := rows.Scan(&id, &cat); err != nil {
			return nil, err
		}
		result[id] = cat
	}
	return result, rows.Err()
}

// GetUnclassifiedIDs returns message IDs in a folder that have no classification yet
func (c *Cache) GetUnclassifiedIDs(folder string) ([]string, error) {
	rows, err := c.db.Query(`
		SELECT e.message_id
		FROM emails e
		LEFT JOIN email_classifications ec ON ec.message_id = e.message_id
		WHERE e.folder = ? AND ec.message_id IS NULL
		ORDER BY e.date DESC`, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
		(message_id, uid, sender, subject, date, size, has_attachments, folder, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	hasAttachments := 0
	if email.HasAttachments {
		hasAttachments = 1
	}

	_, err := c.db.Exec(query,
		email.MessageID,
		email.UID,
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
		SELECT message_id, sender, subject, date, size, has_attachments
		FROM emails WHERE folder = ?
	`
	rows, err := c.db.Query(query, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	emailsBySender := make(map[string][]*models.EmailData)

	for rows.Next() {
		var messageID, sender, subject string
		var date time.Time
		var size, hasAttachments int

		if err := rows.Scan(&messageID, &sender, &subject, &date, &size, &hasAttachments); err != nil {
			return nil, err
		}

		email := &models.EmailData{
			MessageID:      messageID,
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

// DeleteDomainEmails removes all emails from a specific domain (including subdomains).
// e.g. domain "example.com" deletes sender "user@example.com" and "user@mail.example.com".
func (c *Cache) DeleteDomainEmails(domain, folder string) error {
	query := "DELETE FROM emails WHERE (sender LIKE ? OR sender LIKE ?) AND folder = ?"
	exact := "%@" + domain
	subdomain := "%@%." + domain
	_, err := c.db.Exec(query, exact, subdomain, folder)
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

	// MAX() always returns a single row; use NullString to handle the NULL case
	// when the folder has no emails.
	var dateStr sql.NullString
	if err := c.db.QueryRow(query, folder).Scan(&dateStr); err != nil {
		return time.Time{}, err
	}

	if !dateStr.Valid || dateStr.String == "" {
		return time.Time{}, nil
	}

	date, err := time.Parse(time.RFC3339, dateStr.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse cached date: %w", err)
	}

	return date, nil
}

// GetEmailByID returns a single email by message ID
func (c *Cache) GetEmailByID(messageID string) (*models.EmailData, error) {
	row := c.db.QueryRow(`SELECT message_id, uid, sender, subject, date, size, has_attachments, folder
	                       FROM emails WHERE message_id = ?`, messageID)
	var msgID, sender, subject, folder string
	var uid uint32
	var date time.Time
	var size, hasAtt int
	if err := row.Scan(&msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &folder); err != nil {
		return nil, err
	}
	return &models.EmailData{
		MessageID:      msgID,
		UID:            uid,
		Sender:         sender,
		Subject:        subject,
		Date:           date,
		Size:           size,
		HasAttachments: hasAtt == 1,
		Folder:         folder,
	}, nil
}

// GetEmailsSortedByDate returns all emails for a folder sorted by date descending
func (c *Cache) GetEmailsSortedByDate(folder string) ([]*models.EmailData, error) {
	query := `SELECT message_id, uid, sender, subject, date, size, has_attachments
	          FROM emails WHERE folder = ? ORDER BY date DESC`
	rows, err := c.db.Query(query, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []*models.EmailData
	for rows.Next() {
		var messageID, sender, subject string
		var uid uint32
		var date time.Time
		var size, hasAttachments int
		if err := rows.Scan(&messageID, &uid, &sender, &subject, &date, &size, &hasAttachments); err != nil {
			return nil, err
		}
		emails = append(emails, &models.EmailData{
			MessageID:      messageID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAttachments == 1,
			Folder:         folder,
		})
	}
	return emails, rows.Err()
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