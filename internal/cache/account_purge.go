package cache

import (
	"database/sql"
	"fmt"

	"github.com/herald-email/herald-mail-app/internal/models"
)

// PurgeAccountCache removes local cached data for one Herald account. It never
// talks to providers and therefore cannot delete provider-side mail or calendars.
func (c *Cache) PurgeAccountCache(accountID models.AccountID, sourceIDs []models.SourceID) error {
	if c == nil || c.db == nil || accountID == "" {
		return nil
	}
	account := string(models.NormalizeAccountID(accountID))
	sourceSet := make(map[string]struct{}, len(sourceIDs))
	for _, id := range sourceIDs {
		if id != "" {
			sourceSet[string(id)] = struct{}{}
		}
	}

	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	scopedTables := []string{
		"emails",
		"email_classifications",
		"email_preview_bodies",
		"email_embeddings",
		"email_embedding_chunks",
		"folder_sync_state",
		"calendar_collections",
		"calendar_events",
		"cleanup_rules",
	}
	for _, table := range scopedTables {
		hasAccountID, err := txTableHasColumn(tx, table, "account_id")
		if err != nil {
			return fmt.Errorf("inspect %s: %w", table, err)
		}
		hasSourceID, err := txTableHasColumn(tx, table, "source_id")
		if err != nil {
			return fmt.Errorf("inspect %s source scope: %w", table, err)
		}
		if hasAccountID {
			if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE COALESCE(account_id, 'default') = ?", table), account); err != nil {
				return fmt.Errorf("purge %s: %w", table, err)
			}
		}
		if hasSourceID && len(sourceSet) > 0 {
			for source := range sourceSet {
				if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE COALESCE(source_id, 'default-mail') = ?", table), source); err != nil {
					return fmt.Errorf("purge %s source %s: %w", table, source, err)
				}
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO emails_fts(emails_fts) VALUES('rebuild')`); err != nil {
		// Some builds lack FTS5. The regular row deletes above already removed data.
	}
	return tx.Commit()
}

func txTableHasColumn(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
