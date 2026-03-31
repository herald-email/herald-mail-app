package cleanup

import (
	"context"
	"time"

	"mail-processor/internal/backend"
	"mail-processor/internal/cache"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// Engine executes cleanup rules against the email backend.
type Engine struct {
	cache   *cache.Cache
	backend backend.Backend
	log     *logger.Logger
}

// NewEngine creates a new cleanup Engine.
func NewEngine(c *cache.Cache, b backend.Backend, l *logger.Logger) *Engine {
	return &Engine{cache: c, backend: b, log: l}
}

// RunRule executes one cleanup rule. Returns count of emails processed.
func (e *Engine) RunRule(ctx context.Context, rule *models.CleanupRule) (int, error) {
	emails, err := e.cache.FindEmailsMatchingCleanupRule(rule)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, email := range emails {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		var actionErr error
		if rule.Action == "delete" {
			actionErr = e.backend.DeleteEmail(email.MessageID, email.Folder)
		} else {
			actionErr = e.backend.MoveEmail(email.MessageID, email.Folder, "Archive")
		}
		if actionErr != nil {
			e.log.Debug("cleanup rule %d: failed to %s email %s: %v", rule.ID, rule.Action, email.MessageID, actionErr)
			continue
		}
		count++
	}

	now := time.Now()
	if err := e.cache.UpdateCleanupRuleLastRun(rule.ID, now); err != nil {
		e.log.Debug("cleanup: failed to update last_run for rule %d: %v", rule.ID, err)
	}
	return count, nil
}

// RunAll runs all enabled rules and returns a map of ruleID -> count.
func (e *Engine) RunAll(ctx context.Context) (map[int64]int, error) {
	rules, err := e.cache.GetAllCleanupRules()
	if err != nil {
		return nil, err
	}

	results := make(map[int64]int)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		n, err := e.RunRule(ctx, rule)
		if err != nil {
			e.log.Debug("cleanup rule %d failed: %v", rule.ID, err)
		}
		results[rule.ID] = n
	}
	return results, nil
}
