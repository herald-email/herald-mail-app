package cache

import (
	"database/sql"
	"fmt"
	"time"

	"mail-processor/internal/models"
)

// SaveCleanupRule inserts a new rule (ID==0) or updates an existing one.
func (c *Cache) SaveCleanupRule(rule *models.CleanupRule) error {
	if rule.ID == 0 {
		// INSERT
		res, err := c.db.Exec(`
			INSERT INTO cleanup_rules (name, match_type, match_value, action, older_than_days, enabled, last_run, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			rule.Name,
			rule.MatchType,
			rule.MatchValue,
			rule.Action,
			rule.OlderThanDays,
			boolToInt(rule.Enabled),
			timeToNullable(rule.LastRun),
			rule.CreatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("insert cleanup rule: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("get last insert id: %w", err)
		}
		rule.ID = id
		return nil
	}

	// UPDATE
	_, err := c.db.Exec(`
		UPDATE cleanup_rules
		SET name=?, match_type=?, match_value=?, action=?, older_than_days=?, enabled=?, last_run=?
		WHERE id=?`,
		rule.Name,
		rule.MatchType,
		rule.MatchValue,
		rule.Action,
		rule.OlderThanDays,
		boolToInt(rule.Enabled),
		timeToNullable(rule.LastRun),
		rule.ID,
	)
	return err
}

// GetCleanupRule returns a single cleanup rule by ID.
func (c *Cache) GetCleanupRule(id int64) (*models.CleanupRule, error) {
	row := c.db.QueryRow(`
		SELECT id, name, match_type, match_value, action, older_than_days, enabled, last_run, created_at
		FROM cleanup_rules WHERE id=?`, id)
	return scanCleanupRule(row)
}

// GetAllCleanupRules returns all cleanup rules ordered by id.
func (c *Cache) GetAllCleanupRules() ([]*models.CleanupRule, error) {
	rows, err := c.db.Query(`
		SELECT id, name, match_type, match_value, action, older_than_days, enabled, last_run, created_at
		FROM cleanup_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*models.CleanupRule
	for rows.Next() {
		rule, err := scanCleanupRuleRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// DeleteCleanupRule removes a cleanup rule by ID.
func (c *Cache) DeleteCleanupRule(id int64) error {
	_, err := c.db.Exec(`DELETE FROM cleanup_rules WHERE id=?`, id)
	return err
}

// UpdateCleanupRuleLastRun updates the last_run timestamp for a rule.
func (c *Cache) UpdateCleanupRuleLastRun(id int64, t time.Time) error {
	_, err := c.db.Exec(`UPDATE cleanup_rules SET last_run=? WHERE id=?`,
		t.Format(time.RFC3339), id)
	return err
}

// FindEmailsMatchingCleanupRule returns emails that match the rule's criteria.
// It filters by sender or domain and restricts to emails older than OlderThanDays.
func (c *Cache) FindEmailsMatchingCleanupRule(rule *models.CleanupRule) ([]*models.EmailData, error) {
	var rows *sql.Rows
	var err error

	baseSelect := `SELECT message_id, COALESCE(uid,0), sender, subject, date, size, has_attachments, folder, COALESCE(is_read,0), COALESCE(is_starred,0), COALESCE(is_draft,0)
		FROM emails
		WHERE date < datetime('now', ? || ' days')`

	olderThan := fmt.Sprintf("-%d", rule.OlderThanDays)

	switch rule.MatchType {
	case "sender":
		rows, err = c.db.Query(baseSelect+` AND sender = ? ORDER BY date DESC`, olderThan, rule.MatchValue)
	case "domain":
		domainPattern := "%@" + rule.MatchValue
		rows, err = c.db.Query(baseSelect+` AND sender LIKE ? ORDER BY date DESC`, olderThan, domainPattern)
	default:
		return nil, fmt.Errorf("unknown match_type: %s", rule.MatchType)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEmailRows(rows)
}

// --- helpers ---

func timeToNullable(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

// scanCleanupRule scans a single *sql.Row into a CleanupRule.
func scanCleanupRule(row *sql.Row) (*models.CleanupRule, error) {
	var rule models.CleanupRule
	var lastRunStr sql.NullString
	var createdAtStr string
	var enabled int

	err := row.Scan(
		&rule.ID,
		&rule.Name,
		&rule.MatchType,
		&rule.MatchValue,
		&rule.Action,
		&rule.OlderThanDays,
		&enabled,
		&lastRunStr,
		&createdAtStr,
	)
	if err != nil {
		return nil, err
	}

	rule.Enabled = enabled == 1
	if lastRunStr.Valid && lastRunStr.String != "" {
		t, err := time.Parse(time.RFC3339, lastRunStr.String)
		if err == nil {
			rule.LastRun = &t
		}
	}
	rule.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &rule, nil
}

// scanCleanupRuleRow scans *sql.Rows (multi-row query) into a CleanupRule.
func scanCleanupRuleRow(rows *sql.Rows) (*models.CleanupRule, error) {
	var rule models.CleanupRule
	var lastRunStr sql.NullString
	var createdAtStr string
	var enabled int

	err := rows.Scan(
		&rule.ID,
		&rule.Name,
		&rule.MatchType,
		&rule.MatchValue,
		&rule.Action,
		&rule.OlderThanDays,
		&enabled,
		&lastRunStr,
		&createdAtStr,
	)
	if err != nil {
		return nil, err
	}

	rule.Enabled = enabled == 1
	if lastRunStr.Valid && lastRunStr.String != "" {
		t, err := time.Parse(time.RFC3339, lastRunStr.String)
		if err == nil {
			rule.LastRun = &t
		}
	}
	rule.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &rule, nil
}
