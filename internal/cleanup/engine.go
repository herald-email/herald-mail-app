package cleanup

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
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
	results, err := e.runPlanned(ctx, models.RuleDryRunRequest{
		Kind:            models.RuleDryRunKindCleanup,
		RuleID:          rule.ID,
		AllFolders:      true,
		IncludeDisabled: true,
	}, []*models.CleanupRule{rule})
	if err != nil {
		return 0, err
	}
	return results[rule.ID], nil
}

func (e *Engine) runPlanned(ctx context.Context, req models.RuleDryRunRequest, rules []*models.CleanupRule) (map[int64]int, error) {
	report, err := PlanDryRun(e.cache, req, rules)
	if err != nil {
		return nil, err
	}

	rulesByID := make(map[int64]*models.CleanupRule, len(rules))
	results := make(map[int64]int, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		rulesByID[rule.ID] = rule
		results[rule.ID] = 0
	}

	processedMessages := make(map[string]bool)
	for _, row := range report.Rows {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		rule := rulesByID[row.RuleID]
		if rule == nil {
			continue
		}
		if processedMessages[row.MessageID] {
			e.log.Info("cleanup rule %d: skipped email %s because an earlier previewed action already handled it", rule.ID, row.MessageID)
			continue
		}
		if e.dryRun {
			e.log.Info("[DRY RUN] Would %s email %s (rule: %s)", rule.Action, row.MessageID, rule.Name)
			results[rule.ID]++
			continue
		}
		var actionErr error
		if rule.Action == "delete" {
			actionErr = e.backend.DeleteEmail(row.MessageID, row.Folder)
		} else {
			actionErr = e.backend.MoveEmail(row.MessageID, row.Folder, "Archive")
		}
		if actionErr != nil {
			e.log.Debug("cleanup rule %d: failed to %s email %s: %v", rule.ID, rule.Action, row.MessageID, actionErr)
			continue
		}
		processedMessages[row.MessageID] = true
		results[rule.ID]++
	}

	if e.dryRun {
		return results, nil
	}

	now := time.Now()
	for _, rule := range rules {
		if rule == nil || rule.ID == 0 {
			continue
		}
		if err := e.cache.UpdateCleanupRuleLastRun(rule.ID, now); err != nil {
			e.log.Debug("cleanup: failed to update last_run for rule %d: %v", rule.ID, err)
		}
	}
	return results, nil
}

type cleanupPlannerCache interface {
	FindEmailsMatchingCleanupRule(*models.CleanupRule) ([]*models.EmailData, error)
}

// PlanDryRun builds a structured cleanup-rule preview without mutating IMAP
// mail, cache rows, or cleanup rule last_run metadata.
func PlanDryRun(c cleanupPlannerCache, req models.RuleDryRunRequest, rules []*models.CleanupRule) (*models.RuleDryRunReport, error) {
	if req.Kind == "" {
		req.Kind = models.RuleDryRunKindCleanup
	}
	report := &models.RuleDryRunReport{
		Kind:        req.Kind,
		Scope:       cleanupDryRunScope(req),
		Folder:      req.Folder,
		DryRun:      true,
		GeneratedAt: time.Now(),
	}

	selected := make([]*models.CleanupRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		if req.RuleID != 0 && rule.ID != req.RuleID {
			continue
		}
		if !req.IncludeDisabled && !rule.Enabled {
			continue
		}
		selected = append(selected, rule)
	}
	report.RuleCount = len(selected)

	matches := make(map[string]bool)
	for _, rule := range selected {
		emails, err := c.FindEmailsMatchingCleanupRule(rule)
		if err != nil {
			return nil, err
		}
		for _, email := range emails {
			if email == nil {
				continue
			}
			if req.Folder != "" && !req.AllFolders && email.Folder != req.Folder {
				continue
			}
			matches[email.MessageID] = true
			report.Rows = append(report.Rows, models.RuleDryRunRow{
				RuleID:    rule.ID,
				RuleName:  cleanupRuleName(rule),
				MessageID: email.MessageID,
				Sender:    email.Sender,
				Domain:    senderDomain(email.Sender),
				Folder:    email.Folder,
				Subject:   email.Subject,
				Date:      email.Date,
				Action:    rule.Action,
				Target:    cleanupActionTarget(rule.Action),
			})
		}
	}
	report.MatchCount = len(matches)
	report.ActionCount = len(report.Rows)
	return report, nil
}

func cleanupDryRunScope(req models.RuleDryRunRequest) string {
	scope := "selected"
	if req.RuleID == 0 {
		scope = "all"
	}
	if req.AllFolders {
		return scope + " cleanup rules / all folders"
	}
	if req.Folder != "" {
		return scope + " cleanup rules / " + req.Folder
	}
	return scope + " cleanup rules"
}

func cleanupRuleName(rule *models.CleanupRule) string {
	if strings.TrimSpace(rule.Name) != "" {
		return rule.Name
	}
	return fmt.Sprintf("%s:%s", rule.MatchType, rule.MatchValue)
}

func cleanupActionTarget(action string) string {
	if action == "delete" {
		return "Trash"
	}
	return "Archive"
}

func senderAddress(sender string) string {
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return ""
	}
	if addr, err := mail.ParseAddress(sender); err == nil && addr.Address != "" {
		return strings.ToLower(strings.TrimSpace(addr.Address))
	}
	if lt := strings.LastIndex(sender, "<"); lt >= 0 {
		if gt := strings.Index(sender[lt:], ">"); gt >= 0 {
			return strings.ToLower(strings.TrimSpace(sender[lt+1 : lt+gt]))
		}
	}
	return strings.ToLower(sender)
}

func senderDomain(sender string) string {
	sender = senderAddress(sender)
	at := strings.LastIndex(sender, "@")
	if at < 0 {
		return ""
	}
	return strings.ToLower(sender[at+1:])
}

// RunAll runs all enabled rules and returns a map of ruleID -> count.
func (e *Engine) RunAll(ctx context.Context) (map[int64]int, error) {
	return e.Run(ctx, models.RuleDryRunRequest{Kind: models.RuleDryRunKindCleanup, AllFolders: true})
}

// Run executes cleanup rules in the same rule scope used by dry-run preview.
func (e *Engine) Run(ctx context.Context, req models.RuleDryRunRequest) (map[int64]int, error) {
	rules, err := e.cache.GetAllCleanupRules()
	if err != nil {
		return nil, err
	}

	selected := make([]*models.CleanupRule, 0, len(rules))
	for _, rule := range rules {
		if req.RuleID != 0 && rule.ID != req.RuleID {
			continue
		}
		if !rule.Enabled {
			continue
		}
		selected = append(selected, rule)
	}
	req.IncludeDisabled = false
	return e.runPlanned(ctx, req, selected)
}
