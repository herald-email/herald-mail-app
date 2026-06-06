package agent

import "context"

const (
	TimelineModeExplicitIDs = "explicit_ids"
	TimelineModeKeyword     = "keyword"
	TimelineModeSemantic    = "semantic"
	TimelineModeHybrid      = "hybrid"
)

// Runner executes one UI chat-agent turn and returns typed output for the TUI.
type Runner interface {
	Run(ctx context.Context, input ChatInput) (ChatResult, error)
}

type ChatInput struct {
	UserMessage     string
	CurrentFolder   string
	ActiveTab       string
	VisibleIDs      []string
	SelectedIDs     []string
	ComposeSnapshot *ComposeSnapshot
	History         []ChatTurn
}

type ChatTurn struct {
	Role    string
	Content string
}

type ComposeSnapshot struct {
	To      string
	CC      string
	BCC     string
	Subject string
	Body    string
}

type ChatResult struct {
	Reply    string          `json:"reply"`
	Timeline *TimelineIntent `json:"timeline,omitempty"`
	Summary  *EmailSummary   `json:"summary,omitempty"`
	Compose  *ComposeIntent  `json:"compose,omitempty"`
}

type TimelineIntent struct {
	Mode       string   `json:"mode"`
	Query      string   `json:"query,omitempty"`
	MessageIDs []string `json:"message_ids,omitempty"`
	Label      string   `json:"label"`
}

type EmailSummary struct {
	Topic         string   `json:"topic,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	People        []Person `json:"people,omitempty"`
	Dates         []string `json:"dates,omitempty"`
	ActionItems   []string `json:"action_items,omitempty"`
	OpenQuestions []string `json:"open_questions,omitempty"`
	CitedIDs      []string `json:"cited_ids,omitempty"`
}

type Person struct {
	NameOrEmail string `json:"name_or_email"`
	Role        string `json:"role,omitempty"`
	EvidenceID  string `json:"evidence_id,omitempty"`
}

type ComposeIntent struct {
	SubjectSuggestion string `json:"subject_suggestion,omitempty"`
	BodySuggestion    string `json:"body_suggestion,omitempty"`
	Rationale         string `json:"rationale,omitempty"`
}
