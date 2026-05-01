package app

import (
	"errors"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// PromptEditorDoneMsg is sent when the user completes the prompt editor form.
type PromptEditorDoneMsg struct{ Prompt *models.CustomPrompt }

// PromptEditorCancelledMsg is sent when the user cancels the prompt editor form.
type PromptEditorCancelledMsg struct{}

// PromptEditor is a self-contained huh-based form component for creating/editing custom prompts.
type PromptEditor struct {
	form   *huh.Form
	width  int
	height int
	done   bool // set once we've emitted the completion message

	// backing variables
	name         string
	systemText   string
	userTemplate string
	outputVar    string

	// non-zero when editing an existing prompt
	editingID    int64
	savedPrompts []*models.CustomPrompt
	savedErr     string
}

// NewPromptEditor creates a PromptEditor. Pass existing to pre-fill for editing; pass nil to create a new prompt.
func NewPromptEditor(existing *models.CustomPrompt, width, height int) *PromptEditor {
	p := &PromptEditor{
		width:  width,
		height: height,
	}
	if existing != nil {
		p.editingID = existing.ID
		p.name = existing.Name
		p.systemText = existing.SystemText
		p.userTemplate = existing.UserTemplate
		p.outputVar = existing.OutputVar
	}
	p.buildForm()
	return p
}

func (p *PromptEditor) WithSavedPrompts(prompts []*models.CustomPrompt, err error) *PromptEditor {
	p.savedPrompts = append([]*models.CustomPrompt(nil), prompts...)
	if err != nil {
		p.savedErr = err.Error()
	} else {
		p.savedErr = ""
	}
	return p
}

// buildForm constructs the huh.Form with three groups.
func (p *PromptEditor) buildForm() {
	// Group 1 — Identity
	identityGroup := huh.NewGroup(
		huh.NewInput().
			Title("Name").
			Description("e.g. Newsletter Detector").
			Validate(func(v string) error {
				if strings.TrimSpace(v) == "" {
					return errors.New("name is required")
				}
				return nil
			}).
			Value(&p.name),
		huh.NewInput().
			Title("Output variable").
			Description("e.g. newsletter_score, result").
			Value(&p.outputVar),
	)

	// Group 2 — System Prompt
	systemGroup := huh.NewGroup(
		huh.NewText().
			Title("System prompt").
			Description("Instructions for the AI. {{.Sender}} and {{.Subject}} are ready; {{.Body}} may be empty.").
			Lines(6).
			Value(&p.systemText),
	)

	// Group 3 — User Template
	userGroup := huh.NewGroup(
		huh.NewText().
			Title("User template").
			Description("Message sent to the model per email. Prefer {{.Sender}} and {{.Subject}} today.").
			Lines(6).
			Value(&p.userTemplate),
	)

	p.form = huh.NewForm(identityGroup, systemGroup, userGroup).
		WithShowHelp(true).
		WithShowErrors(true)

	if p.width > 0 {
		p.form = p.form.WithWidth(p.formWidth())
	}
	if p.height > 0 {
		p.form = p.form.WithHeight(p.formHeight())
	}
}

// formWidth returns the width the form should use (80% of terminal, min 40).
func (p *PromptEditor) formWidth() int {
	w := int(float64(p.width) * 0.8)
	if w < 40 {
		w = 40
	}
	if w > p.width {
		w = p.width
	}
	return w
}

func (p *PromptEditor) formHeight() int {
	h := p.height - 9 - p.guideHeight()
	if h < 6 {
		h = 6
	}
	if p.height > 0 && h > p.height {
		h = p.height
	}
	return h
}

// Init implements tea.Model.
func (p *PromptEditor) Init() tea.Cmd {
	return p.form.Init()
}

// Update implements tea.Model.
func (p *PromptEditor) Update(msg tea.Msg) (*PromptEditor, tea.Cmd) {
	if p.done {
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		p.form = p.form.WithWidth(p.formWidth()).WithHeight(p.formHeight())
		return p, nil

	case tea.KeyPressMsg:
		if msg.Code == tea.KeyEscape {
			if p.form.State != huh.StateCompleted {
				p.done = true
				return p, func() tea.Msg { return PromptEditorCancelledMsg{} }
			}
		}
	}

	// Forward to the form.
	model, cmd := p.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		p.form = f
	}

	// Check if the form just completed.
	if p.form.State == huh.StateCompleted && !p.done {
		p.done = true
		prompt := p.buildPrompt()
		if prompt == nil {
			// Name was empty — treat as cancellation.
			return p, tea.Batch(cmd, func() tea.Msg { return PromptEditorCancelledMsg{} })
		}
		return p, tea.Batch(cmd, func() tea.Msg {
			return PromptEditorDoneMsg{Prompt: prompt}
		})
	}

	// Check if the form was aborted (e.g. ctrl+c within the form).
	if p.form.State == huh.StateAborted && !p.done {
		p.done = true
		return p, tea.Batch(cmd, func() tea.Msg { return PromptEditorCancelledMsg{} })
	}

	return p, cmd
}

// View implements tea.Model.
func (p *PromptEditor) View() tea.View {
	if p.width > 0 && p.width < minTermWidth {
		return newHeraldView(renderMinSizeMessage(p.width, p.height))
	}
	if p.height > 0 && p.height < minTermHeight {
		return newHeraldView(renderMinSizeMessage(p.width, p.height))
	}

	formView := p.form.View()

	title := "New Custom Prompt"
	if p.editingID != 0 {
		title = "Edit Custom Prompt"
	}

	w := p.formWidth()
	innerW := w - 4
	if innerW < 20 {
		innerW = w
	}
	box := lipgloss.NewStyle().
		Width(w).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render(title)
	rendered := box.Render(
		header + "\n\n" +
			p.guideView(innerW) + "\n\n" +
			formView,
	)
	return newHeraldView(lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Center, rendered))
}

// buildPrompt constructs a models.CustomPrompt from the current form field values.
// Returns nil if the name is empty (indicating an invalid/cancelled submission).
func (p *PromptEditor) buildPrompt() *models.CustomPrompt {
	if p.name == "" {
		return nil
	}
	return &models.CustomPrompt{
		ID:           p.editingID,
		Name:         p.name,
		SystemText:   p.systemText,
		UserTemplate: p.userTemplate,
		OutputVar:    p.outputVar,
	}
}

func (p *PromptEditor) savedPromptsSummary() string {
	if p.savedErr != "" {
		return "Saved prompts: unavailable (" + p.savedErr + ")"
	}
	if len(p.savedPrompts) == 0 {
		return "Saved prompts: none yet. Reopen P after saving."
	}
	const maxShown = 3
	parts := make([]string, 0, maxShown)
	for i, prompt := range p.savedPrompts {
		if i >= maxShown {
			break
		}
		if prompt == nil {
			continue
		}
		part := strings.TrimSpace(prompt.Name)
		if strings.TrimSpace(prompt.OutputVar) != "" {
			part += " (" + prompt.OutputVar + ")"
		}
		parts = append(parts, part)
	}
	if len(p.savedPrompts) > maxShown {
		parts = append(parts, "+"+strconv.Itoa(len(p.savedPrompts)-maxShown)+" more")
	}
	return "Saved prompts: " + strings.Join(parts, " | ")
}

func (p *PromptEditor) guideHeight() int {
	if p.compactGuide() {
		return 6
	}
	return 14
}

func (p *PromptEditor) compactGuide() bool {
	return p.width < 110 || p.height < 32
}

func (p *PromptEditor) guideView(width int) string {
	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MaxWidth(width)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))

	if p.compactGuide() {
		lines := []string{
			"Purpose: reusable AI instructions.",
			"Saving does not run the prompt.",
			"Example: sender triage with {{.Sender}} and {{.Subject}}.",
			"Next: Use W to attach, or MCP classify_email_custom.",
			"Results are stored per email in custom category storage.",
			p.savedPromptsSummary(),
		}
		for i, line := range lines {
			lines[i] = noteStyle.Render(line)
		}
		return strings.Join(lines, "\n")
	}

	sections := []struct {
		label string
		body  string
	}{
		{
			label: "How it works",
			body:  "reusable AI instructions for one email at a time. Saving does not run the prompt.",
		},
		{
			label: "Example prompts",
			body:  "Example: sender triage -> label {{.Sender}} / {{.Subject}}.\nInvoice extractor -> return an invoice id from the subject.",
		},
		{
			label: "Template variables",
			body:  "{{.Sender}} and {{.Subject}} are ready. {{.Body}} may be empty in current rule and MCP runs.",
		},
		{
			label: "Next",
			body:  "Use W to attach this prompt to an automation rule.\nRun manually through MCP classify_email_custom.",
		},
		{
			label: "Results",
			body:  "Results are stored per email in custom category storage and can be read back through MCP.",
		},
		{
			label: "Saved prompts",
			body:  strings.TrimPrefix(p.savedPromptsSummary(), "Saved prompts: "),
		},
	}

	lines := make([]string, 0, len(sections)*2)
	for _, section := range sections {
		lines = append(lines, labelStyle.Render(section.label))
		lines = append(lines, noteStyle.Render(section.body))
	}
	return strings.Join(lines, "\n")
}
