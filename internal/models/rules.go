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
	FiredCount int
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

type RuleDryRunKind string

const (
	RuleDryRunKindAutomation RuleDryRunKind = "automation"
	RuleDryRunKindCleanup    RuleDryRunKind = "cleanup"
)

// RuleDryRunRequest describes the preview scope for rule planning. RuleID==0
// means all supplied rules. AllFolders=true means Folder is only descriptive.
type RuleDryRunRequest struct {
	Kind            RuleDryRunKind `json:"kind"`
	RuleID          int64          `json:"rule_id,omitempty"`
	Folder          string         `json:"folder,omitempty"`
	AllFolders      bool           `json:"all_folders,omitempty"`
	IncludeDisabled bool           `json:"include_disabled,omitempty"`
	Rule            *Rule          `json:"rule,omitempty"`
	CleanupRule     *CleanupRule   `json:"cleanup_rule,omitempty"`
}

// RuleDryRunReport is the structured, side-effect-free preview returned to UI,
// daemon, and MCP callers before live rule actions are allowed.
type RuleDryRunReport struct {
	Kind        RuleDryRunKind  `json:"kind"`
	Scope       string          `json:"scope"`
	Folder      string          `json:"folder,omitempty"`
	RuleCount   int             `json:"rule_count"`
	MatchCount  int             `json:"match_count"`
	ActionCount int             `json:"action_count"`
	DryRun      bool            `json:"dry_run"`
	GeneratedAt time.Time       `json:"generated_at"`
	Rows        []RuleDryRunRow `json:"rows"`
}

type RuleDryRunRow struct {
	RuleID    int64     `json:"rule_id"`
	RuleName  string    `json:"rule_name"`
	MessageID string    `json:"message_id"`
	Sender    string    `json:"sender"`
	Domain    string    `json:"domain"`
	Category  string    `json:"category,omitempty"`
	Folder    string    `json:"folder"`
	Subject   string    `json:"subject"`
	Date      time.Time `json:"date"`
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
}
