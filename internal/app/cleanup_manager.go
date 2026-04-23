package app

import (
	"fmt"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/backend"
	"mail-processor/internal/models"
)

// CleanupManagerOpenMsg opens the cleanup manager overlay.
type CleanupManagerOpenMsg struct{}

// CleanupManagerCloseMsg closes the cleanup manager overlay.
type CleanupManagerCloseMsg struct{}

// CleanupRunNowMsg triggers immediate execution of cleanup rules.
// RuleID == 0 means run all rules.
type CleanupRunNowMsg struct{ RuleID int64 }

// cleanupManagerState tracks the sub-state of the cleanup manager overlay.
type cleanupManagerState int

const (
	cleanupManagerList cleanupManagerState = iota
	cleanupManagerEdit
)

// CleanupManager is a self-contained TUI overlay for managing cleanup rules.
type CleanupManager struct {
	state   cleanupManagerState
	rules   []*models.CleanupRule
	cursor  int
	form    *huh.Form
	editing *models.CleanupRule
	backend backend.Backend
	width   int
	height  int

	// form backing fields
	formName          string
	formMatchType     string
	formMatchValue    string
	formAction        string
	formOlderThanDays string
	formEnabled       bool
}

// NewCleanupManager creates a new CleanupManager overlay.
func NewCleanupManager(b backend.Backend, width, height int) *CleanupManager {
	return &CleanupManager{
		backend: b,
		width:   width,
		height:  height,
	}
}

// Init loads rules from the backend.
func (m *CleanupManager) Init() tea.Cmd {
	return func() tea.Msg {
		rules, err := m.backend.GetAllCleanupRules()
		if err != nil {
			return nil
		}
		m.rules = rules
		return nil
	}
}

// buildForm constructs the huh form for creating/editing a rule.
func (m *CleanupManager) buildForm() {
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Rule name").
				Value(&m.formName),
			huh.NewSelect[string]().
				Title("Match type").
				Options(
					huh.NewOption("Sender address", "sender"),
					huh.NewOption("Sender domain", "domain"),
				).
				Value(&m.formMatchType),
			huh.NewInput().
				Title("Match value").
				Description("e.g. newsletter@example.com or example.com").
				Value(&m.formMatchValue),
			huh.NewSelect[string]().
				Title("Action").
				Options(
					huh.NewOption("Delete", "delete"),
					huh.NewOption("Archive", "archive"),
				).
				Value(&m.formAction),
			huh.NewInput().
				Title("Older than (days)").
				Description("Only affect emails older than this many days").
				Value(&m.formOlderThanDays),
			huh.NewConfirm().
				Title("Enabled?").
				Value(&m.formEnabled),
		),
	).WithWidth(m.width - 4)
}

// Update handles keys and form events.
func (m *CleanupManager) Update(msg tea.Msg) (*CleanupManager, tea.Cmd) {
	switch m.state {
	case cleanupManagerList:
		return m.updateList(msg)
	case cleanupManagerEdit:
		return m.updateEdit(msg)
	}
	return m, nil
}

func (m *CleanupManager) updateList(msg tea.Msg) (*CleanupManager, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return CleanupManagerCloseMsg{} }

		case "n":
			// Create new rule
			m.editing = &models.CleanupRule{
				MatchType:     "sender",
				Action:        "delete",
				OlderThanDays: 30,
				Enabled:       true,
				CreatedAt:     time.Now(),
			}
			m.formName = ""
			m.formMatchType = "sender"
			m.formMatchValue = ""
			m.formAction = "delete"
			m.formOlderThanDays = "30"
			m.formEnabled = true
			m.buildForm()
			m.state = cleanupManagerEdit
			return m, m.form.Init()

		case "enter":
			if len(m.rules) == 0 {
				return m, nil
			}
			if m.cursor >= len(m.rules) {
				m.cursor = len(m.rules) - 1
			}
			rule := m.rules[m.cursor]
			m.editing = rule
			m.formName = rule.Name
			m.formMatchType = rule.MatchType
			m.formMatchValue = rule.MatchValue
			m.formAction = rule.Action
			m.formOlderThanDays = strconv.Itoa(rule.OlderThanDays)
			m.formEnabled = rule.Enabled
			m.buildForm()
			m.state = cleanupManagerEdit
			return m, m.form.Init()

		case "d", "D":
			if len(m.rules) == 0 {
				return m, nil
			}
			if m.cursor >= len(m.rules) {
				m.cursor = len(m.rules) - 1
			}
			id := m.rules[m.cursor].ID
			_ = m.backend.DeleteCleanupRule(id)
			rules, _ := m.backend.GetAllCleanupRules()
			m.rules = rules
			if m.cursor >= len(m.rules) && m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "r":
			return m, func() tea.Msg { return CleanupRunNowMsg{RuleID: 0} }

		case "j", "down":
			if m.cursor < len(m.rules)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

func (m *CleanupManager) updateEdit(msg tea.Msg) (*CleanupManager, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.state = cleanupManagerList
			m.form = nil
			m.editing = nil
			return m, nil
		}
	}

	if m.form == nil {
		return m, nil
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		// Save the rule
		rule := m.editing
		rule.Name = m.formName
		rule.MatchType = m.formMatchType
		rule.MatchValue = m.formMatchValue
		rule.Action = m.formAction
		days, _ := strconv.Atoi(m.formOlderThanDays)
		if days <= 0 {
			days = 30
		}
		rule.OlderThanDays = days
		rule.Enabled = m.formEnabled
		if rule.CreatedAt.IsZero() {
			rule.CreatedAt = time.Now()
		}
		_ = m.backend.SaveCleanupRule(rule)

		// Reload rules
		rules, _ := m.backend.GetAllCleanupRules()
		m.rules = rules
		m.state = cleanupManagerList
		m.form = nil
		m.editing = nil
	}

	return m, cmd
}

// View renders the overlay.
func (m *CleanupManager) View() string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(m.width - 4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	switch m.state {
	case cleanupManagerList:
		return m.viewList(borderStyle, titleStyle)
	case cleanupManagerEdit:
		return m.viewEdit(borderStyle, titleStyle)
	}
	return ""
}

func (m *CleanupManager) viewList(borderStyle, titleStyle lipgloss.Style) string {
	var content string
	content += titleStyle.Render("Auto-Cleanup Rules") + "\n\n"
	innerW := m.width - 10
	if innerW < 20 {
		innerW = m.width
	}
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MaxWidth(innerW)
	content += infoStyle.Render("Runs on demand or on schedule. Saved cleanup rules live here.") + "\n"
	content += infoStyle.Render("Results show up in the status bar and by mail being archived or deleted after a run.") + "\n\n"

	if len(m.rules) == 0 {
		content += lipgloss.NewStyle().Foreground(lipgloss.Color("243")).
			Render("No rules yet. Press n to create one. Reopen C any time to review saved cleanup rules.") + "\n"
	} else {
		for i, rule := range m.rules {
			line := fmt.Sprintf("%-20s  %-8s  %-30s  %-7s  %d days",
				cleanupTruncate(rule.Name, 20),
				rule.MatchType,
				cleanupTruncate(rule.MatchValue, 30),
				rule.Action,
				rule.OlderThanDays,
			)
			if !rule.Enabled {
				line += "  [disabled]"
			}
			if rule.LastRun != nil {
				line += fmt.Sprintf("  last: %s", rule.LastRun.Format("2006-01-02"))
			}

			if i == m.cursor {
				line = lipgloss.NewStyle().
					Foreground(lipgloss.Color("229")).
					Background(lipgloss.Color("57")).
					Render(line)
			}
			content += line + "\n"
		}
	}

	content += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("243")).
		Render("n: new  enter: edit  d: delete  r: run all  esc: close")

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		borderStyle.Render(content))
}

func (m *CleanupManager) viewEdit(borderStyle, titleStyle lipgloss.Style) string {
	title := "New Cleanup Rule"
	if m.editing != nil && m.editing.ID != 0 {
		title = "Edit Cleanup Rule"
	}
	var content string
	content += titleStyle.Render(title) + "\n\n"
	innerW := m.width - 10
	if innerW < 20 {
		innerW = m.width
	}
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MaxWidth(innerW)
	content += infoStyle.Render("This rule targets older sender/domain mail. Save it here, then run it from the manager or let the configured schedule pick it up.") + "\n\n"
	if m.form != nil {
		content += m.form.View()
	}
	content += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("243")).
		Render("esc: cancel")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		borderStyle.Render(content))
}

// cleanupTruncate shortens s to max n runes, adding "..." if truncated.
func cleanupTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 3 {
		return string(r[:n])
	}
	return string(r[:n-3]) + "..."
}
