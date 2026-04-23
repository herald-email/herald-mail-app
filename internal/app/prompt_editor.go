package app

import (
	"errors"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/models"
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
			Description("Instructions for the AI. Use {{.Sender}}, {{.Subject}}, {{.Body}} as placeholders.").
			Lines(6).
			Value(&p.systemText),
	)

	// Group 3 — User Template
	userGroup := huh.NewGroup(
		huh.NewText().
			Title("User template").
			Description("Message sent to the model per email. Use {{.Sender}}, {{.Subject}}, {{.Body}} as placeholders.").
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
	h := p.height - 12
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

	case tea.KeyMsg:
		if msg.Type == tea.KeyEscape {
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
func (p *PromptEditor) View() string {
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
	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MaxWidth(innerW)
	rendered := box.Render(
		header + "\n\n" +
			noteStyle.Render("Purpose: reusable AI instructions.") + "\n" +
			noteStyle.Render("Results: a rule or MCP tool must use the prompt.") + "\n" +
			noteStyle.Render(p.savedPromptsSummary()) + "\n\n" +
			formView,
	)
	return lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Center, rendered)
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
