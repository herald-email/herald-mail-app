package cache

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

// Cache manages the SQLite email cache
type Cache struct {
	db *sql.DB
}

const (
	cacheMetaEmbeddingModelKey  = "embedding_model"
	legacyEmbeddingModelDefault = "nomic-embed-text"
)

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
	for _, stmt := range []string{
		`ALTER TABLE emails ADD COLUMN source_id TEXT NOT NULL DEFAULT 'default-mail'`,
		`ALTER TABLE emails ADD COLUMN account_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE emails ADD COLUMN local_id TEXT`,
		`ALTER TABLE emails ADD COLUMN uid_validity INTEGER NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_emails_source_folder_date ON emails(source_id, account_id, folder, date DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_emails_local_id ON emails(local_id) WHERE local_id IS NOT NULL`,
	} {
		if _, err := c.db.Exec(stmt); err != nil {
			logger.Debug("source identity email migration might already be applied: %v", err)
		}
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

	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS email_preview_bodies (
			message_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL DEFAULT 'default-mail',
			account_id TEXT NOT NULL DEFAULT 'default',
			local_id TEXT,
			uid_validity INTEGER NOT NULL DEFAULT 0,
			from_addr TEXT,
			to_addr TEXT,
			cc TEXT,
			bcc TEXT,
			subject TEXT,
			text_plain TEXT,
			text_html TEXT,
			is_from_html INTEGER NOT NULL DEFAULT 0,
			list_unsubscribe TEXT,
			list_unsubscribe_post TEXT,
			inline_images_json TEXT NOT NULL DEFAULT '[]',
			attachments_json TEXT NOT NULL DEFAULT '[]',
			cached_at DATETIME NOT NULL,
			cache_policy TEXT NOT NULL DEFAULT 'no_attachments',
			invalidated_at DATETIME
		)
	`); err != nil {
		return err
	}
	for _, stmt := range []string{
		`ALTER TABLE email_preview_bodies ADD COLUMN source_id TEXT NOT NULL DEFAULT 'default-mail'`,
		`ALTER TABLE email_preview_bodies ADD COLUMN account_id TEXT NOT NULL DEFAULT 'default'`,
		`ALTER TABLE email_preview_bodies ADD COLUMN local_id TEXT`,
		`ALTER TABLE email_preview_bodies ADD COLUMN uid_validity INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE email_preview_bodies ADD COLUMN cache_policy TEXT NOT NULL DEFAULT 'no_attachments'`,
		`ALTER TABLE email_preview_bodies ADD COLUMN invalidated_at DATETIME`,
		`CREATE INDEX IF NOT EXISTS idx_preview_bodies_scope ON email_preview_bodies(source_id, account_id, local_id)`,
		`CREATE INDEX IF NOT EXISTS idx_preview_bodies_invalidated ON email_preview_bodies(invalidated_at)`,
	} {
		if _, err := c.db.Exec(stmt); err != nil {
			logger.Debug("preview source lifecycle migration might already be applied: %v", err)
		}
	}

	// is_read column for read/unread tracking
	if _, err := c.db.Exec(`ALTER TABLE emails ADD COLUMN is_read INTEGER NOT NULL DEFAULT 0`); err != nil {
		logger.Debug("is_read column might already exist: %v", err)
	}

	// is_starred column for star/pin tracking
	_, _ = c.db.Exec(`ALTER TABLE emails ADD COLUMN is_starred INTEGER NOT NULL DEFAULT 0`)

	// is_draft column for IMAP/Gmail draft workflow tracking
	_, _ = c.db.Exec(`ALTER TABLE emails ADD COLUMN is_draft INTEGER NOT NULL DEFAULT 0`)
	if err := c.backfillDraftFolderFlags(); err != nil {
		logger.Warn("Failed to backfill draft folder flags: %v", err)
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
	ftsAvailable := true
	if _, err := c.db.Exec(ftsQuery); err != nil {
		logger.Warn("Failed to create FTS5 table (SQLite might lack FTS5 support): %v", err)
		ftsAvailable = false
		// Drop any stale FTS triggers left from a previous run when FTS5 was available.
		// If these triggers exist they will fire on INSERT and fail with "no such table: emails_fts".
		for _, trig := range []string{"emails_ai", "emails_ad", "emails_au"} {
			c.db.Exec("DROP TRIGGER IF EXISTS " + trig)
		}
	}

	// Triggers to keep FTS index in sync with emails table — only when FTS5 is available.
	if ftsAvailable {
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

	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS cache_metadata (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)
	`); err != nil {
		return err
	}

	// folder_sync_state: persists UIDVALIDITY and UIDNEXT per folder for incremental sync
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS folder_sync_state (
			folder      TEXT PRIMARY KEY,
			uidvalidity INTEGER NOT NULL DEFAULT 0,
			uidnext     INTEGER NOT NULL DEFAULT 0,
			updated_at  DATETIME NOT NULL
		)`); err != nil {
		return err
	}
	for _, stmt := range []string{
		`ALTER TABLE folder_sync_state ADD COLUMN source_id TEXT NOT NULL DEFAULT 'default-mail'`,
		`ALTER TABLE folder_sync_state ADD COLUMN account_id TEXT NOT NULL DEFAULT 'default'`,
		`CREATE INDEX IF NOT EXISTS idx_folder_sync_state_source ON folder_sync_state(source_id, account_id, folder)`,
	} {
		if _, err := c.db.Exec(stmt); err != nil {
			logger.Debug("source identity folder sync migration might already be applied: %v", err)
		}
	}

	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS calendar_collections (
			local_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			account_id TEXT NOT NULL,
			calendar_id TEXT NOT NULL,
			display_name TEXT,
			color TEXT,
			sync_token TEXT,
			etag TEXT,
			last_synced DATETIME,
			updated_at DATETIME NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := c.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_calendar_collections_scope
		ON calendar_collections(source_id, account_id, calendar_id)
	`); err != nil {
		return err
	}
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS calendar_events (
			local_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			account_id TEXT NOT NULL,
			calendar_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			instance_id TEXT NOT NULL DEFAULT '',
			provider_uid TEXT,
			etag TEXT,
			revision TEXT,
			title TEXT,
			description TEXT,
			location TEXT,
			starts_at DATETIME,
			ends_at DATETIME,
			all_day INTEGER NOT NULL DEFAULT 0,
			status TEXT,
			updated_at DATETIME,
			raw TEXT,
			cached_at DATETIME NOT NULL,
			invalidated_at DATETIME
		)
	`); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_calendar_events_scope_time ON calendar_events(source_id, account_id, calendar_id, starts_at)`,
		`CREATE INDEX IF NOT EXISTS idx_calendar_events_invalidated ON calendar_events(invalidated_at)`,
	} {
		if _, err := c.db.Exec(stmt); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`ALTER TABLE calendar_events ADD COLUMN timezone TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE calendar_events ADD COLUMN organizer TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE calendar_events ADD COLUMN organizer_email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE calendar_events ADD COLUMN attendees_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE calendar_events ADD COLUMN recurrence_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE calendar_events ADD COLUMN recurrence_summary TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE calendar_events ADD COLUMN attachments_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE calendar_events ADD COLUMN alternate_timezones_json TEXT NOT NULL DEFAULT '[]'`,
	} {
		if _, err := c.db.Exec(stmt); err != nil {
			logger.Debug("calendar event migration might already be applied: %v", err)
		}
	}

	// custom_prompts: stores reusable AI prompt templates
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS custom_prompts (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			name          TEXT    NOT NULL,
			system_text   TEXT    NOT NULL DEFAULT '',
			user_template TEXT    NOT NULL,
			output_var    TEXT    NOT NULL DEFAULT 'result',
			created_at    DATETIME NOT NULL
		)`); err != nil {
		return err
	}

	// email_rules: stores automation rules triggered on email events
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS email_rules (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			name             TEXT    NOT NULL DEFAULT '' CHECK(name != ''),
			enabled          INTEGER NOT NULL DEFAULT 1,
			priority         INTEGER NOT NULL DEFAULT 0,
			trigger_type     TEXT    NOT NULL,
			trigger_value    TEXT    NOT NULL,
			custom_prompt_id INTEGER REFERENCES custom_prompts(id) ON DELETE SET NULL,
			actions_json     TEXT    NOT NULL DEFAULT '[]',
			created_at       DATETIME NOT NULL,
			last_triggered   DATETIME
		)`); err != nil {
		return err
	}

	// rule_action_log: audit trail for rule executions
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS rule_action_log (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_id      INTEGER NOT NULL REFERENCES email_rules(id) ON DELETE CASCADE,
			message_id   TEXT    NOT NULL,
			action_type  TEXT    NOT NULL,
			status       TEXT    NOT NULL,
			detail       TEXT,
			executed_at  DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_rule_action_log_rule_id ON rule_action_log(rule_id);
	`); err != nil {
		return err
	}

	// email_embedding_chunks: chunked embeddings for semantic search
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS email_embedding_chunks (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id   TEXT NOT NULL,
			chunk_index  INTEGER NOT NULL DEFAULT 0,
			embedding    BLOB NOT NULL,
			content_hash TEXT NOT NULL,
			embedded_at  DATETIME NOT NULL,
			UNIQUE(message_id, chunk_index)
		)
	`); err != nil {
		return err
	}

	// contacts: enriched contact book built from email headers
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS contacts (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			email        TEXT UNIQUE NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			company      TEXT NOT NULL DEFAULT '',
			topics       TEXT NOT NULL DEFAULT '[]',
			notes        TEXT NOT NULL DEFAULT '',
			first_seen   DATETIME NOT NULL,
			last_seen    DATETIME NOT NULL,
			email_count  INTEGER NOT NULL DEFAULT 0,
			sent_count   INTEGER NOT NULL DEFAULT 0,
			carddav_uid  TEXT,
			enriched_at  DATETIME,
			embedding    BLOB
		)
	`); err != nil {
		return err
	}

	if _, err := c.db.Exec(`CREATE INDEX IF NOT EXISTS idx_contacts_last_seen ON contacts(last_seen DESC)`); err != nil {
		return err
	}

	// custom_categories: stores per-message results from running custom prompts
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS custom_categories (
			message_id    TEXT NOT NULL,
			prompt_id     INTEGER NOT NULL,
			result        TEXT NOT NULL,
			classified_at DATETIME NOT NULL,
			PRIMARY KEY (message_id, prompt_id)
		)
	`); err != nil {
		return err
	}

	// cleanup_rules: automated inbox cleanup rules
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS cleanup_rules (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			name            TEXT NOT NULL,
			match_type      TEXT NOT NULL CHECK(match_type IN ('sender','domain')),
			match_value     TEXT NOT NULL,
			action          TEXT NOT NULL CHECK(action IN ('delete','archive')),
			older_than_days INTEGER NOT NULL DEFAULT 30,
			enabled         INTEGER NOT NULL DEFAULT 1,
			last_run        DATETIME,
			created_at      DATETIME NOT NULL
		)
	`); err != nil {
		return err
	}

	// unsubscribed_senders: tracks senders the user has unsubscribed from
	if _, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS unsubscribed_senders (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			sender          TEXT NOT NULL UNIQUE,
			unsubbed_at     DATETIME NOT NULL,
			method          TEXT NOT NULL DEFAULT '',
			unsubscribe_url TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return err
	}

	return nil
}

func (c *Cache) backfillDraftFolderFlags() error {
	_, err := c.db.Exec(`
		UPDATE emails
		SET is_draft = 1
		WHERE COALESCE(is_draft, 0) = 0
		  AND (
			lower(trim(folder)) IN ('drafts', '[gmail]/drafts', 'inbox.drafts', 'inbox/drafts')
			OR lower(trim(folder)) LIKE '%/drafts'
			OR lower(trim(folder)) LIKE '%.drafts'
		  )
	`)
	return err
}

func (c *Cache) getMetadata(key string) (string, bool, error) {
	var value string
	err := c.db.QueryRow(`SELECT value FROM cache_metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (c *Cache) setMetadataTx(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(
		`INSERT INTO cache_metadata (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().Format(time.RFC3339),
	)
	return err
}

func (c *Cache) setMetadata(key, value string) error {
	_, err := c.db.Exec(
		`INSERT INTO cache_metadata (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().Format(time.RFC3339),
	)
	return err
}

func (c *Cache) hasAnyEmbeddings() (bool, error) {
	queries := []string{
		`SELECT EXISTS(SELECT 1 FROM email_embeddings LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM email_embedding_chunks LIMIT 1)`,
		`SELECT EXISTS(SELECT 1 FROM contacts WHERE embedding IS NOT NULL LIMIT 1)`,
	}
	for _, query := range queries {
		var exists int
		if err := c.db.QueryRow(query).Scan(&exists); err != nil {
			return false, err
		}
		if exists == 1 {
			return true, nil
		}
	}
	return false, nil
}

func (c *Cache) invalidateEmbeddingsTx(tx *sql.Tx, model string) error {
	for _, query := range []string{
		`DELETE FROM email_embeddings`,
		`DELETE FROM email_embedding_chunks`,
		`UPDATE contacts SET embedding = NULL`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return err
		}
	}
	return c.setMetadataTx(tx, cacheMetaEmbeddingModelKey, model)
}

// EnsureEmbeddingModel records the active embedding model in cache metadata.
// When the configured model changes, it invalidates cached email and contact
// embeddings so semantic features can rebuild on a consistent vector space.
func (c *Cache) EnsureEmbeddingModel(model string) (bool, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return false, nil
	}

	current, found, err := c.getMetadata(cacheMetaEmbeddingModelKey)
	if err != nil {
		return false, err
	}
	if found && current == model {
		return false, nil
	}

	hasEmbeddings, err := c.hasAnyEmbeddings()
	if err != nil {
		return false, err
	}
	if !found {
		if !hasEmbeddings || model == legacyEmbeddingModelDefault {
			return false, c.setMetadata(cacheMetaEmbeddingModelKey, model)
		}
	} else if !hasEmbeddings {
		return false, c.setMetadata(cacheMetaEmbeddingModelKey, model)
	}

	tx, err := c.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck

	if err := c.invalidateEmbeddingsTx(tx, model); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// GetFolderSyncState returns the stored UIDVALIDITY and UIDNEXT for a folder.
// Returns 0, 0, nil when no record exists yet.
func (c *Cache) GetFolderSyncState(folder string) (uidValidity, uidNext uint32, err error) {
	var v, n int64
	err = c.db.QueryRow(
		`SELECT uidvalidity, uidnext FROM folder_sync_state WHERE folder = ?`, folder,
	).Scan(&v, &n)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return uint32(v), uint32(n), nil
}

// SetFolderSyncState persists UIDVALIDITY and UIDNEXT for a folder.
func (c *Cache) SetFolderSyncState(folder string, uidValidity, uidNext uint32) error {
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO folder_sync_state (folder, uidvalidity, uidnext, updated_at) VALUES (?, ?, ?, ?)`,
		folder, uidValidity, uidNext, time.Now().Format(time.RFC3339),
	)
	return err
}

// CountEmailsInFolder returns the number of cached email rows for a folder.
func (c *Cache) CountEmailsInFolder(folder string) (int, error) {
	var count int
	err := c.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE folder = ?`, folder).Scan(&count)
	return count, err
}

// GetCachedUIDsAndMessageIDs returns (message_id, uid) pairs for all rows in a folder.
// Rows with NULL uid are returned with UID=0.
func (c *Cache) GetCachedUIDsAndMessageIDs(folder string) ([]struct {
	MessageID string
	UID       uint32
}, error) {
	rows, err := c.db.Query(
		`SELECT message_id, COALESCE(uid, 0) FROM emails WHERE folder = ?`, folder,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []struct {
		MessageID string
		UID       uint32
	}
	for rows.Next() {
		var msgID string
		var uid int64
		if err := rows.Scan(&msgID, &uid); err != nil {
			return nil, err
		}
		result = append(result, struct {
			MessageID string
			UID       uint32
		}{msgID, uint32(uid)})
	}
	return result, rows.Err()
}

// ClearFolder removes all cached emails for a folder.
// Called when UIDVALIDITY changes and the cache must be rebuilt from scratch.
func (c *Cache) ClearFolder(folder string) error {
	_, err := c.db.Exec(`DELETE FROM emails WHERE folder = ?`, folder)
	return err
}

// DeleteEmailsByUIDs removes cache rows whose UID is in the given slice for a folder.
// No-op if uids is empty.
func (c *Cache) DeleteEmailsByUIDs(folder string, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}
	// Build placeholder list: ?,?,?...
	placeholders := make([]string, len(uids))
	args := make([]interface{}, 0, len(uids)+1)
	args = append(args, folder)
	for i, uid := range uids {
		placeholders[i] = "?"
		args = append(args, uid)
	}
	query := `DELETE FROM emails WHERE folder = ? AND uid IN (` + strings.Join(placeholders, ",") + `)`
	_, err := c.db.Exec(query, args...)
	return err
}

// DeleteEmailsByMessageIDs removes cache rows whose message IDs are in the given slice for a folder.
// No-op if messageIDs is empty.
func (c *Cache) DeleteEmailsByMessageIDs(folder string, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(messageIDs))
	args := make([]interface{}, 0, len(messageIDs)+1)
	args = append(args, folder)
	for i, messageID := range messageIDs {
		placeholders[i] = "?"
		args = append(args, messageID)
	}
	query := `DELETE FROM emails WHERE folder = ? AND message_id IN (` + strings.Join(placeholders, ",") + `)`
	_, err := c.db.Exec(query, args...)
	return err
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

func normalizeEmailScope(email *models.EmailData) *models.EmailData {
	if email == nil {
		return nil
	}
	ref := email.MessageRef()
	email.SourceID = ref.SourceID
	email.AccountID = ref.AccountID
	email.LocalID = ref.LocalID
	email.UIDValidity = ref.UIDValidity
	return email
}

// CacheEmail stores email data in cache
func (c *Cache) CacheEmail(email *models.EmailData) error {
	ref := email.MessageRef()
	query := `
		INSERT OR REPLACE INTO emails
		(message_id, uid, uid_validity, source_id, account_id, local_id, sender, subject, date, size, has_attachments, folder, last_updated, is_read, is_starred, is_draft)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	hasAttachments := 0
	if email.HasAttachments {
		hasAttachments = 1
	}
	isRead := 0
	if email.IsRead {
		isRead = 1
	}
	isStarred := 0
	if email.IsStarred {
		isStarred = 1
	}
	isDraft := 0
	if email.IsDraft {
		isDraft = 1
	}

	_, err := c.db.Exec(query,
		email.MessageID,
		email.UID,
		ref.UIDValidity,
		string(ref.SourceID),
		string(ref.AccountID),
		ref.LocalID,
		email.Sender,
		email.Subject,
		email.Date.Format(time.RFC3339),
		email.Size,
		hasAttachments,
		email.Folder,
		time.Now().Format(time.RFC3339),
		isRead,
		isStarred,
		isDraft,
	)

	if err != nil {
		return fmt.Errorf("failed to cache email %s: %w", email.MessageID, err)
	}

	email.SourceID = ref.SourceID
	email.AccountID = ref.AccountID
	email.LocalID = ref.LocalID
	email.UIDValidity = ref.UIDValidity
	return nil
}

// BatchCacheEmails inserts or updates multiple emails in a single SQLite transaction.
// Use this instead of repeated CacheEmail calls to reduce disk flush overhead.
func (c *Cache) BatchCacheEmails(emails []*models.EmailData) error {
	if len(emails) == 0 {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("BatchCacheEmails: begin tx: %w", err)
	}
	query := `
		INSERT OR REPLACE INTO emails
		(message_id, uid, uid_validity, source_id, account_id, local_id, sender, subject, date, size, has_attachments, folder, last_updated, is_read, is_starred, is_draft)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	stmt, err := tx.Prepare(query)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("BatchCacheEmails: prepare: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Format(time.RFC3339)
	for _, email := range emails {
		ref := email.MessageRef()
		hasAttachments := 0
		if email.HasAttachments {
			hasAttachments = 1
		}
		isRead := 0
		if email.IsRead {
			isRead = 1
		}
		isStarred := 0
		if email.IsStarred {
			isStarred = 1
		}
		isDraft := 0
		if email.IsDraft {
			isDraft = 1
		}
		if _, err = stmt.Exec(
			email.MessageID, email.UID, ref.UIDValidity, string(ref.SourceID), string(ref.AccountID), ref.LocalID, email.Sender, email.Subject,
			email.Date.Format(time.RFC3339), email.Size, hasAttachments,
			email.Folder, now, isRead, isStarred, isDraft,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("BatchCacheEmails: exec for %s: %w", email.MessageID, err)
		}
		email.SourceID = ref.SourceID
		email.AccountID = ref.AccountID
		email.LocalID = ref.LocalID
		email.UIDValidity = ref.UIDValidity
	}
	return tx.Commit()
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

// UpdateStarred sets or clears the starred flag for an email in the cache.
func (c *Cache) UpdateStarred(messageID string, starred bool) error {
	val := 0
	if starred {
		val = 1
	}
	_, err := c.db.Exec(`UPDATE emails SET is_starred=? WHERE message_id=?`, val, messageID)
	return err
}

// EmailFlagState is the authoritative IMAP flag state for one cached UID.
type EmailFlagState struct {
	UID       uint32
	IsRead    bool
	IsStarred bool
	IsDraft   bool
}

// BatchUpdateEmailFlagsByUID refreshes cached message flags from IMAP during
// reconcile without refetching envelopes.
func (c *Cache) BatchUpdateEmailFlagsByUID(folder string, states []EmailFlagState) error {
	if len(states) == 0 {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("BatchUpdateEmailFlagsByUID: begin tx: %w", err)
	}
	stmt, err := tx.Prepare(`UPDATE emails SET is_read=?, is_starred=?, is_draft=? WHERE folder=? AND uid=?`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("BatchUpdateEmailFlagsByUID: prepare: %w", err)
	}
	defer stmt.Close()

	for _, state := range states {
		if _, err := stmt.Exec(boolToInt(state.IsRead), boolToInt(state.IsStarred), boolToInt(state.IsDraft), folder, state.UID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("BatchUpdateEmailFlagsByUID: uid %d: %w", state.UID, err)
		}
	}
	return tx.Commit()
}

// GetBodyText returns the cached plain-text body for a message
func (c *Cache) GetBodyText(messageID string) (string, error) {
	var text sql.NullString
	err := c.db.QueryRow(`SELECT body_text FROM emails WHERE message_id=?`, messageID).Scan(&text)
	if err != nil {
		return "", err
	}
	if text.Valid && strings.TrimSpace(text.String) != "" {
		return text.String, nil
	}
	preview, err := c.GetPreviewBody(messageID)
	if err != nil || preview == nil {
		return "", err
	}
	return preview.TextPlain, nil
}

// CachePreviewBody stores the preview body using the configured offline-cache
// policy. Lightweight caches keep text, headers, and attachment metadata only.
func (c *Cache) CachePreviewBody(messageID string, body *models.EmailBody, policy string) error {
	ref := models.MessageRef{MessageID: messageID}
	return c.CachePreviewBodyByRef(ref, body, policy)
}

// CachePreviewBodyByRef stores a scoped preview body using the configured
// offline-cache policy. It keeps the legacy message_id primary key while adding
// source/account/local identity for cache-first callers.
func (c *Cache) CachePreviewBodyByRef(ref models.MessageRef, body *models.EmailBody, policy string) error {
	if strings.TrimSpace(ref.MessageID) == "" && body != nil && strings.TrimSpace(body.MessageID) != "" {
		ref.MessageID = body.MessageID
	}
	if strings.TrimSpace(ref.MessageID) == "" || body == nil {
		return nil
	}
	ref = ref.WithDefaults()
	policy = normalizePreviewStoragePolicy(policy)
	cached := previewBodyForPolicy(body, policy)
	inlineJSON, err := json.Marshal(cached.InlineImages)
	if err != nil {
		return fmt.Errorf("marshal inline preview images: %w", err)
	}
	attachmentsJSON, err := json.Marshal(cached.Attachments)
	if err != nil {
		return fmt.Errorf("marshal preview attachments: %w", err)
	}
	_, err = c.db.Exec(`
		INSERT OR REPLACE INTO email_preview_bodies
		(message_id, source_id, account_id, local_id, uid_validity,
		 from_addr, to_addr, cc, bcc, subject, text_plain, text_html, is_from_html,
		 list_unsubscribe, list_unsubscribe_post, inline_images_json, attachments_json, cached_at,
		 cache_policy, invalidated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`,
		ref.MessageID,
		string(ref.SourceID),
		string(ref.AccountID),
		ref.LocalID,
		ref.UIDValidity,
		cached.From,
		cached.To,
		cached.CC,
		cached.BCC,
		cached.Subject,
		cached.TextPlain,
		cached.TextHTML,
		boolToInt(cached.IsFromHTML),
		cached.ListUnsubscribe,
		cached.ListUnsubscribePost,
		string(inlineJSON),
		string(attachmentsJSON),
		time.Now().Format(time.RFC3339),
		policy,
	)
	return err
}

// PutMessageBodyByRef stores a scoped full-body cache row. Only rows persisted
// with preserve_all are eligible for full-body reads.
func (c *Cache) PutMessageBodyByRef(ref models.MessageRef, body *models.EmailBody, policy string) error {
	if err := c.CachePreviewBodyByRef(ref, body, policy); err != nil {
		return err
	}
	if body != nil && strings.TrimSpace(ref.MessageID) != "" && strings.TrimSpace(body.TextPlain) != "" {
		if err := c.CacheBodyText(ref.MessageID, body.TextPlain); err != nil {
			return err
		}
	}
	return nil
}

// GetPreviewBody returns a cached preview body, or nil when the message has not
// been cached for preview yet.
func (c *Cache) GetPreviewBody(messageID string) (*models.EmailBody, error) {
	return c.GetPreviewBodyByRef(models.MessageRef{MessageID: messageID})
}

// GetPreviewBodyByRef returns a scoped cached preview body, or nil when the
// preview is absent or has been invalidated.
func (c *Cache) GetPreviewBodyByRef(ref models.MessageRef) (*models.EmailBody, error) {
	body, _, err := c.getPreviewBodyByRef(ref)
	return body, err
}

// GetMessageBodyByRef returns a scoped cached full body only when the row was
// stored under preserve_all. Rows cached under lighter preview policies remain
// previews and are treated as a full-body miss.
func (c *Cache) GetMessageBodyByRef(ref models.MessageRef) (*models.EmailBody, error) {
	body, policy, err := c.getPreviewBodyByRef(ref)
	if err != nil || body == nil {
		return body, err
	}
	if normalizePreviewStoragePolicy(policy) != "preserve_all" {
		return nil, nil
	}
	return body, nil
}

func (c *Cache) getPreviewBodyByRef(ref models.MessageRef) (*models.EmailBody, string, error) {
	ref = ref.WithDefaults()
	if strings.TrimSpace(ref.MessageID) == "" && strings.TrimSpace(ref.LocalID) == "" {
		return nil, "", nil
	}
	var body models.EmailBody
	var isFromHTML int
	var messageID, inlineJSON, attachmentsJSON, policy string
	err := c.db.QueryRow(`
		SELECT message_id, from_addr, to_addr, cc, bcc, subject, text_plain, text_html, is_from_html,
		       list_unsubscribe, list_unsubscribe_post, inline_images_json, attachments_json, cache_policy
		FROM email_preview_bodies
		WHERE invalidated_at IS NULL
		  AND (
		    (local_id = ? AND local_id <> '')
		    OR (? <> '' AND message_id = ?)
		  )
		ORDER BY CASE WHEN local_id = ? AND local_id <> '' THEN 0 ELSE 1 END
		LIMIT 1
	`, ref.LocalID, ref.MessageID, ref.MessageID, ref.LocalID).Scan(
		&messageID,
		&body.From,
		&body.To,
		&body.CC,
		&body.BCC,
		&body.Subject,
		&body.TextPlain,
		&body.TextHTML,
		&isFromHTML,
		&body.ListUnsubscribe,
		&body.ListUnsubscribePost,
		&inlineJSON,
		&attachmentsJSON,
		&policy,
	)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	body.MessageID = messageID
	body.IsFromHTML = isFromHTML != 0
	if strings.TrimSpace(inlineJSON) != "" {
		if err := json.Unmarshal([]byte(inlineJSON), &body.InlineImages); err != nil {
			return nil, "", fmt.Errorf("unmarshal inline preview images: %w", err)
		}
	}
	if strings.TrimSpace(attachmentsJSON) != "" {
		if err := json.Unmarshal([]byte(attachmentsJSON), &body.Attachments); err != nil {
			return nil, "", fmt.Errorf("unmarshal preview attachments: %w", err)
		}
	}
	return &body, policy, nil
}

// InvalidatePreviewByRef marks a scoped preview/full-body cache row stale
// without deleting it, so background refreshes can make future reads fast again.
func (c *Cache) InvalidatePreviewByRef(ref models.MessageRef) error {
	ref = ref.WithDefaults()
	now := time.Now().Format(time.RFC3339)
	if strings.TrimSpace(ref.LocalID) != "" {
		if _, err := c.db.Exec(`UPDATE email_preview_bodies SET invalidated_at=? WHERE local_id=?`, now, ref.LocalID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(ref.MessageID) != "" {
		if _, err := c.db.Exec(`UPDATE email_preview_bodies SET invalidated_at=? WHERE message_id=?`, now, ref.MessageID); err != nil {
			return err
		}
	}
	return nil
}

// InvalidateMessageByRef marks the full message body stale and clears indexed
// body text so future foreground reads must refresh or replay a valid cache row.
func (c *Cache) InvalidateMessageByRef(ref models.MessageRef) error {
	ref = ref.WithDefaults()
	if err := c.InvalidatePreviewByRef(ref); err != nil {
		return err
	}
	if strings.TrimSpace(ref.LocalID) != "" {
		if _, err := c.db.Exec(`UPDATE emails SET body_text='' WHERE local_id=?`, ref.LocalID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(ref.MessageID) != "" {
		if _, err := c.db.Exec(`UPDATE emails SET body_text='' WHERE message_id=?`, ref.MessageID); err != nil {
			return err
		}
	}
	return nil
}

// EstimatePreviewCacheStorageForPolicy summarizes binary preview-cache bytes
// that would be removed by applying the given offline-cache policy.
func (c *Cache) EstimatePreviewCacheStorageForPolicy(policy string) (models.PreviewCacheStorageEstimate, error) {
	policy = normalizePreviewStoragePolicy(policy)
	estimate := models.PreviewCacheStorageEstimate{Policy: policy}

	rows, err := c.db.Query(`
		SELECT message_id, inline_images_json, attachments_json
		FROM email_preview_bodies
	`)
	if err != nil {
		return estimate, err
	}
	defer rows.Close()

	for rows.Next() {
		var messageID, inlineJSON, attachmentsJSON string
		if err := rows.Scan(&messageID, &inlineJSON, &attachmentsJSON); err != nil {
			return estimate, err
		}
		estimate.RowsScanned++

		var images []models.InlineImage
		if strings.TrimSpace(inlineJSON) != "" {
			if err := json.Unmarshal([]byte(inlineJSON), &images); err != nil {
				return estimate, fmt.Errorf("unmarshal inline preview images for %s: %w", messageID, err)
			}
		}
		var attachments []models.Attachment
		if strings.TrimSpace(attachmentsJSON) != "" {
			if err := json.Unmarshal([]byte(attachmentsJSON), &attachments); err != nil {
				return estimate, fmt.Errorf("unmarshal preview attachments for %s: %w", messageID, err)
			}
		}

		inlineBytes := inlineImageBytes(images)
		attachmentBytes := attachmentPayloadBytes(attachments)
		reclaimableInline := reclaimableInlineImageBytes(images, policy)
		reclaimableAttachments := reclaimableAttachmentBytes(attachments, policy)

		estimate.InlineImageBytes += inlineBytes
		estimate.AttachmentBytes += attachmentBytes
		estimate.CurrentBytes += inlineBytes + attachmentBytes
		estimate.ReclaimableInlineImageBytes += reclaimableInline
		estimate.ReclaimableAttachmentBytes += reclaimableAttachments
		if reclaimableInline+reclaimableAttachments > 0 {
			estimate.RowsReclaimable++
		}
	}
	if err := rows.Err(); err != nil {
		return estimate, err
	}

	estimate.ReclaimableBytes = estimate.ReclaimableInlineImageBytes + estimate.ReclaimableAttachmentBytes
	estimate.EstimatedAfterBytes = estimate.CurrentBytes - estimate.ReclaimableBytes
	if estimate.EstimatedAfterBytes < 0 {
		estimate.EstimatedAfterBytes = 0
	}
	return estimate, nil
}

// ReclaimPreviewCacheStorageForPolicy applies the selected policy to cached
// preview rows and then runs SQLite VACUUM as best-effort file compaction.
func (c *Cache) ReclaimPreviewCacheStorageForPolicy(policy string) (models.PreviewCacheReclaimResult, error) {
	policy = normalizePreviewStoragePolicy(policy)
	estimate, err := c.EstimatePreviewCacheStorageForPolicy(policy)
	if err != nil {
		return models.PreviewCacheReclaimResult{}, err
	}
	pruneResult, err := c.PrunePreviewBodiesForPolicy(policy)
	if err != nil {
		return models.PreviewCacheReclaimResult{Estimate: estimate}, err
	}
	result := models.PreviewCacheReclaimResult{
		Estimate:    estimate,
		PruneResult: pruneResult,
	}
	if _, err := c.db.Exec(`VACUUM`); err != nil {
		result.CompactionError = err.Error()
		return result, nil
	}
	result.Compacted = true
	return result, nil
}

// PrunePreviewBodiesForPolicy rewrites cached preview rows so they match the
// target offline-cache policy. It never removes text, headers, or attachment
// metadata; only binary payloads disallowed by the selected policy are pruned.
func (c *Cache) PrunePreviewBodiesForPolicy(policy string) (models.PreviewCachePruneResult, error) {
	policy = normalizePreviewStoragePolicy(policy)
	var result models.PreviewCachePruneResult
	if policy == "preserve_all" {
		return result, nil
	}

	rows, err := c.db.Query(`
		SELECT message_id, inline_images_json, attachments_json
		FROM email_preview_bodies
	`)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	type previewPruneUpdate struct {
		messageID       string
		inlineJSON      string
		attachmentsJSON string
	}
	var updates []previewPruneUpdate
	for rows.Next() {
		var messageID, inlineJSON, attachmentsJSON string
		if err := rows.Scan(&messageID, &inlineJSON, &attachmentsJSON); err != nil {
			return result, err
		}
		result.RowsScanned++

		var images []models.InlineImage
		if strings.TrimSpace(inlineJSON) != "" {
			if err := json.Unmarshal([]byte(inlineJSON), &images); err != nil {
				return result, fmt.Errorf("unmarshal inline preview images for %s: %w", messageID, err)
			}
		}
		var attachments []models.Attachment
		if strings.TrimSpace(attachmentsJSON) != "" {
			if err := json.Unmarshal([]byte(attachmentsJSON), &attachments); err != nil {
				return result, fmt.Errorf("unmarshal preview attachments for %s: %w", messageID, err)
			}
		}

		prunedImages, inlineBytesRemoved, imagesChanged := pruneInlineImagesForPolicy(images, policy)
		prunedAttachments, attachmentBytesRemoved, attachmentsChanged := pruneAttachmentsForPolicy(attachments, policy)
		if !imagesChanged && !attachmentsChanged {
			continue
		}

		nextInlineJSON, err := json.Marshal(prunedImages)
		if err != nil {
			return result, fmt.Errorf("marshal pruned inline preview images for %s: %w", messageID, err)
		}
		nextAttachmentsJSON, err := json.Marshal(prunedAttachments)
		if err != nil {
			return result, fmt.Errorf("marshal pruned preview attachments for %s: %w", messageID, err)
		}
		updates = append(updates, previewPruneUpdate{
			messageID:       messageID,
			inlineJSON:      string(nextInlineJSON),
			attachmentsJSON: string(nextAttachmentsJSON),
		})
		result.RowsChanged++
		result.InlineImageBytesRemoved += inlineBytesRemoved
		result.AttachmentBytesRemoved += attachmentBytesRemoved
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(updates) == 0 {
		return result, nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return result, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.Prepare(`
		UPDATE email_preview_bodies
		SET inline_images_json=?, attachments_json=?, cached_at=?, cache_policy=?
		WHERE message_id=?
	`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()
	now := time.Now().Format(time.RFC3339)
	for _, update := range updates {
		if _, err := stmt.Exec(update.inlineJSON, update.attachmentsJSON, now, policy, update.messageID); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	tx = nil
	return result, nil
}

func previewBodyForPolicy(body *models.EmailBody, policy string) *models.EmailBody {
	policy = normalizePreviewStoragePolicy(policy)
	cached := *body
	cached.InlineImages = cloneInlineImagesForPolicy(body.InlineImages, policy)
	cached.Attachments = cloneAttachmentsForPolicy(body.Attachments, policy)
	return &cached
}

func normalizePreviewStoragePolicy(policy string) string {
	switch strings.TrimSpace(policy) {
	case "lightweight":
		return "lightweight"
	case "no_attachments":
		return "no_attachments"
	case "preserve_all":
		return "preserve_all"
	default:
		return "no_attachments"
	}
}

func cloneInlineImagesForPolicy(images []models.InlineImage, policy string) []models.InlineImage {
	if policy == "lightweight" || len(images) == 0 {
		return nil
	}
	cloned := make([]models.InlineImage, len(images))
	for i, image := range images {
		cloned[i] = image
		if image.Data != nil {
			cloned[i].Data = append([]byte(nil), image.Data...)
		}
	}
	return cloned
}

func cloneAttachmentsForPolicy(attachments []models.Attachment, policy string) []models.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	cloned := make([]models.Attachment, len(attachments))
	for i, attachment := range attachments {
		cloned[i] = attachment
		switch policy {
		case "preserve_all":
			if attachment.Data != nil {
				cloned[i].Data = append([]byte(nil), attachment.Data...)
			}
		default:
			cloned[i].Data = nil
		}
	}
	return cloned
}

func pruneInlineImagesForPolicy(images []models.InlineImage, policy string) ([]models.InlineImage, int64, bool) {
	if policy != "lightweight" || len(images) == 0 {
		return images, 0, false
	}
	var removed int64
	for _, image := range images {
		removed += int64(len(image.Data))
	}
	return nil, removed, true
}

func pruneAttachmentsForPolicy(attachments []models.Attachment, policy string) ([]models.Attachment, int64, bool) {
	if policy == "preserve_all" || len(attachments) == 0 {
		return attachments, 0, false
	}
	cloned := make([]models.Attachment, len(attachments))
	var removed int64
	changed := false
	for i, attachment := range attachments {
		cloned[i] = attachment
		if attachment.Data != nil {
			removed += int64(len(attachment.Data))
			cloned[i].Data = nil
			changed = true
		}
	}
	return cloned, removed, changed
}

func inlineImageBytes(images []models.InlineImage) int64 {
	var total int64
	for _, image := range images {
		total += int64(len(image.Data))
	}
	return total
}

func attachmentPayloadBytes(attachments []models.Attachment) int64 {
	var total int64
	for _, attachment := range attachments {
		total += int64(len(attachment.Data))
	}
	return total
}

func reclaimableInlineImageBytes(images []models.InlineImage, policy string) int64 {
	if policy != "lightweight" {
		return 0
	}
	return inlineImageBytes(images)
}

func reclaimableAttachmentBytes(attachments []models.Attachment, policy string) int64 {
	if policy == "preserve_all" {
		return 0
	}
	return attachmentPayloadBytes(attachments)
}

// GetUnreadEmails returns unread emails in a folder, newest first
func (c *Cache) GetUnreadEmails(folder string, limit int) ([]*models.EmailData, error) {
	q := `SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, is_read, COALESCE(is_starred,0), COALESCE(is_draft,0)
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
	rows, err := c.db.Query(`SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, is_read, COALESCE(is_starred,0), COALESCE(is_draft,0)
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
		rows, err = c.db.Query(`SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, is_read, folder, COALESCE(is_starred,0), COALESCE(is_draft,0)
		                         FROM emails WHERE sender LIKE ? ESCAPE '\' ORDER BY date DESC LIMIT 200`, like)
	} else {
		rows, err = c.db.Query(`SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, is_read, folder, COALESCE(is_starred,0), COALESCE(is_draft,0)
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
		var size, hasAtt, isRead, isStarred, isDraft int
		if err := rows.Scan(&msgID, &uid, &sndr, &subj, &date, &size, &hasAtt, &isRead, &fldr, &isStarred, &isDraft); err != nil {
			return nil, err
		}
		email := normalizeEmailScope(&models.EmailData{
			MessageID:      msgID,
			UID:            uid,
			Sender:         sndr,
			Subject:        subj,
			Date:           date,
			Size:           size,
			HasAttachments: hasAtt == 1,
			IsRead:         isRead == 1,
			IsStarred:      isStarred == 1,
			IsDraft:        isDraft == 1,
			Folder:         fldr,
		})
		emails = append(emails, email)
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

// scanEmailRowsWithRead scans rows with is_read, is_starred, and is_draft columns (no folder column; folder passed separately)
func scanEmailRowsWithRead(rows *sql.Rows, folder string) ([]*models.EmailData, error) {
	var emails []*models.EmailData
	for rows.Next() {
		var msgID, sender, subject string
		var uid uint32
		var date time.Time
		var size, hasAtt, isRead, isStarred, isDraft int
		if err := rows.Scan(&msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &isRead, &isStarred, &isDraft); err != nil {
			return nil, err
		}
		email := normalizeEmailScope(&models.EmailData{
			MessageID:      msgID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAtt == 1,
			IsRead:         isRead == 1,
			IsStarred:      isStarred == 1,
			IsDraft:        isDraft == 1,
			Folder:         folder,
		})
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

// GetAllEmails retrieves all cached emails for a folder
func (c *Cache) GetAllEmails(folder string, groupByDomain bool) (map[string][]*models.EmailData, error) {
	query := `
		SELECT message_id, COALESCE(uid, 0), sender, subject, date, size, has_attachments, COALESCE(is_read, 0), COALESCE(is_starred, 0), COALESCE(is_draft, 0)
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
		var size, hasAttachments, isRead, isStarred, isDraft int

		if err := rows.Scan(&messageID, &uid, &sender, &subject, &date, &size, &hasAttachments, &isRead, &isStarred, &isDraft); err != nil {
			return nil, err
		}

		email := normalizeEmailScope(&models.EmailData{
			MessageID:      messageID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAttachments == 1,
			IsRead:         isRead == 1,
			IsStarred:      isStarred == 1,
			IsDraft:        isDraft == 1,
			Folder:         folder,
		})

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
		SELECT message_id, COALESCE(uid, 0), sender, subject, date, size, has_attachments, COALESCE(is_read, 0), COALESCE(is_starred, 0), COALESCE(is_draft, 0)
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
		var size, hasAtt, isRead, isStarred, isDraft int
		if err := rows.Scan(&msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &isRead, &isStarred, &isDraft); err != nil {
			return nil, err
		}
		email := normalizeEmailScope(&models.EmailData{
			MessageID:      msgID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAtt == 1,
			IsRead:         isRead == 1,
			IsStarred:      isStarred == 1,
			IsDraft:        isDraft == 1,
			Folder:         folder,
		})
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

// GetEmailByID returns a single email by message ID
func (c *Cache) GetEmailByID(messageID string) (*models.EmailData, error) {
	row := c.db.QueryRow(`SELECT COALESCE(source_id, 'default-mail'), COALESCE(account_id, 'default'), COALESCE(local_id, ''), COALESCE(uid_validity, 0), message_id, COALESCE(uid, 0), sender, subject, date, size, has_attachments, folder, COALESCE(is_read, 0), COALESCE(is_starred, 0), COALESCE(is_draft, 0)
	                       FROM emails WHERE message_id = ?`, messageID)
	var msgID, sender, subject, folder string
	var sourceID, accountID, localID string
	var uid uint32
	var uidValidity uint32
	var date time.Time
	var size, hasAtt, isRead, isStarred, isDraft int
	if err := row.Scan(&sourceID, &accountID, &localID, &uidValidity, &msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &folder, &isRead, &isStarred, &isDraft); err != nil {
		return nil, err
	}
	return normalizeEmailScope(&models.EmailData{
		SourceID:       models.SourceID(sourceID),
		AccountID:      models.AccountID(accountID),
		LocalID:        localID,
		UIDValidity:    uidValidity,
		MessageID:      msgID,
		UID:            uid,
		Sender:         sender,
		Subject:        subject,
		Date:           date,
		Size:           size,
		HasAttachments: hasAtt == 1,
		IsRead:         isRead == 1,
		IsStarred:      isStarred == 1,
		IsDraft:        isDraft == 1,
		Folder:         folder,
	}), nil
}

func (c *Cache) GetEmailByRef(ref models.MessageRef) (*models.EmailData, error) {
	ref = ref.WithDefaults()
	if ref.LocalID != "" {
		row := c.db.QueryRow(`SELECT message_id FROM emails WHERE local_id = ?`, ref.LocalID)
		var messageID string
		if err := row.Scan(&messageID); err == nil {
			return c.GetEmailByID(messageID)
		} else if err != sql.ErrNoRows {
			return nil, err
		}
	}
	if ref.MessageID == "" {
		return nil, sql.ErrNoRows
	}
	return c.GetEmailByID(ref.MessageID)
}

// GetEmailByFolderUID returns a cached email by its legacy folder/UID identity.
// It is used by compatibility callers that have not yet been migrated to
// MessageRef but still need to enter the cache-first read path.
func (c *Cache) GetEmailByFolderUID(folder string, uid uint32) (*models.EmailData, error) {
	row := c.db.QueryRow(`SELECT message_id FROM emails WHERE folder = ? AND uid = ? LIMIT 1`, folder, uid)
	var messageID string
	if err := row.Scan(&messageID); err != nil {
		return nil, err
	}
	return c.GetEmailByID(messageID)
}

// GetEmailsSortedByDate returns all emails for a folder sorted by date descending
func (c *Cache) GetEmailsSortedByDate(folder string) ([]*models.EmailData, error) {
	query := `SELECT message_id, COALESCE(uid, 0), sender, subject, date, size, has_attachments, COALESCE(is_read, 0), COALESCE(is_starred, 0), COALESCE(is_draft, 0)
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
		var size, hasAttachments, isRead, isStarred, isDraft int
		if err := rows.Scan(&messageID, &uid, &sender, &subject, &date, &size, &hasAttachments, &isRead, &isStarred, &isDraft); err != nil {
			return nil, err
		}
		email := normalizeEmailScope(&models.EmailData{
			MessageID:      messageID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAttachments == 1,
			IsRead:         isRead == 1,
			IsStarred:      isStarred == 1,
			IsDraft:        isDraft == 1,
			Folder:         folder,
		})
		emails = append(emails, email)
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
			SELECT e.message_id, COALESCE(e.uid,0), e.sender, e.subject, e.date, e.size, e.has_attachments, e.folder, COALESCE(e.is_read,0), COALESCE(e.is_starred,0), COALESCE(e.is_draft,0)
			FROM emails_fts f
			JOIN emails e ON e.rowid = f.rowid
			WHERE emails_fts MATCH ?
			ORDER BY e.date DESC LIMIT 100`, query)
	} else {
		rows, err = c.db.Query(`
			SELECT e.message_id, COALESCE(e.uid,0), e.sender, e.subject, e.date, e.size, e.has_attachments, e.folder, COALESCE(e.is_read,0), COALESCE(e.is_starred,0), COALESCE(e.is_draft,0)
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
		SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, folder, COALESCE(is_read,0), COALESCE(is_starred,0), COALESCE(is_draft,0)
		FROM emails
		WHERE sender LIKE ? ESCAPE '\' OR subject LIKE ? ESCAPE '\'
		ORDER BY date DESC LIMIT 100`, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmailRows(rows)
}

// GetCachedFolders returns the distinct set of folder names present in the emails cache.
func (c *Cache) GetCachedFolders() ([]string, error) {
	rows, err := c.db.Query(`SELECT DISTINCT folder FROM emails WHERE folder != '' ORDER BY folder`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var folders []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// normalizeSubjectSQL is a SQL expression that strips Re:, Fwd:, Fw:, AW: prefixes
// from a subject column for thread grouping. Applied inline in queries.
// Uses a chain of REPLACE + TRIM to handle the most common prefixes.
// Note: This is case-sensitive in SQLite by default; LOWER() is applied to match.
const normalizeSubjectSQL = `TRIM(
    REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(
        LOWER(subject),
        're: ', ''), 'fwd: ', ''), 'fw: ', ''), 'aw: ', ''),
        're:', ''), 'fwd:', ''), 'fw:', ''), 'aw:', '')
)`

// normalizeSubjectGo mirrors normalizeSubjectSQL in Go for use in tests and
// code that cannot run SQL. It does NOT import from package app (avoids circular import).
func normalizeSubjectGo(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	prefixes := []string{"re: ", "fwd: ", "fw: ", "aw: ", "re:", "fwd:", "fw:", "aw:"}
	for _, p := range prefixes {
		s = strings.TrimPrefix(s, p)
	}
	return strings.TrimSpace(s)
}

// GetEmailsByThread returns all emails in folder whose normalized subject matches
// the normalized form of subject. Results are sorted newest first.
func (c *Cache) GetEmailsByThread(folder, subject string) ([]*models.EmailData, error) {
	normalizedSubject := normalizeSubjectGo(subject)
	query := `
		SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, folder, COALESCE(is_read,0), COALESCE(is_starred,0), COALESCE(is_draft,0)
		FROM emails
		WHERE folder = ?
		  AND ` + normalizeSubjectSQL + ` = ?
		ORDER BY date DESC`
	rows, err := c.db.Query(query, folder, normalizedSubject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmailRows(rows)
}

// scanEmailRows is a helper to scan email result rows that include folder, is_read, is_starred, and is_draft columns.
func scanEmailRows(rows *sql.Rows) ([]*models.EmailData, error) {
	var emails []*models.EmailData
	for rows.Next() {
		var msgID, sender, subject, folder string
		var uid uint32
		var date time.Time
		var size, hasAtt, isRead, isStarred, isDraft int
		if err := rows.Scan(&msgID, &uid, &sender, &subject, &date, &size, &hasAtt, &folder, &isRead, &isStarred, &isDraft); err != nil {
			return nil, err
		}
		email := normalizeEmailScope(&models.EmailData{
			MessageID:      msgID,
			UID:            uid,
			Sender:         sender,
			Subject:        subject,
			Date:           date,
			Size:           size,
			HasAttachments: hasAtt == 1,
			IsRead:         isRead == 1,
			IsStarred:      isStarred == 1,
			IsDraft:        isDraft == 1,
			Folder:         folder,
		})
		emails = append(emails, email)
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

// StoreEmbeddingChunks replaces all existing chunks for messageID with the provided chunks.
// Uses a transaction: deletes old chunks first, then inserts all new ones.
func (c *Cache) StoreEmbeddingChunks(messageID string, chunks []models.EmbeddingChunk) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM email_embedding_chunks WHERE message_id = ?`, messageID); err != nil {
		return err
	}
	for _, chunk := range chunks {
		buf := make([]byte, len(chunk.Embedding)*4)
		for i, v := range chunk.Embedding {
			binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
		}
		if _, err := tx.Exec(
			`INSERT INTO email_embedding_chunks (message_id, chunk_index, embedding, content_hash, embedded_at) VALUES (?, ?, ?, ?, ?)`,
			messageID, chunk.ChunkIndex, buf, chunk.ContentHash, time.Now().Format(time.RFC3339),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetUnembeddedIDsWithBody returns message IDs in a folder that have body_text cached
// but have no rows in email_embedding_chunks. Ordered newest-first.
func (c *Cache) GetUnembeddedIDsWithBody(folder string) ([]string, error) {
	rows, err := c.db.Query(`
		SELECT e.message_id
		FROM emails e
		LEFT JOIN email_embedding_chunks eec ON eec.message_id = e.message_id
		WHERE e.folder = ?
		  AND e.body_text IS NOT NULL
		  AND e.body_text != ''
		  AND eec.message_id IS NULL
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

// GetUncachedBodyIDs returns up to limit message IDs in a folder that have
// neither body_text nor any embedding chunks. Ordered newest-first.
func (c *Cache) GetUncachedBodyIDs(folder string, limit int) ([]string, error) {
	rows, err := c.db.Query(`
		SELECT e.message_id
		FROM emails e
		LEFT JOIN email_embedding_chunks eec ON eec.message_id = e.message_id
		WHERE e.folder = ?
		  AND (e.body_text IS NULL OR e.body_text = '')
		  AND eec.message_id IS NULL
		ORDER BY e.date DESC
		LIMIT ?`, folder, limit)
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

// GetEmbeddingProgress returns the number of emails with at least one embedding chunk
// and the total number of emails in the folder.
func (c *Cache) GetEmbeddingProgress(folder string) (done, total int, err error) {
	if err = c.db.QueryRow(`
		SELECT COUNT(DISTINCT eec.message_id)
		FROM email_embedding_chunks eec
		JOIN emails e ON e.message_id = eec.message_id
		WHERE e.folder = ?`, folder).Scan(&done); err != nil {
		return
	}
	err = c.db.QueryRow(`SELECT COUNT(*) FROM emails WHERE folder = ?`, folder).Scan(&total)
	return
}

// SearchSemanticChunked finds emails in a folder using cosine similarity against queryVec.
// It loads all chunk embeddings, computes similarity per chunk, de-duplicates by message_id
// keeping the maximum score per email, then returns the top limit results above minScore
// paired with their similarity scores.
func (c *Cache) SearchSemanticChunked(folder string, queryVec []float32, limit int, minScore float64) ([]*models.SemanticSearchResult, error) {
	rows, err := c.db.Query(`
		SELECT eec.message_id, eec.embedding
		FROM email_embedding_chunks eec
		JOIN emails e ON e.message_id = eec.message_id
		WHERE e.folder = ?`, folder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bestScore := make(map[string]float64)
	for rows.Next() {
		var msgID string
		var buf []byte
		if err := rows.Scan(&msgID, &buf); err != nil {
			return nil, err
		}
		if len(buf)%4 != 0 {
			logger.Warn("corrupt embedding blob for message %s (len=%d), skipping", msgID, len(buf))
			continue
		}
		chunkVec := make([]float32, len(buf)/4)
		for i := range chunkVec {
			chunkVec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
		}
		score := cosineSimilarity(queryVec, chunkVec)
		if score > bestScore[msgID] {
			bestScore[msgID] = score
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	type scored struct {
		messageID string
		score     float64
	}
	var candidates []scored
	for msgID, score := range bestScore {
		if score >= minScore {
			candidates = append(candidates, scored{msgID, score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	var results []*models.SemanticSearchResult
	for _, r := range candidates {
		email, err := c.GetEmailByID(r.messageID)
		if err != nil {
			logger.Debug("SearchSemanticChunked: GetEmailByID %s: %v", r.messageID, err)
			continue
		}
		results = append(results, &models.SemanticSearchResult{Email: email, Score: r.score})
	}
	return results, nil
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

// UpsertContacts inserts or updates contacts from seen email addresses.
// direction is "from" (increments email_count) or "to" (increments sent_count).
func (c *Cache) UpsertContacts(addrs []models.ContactAddr, direction string) error {
	if len(addrs) == 0 {
		return nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("UpsertContacts: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().Format(time.RFC3339)
	var emailCount, sentCount int
	if direction == "from" {
		emailCount = 1
	} else {
		sentCount = 1
	}

	stmt, err := tx.Prepare(`
		INSERT INTO contacts (email, display_name, first_seen, last_seen, email_count, sent_count)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET
			last_seen    = excluded.last_seen,
			display_name = CASE WHEN display_name = '' THEN excluded.display_name ELSE display_name END,
			email_count  = email_count + CASE WHEN ? = 'from' THEN 1 ELSE 0 END,
			sent_count   = sent_count  + CASE WHEN ? = 'to'   THEN 1 ELSE 0 END
	`)
	if err != nil {
		return fmt.Errorf("UpsertContacts: prepare: %w", err)
	}
	defer stmt.Close()

	for _, addr := range addrs {
		if addr.Email == "" {
			continue
		}
		// Normalize email to lowercase to prevent case-only duplicates
		// (e.g. "USER@example.com" vs "user@example.com").
		email := strings.ToLower(addr.Email)
		if _, err := stmt.Exec(email, addr.Name, now, now, emailCount, sentCount, direction, direction); err != nil {
			return fmt.Errorf("UpsertContacts: exec for %q: %w", addr.Email, err)
		}
	}

	return tx.Commit()
}

// GetContactsToEnrich returns up to limit contacts with email_count >= minCount
// that have not been enriched yet (enriched_at IS NULL).
// Only id, email, and display_name are populated in the returned ContactData.
func (c *Cache) GetContactsToEnrich(minCount, limit int) ([]models.ContactData, error) {
	rows, err := c.db.Query(
		`SELECT id, email, display_name FROM contacts WHERE email_count >= ? AND enriched_at IS NULL LIMIT ?`,
		minCount, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []models.ContactData
	for rows.Next() {
		var cd models.ContactData
		if err := rows.Scan(&cd.ID, &cd.Email, &cd.DisplayName); err != nil {
			return nil, err
		}
		contacts = append(contacts, cd)
	}
	return contacts, rows.Err()
}

// GetRecentSubjectsByContact returns up to limit email subjects where the sender
// field contains the given email address, ordered by date descending.
func (c *Cache) GetRecentSubjectsByContact(email string, limit int) ([]string, error) {
	like := "%" + escapeLike(email) + "%"
	rows, err := c.db.Query(
		`SELECT subject FROM emails WHERE sender LIKE ? ESCAPE '\' ORDER BY date DESC LIMIT ?`,
		like, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subjects []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		subjects = append(subjects, s)
	}
	return subjects, rows.Err()
}

// UpdateContactEnrichment saves the LLM-extracted company and topics for a contact.
// Sets enriched_at to the current time. Topics are JSON-encoded for storage.
func (c *Cache) UpdateContactEnrichment(email, company string, topics []string) error {
	if topics == nil {
		topics = []string{}
	}
	topicsJSON, err := json.Marshal(topics)
	if err != nil {
		return fmt.Errorf("UpdateContactEnrichment: marshal topics: %w", err)
	}
	_, err = c.db.Exec(
		`UPDATE contacts SET company = ?, topics = ?, enriched_at = datetime('now') WHERE email = ?`,
		company, string(topicsJSON), email,
	)
	return err
}

// UpdateContactEmbedding saves the semantic embedding vector for a contact.
// The embedding is encoded as a little-endian float32 blob (same as email_embedding_chunks).
func (c *Cache) UpdateContactEmbedding(email string, embedding []float32) error {
	buf := make([]byte, len(embedding)*4)
	for i, v := range embedding {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	_, err := c.db.Exec(`UPDATE contacts SET embedding = ? WHERE email = ?`, buf, email)
	return err
}

// SearchContactsSemantic finds contacts using cosine similarity against queryVec.
// Returns up to limit contacts with score >= minScore, ordered by score descending.
// Fields populated: id, email, display_name, company, topics, embedding.
func (c *Cache) SearchContactsSemantic(queryVec []float32, limit int, minScore float64) ([]*models.ContactSearchResult, error) {
	rows, err := c.db.Query(
		`SELECT id, email, display_name, company, topics, embedding FROM contacts WHERE embedding IS NOT NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		cd    models.ContactData
		score float64
	}
	var candidates []scored

	for rows.Next() {
		var cd models.ContactData
		var topicsJSON string
		var embBlob []byte
		if err := rows.Scan(&cd.ID, &cd.Email, &cd.DisplayName, &cd.Company, &topicsJSON, &embBlob); err != nil {
			return nil, err
		}
		if len(embBlob)%4 != 0 || len(embBlob) == 0 {
			logger.Warn("SearchContactsSemantic: corrupt embedding for %s (len=%d), skipping", cd.Email, len(embBlob))
			continue
		}
		vec := make([]float32, len(embBlob)/4)
		for i := range vec {
			vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(embBlob[i*4:]))
		}
		score := cosineSimilarity(queryVec, vec)
		if score < minScore {
			continue
		}
		// Decode topics
		if jsonErr := json.Unmarshal([]byte(topicsJSON), &cd.Topics); jsonErr != nil {
			cd.Topics = nil
		}
		cd.Embedding = vec
		candidates = append(candidates, scored{cd, score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	result := make([]*models.ContactSearchResult, len(candidates))
	for i, cand := range candidates {
		result[i] = &models.ContactSearchResult{Contact: cand.cd, Score: cand.score}
	}
	return result, nil
}

// ListContacts returns contacts sorted by the given criterion.
// sortBy accepts "last_seen" (default), "name", or "email_count".
// All ContactData fields are populated (topics decoded, embedding decoded).
func (c *Cache) ListContacts(limit int, sortBy string) ([]models.ContactData, error) {
	var orderBy string
	switch sortBy {
	case "name":
		orderBy = "display_name ASC, email ASC"
	case "email_count":
		orderBy = "email_count DESC"
	default:
		orderBy = "last_seen DESC"
	}
	query := fmt.Sprintf(
		`SELECT id, email, display_name, company, topics, notes, first_seen, last_seen, email_count, sent_count, COALESCE(carddav_uid,''), enriched_at, embedding
		 FROM contacts ORDER BY %s LIMIT ?`, orderBy,
	)
	rows, err := c.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []models.ContactData
	for rows.Next() {
		var cd models.ContactData
		var topicsJSON string
		var embBlob []byte
		var enrichedAt sql.NullString
		var firstSeen, lastSeen string
		if err := rows.Scan(
			&cd.ID, &cd.Email, &cd.DisplayName, &cd.Company,
			&topicsJSON, &cd.Notes, &firstSeen, &lastSeen,
			&cd.EmailCount, &cd.SentCount, &cd.CardDAVUID,
			&enrichedAt, &embBlob,
		); err != nil {
			return nil, err
		}
		// Parse timestamps
		if t, err := time.Parse(time.RFC3339, firstSeen); err == nil {
			cd.FirstSeen = t
		}
		if t, err := time.Parse(time.RFC3339, lastSeen); err == nil {
			cd.LastSeen = t
		}
		if enrichedAt.Valid {
			if t, err := time.Parse(time.RFC3339, enrichedAt.String); err == nil {
				cd.EnrichedAt = &t
			}
		}
		// Decode topics JSON
		if jsonErr := json.Unmarshal([]byte(topicsJSON), &cd.Topics); jsonErr != nil {
			cd.Topics = nil
		}
		// Decode embedding blob
		if len(embBlob)%4 == 0 && len(embBlob) > 0 {
			vec := make([]float32, len(embBlob)/4)
			for i := range vec {
				vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(embBlob[i*4:]))
			}
			cd.Embedding = vec
		}
		contacts = append(contacts, cd)
	}
	return contacts, rows.Err()
}

// SearchContacts performs a keyword search on display_name, email, company, and topics.
func (c *Cache) SearchContacts(query string) ([]models.ContactData, error) {
	like := "%" + escapeLike(query) + "%"
	rows, err := c.db.Query(
		`SELECT id, email, display_name, company, topics, notes, first_seen, last_seen, email_count, sent_count, COALESCE(carddav_uid,''), enriched_at, embedding
		 FROM contacts
		 WHERE display_name LIKE ? ESCAPE '\'
		    OR email        LIKE ? ESCAPE '\'
		    OR company      LIKE ? ESCAPE '\'
		    OR topics       LIKE ? ESCAPE '\'
		 ORDER BY last_seen DESC`,
		like, like, like, like,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []models.ContactData
	for rows.Next() {
		var cd models.ContactData
		var topicsJSON string
		var embBlob []byte
		var enrichedAt sql.NullString
		var firstSeen, lastSeen string
		if err := rows.Scan(
			&cd.ID, &cd.Email, &cd.DisplayName, &cd.Company,
			&topicsJSON, &cd.Notes, &firstSeen, &lastSeen,
			&cd.EmailCount, &cd.SentCount, &cd.CardDAVUID,
			&enrichedAt, &embBlob,
		); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, firstSeen); err == nil {
			cd.FirstSeen = t
		}
		if t, err := time.Parse(time.RFC3339, lastSeen); err == nil {
			cd.LastSeen = t
		}
		if enrichedAt.Valid {
			if t, err := time.Parse(time.RFC3339, enrichedAt.String); err == nil {
				cd.EnrichedAt = &t
			}
		}
		if jsonErr := json.Unmarshal([]byte(topicsJSON), &cd.Topics); jsonErr != nil {
			cd.Topics = nil
		}
		if len(embBlob)%4 == 0 && len(embBlob) > 0 {
			vec := make([]float32, len(embBlob)/4)
			for i := range vec {
				vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(embBlob[i*4:]))
			}
			cd.Embedding = vec
		}
		contacts = append(contacts, cd)
	}
	return contacts, rows.Err()
}

// GetContactEmails returns recent emails where sender matches the given email address.
// GetContactByEmail returns the contact with the exact email address, or nil if not found.
func (c *Cache) GetContactByEmail(email string) (*models.ContactData, error) {
	row := c.db.QueryRow(
		`SELECT id, email, display_name, company, topics, notes, first_seen, last_seen, email_count, sent_count, COALESCE(carddav_uid,''), enriched_at
		 FROM contacts WHERE email = ?`, email,
	)
	var cd models.ContactData
	var topicsJSON string
	var enrichedAt sql.NullString
	var firstSeen, lastSeen string
	err := row.Scan(
		&cd.ID, &cd.Email, &cd.DisplayName, &cd.Company,
		&topicsJSON, &cd.Notes, &firstSeen, &lastSeen,
		&cd.EmailCount, &cd.SentCount, &cd.CardDAVUID,
		&enrichedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t, err := time.Parse(time.RFC3339, firstSeen); err == nil {
		cd.FirstSeen = t
	}
	if t, err := time.Parse(time.RFC3339, lastSeen); err == nil {
		cd.LastSeen = t
	}
	if enrichedAt.Valid {
		if t, err := time.Parse(time.RFC3339, enrichedAt.String); err == nil {
			cd.EnrichedAt = &t
		}
	}
	if jsonErr := json.Unmarshal([]byte(topicsJSON), &cd.Topics); jsonErr != nil {
		cd.Topics = nil
	}
	return &cd, nil
}

func (c *Cache) GetContactEmails(contactEmail string, limit int) ([]*models.EmailData, error) {
	like := "%" + escapeLike(contactEmail) + "%"
	rows, err := c.db.Query(
		`SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, folder, last_updated, COALESCE(is_read,0), COALESCE(is_starred,0), COALESCE(is_draft,0)
		 FROM emails WHERE sender LIKE ? ESCAPE '\' ORDER BY date DESC LIMIT ?`,
		like, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []*models.EmailData
	for rows.Next() {
		var e models.EmailData
		var dateStr, lastUpdStr string
		var hasAtt, isRead, isStarred, isDraft int
		if err := rows.Scan(
			&e.MessageID, &e.UID, &e.Sender, &e.Subject,
			&dateStr, &e.Size, &hasAtt, &e.Folder, &lastUpdStr, &isRead, &isStarred, &isDraft,
		); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
			e.Date = t
		}
		if t, err := time.Parse(time.RFC3339, lastUpdStr); err == nil {
			e.LastUpdated = t
		}
		e.HasAttachments = hasAtt != 0
		e.IsRead = isRead != 0
		e.IsStarred = isStarred != 0
		e.IsDraft = isDraft != 0
		emails = append(emails, normalizeEmailScope(&e))
	}
	return emails, rows.Err()
}
