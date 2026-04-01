package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"mail-processor/internal/models"
)

// SaveRule inserts or replaces a rule. Marshals Actions to JSON.
// Sets CreatedAt if zero value. Sets r.ID from LastInsertId after insert.
func (c *Cache) SaveRule(r *models.Rule) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}

	actionsJSON, err := json.Marshal(r.Actions)
	if err != nil {
		return fmt.Errorf("failed to marshal rule actions: %w", err)
	}

	if r.ID == 0 {
		result, err := c.db.Exec(
			`INSERT INTO email_rules (name, enabled, priority, trigger_type, trigger_value, custom_prompt_id, actions_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			r.Name,
			boolToInt(r.Enabled),
			r.Priority,
			string(r.TriggerType),
			r.TriggerValue,
			r.CustomPromptID,
			string(actionsJSON),
			r.CreatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("failed to insert rule: %w", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get last insert id: %w", err)
		}
		r.ID = id
	} else {
		result, err := c.db.Exec(
			`UPDATE email_rules SET name=?, enabled=?, priority=?, trigger_type=?, trigger_value=?, custom_prompt_id=?, actions_json=? WHERE id=?`,
			r.Name,
			boolToInt(r.Enabled),
			r.Priority,
			string(r.TriggerType),
			r.TriggerValue,
			r.CustomPromptID,
			string(actionsJSON),
			r.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update rule: %w", err)
		}
		rowsAff, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("save rule: check rows affected: %w", err)
		}
		if rowsAff == 0 {
			return fmt.Errorf("save rule: rule ID %d not found", r.ID)
		}
	}
	return nil
}

// GetAllRules returns all rules ordered by priority ASC, regardless of enabled state.
func (c *Cache) GetAllRules() ([]*models.Rule, error) {
	rows, err := c.db.Query(
		`SELECT id, name, enabled, priority, trigger_type, trigger_value, custom_prompt_id, actions_json, created_at, last_triggered FROM email_rules ORDER BY priority ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query all rules: %w", err)
	}
	defer rows.Close()

	var rules []*models.Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// GetEnabledRules returns all enabled rules ordered by priority ASC.
func (c *Cache) GetEnabledRules() ([]*models.Rule, error) {
	rows, err := c.db.Query(
		`SELECT id, name, enabled, priority, trigger_type, trigger_value, custom_prompt_id, actions_json, created_at, last_triggered FROM email_rules WHERE enabled = 1 ORDER BY priority ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled rules: %w", err)
	}
	defer rows.Close()

	var rules []*models.Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// GetRuleByID returns a single rule by ID.
func (c *Cache) GetRuleByID(id int64) (*models.Rule, error) {
	row := c.db.QueryRow(
		`SELECT id, name, enabled, priority, trigger_type, trigger_value, custom_prompt_id, actions_json, created_at, last_triggered FROM email_rules WHERE id = ?`,
		id,
	)
	rule, err := scanRule(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get rule by id %d: %w", id, err)
	}
	return rule, nil
}

// DeleteRule removes a rule by ID.
func (c *Cache) DeleteRule(id int64) error {
	_, err := c.db.Exec(`DELETE FROM email_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete rule %d: %w", id, err)
	}
	return nil
}

// SaveCustomPrompt inserts or replaces a custom prompt.
// Sets CreatedAt if zero value.
func (c *Cache) SaveCustomPrompt(p *models.CustomPrompt) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}

	if p.ID == 0 {
		result, err := c.db.Exec(
			`INSERT INTO custom_prompts (name, system_text, user_template, output_var, created_at) VALUES (?, ?, ?, ?, ?)`,
			p.Name,
			p.SystemText,
			p.UserTemplate,
			p.OutputVar,
			p.CreatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("failed to insert custom prompt: %w", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get last insert id: %w", err)
		}
		p.ID = id
	} else {
		result, err := c.db.Exec(
			`UPDATE custom_prompts SET name=?, system_text=?, user_template=?, output_var=? WHERE id=?`,
			p.Name,
			p.SystemText,
			p.UserTemplate,
			p.OutputVar,
			p.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update custom prompt: %w", err)
		}
		rowsAff, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("save custom prompt: check rows affected: %w", err)
		}
		if rowsAff == 0 {
			return fmt.Errorf("save custom prompt: prompt ID %d not found", p.ID)
		}
	}
	return nil
}

// GetCustomPrompt returns a single custom prompt by ID.
func (c *Cache) GetCustomPrompt(id int64) (*models.CustomPrompt, error) {
	row := c.db.QueryRow(
		`SELECT id, name, system_text, user_template, output_var, created_at FROM custom_prompts WHERE id = ?`,
		id,
	)
	var p models.CustomPrompt
	var createdAt string
	err := row.Scan(&p.ID, &p.Name, &p.SystemText, &p.UserTemplate, &p.OutputVar, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get custom prompt %d: %w", id, err)
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err == nil {
		p.CreatedAt = t
	}
	return &p, nil
}

// GetAllCustomPrompts returns all custom prompts.
func (c *Cache) GetAllCustomPrompts() ([]*models.CustomPrompt, error) {
	rows, err := c.db.Query(
		`SELECT id, name, system_text, user_template, output_var, created_at FROM custom_prompts ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query custom prompts: %w", err)
	}
	defer rows.Close()

	var prompts []*models.CustomPrompt
	for rows.Next() {
		var p models.CustomPrompt
		var createdAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.SystemText, &p.UserTemplate, &p.OutputVar, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan custom prompt: %w", err)
		}
		t, err := time.Parse(time.RFC3339, createdAt)
		if err == nil {
			p.CreatedAt = t
		}
		prompts = append(prompts, &p)
	}
	return prompts, rows.Err()
}

// AppendActionLog inserts a rule action log entry.
func (c *Cache) AppendActionLog(entry *models.RuleActionLogEntry) error {
	if entry.ExecutedAt.IsZero() {
		entry.ExecutedAt = time.Now()
	}
	_, err := c.db.Exec(
		`INSERT INTO rule_action_log (rule_id, message_id, action_type, status, detail, executed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.RuleID,
		entry.MessageID,
		string(entry.ActionType),
		entry.Status,
		entry.Detail,
		entry.ExecutedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to insert action log: %w", err)
	}
	return nil
}

// TouchRuleLastTriggered updates the last_triggered timestamp for a rule.
func (c *Cache) TouchRuleLastTriggered(ruleID int64) error {
	_, err := c.db.Exec(
		`UPDATE email_rules SET last_triggered = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339),
		ruleID,
	)
	if err != nil {
		return fmt.Errorf("failed to touch last_triggered for rule %d: %w", ruleID, err)
	}
	return nil
}

// ruleScanner is a common interface satisfied by *sql.Row and *sql.Rows.
type ruleScanner interface {
	Scan(dest ...any) error
}

// scanRule scans a rule row from either *sql.Row or *sql.Rows.
func scanRule(s ruleScanner) (*models.Rule, error) {
	var rule models.Rule
	var enabledInt int
	var triggerType string
	var actionsJSON string
	var createdAt string
	var lastTriggered sql.NullTime
	var promptID sql.NullInt64

	err := s.Scan(
		&rule.ID,
		&rule.Name,
		&enabledInt,
		&rule.Priority,
		&triggerType,
		&rule.TriggerValue,
		&promptID,
		&actionsJSON,
		&createdAt,
		&lastTriggered,
	)
	if err != nil {
		return nil, err
	}

	if promptID.Valid {
		rule.CustomPromptID = &promptID.Int64
	}

	rule.Enabled = enabledInt != 0
	rule.TriggerType = models.RuleTriggerType(triggerType)

	t, err := time.Parse(time.RFC3339, createdAt)
	if err == nil {
		rule.CreatedAt = t
	}

	if lastTriggered.Valid {
		lt := lastTriggered.Time
		rule.LastTriggered = &lt
	}

	if err := json.Unmarshal([]byte(actionsJSON), &rule.Actions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rule actions: %w", err)
	}

	return &rule, nil
}

// boolToInt converts a bool to an integer (1 or 0) for SQLite storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
