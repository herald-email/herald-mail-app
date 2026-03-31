package rules

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"

	"mail-processor/internal/ai"
	"mail-processor/internal/models"
)

// Store is implemented by *cache.Cache
type Store interface {
	GetEnabledRules() ([]*models.Rule, error)
	GetCustomPrompt(id int64) (*models.CustomPrompt, error)
	SaveCustomCategory(messageID string, promptID int64, result string) error
	AppendActionLog(*models.RuleActionLogEntry) error
	TouchRuleLastTriggered(int64) error
}

// Executor is implemented by *LocalBackend
type Executor interface {
	MoveEmail(messageID, from, to string) error
	ArchiveEmail(messageID, folder string) error
	DeleteEmail(messageID, folder string) error
}

// Engine evaluates rules against incoming emails and executes their actions.
type Engine struct {
	store    Store
	executor Executor
	ai       ai.AIClient
}

// New creates a new Engine.
func New(store Store, executor Executor, classifier ai.AIClient) *Engine {
	return &Engine{store: store, executor: executor, ai: classifier}
}

// EvaluateEmail checks all enabled rules against the email and fires matching ones.
// Returns the number of rules fired and the first error encountered.
func (e *Engine) EvaluateEmail(email *models.EmailData, category string) (int, error) {
	rules, err := e.store.GetEnabledRules()
	if err != nil {
		return 0, fmt.Errorf("get enabled rules: %w", err)
	}

	fired := 0
	var firstErr error
	for _, rule := range rules {
		if !MatchRule(rule, email, category) {
			continue
		}

		// Build base RuleContext
		ctx := models.RuleContext{
			Sender:    email.Sender,
			Domain:    extractDomain(email.Sender),
			Subject:   email.Subject,
			Category:  category,
			MessageID: email.MessageID,
			Folder:    email.Folder,
		}

		// Run optional custom prompt — best-effort; failure does not block actions.
		if rule.CustomPromptID != nil && e.ai != nil {
			prompt, err := e.store.GetCustomPrompt(*rule.CustomPromptID)
			if err == nil && prompt != nil {
				result, execErr := e.runCustomPrompt(prompt, email)
				if execErr == nil {
					_ = e.store.SaveCustomCategory(email.MessageID, *rule.CustomPromptID, result)
					ctx.PromptResult = result
				}
			}
		}

		// Execute actions
		for _, action := range rule.Actions {
			actionErr := e.executeAction(action, email, ctx)
			status := "ok"
			detail := ""
			if actionErr != nil {
				status = "error"
				detail = actionErr.Error()
				if firstErr == nil {
					firstErr = actionErr
				}
			}
			_ = e.store.AppendActionLog(&models.RuleActionLogEntry{
				RuleID:     rule.ID,
				MessageID:  email.MessageID,
				ActionType: action.Type,
				Status:     status,
				Detail:     detail,
				ExecutedAt: time.Now(),
			})
		}
		_ = e.store.TouchRuleLastTriggered(rule.ID)
		fired++
	}
	return fired, firstErr
}

func (e *Engine) executeAction(action models.RuleAction, email *models.EmailData, ctx models.RuleContext) error {
	switch action.Type {
	case models.ActionMove:
		return e.executor.MoveEmail(email.MessageID, email.Folder, action.DestFolder)
	case models.ActionArchive:
		return e.executor.ArchiveEmail(email.MessageID, email.Folder)
	case models.ActionDelete:
		return e.executor.DeleteEmail(email.MessageID, email.Folder)
	case models.ActionNotify:
		title := renderTemplate(action.NotifyTitle, ctx)
		body := renderTemplate(action.NotifyBody, ctx)
		return notify(title, body)
	case models.ActionWebhook:
		return webhook(action.WebhookURL, action.WebhookBody, action.Headers, ctx)
	case models.ActionCommand:
		return runCommand(action.Command, ctx)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

// MatchRule returns true if the email matches the rule's trigger.
func MatchRule(r *models.Rule, email *models.EmailData, category string) bool {
	switch r.TriggerType {
	case models.TriggerSender:
		return strings.EqualFold(email.Sender, r.TriggerValue)
	case models.TriggerDomain:
		return strings.EqualFold(extractDomain(email.Sender), r.TriggerValue)
	case models.TriggerCategory:
		return strings.EqualFold(category, r.TriggerValue)
	default:
		return false
	}
}

// extractDomain extracts the domain part from an email address (e.g. "foo@example.com" → "example.com").
// Handles display names like "Name <addr@domain.com>". Returns empty string if no @ is found.
func extractDomain(sender string) string {
	// Strip display name: "Name <addr@domain.com>" → "addr@domain.com"
	if lt := strings.LastIndex(sender, "<"); lt >= 0 {
		if gt := strings.Index(sender[lt:], ">"); gt >= 0 {
			sender = sender[lt+1 : lt+gt]
		}
	}
	at := strings.LastIndex(sender, "@")
	if at < 0 {
		return ""
	}
	return strings.ToLower(sender[at+1:])
}

// renderTemplate executes a Go text/template with the given context.
// Returns the input string unchanged if the template fails to parse or execute.
func renderTemplate(tmpl string, ctx models.RuleContext) string {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return tmpl
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return tmpl
	}
	return buf.String()
}

// promptData is the template data struct used when expanding a CustomPrompt's UserTemplate.
type promptData struct {
	Subject string
	Sender  string
	Body    string
}

// RunCustomPromptForEmail expands the prompt's UserTemplate with email data and
// calls the AI client. It is used by both the rules engine and the MCP server.
func RunCustomPromptForEmail(aiClient ai.AIClient, prompt *models.CustomPrompt, sender, subject string) (string, error) {
	tmplText := prompt.UserTemplate
	if strings.TrimSpace(tmplText) == "" {
		tmplText = "Email from: {{.Sender}}\nSubject: {{.Subject}}"
	}

	t, err := template.New("prompt").Parse(tmplText)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf strings.Builder
	data := struct {
		Sender  string
		Subject string
		Body    string
	}{Sender: sender, Subject: subject, Body: ""}
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	expanded := buf.String()

	msgs := []ai.ChatMessage{}
	if strings.TrimSpace(prompt.SystemText) != "" {
		msgs = append(msgs, ai.ChatMessage{Role: "system", Content: prompt.SystemText})
	}
	msgs = append(msgs, ai.ChatMessage{Role: "user", Content: expanded})
	return aiClient.Chat(msgs)
}

// runCustomPrompt expands the prompt's UserTemplate with email fields,
// calls the AI Chat endpoint, and returns the raw response string.
func (e *Engine) runCustomPrompt(prompt *models.CustomPrompt, email *models.EmailData) (string, error) {
	return RunCustomPromptForEmail(e.ai, prompt, email.Sender, email.Subject)
}
