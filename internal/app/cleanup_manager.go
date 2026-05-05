package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// CleanupManagerOpenMsg opens the cleanup manager overlay.
type CleanupManagerOpenMsg struct{}

// CleanupManagerCloseMsg closes the cleanup manager overlay.
type CleanupManagerCloseMsg struct{}

// CleanupRunNowMsg triggers immediate execution of cleanup rules.
// RuleID == 0 means run all rules.
type CleanupRunNowMsg struct{ RuleID int64 }

// CleanupDryRunMsg opens a structured dry-run preview for cleanup rules.
// RuleID == 0 means preview all enabled rules.
type CleanupDryRunMsg struct {
	RuleID      int64
	CleanupRule *models.CleanupRule
}

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
	).WithWidth(m.formWidth()).WithHeight(m.formHeight())
}

// Update handles keys and form events.
func (m *CleanupManager) Update(msg tea.Msg) (*CleanupManager, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.setSize(sizeMsg.Width, sizeMsg.Height)
		return m, nil
	}

	switch m.state {
	case cleanupManagerList:
		return m.updateList(msg)
	case cleanupManagerEdit:
		return m.updateEdit(msg)
	}
	return m, nil
}

func (m *CleanupManager) setSize(width, height int) {
	m.width = width
	m.height = height
	if m.form != nil {
		m.form = m.form.WithWidth(m.formWidth()).WithHeight(m.formHeight())
	}
}

func (m *CleanupManager) panelLayout() compactOverlayLayout {
	return newCompactOverlayLayout(m.width, m.height)
}

func (m *CleanupManager) formWidth() int {
	return m.panelLayout().contentWidth
}

func (m *CleanupManager) formHeight() int {
	h := m.panelLayout().contentHeight - 5
	if h < 4 {
		h = 4
	}
	return h
}

func (m *CleanupManager) updateList(msg tea.Msg) (*CleanupManager, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch shortcutKey(msg) {
		case "esc":
			return m, func() tea.Msg { return CleanupManagerCloseMsg{} }

		case "n":
			// Create new rule
			m.editing = &models.CleanupRule{
				MatchType:     "sender",
				Action:        "delete",
				OlderThanDays: 30,
				Enabled:       false,
				CreatedAt:     time.Now(),
			}
			m.formName = ""
			m.formMatchType = "sender"
			m.formMatchValue = ""
			m.formAction = "delete"
			m.formOlderThanDays = "30"
			m.formEnabled = false
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
			rule := *m.rules[m.cursor]
			m.editing = &rule
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
			return m, func() tea.Msg { return CleanupDryRunMsg{RuleID: 0} }

		case "p":
			if len(m.rules) == 0 {
				return m, nil
			}
			if m.cursor >= len(m.rules) {
				m.cursor = len(m.rules) - 1
			}
			return m, func() tea.Msg { return CleanupDryRunMsg{RuleID: m.rules[m.cursor].ID} }

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
	case tea.KeyPressMsg:
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
		rule.Enabled = false
		m.state = cleanupManagerList
		m.form = nil
		m.editing = nil
		return m, func() tea.Msg { return CleanupDryRunMsg{CleanupRule: rule} }
	}

	return m, cmd
}

// View renders the overlay.
func (m *CleanupManager) View() tea.View {
	if m.width > 0 && (m.width < minTermWidth || m.height < minTermHeight) {
		return newHeraldView(renderMinSizeMessage(m.width, m.height))
	}
	return newHeraldView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderPanel()))
}

func (m *CleanupManager) renderPanel() string {
	switch m.state {
	case cleanupManagerEdit:
		return m.renderEditPanel()
	default:
		return m.renderListPanel()
	}
}

func (m *CleanupManager) renderListPanel() string {
	layout := m.panelLayout()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MaxWidth(layout.contentWidth)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	lines := []string{
		titleStyle.Render("Auto-Cleanup Rules"),
		"",
		infoStyle.Render("Runs on demand or on schedule. Saved cleanup rules live here."),
		infoStyle.Render("Run results appear in the status bar and as archive/delete changes."),
		"",
	}

	if len(m.rules) == 0 {
		lines = append(lines, dimStyle.Render("No rules yet. Press n to create one. Reopen C any time to review saved cleanup rules."))
	} else {
		rowsAvailable := layout.contentHeight - len(lines) - 2
		if rowsAvailable < 1 {
			rowsAvailable = 1
		}
		start := m.cursor - rowsAvailable + 1
		if start < 0 {
			start = 0
		}
		end := start + rowsAvailable
		if end > len(m.rules) {
			end = len(m.rules)
		}
		for i := start; i < end; i++ {
			line := m.formatRuleLine(m.rules[i], layout.contentWidth)
			if i == m.cursor {
				line = selectedStyle.Render(line)
			}
			lines = append(lines, line)
		}
	}

	lines = append(lines, "", dimStyle.Render(truncateVisual("n: new  enter: edit  d: delete  p: preview  r: preview all  esc: close", layout.contentWidth)))
	return renderCompactOverlayBox(strings.Join(lines, "\n"), layout)
}

func (m *CleanupManager) formatRuleLine(rule *models.CleanupRule, width int) string {
	if rule == nil {
		return truncateVisual("(unknown cleanup rule)", width)
	}
	if width < 74 {
		line := fmt.Sprintf("%s  %s  %s  %dd",
			cleanupTruncate(rule.Name, 18),
			rule.Action,
			cleanupTruncate(rule.MatchValue, 24),
			rule.OlderThanDays,
		)
		if !rule.Enabled {
			line += "  [off]"
		}
		return truncateVisual(line, width)
	}
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
	return truncateVisual(line, width)
}

func (m *CleanupManager) renderEditPanel() string {
	layout := m.panelLayout()
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	title := "New Cleanup Rule"
	if m.editing != nil && m.editing.ID != 0 {
		title = "Edit Cleanup Rule"
	}
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MaxWidth(layout.contentWidth)
	var content strings.Builder
	content.WriteString(titleStyle.Render(title) + "\n\n")
	content.WriteString(infoStyle.Render("This rule targets older sender/domain mail. Save it here, then run it from the manager or let the configured schedule pick it up.") + "\n\n")
	if m.form != nil {
		content.WriteString(strings.TrimRight(m.form.View(), "\n"))
	}
	content.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("esc: cancel"))
	return renderCompactOverlayBox(content.String(), layout)
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
