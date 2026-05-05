package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type RuleDryRunPreviewMsg struct {
	Report         *models.RuleDryRunReport
	Rule           *models.Rule
	CleanupRequest models.RuleDryRunRequest
	Err            error
}

type ruleDryRunPreview struct {
	report             *models.RuleDryRunReport
	pendingRule        *models.Rule
	pendingCleanupRule *models.CleanupRule
	cleanupRequest     models.RuleDryRunRequest
	cursor             int
	confirmEnable      bool
	confirmRun         bool
	liveRunDisabled    bool
	err                string
}

func newAutomationDryRunPreview(report *models.RuleDryRunReport, rule *models.Rule, err error) *ruleDryRunPreview {
	p := &ruleDryRunPreview{report: report, pendingRule: rule}
	if err != nil {
		p.err = err.Error()
	}
	if p.report == nil {
		p.report = &models.RuleDryRunReport{Kind: models.RuleDryRunKindAutomation, DryRun: true}
	}
	return p
}

func newCleanupDryRunPreview(report *models.RuleDryRunReport, req models.RuleDryRunRequest, err error) *ruleDryRunPreview {
	p := &ruleDryRunPreview{report: report, pendingCleanupRule: req.CleanupRule, cleanupRequest: req}
	if err != nil {
		p.err = err.Error()
	}
	if p.report == nil {
		p.report = &models.RuleDryRunReport{Kind: models.RuleDryRunKindCleanup, DryRun: true}
	}
	return p
}

func (m *Model) previewAutomationRuleCmd(rule *models.Rule) tea.Cmd {
	if rule != nil {
		rule.Enabled = false
	}
	req := models.RuleDryRunRequest{
		Kind:            models.RuleDryRunKindAutomation,
		Folder:          m.currentFolder,
		IncludeDisabled: true,
		Rule:            rule,
	}
	return func() tea.Msg {
		report, err := m.backend.PreviewRulesDryRun(req)
		return RuleDryRunPreviewMsg{Report: report, Rule: rule, Err: err}
	}
}

func (m *Model) previewCleanupRulesCmd(req models.RuleDryRunRequest) tea.Cmd {
	req.Kind = models.RuleDryRunKindCleanup
	if req.Folder == "" {
		req.AllFolders = true
	}
	if req.RuleID != 0 {
		req.IncludeDisabled = true
	}
	return func() tea.Msg {
		report, err := m.backend.PreviewCleanupRulesDryRun(req)
		return RuleDryRunPreviewMsg{Report: report, CleanupRequest: req, Err: err}
	}
}

func (m *Model) handleDryRunPreviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.ruleDryRunPreview == nil {
		return m, nil, false
	}
	p := m.ruleDryRunPreview
	switch shortcutKey(msg) {
	case "esc":
		m.ruleDryRunPreview = nil
		return m, nil, true
	case "j", "down":
		if p.cursor < len(p.report.Rows)-1 {
			p.cursor++
		}
		return m, nil, true
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return m, nil, true
	case "s":
		if p.pendingRule == nil && p.pendingCleanupRule == nil {
			return m, nil, true
		}
		if p.pendingCleanupRule != nil {
			p.pendingCleanupRule.Enabled = false
			if err := m.backend.SaveCleanupRule(p.pendingCleanupRule); err != nil {
				m.statusMessage = "Error saving cleanup rule: " + err.Error()
				return m, nil, true
			}
			m.statusMessage = "Cleanup rule saved disabled: " + p.pendingCleanupRule.Name + ". Reopen C to review it."
			m.ruleDryRunPreview = nil
			m.showCleanupMgr = false
			m.cleanupManager = nil
			return m, nil, true
		}
		p.pendingRule.Enabled = false
		if err := m.backend.SaveRule(p.pendingRule); err != nil {
			m.statusMessage = "Error saving rule: " + err.Error()
			return m, nil, true
		}
		m.statusMessage = "Rule saved disabled: " + p.pendingRule.Name + ". Reopen W to review it."
		m.ruleDryRunPreview = nil
		return m, nil, true
	case "E":
		if p.pendingRule == nil && p.pendingCleanupRule == nil {
			return m, nil, true
		}
		if p.pendingCleanupRule != nil {
			if cleanupRuleNeedsEnableConfirmation(p.pendingCleanupRule) && !p.confirmEnable {
				p.confirmEnable = true
				p.confirmRun = false
				m.statusMessage = "Press E again to enable this cleanup rule after preview"
				return m, nil, true
			}
			p.pendingCleanupRule.Enabled = true
			if err := m.backend.SaveCleanupRule(p.pendingCleanupRule); err != nil {
				m.statusMessage = "Error saving cleanup rule: " + err.Error()
				return m, nil, true
			}
			m.statusMessage = "Cleanup rule enabled: " + p.pendingCleanupRule.Name + ". Reopen C to review saved cleanup rules."
			m.ruleDryRunPreview = nil
			m.showCleanupMgr = false
			m.cleanupManager = nil
			return m, nil, true
		}
		if ruleNeedsEnableConfirmation(p.pendingRule) && !p.confirmEnable {
			p.confirmEnable = true
			p.confirmRun = false
			m.statusMessage = "Press E again to enable this rule after preview"
			return m, nil, true
		}
		p.pendingRule.Enabled = true
		if err := m.backend.SaveRule(p.pendingRule); err != nil {
			m.statusMessage = "Error saving rule: " + err.Error()
			return m, nil, true
		}
		m.statusMessage = "Rule enabled: " + p.pendingRule.Name + ". Reopen W to review saved automation rules."
		m.ruleDryRunPreview = nil
		return m, nil, true
	case "R":
		if p.report.Kind != models.RuleDryRunKindCleanup {
			return m, nil, true
		}
		if m.dryRun {
			msg := "Live cleanup blocked in --dry-run; relaunch without --dry-run to run it"
			m.statusMessage = msg
			p.err = msg
			p.confirmRun = false
			return m, nil, true
		}
		if !p.confirmRun {
			p.confirmRun = true
			p.confirmEnable = false
			m.statusMessage = "Press R again to run the previewed cleanup actions live"
			return m, nil, true
		}
		if m.cleanupScheduler == nil {
			m.statusMessage = "Cleanup run unavailable: no cleanup scheduler is configured"
			return m, nil, true
		}
		req := p.cleanupRequest
		m.ruleDryRunPreview = nil
		m.showCleanupMgr = false
		m.cleanupManager = nil
		m.statusMessage = "Running previewed cleanup rules..."
		return m, m.cleanupScheduler.RunNowRequest(req), true
	}
	return m, nil, true
}

func ruleNeedsEnableConfirmation(rule *models.Rule) bool {
	for _, action := range rule.Actions {
		switch action.Type {
		case models.ActionMove, models.ActionArchive, models.ActionDelete, models.ActionWebhook, models.ActionCommand:
			return true
		}
	}
	return false
}

func cleanupRuleNeedsEnableConfirmation(rule *models.CleanupRule) bool {
	if rule == nil {
		return false
	}
	switch rule.Action {
	case "archive", "delete", "move":
		return true
	}
	return false
}

func (p *ruleDryRunPreview) View(width, height int) tea.View {
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 40
	}
	if width > 0 && (width < minTermWidth || height < minTermHeight) {
		return newHeraldView(renderMinSizeMessage(width, height))
	}
	return newHeraldView(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, p.renderPanel(width, height)))
}

func (p *ruleDryRunPreview) renderPanel(width, height int) string {
	layout := newCompactOverlayLayoutWithMax(width, height, 118, shortcutHelpMaxHeight)
	innerW := layout.contentWidth
	tableW := dryRunPreviewTableWidth(innerW)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243")).MaxWidth(innerW)
	tableStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	rowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))

	var content []string
	kind := "Automation Rules"
	if p.report.Kind == models.RuleDryRunKindCleanup {
		kind = "Cleanup Rules"
	}
	content = append(content, titleStyle.Render(kind+" Preview")+" "+warnStyle.Render("[DRY RUN]"))
	content = append(content, noteStyle.Render(fmt.Sprintf("%s  rules:%d  matches:%d  actions:%d", p.report.Scope, p.report.RuleCount, p.report.MatchCount, p.report.ActionCount)))
	if p.pendingRule != nil && strings.TrimSpace(p.pendingRule.Name) != "" {
		content = append(content, noteStyle.Render(truncateVisual("Pending rule: "+p.pendingRule.Name, innerW)))
	}
	if p.pendingCleanupRule != nil && strings.TrimSpace(p.pendingCleanupRule.Name) != "" {
		content = append(content, noteStyle.Render(truncateVisual("Pending cleanup rule: "+p.pendingCleanupRule.Name, innerW)))
	}
	if p.err != "" {
		content = append(content, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(truncateVisual("Error: "+p.err, innerW)))
	}
	content = append(content, "")

	header := p.formatHeader(tableW)
	content = append(content, tableStyle.Render(header))
	rowsAvailable := layout.contentHeight - len(content) - 2
	if rowsAvailable < 1 {
		rowsAvailable = 1
	}
	start := p.cursor - rowsAvailable + 1
	if start < 0 {
		start = 0
	}
	end := start + rowsAvailable
	if end > len(p.report.Rows) {
		end = len(p.report.Rows)
	}
	if len(p.report.Rows) == 0 {
		content = append(content, noteStyle.Render("No cached messages match this preview scope."))
	} else {
		for i := start; i < end; i++ {
			line := p.formatRow(p.report.Rows[i], tableW)
			if i == p.cursor {
				content = append(content, selectedStyle.Render(line))
			} else {
				content = append(content, rowStyle.Render(line))
			}
		}
	}
	content = append(content, "")
	footer := p.footer()
	if p.confirmEnable {
		footer += "  |  Press E again to enable"
	}
	if p.confirmRun {
		footer += "  |  Press R again to run live"
	}
	content = append(content, noteStyle.Render(truncateVisual(footer, innerW)))

	return renderCompactOverlayBox(strings.Join(content, "\n"), layout)
}

func dryRunPreviewTableWidth(innerW int) int {
	if innerW <= 4 {
		return innerW
	}
	return innerW - 2
}

func dryRunPreviewContentWidth(outerW int) int {
	contentW := outerW - 6 // two border columns plus two columns of horizontal padding on each side
	if contentW < 1 {
		return outerW
	}
	return contentW
}

func (p *ruleDryRunPreview) formatHeader(width int) string {
	if width < 74 {
		return dryRunTableLine(width, []dryRunColumn{
			{Value: "Action", Width: 7},
			{Value: "Folder", Width: 8},
			{Value: "Sender/Category", Width: 22},
		}, "Subject")
	}
	return dryRunTableLine(width, []dryRunColumn{
		{Value: "Rule", Width: 20},
		{Value: "Action", Width: 7},
		{Value: "Folder", Width: 8},
		{Value: "Sender/Domain/Category", Width: 28},
		{Value: "Date", Width: 6},
	}, "Subject")
}

func (p *ruleDryRunPreview) formatRow(row models.RuleDryRunRow, width int) string {
	who := row.Sender
	if row.Category != "" {
		who += " [" + row.Category + "]"
	} else if row.Domain != "" {
		who += " [" + row.Domain + "]"
	}
	if width < 74 {
		return dryRunTableLine(width, []dryRunColumn{
			{Value: row.Action, Width: 7},
			{Value: row.Folder, Width: 8},
			{Value: who, Width: 22},
		}, row.Subject)
	}
	date := ""
	if !row.Date.IsZero() {
		date = row.Date.Format("Jan 2")
	}
	return dryRunTableLine(width, []dryRunColumn{
		{Value: row.RuleName, Width: 20},
		{Value: row.Action, Width: 7},
		{Value: row.Folder, Width: 8},
		{Value: who, Width: 28},
		{Value: date, Width: 6},
	}, row.Subject)
}

type dryRunColumn struct {
	Value string
	Width int
}

func dryRunTableLine(width int, columns []dryRunColumn, tail string) string {
	parts := make([]string, 0, len(columns)+1)
	for _, col := range columns {
		parts = append(parts, dryRunCell(col.Value, col.Width))
	}
	parts = append(parts, tail)
	return truncateVisual(strings.Join(parts, "  "), width)
}

func dryRunCell(value string, width int) string {
	cell := truncateVisual(value, width)
	if pad := width - ansi.StringWidth(cell); pad > 0 {
		cell += strings.Repeat(" ", pad)
	}
	return cell
}

func (p *ruleDryRunPreview) footer() string {
	if p.pendingRule != nil || p.pendingCleanupRule != nil {
		return "j/k: scroll  s: save disabled  E: enable  esc: close"
	}
	if p.report.Kind == models.RuleDryRunKindCleanup {
		if p.liveRunDisabled {
			return "j/k: scroll  live run disabled in --dry-run  esc: close"
		}
		return "j/k: scroll  R: run live  esc: close"
	}
	return "j/k: scroll  esc: close"
}
