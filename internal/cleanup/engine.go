package cleanup

import (
	"context"
	"time"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// Engine executes cleanup rules against the email backend.
type Engine struct {
	cache   *cache.Cache
	backend backend.Backend
	log     *logger.Logger
	dryRun  bool // when true, log actions without executing destructive ones
}

// NewEngine creates a new cleanup Engine.
func NewEngine(c *cache.Cache, b backend.Backend, l *logger.Logger) *Engine {
	return &Engine{cache: c, backend: b, log: l}
}

// NewEngineWithDryRun creates a new cleanup Engine with an optional dry-run mode.
func NewEngineWithDryRun(c *cache.Cache, b backend.Backend, l *logger.Logger, dryRun bool) *Engine {
	return &Engine{cache: c, backend: b, log: l, dryRun: dryRun}
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

		if e.dryRun {
			e.log.Info("[DRY RUN] Would %s email %s (rule: %s)", rule.Action, email.MessageID, rule.Name)
			count++
			continue
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
