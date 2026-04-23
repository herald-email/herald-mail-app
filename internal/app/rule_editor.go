package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/models"
)

// RuleEditorDoneMsg is sent when the user completes the rule editor form.
type RuleEditorDoneMsg struct{ Rule *models.Rule }

// RuleEditorCancelledMsg is sent when the user cancels the rule editor form.
type RuleEditorCancelledMsg struct{}

// RuleEditor is a self-contained huh-based form component for creating email rules.
type RuleEditor struct {
	form   *huh.Form
	width  int
	height int
	done   bool // set once we've emitted the completion message

	// pre-filled context
	senderHint string
	domainHint string

	// backing variables — trigger
	triggerType  string // "sender" | "domain" | "category"
	triggerValue string

	// backing variables — actions
	selectedActions []string // multi-select: "notify", "move", "archive", "delete", "webhook", "command"

	// backing variables — action details
	destFolder   string
	webhookURL   string
	webhookBody  string
	shellCommand string
	notifyTitle  string
	notifyBody   string
}

// NewRuleEditor creates a RuleEditor pre-filled with the given sender/domain context.
func NewRuleEditor(sender, domain string, width, height int) *RuleEditor {
	r := &RuleEditor{
		senderHint:   sender,
		domainHint:   domain,
		triggerValue: sender,
		triggerType:  "sender",
		width:        width,
		height:       height,
	}
	r.buildForm()
	return r
}

// buildForm constructs the huh.Form with three groups.
func (r *RuleEditor) buildForm() {
	// Group 1 — Trigger
	triggerGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Trigger type").
			Options(
				huh.NewOption("Sender address", "sender"),
				huh.NewOption("Sender domain", "domain"),
				huh.NewOption("AI category", "category"),
			).
			Value(&r.triggerType),
		huh.NewInput().
			Title("Trigger value").
			Description("e.g. newsletter@example.com, example.com, or Newsletters").
			Value(&r.triggerValue),
	)

	// Group 2 — Actions
	actionsGroup := huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Actions to perform").
			Options(
				huh.NewOption("Desktop notification", "notify"),
				huh.NewOption("Move to folder", "move"),
				huh.NewOption("Archive", "archive"),
				huh.NewOption("Delete", "delete"),
				huh.NewOption("Webhook (POST)", "webhook"),
				huh.NewOption("Shell command", "command"),
			).
			Value(&r.selectedActions),
	)

	// Group 3 — Action details
	detailsGroup := huh.NewGroup(
		huh.NewInput().Title("Move to folder").Value(&r.destFolder),
		huh.NewInput().Title("Webhook URL").Value(&r.webhookURL),
		huh.NewInput().
			Title("Webhook body template").
			Description("Go template: {{.Sender}} {{.Subject}} etc").
			Value(&r.webhookBody),
		huh.NewInput().
			Title("Shell command").
			Description("sh -c <cmd>; env vars: HERALD_SENDER, HERALD_SUBJECT etc").
			Value(&r.shellCommand),
		huh.NewInput().Title("Notify title").Value(&r.notifyTitle),
		huh.NewInput().Title("Notify body").Value(&r.notifyBody),
	)

	r.form = huh.NewForm(triggerGroup, actionsGroup, detailsGroup).
		WithShowHelp(true).
		WithShowErrors(true)

	if r.width > 0 {
		r.form = r.form.WithWidth(r.formWidth())
	}
	if r.height > 0 {
		r.form = r.form.WithHeight(r.formHeight())
	}
}

// formWidth returns the width the form should use (80% of terminal, min 40).
func (r *RuleEditor) formWidth() int {
	w := int(float64(r.width) * 0.8)
	if w < 40 {
		w = 40
	}
	if w > r.width {
		w = r.width
	}
	return w
}

func (r *RuleEditor) formHeight() int {
	h := r.height - 5
	if h < 10 {
		h = 10
	}
	if r.height > 0 && h > r.height {
		h = r.height
	}
	return h
}

// Init implements tea.Model.
func (r *RuleEditor) Init() tea.Cmd {
	return r.form.Init()
}

// Update implements tea.Model.
func (r *RuleEditor) Update(msg tea.Msg) (*RuleEditor, tea.Cmd) {
	if r.done {
		return r, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		r.form = r.form.WithWidth(r.formWidth()).WithHeight(r.formHeight())
		return r, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEscape {
			if r.form.State != huh.StateCompleted {
				r.done = true
				return r, func() tea.Msg { return RuleEditorCancelledMsg{} }
			}
		}
	}

	// Forward to the form.
	model, cmd := r.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		r.form = f
	}

	// Check if the form just completed.
	if r.form.State == huh.StateCompleted && !r.done {
		r.done = true
		rule := r.buildRule()
		return r, tea.Batch(cmd, func() tea.Msg {
			return RuleEditorDoneMsg{Rule: rule}
		})
	}

	// Check if the form was aborted (e.g. ctrl+c within the form).
	if r.form.State == huh.StateAborted && !r.done {
		r.done = true
		return r, tea.Batch(cmd, func() tea.Msg { return RuleEditorCancelledMsg{} })
	}

	return r, cmd
}

// View implements tea.Model.
func (r *RuleEditor) View() string {
	formView := r.form.View()

	w := r.formWidth()
	box := lipgloss.NewStyle().
		Width(w).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	rendered := box.Render(formView)
	return lipgloss.Place(r.width, r.height, lipgloss.Center, lipgloss.Center, rendered)
}

// buildRule constructs a models.Rule from the current form field values.
func (r *RuleEditor) buildRule() *models.Rule {
	rule := &models.Rule{
		Enabled:      true,
		TriggerType:  models.RuleTriggerType(r.triggerType),
		TriggerValue: r.triggerValue,
		Name:         r.triggerType + ": " + r.triggerValue,
	}
	for _, actionType := range r.selectedActions {
		action := models.RuleAction{Type: models.RuleActionType(actionType)}
		switch actionType {
		case "move":
			action.DestFolder = r.destFolder
		case "webhook":
			action.WebhookURL = r.webhookURL
			action.WebhookBody = r.webhookBody
		case "command":
			action.Command = r.shellCommand
		case "notify":
			action.NotifyTitle = r.notifyTitle
			action.NotifyBody = r.notifyBody
		}
		rule.Actions = append(rule.Actions, action)
	}
	return rule
}
