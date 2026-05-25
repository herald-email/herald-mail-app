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

type AutomationEventKind string

const (
	AutomationEventMailMessageReceived  AutomationEventKind = "mail_message_received"
	AutomationEventCalendarEventChanged AutomationEventKind = "calendar_event_changed"
)

type AutomationEvent struct {
	Kind       AutomationEventKind `json:"kind"`
	SourceID   SourceID            `json:"source_id,omitempty"`
	AccountID  AccountID           `json:"account_id,omitempty"`
	Collection CollectionRef       `json:"collection,omitempty"`

	Email      *EmailData     `json:"email,omitempty"`
	MessageRef MessageRef     `json:"message_ref,omitempty"`
	Category   string         `json:"category,omitempty"`
	Event      *CalendarEvent `json:"event,omitempty"`
	EventRef   EventRef       `json:"event_ref,omitempty"`
}

func NewMailAutomationEvent(email *EmailData, category string) AutomationEvent {
	return AutomationEvent{
		Kind:     AutomationEventMailMessageReceived,
		Email:    email,
		Category: category,
	}.WithDefaults()
}

func NewCalendarAutomationEvent(event CalendarEvent) AutomationEvent {
	return AutomationEvent{
		Kind:  AutomationEventCalendarEventChanged,
		Event: &event,
	}.WithDefaults()
}

func (e AutomationEvent) WithDefaults() AutomationEvent {
	if e.Kind == "" {
		if e.Event != nil || e.EventRef.EventID != "" || e.EventRef.LocalID != "" {
			e.Kind = AutomationEventCalendarEventChanged
		} else {
			e.Kind = AutomationEventMailMessageReceived
		}
	}

	switch e.Kind {
	case AutomationEventCalendarEventChanged:
		ref := e.EventRef
		if e.Event != nil {
			ref = e.Event.EventRef()
		}
		if e.SourceID != "" {
			ref.SourceID = e.SourceID
		}
		if e.AccountID != "" {
			ref.AccountID = e.AccountID
		}
		ref = ref.WithDefaults()
		e.EventRef = ref
		e.SourceID = ref.SourceID
		e.AccountID = ref.AccountID
		if e.Collection.Kind == "" {
			e.Collection = CollectionRef{
				SourceID:     ref.SourceID,
				AccountID:    ref.AccountID,
				Kind:         SourceKindCalendar,
				CollectionID: ref.CalendarID,
				DisplayName:  ref.CalendarID,
			}
		}
	default:
		ref := e.MessageRef
		if e.Email != nil {
			ref = e.Email.MessageRef()
		}
		if e.SourceID != "" {
			ref.SourceID = e.SourceID
		}
		if e.AccountID != "" {
			ref.AccountID = e.AccountID
		}
		ref = ref.WithDefaults()
		e.MessageRef = ref
		e.SourceID = ref.SourceID
		e.AccountID = ref.AccountID
		if e.Collection.Kind == "" {
			e.Collection = CollectionRef{
				SourceID:     ref.SourceID,
				AccountID:    ref.AccountID,
				Kind:         SourceKindMail,
				CollectionID: ref.Folder,
				DisplayName:  ref.Folder,
			}
		}
	}
	return e
}

func (e AutomationEvent) ItemID() string {
	e = e.WithDefaults()
	switch e.Kind {
	case AutomationEventCalendarEventChanged:
		if e.EventRef.LocalID != "" {
			return e.EventRef.LocalID
		}
		return e.EventRef.EventID
	default:
		if e.MessageRef.LocalID != "" {
			return e.MessageRef.LocalID
		}
		return e.MessageRef.MessageID
	}
}

func (e AutomationEvent) SourceKey() string {
	e = e.WithDefaults()
	return string(e.SourceID)
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

type RuleRequest = AutomationEvent

type RuleResult struct {
	Kind       AutomationEventKind
	SourceID   SourceID
	AccountID  AccountID
	ItemID     string
	MessageID  string
	EventID    string
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
	SourceID  SourceID  `json:"source_id,omitempty"`
	AccountID AccountID `json:"account_id,omitempty"`
	LocalID   string    `json:"local_id,omitempty"`
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
