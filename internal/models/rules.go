package models

import "time"

type RuleTriggerType string

const (
	TriggerSender   RuleTriggerType = "sender"
	TriggerDomain   RuleTriggerType = "domain"
	TriggerCategory RuleTriggerType = "category"
)

type RuleActionType string

const (
	ActionNotify  RuleActionType = "notify"
	ActionMove    RuleActionType = "move"
	ActionWebhook RuleActionType = "webhook"
	ActionCommand RuleActionType = "command"
	ActionArchive RuleActionType = "archive"
	ActionDelete  RuleActionType = "delete"
)

type RuleAction struct {
	Type        RuleActionType    `json:"type"`
	DestFolder  string            `json:"dest_folder,omitempty"`
	WebhookURL  string            `json:"webhook_url,omitempty"`
	WebhookBody string            `json:"webhook_body,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Command     string            `json:"command,omitempty"`
	NotifyTitle string            `json:"notify_title,omitempty"`
	NotifyBody  string            `json:"notify_body,omitempty"`
}

type Rule struct {
	ID             int64
	Name           string
	Enabled        bool
	Priority       int
	TriggerType    RuleTriggerType
	TriggerValue   string
	CustomPromptID *int64
	Actions        []RuleAction
	CreatedAt      time.Time
	LastTriggered  *time.Time
}

type CustomPrompt struct {
	ID           int64
	Name         string
	SystemText   string
	UserTemplate string
	OutputVar    string
	CreatedAt    time.Time
}

type RuleContext struct {
	Sender       string
	Domain       string
	Subject      string
	Category     string
	PromptResult string
	MessageID    string
	Folder       string
}

type RuleRequest struct {
	Email    *EmailData
	Category string
}

type RuleResult struct {
	MessageID  string
	RulesFired int
	Err        error
}

type RuleActionLogEntry struct {
	RuleID     int64
	MessageID  string
	ActionType RuleActionType
	Status     string // "ok" | "error"
	Detail     string
	ExecutedAt time.Time
}
