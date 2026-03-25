package cache

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
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

	// Enable WAL mode for safe concurrent access (TUI + MCP server running simultaneously)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		logger.Warn("WAL mode not available: %v", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		logger.Warn("busy_timeout not available: %v", err)
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

	// body_text column for FTS search (added lazily — ignore error if already exists)
	if _, err := c.db.Exec(`ALTER TABLE emails ADD COLUMN body_text TEXT`); err != nil {
		logger.Debug("body_text column might already exist: %v", err)
	}

	// is_read column for read/unread tracking
	if _, err := c.db.Exec(`ALTER TABLE emails ADD COLUMN is_read INTEGER NOT NULL DEFAULT 0`); err != nil {
		logger.Debug("is_read column might already exist: %v", err)
	}

	// list_unsubscribe columns for one-click unsubscribe
	if _, err := c.db.Exec(`ALTER TABLE emails ADD COLUMN list_unsubscribe TEXT`); err != nil {
		logger.Debug("list_unsubscribe column might already exist: %v", err)
	}
	if _, err := c.db.Exec(`ALTER TABLE emails ADD COLUMN list_unsubscribe_post TEXT`); err != nil {
		logger.Debug("list_unsubscribe_post column might already exist: %v", err)
	}

	// FTS5 virtual table for full-text search over sender + subject + body
	ftsQuery := `
		CREATE VIRTUAL TABLE IF NOT EXISTS emails_fts USING fts5(
			sender, subject, body_text,
			content=emails,
			content_rowid=rowid
		)
	`
	if _, err := c.db.Exec(ftsQuery); err != nil {
		logger.Warn("Failed to create FTS5 table (SQLite might lack FTS5 support): %v", err)
	}

	// Triggers to keep FTS index in sync with emails table
	for _, trigSQL := range []string{
		`CREATE TRIGGER IF NOT EXISTS emails_ai AFTER INSERT ON emails BEGIN
			INSERT INTO emails_fts(rowid, sender, subject, body_text)
			VALUES (new.rowid, new.sender, new.subject, new.body_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS emails_ad AFTER DELETE ON emails BEGIN
			INSERT INTO emails_fts(emails_fts, rowid, sender, subject, body_text)
			VALUES ('delete', old.rowid, old.sender, old.subject, old.body_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS emails_au AFTER UPDATE ON emails BEGIN
			INSERT INTO emails_fts(emails_fts, rowid, sender, subject, body_text)
			VALUES ('delete', old.rowid, old.sender, old.subject, old.body_text);
			INSERT INTO emails_fts(rowid, sender, subject, body_text)
			VALUES (new.rowid, new.sender, new.subject, new.body_text);
		END`,
	} {
		if _, err := c.db.Exec(trigSQL); err != nil {
			logger.Debug("FTS trigger creation: %v", err)
		}
	}

	// Embeddings table for semantic search
	embQuery := `
		CREATE TABLE IF NOT EXISTS email_embeddings (
			message_id  TEXT PRIMARY KEY,
			embedding   BLOB NOT NULL,
			hash        TEXT NOT NULL,
			embedded_at DATETIME NOT NULL
		)
	`
	if _, err := c.db.Exec(embQuery); err != nil {
		return err
	}

	// Saved searches table
	savedQuery := `
		CREATE TABLE IF NOT EXISTS saved_searches (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			query      TEXT NOT NULL,
			folder     TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL
		)
	`
	if _, err := c.db.Exec(savedQuery); err != nil {
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
		(message_id, uid, sender, subject, date, size, has_attachments, folder, last_updated, is_read)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	hasAttachments := 0
	if email.HasAttachments {
		hasAttachments = 1
	}
	isRead := 0
	if email.IsRead {
		isRead = 1
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
		isRead,
	)

	if err != nil {
		return fmt.Errorf("failed to cache email %s: %w", email.MessageID, err)
	}

	return nil
}

// MarkRead marks an email as read in the cache
func (c *Cache) MarkRead(messageID string) error {
	_, err := c.db.Exec(`UPDATE emails SET is_read=1 WHERE message_id=?`, messageID)
	return err
}

// MarkUnread marks an email as unread in the cache
func (c *Cache) MarkUnread(messageID string) error {
	_, err := c.db.Exec(`UPDATE emails SET is_read=0 WHERE message_id=?`, messageID)
	return err
}

// GetBodyText returns the cached plain-text body for a message
func (c *Cache) GetBodyText(messageID string) (string, error) {
	var text sql.NullString
	err := c.db.QueryRow(`SELECT body_text FROM emails WHERE message_id=?`, messageID).Scan(&text)
	if err != nil {
		return "", err
	}
	if !text.Valid {
		return "", nil
	}
	return text.String, nil
}

// GetUnreadEmails returns unread emails in a folder, newest first
func (c *Cache) GetUnreadEmails(folder string, limit int) ([]*models.EmailData, error) {
	q := `SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, is_read
	      FROM emails WHERE folder=? AND is_read=0 ORDER BY date DESC`
	args := []interface{}{folder}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := c.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmailRowsWithRead(rows, folder)
}

// SearchByDate returns emails in a folder within an optional date range
func (c *Cache) SearchByDate(folder string, after, before time.Time) ([]*models.EmailData, error) {
	conds := []string{"folder=?"}
	args := []interface{}{folder}
	if !after.IsZero() {
		conds = append(conds, "date >= ?")
		args = append(args, after.Format(time.RFC3339))
	}
	if !before.IsZero() {
		conds = append(conds, "date <= ?")
		args = append(args, before.Format(time.RFC3339))
	}
	where := strings.Join(conds, " AND ")
	rows, err := c.db.Query(`SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, is_read
	                          FROM emails WHERE `+where+` ORDER BY date DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmailRowsWithRead(rows, folder)
}

// SearchBySender returns emails matching a sender prefix/exact, optionally scoped to a folder
func (c *Cache) SearchBySender(sender, folder string) ([]*models.EmailData, error) {
	like := "%" + escapeLike(sender) + "%"
	var rows *sql.Rows
	var err error
	if folder == "" {
		rows, err = c.db.Query(`SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, is_read, folder
		                         FROM emails WHERE sender LIKE ? ESCAPE '\' ORDER BY date DESC LIMIT 200`, like)
	} else {
		rows, err = c.db.Query(`SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, is_read, folder
		                         FROM emails WHERE folder=? AND sender LIKE ? ESCAPE '\' ORDER BY date DESC LIMIT 200`, folder, like)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var emails []*models.EmailData
	for rows.Next() {
		var msgID, sndr, subj, fldr string
		var uid uint32
		var date time.Time
		var size, hasAtt, isRead int
		if err := rows.Scan(&msgID, &uid, &sndr, &subj, &date, &size, &hasAtt, &isRead, &fldr); err != nil {
			return nil, err
		}
		emails = append(emails, &models.EmailData{
			MessageID:      msgID,
			UID:            uid,
			Sender:         sndr,
			Subject:        subj,
			Date:           date,
			Size:           size,
			HasAttachments: hasAtt == 1,
			IsRead:         isRead == 1,
			Folder:         fldr,
		})
	}
	return emails, rows.Err()
}

// UpdateUnsubscribeHeaders stores List-Unsubscribe headers for a message
func (c *Cache) UpdateUnsubscribeHeaders(messageID, listUnsub, listUnsubPost string) error {
	_, err := c.db.Exec(`UPDATE emails SET list_unsubscribe=?, list_unsubscribe_post=? WHERE message_id=?`,
		listUnsub, listUnsubPost, messageID)
	return err
}

// GetUnsubscribeHeaders returns the List-Unsubscribe headers for a message
func (c *Cache) GetUnsubscribeHeaders(messageID string) (listUnsub, listUnsubPost string, err error) {
	var u, p sql.NullString
	err = c.db.QueryRow(`SELECT list_unsubscribe, list_unsubscribe_post FROM emails WHERE message_id=?`, messageID).Scan(&u, &p)
	if err != nil {
		return "", "", err
	}
	if u.Valid {
		listUnsub = u.String
	}
	if p.Valid {
		listUnsubPost = p.String
	}
	return listUnsub, listUnsubPost, nil
}

// scanEmailRowsWithRead scans rows with is_read column (no folder column — folder passed separately)
func scanEmailRowsWithRead(rows *sql.Rows, folder string) ([]*models.EmailData, error) {
	var emails []*models.EmailData
	for rows.Next() {
		var msgID, sender, subject string
		var uid uint32
		var date time.Time
		var size, hasAtt, isRead int
		if err := rows.Scan(&msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &isRead); err != nil {
			return nil, err
		}
		emails = append(emails, &models.EmailData{
			MessageID:      msgID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAtt == 1,
			IsRead:         isRead == 1,
			Folder:         folder,
		})
	}
	return emails, rows.Err()
}

// GetAllEmails retrieves all cached emails for a folder
func (c *Cache) GetAllEmails(folder string, groupByDomain bool) (map[string][]*models.EmailData, error) {
	query := `
		SELECT message_id, COALESCE(uid, 0), sender, subject, date, size, has_attachments, COALESCE(is_read, 0)
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
		var uid uint32
		var date time.Time
		var size, hasAttachments, isRead int

		if err := rows.Scan(&messageID, &uid, &sender, &subject, &date, &size, &hasAttachments, &isRead); err != nil {
			return nil, err
		}

		email := &models.EmailData{
			MessageID:      messageID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAttachments == 1,
			IsRead:         isRead == 1,
			Folder:         folder,
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

// escapeLike escapes LIKE special characters (%, _, \) so they are treated literally.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// SearchEmails returns emails in a folder where sender or subject contains the query (case-insensitive)
func (c *Cache) SearchEmails(folder, query string) ([]*models.EmailData, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := c.db.Query(`
		SELECT message_id, COALESCE(uid, 0), sender, subject, date, size, has_attachments, COALESCE(is_read, 0)
		FROM emails
		WHERE folder = ? AND (sender LIKE ? ESCAPE '\' OR subject LIKE ? ESCAPE '\')
		ORDER BY date DESC LIMIT 100`, folder, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var emails []*models.EmailData
	for rows.Next() {
		var msgID, sender, subject string
		var uid uint32
		var date time.Time
		var size, hasAtt, isRead int
		if err := rows.Scan(&msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &isRead); err != nil {
			return nil, err
		}
		emails = append(emails, &models.EmailData{
			MessageID:      msgID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAtt == 1,
			IsRead:         isRead == 1,
			Folder:         folder,
		})
	}
	return emails, rows.Err()
}

// GetEmailByID returns a single email by message ID
func (c *Cache) GetEmailByID(messageID string) (*models.EmailData, error) {
	row := c.db.QueryRow(`SELECT message_id, COALESCE(uid, 0), sender, subject, date, size, has_attachments, folder, COALESCE(is_read, 0)
	                       FROM emails WHERE message_id = ?`, messageID)
	var msgID, sender, subject, folder string
	var uid uint32
	var date time.Time
	var size, hasAtt, isRead int
	if err := row.Scan(&msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &folder, &isRead); err != nil {
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
		IsRead:         isRead == 1,
		Folder:         folder,
	}, nil
}

// GetEmailsSortedByDate returns all emails for a folder sorted by date descending
func (c *Cache) GetEmailsSortedByDate(folder string) ([]*models.EmailData, error) {
	query := `SELECT message_id, COALESCE(uid, 0), sender, subject, date, size, has_attachments, COALESCE(is_read, 0)
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
		var size, hasAttachments, isRead int
		if err := rows.Scan(&messageID, &uid, &sender, &subject, &date, &size, &hasAttachments, &isRead); err != nil {
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
			IsRead:         isRead == 1,
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

// CacheBodyText stores the plain-text body of an email for FTS indexing
func (c *Cache) CacheBodyText(messageID, bodyText string) error {
	_, err := c.db.Exec(`UPDATE emails SET body_text = ? WHERE message_id = ?`, bodyText, messageID)
	return err
}

// SearchEmailsFTS uses the FTS5 index to search sender, subject, and body
func (c *Cache) SearchEmailsFTS(folder, query string) ([]*models.EmailData, error) {
	var rows *sql.Rows
	var err error
	if folder == "" {
		rows, err = c.db.Query(`
			SELECT e.message_id, COALESCE(e.uid,0), e.sender, e.subject, e.date, e.size, e.has_attachments, e.folder, COALESCE(e.is_read,0)
			FROM emails_fts f
			JOIN emails e ON e.rowid = f.rowid
			WHERE emails_fts MATCH ?
			ORDER BY e.date DESC LIMIT 100`, query)
	} else {
		rows, err = c.db.Query(`
			SELECT e.message_id, COALESCE(e.uid,0), e.sender, e.subject, e.date, e.size, e.has_attachments, e.folder, COALESCE(e.is_read,0)
			FROM emails_fts f
			JOIN emails e ON e.rowid = f.rowid
			WHERE emails_fts MATCH ? AND e.folder = ?
			ORDER BY e.date DESC LIMIT 100`, query, folder)
	}
	if err != nil {
		// FTS5 might not be available; fall back gracefully
		return nil, fmt.Errorf("FTS search failed: %w", err)
	}
	defer rows.Close()
	return scanEmailRows(rows)
}

// SearchEmailsCrossFolder searches across all folders via FTS or LIKE fallback
func (c *Cache) SearchEmailsCrossFolder(query string) ([]*models.EmailData, error) {
	// Try FTS5 first
	emails, err := c.SearchEmailsFTS("", query)
	if err == nil {
		return emails, nil
	}
	// Fallback to LIKE across all folders
	like := "%" + escapeLike(query) + "%"
	rows, err := c.db.Query(`
		SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, folder, COALESCE(is_read,0)
		FROM emails
		WHERE sender LIKE ? ESCAPE '\' OR subject LIKE ? ESCAPE '\'
		ORDER BY date DESC LIMIT 100`, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmailRows(rows)
}

// scanEmailRows is a helper to scan email result rows that include folder and is_read columns
func scanEmailRows(rows *sql.Rows) ([]*models.EmailData, error) {
	var emails []*models.EmailData
	for rows.Next() {
		var msgID, sender, subject, folder string
		var uid uint32
		var date time.Time
		var size, hasAtt, isRead int
		if err := rows.Scan(&msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &folder, &isRead); err != nil {
			return nil, err
		}
		emails = append(emails, &models.EmailData{
			MessageID:      msgID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAtt == 1,
			IsRead:         isRead == 1,
			Folder:         folder,
		})
	}
	return emails, rows.Err()
}

// StoreEmbedding saves a float32 embedding vector for a message
func (c *Cache) StoreEmbedding(messageID string, embedding []float32, hash string) error {
	buf := make([]byte, len(embedding)*4)
	for i, v := range embedding {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO email_embeddings (message_id, embedding, hash, embedded_at) VALUES (?, ?, ?, ?)`,
		messageID, buf, hash, time.Now().Format(time.RFC3339),
	)
	return err
}

// GetAllEmbeddings returns all embeddings for emails in a folder as message_id → vector
func (c *Cache) GetAllEmbeddings(folder string) (map[string][]float32, error) {
	rows, err := c.db.Query(`
		SELECT ee.message_id, ee.embedding
		FROM email_embeddings ee
		JOIN emails e ON e.message_id = ee.message_id
		WHERE e.folder = ?`, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]float32)
	for rows.Next() {
		var msgID string
		var buf []byte
		if err := rows.Scan(&msgID, &buf); err != nil {
			return nil, err
		}
		vec := make([]float32, len(buf)/4)
		for i := range vec {
			vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
		}
		result[msgID] = vec
	}
	return result, rows.Err()
}

// GetUnembeddedIDs returns message IDs in a folder that have body_text but no embedding
func (c *Cache) GetUnembeddedIDs(folder string) ([]string, error) {
	rows, err := c.db.Query(`
		SELECT e.message_id
		FROM emails e
		LEFT JOIN email_embeddings ee ON ee.message_id = e.message_id
		WHERE e.folder = ? AND ee.message_id IS NULL
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

// SearchSemantic finds emails in a folder using cosine similarity against queryVec
func (c *Cache) SearchSemantic(folder string, queryVec []float32, limit int, minScore float64) ([]*models.EmailData, error) {
	embeddings, err := c.GetAllEmbeddings(folder)
	if err != nil {
		return nil, err
	}
	type scored struct {
		messageID string
		score     float64
	}
	var results []scored
	for msgID, vec := range embeddings {
		score := cosineSimilarity(queryVec, vec)
		if score >= minScore {
			results = append(results, scored{msgID, score})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	var emails []*models.EmailData
	for _, r := range results {
		email, err := c.GetEmailByID(r.messageID)
		if err != nil {
			continue
		}
		emails = append(emails, email)
	}
	return emails, nil
}

// cosineSimilarity computes cosine similarity between two float32 vectors
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// GetSavedSearches returns all saved searches
func (c *Cache) GetSavedSearches() ([]*models.SavedSearch, error) {
	rows, err := c.db.Query(`SELECT id, name, query, folder, created_at FROM saved_searches ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var searches []*models.SavedSearch
	for rows.Next() {
		s := &models.SavedSearch{}
		var createdStr string
		if err := rows.Scan(&s.ID, &s.Name, &s.Query, &s.Folder, &createdStr); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		searches = append(searches, s)
	}
	return searches, rows.Err()
}

// SaveSearch persists a named search query
func (c *Cache) SaveSearch(name, query, folder string) error {
	_, err := c.db.Exec(
		`INSERT INTO saved_searches (name, query, folder, created_at) VALUES (?, ?, ?, ?)`,
		name, query, folder, time.Now().Format(time.RFC3339),
	)
	return err
}

// DeleteSavedSearch removes a saved search by ID
func (c *Cache) DeleteSavedSearch(id int) error {
	_, err := c.db.Exec(`DELETE FROM saved_searches WHERE id = ?`, id)
	return err
}