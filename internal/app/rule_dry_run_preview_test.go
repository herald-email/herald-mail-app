package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type dryRunPreviewBackend struct {
	stubBackend
	automationReport *models.RuleDryRunReport
	cleanupReport    *models.RuleDryRunReport
	savedRules       []*models.Rule
	savedCleanup     []*models.CleanupRule
}

func (b *dryRunPreviewBackend) SaveRule(rule *models.Rule) error {
	b.savedRules = append(b.savedRules, rule)
	return nil
}

func (b *dryRunPreviewBackend) SaveCleanupRule(rule *models.CleanupRule) error {
	b.savedCleanup = append(b.savedCleanup, rule)
	return nil
}

func (b *dryRunPreviewBackend) PreviewRulesDryRun(req models.RuleDryRunRequest) (*models.RuleDryRunReport, error) {
	if b.automationReport != nil {
		return b.automationReport, nil
	}
	return emptyDryRunReport(models.RuleDryRunKindAutomation), nil
}

func (b *dryRunPreviewBackend) PreviewCleanupRulesDryRun(req models.RuleDryRunRequest) (*models.RuleDryRunReport, error) {
	if b.cleanupReport != nil {
		return b.cleanupReport, nil
	}
	return emptyDryRunReport(models.RuleDryRunKindCleanup), nil
}

func emptyDryRunReport(kind models.RuleDryRunKind) *models.RuleDryRunReport {
	return &models.RuleDryRunReport{Kind: kind, DryRun: true, GeneratedAt: time.Now()}
}

func sampleDryRunReport(kind models.RuleDryRunKind) *models.RuleDryRunReport {
	return &models.RuleDryRunReport{
		Kind:        kind,
		Scope:       "selected rules / INBOX",
		Folder:      "INBOX",
		RuleCount:   1,
		MatchCount:  1,
		ActionCount: 1,
		DryRun:      true,
		GeneratedAt: time.Now(),
		Rows: []models.RuleDryRunRow{
			{
				RuleID:    3,
				RuleName:  "Archive Packet Press",
				MessageID: "msg-1",
				Sender:    "Packet Press <newsletter@packetpress.example>",
				Domain:    "packetpress.example",
				Category:  "Newsletter",
				Folder:    "INBOX",
				Subject:   "Weekly systems digest",
				Date:      time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
				Action:    "archive",
				Target:    "Archive",
			},
		},
	}
}

func TestDryRunPreviewColumnsAlignWithHeader(t *testing.T) {
	preview := newCleanupDryRunPreview(sampleDryRunReport(models.RuleDryRunKindCleanup), models.RuleDryRunRequest{}, nil)
	row := models.RuleDryRunRow{
		RuleName: "Delete old travel offers from Trailpost",
		Action:   "delete",
		Folder:   "INBOX",
		Sender:   "Trailpost Travel <offers@trailpost.example>",
		Domain:   "trailpost.example",
		Subject:  "Weekend fares for mountain towns",
		Date:     time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
	}

	header := preview.formatHeader(100)
	line := preview.formatRow(row, 100)
	for _, check := range []struct {
		header string
		row    string
	}{
		{"Rule", "Delete old travel"},
		{"Action", "delete"},
		{"Folder", "INBOX"},
		{"Sender/Domain/Category", "Trailpost Travel"},
		{"Date", "Apr 20"},
		{"Subject", "Weekend fares"},
	} {
		want := visualIndex(header, check.header)
		got := visualIndex(line, check.row)
		if got != want {
			t.Fatalf("%s column starts at %d, want %d\nheader: %q\nrow:    %q", check.header, got, want, header, line)
		}
	}
}

func TestDryRunPreviewFormatsRowsToBoxContentWidth(t *testing.T) {
	preview := newCleanupDryRunPreview(sampleDryRunReport(models.RuleDryRunKindCleanup), models.RuleDryRunRequest{}, nil)
	row := models.RuleDryRunRow{
		RuleName: "Archive old Packet Press",
		Action:   "archive",
		Folder:   "INBOX",
		Sender:   "Packet Press <newsletter@packetpress.example>",
		Domain:   "packetpress.example",
		Subject:  "Weekly systems digest: queues, caches, latency",
		Date:     time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC),
	}

	contentWidth := dryRunPreviewContentWidth(118)
	line := preview.formatRow(row, contentWidth)
	if got := ansi.StringWidth(line); got > contentWidth {
		t.Fatalf("row width = %d, want <= %d: %q", got, contentWidth, line)
	}
	if strings.Contains(line, "cac…") {
		t.Fatalf("row leaves a tiny wrapped subject tail; want truncation before wrap: %q", line)
	}
}

func TestDryRunPreviewRenderedRowsDoNotWrap(t *testing.T) {
	report := sampleDryRunReport(models.RuleDryRunKindCleanup)
	report.Rows = append(report.Rows, models.RuleDryRunRow{
		RuleID:    4,
		RuleName:  "Archive old Packet Press",
		MessageID: "msg-2",
		Sender:    "Packet Press <newsletter@packetpress.example>",
		Domain:    "packetpress.example",
		Folder:    "INBOX",
		Subject:   "Containers without the churn",
		Date:      time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
		Action:    "archive",
		Target:    "Archive",
	})
	report.Rows[0].Subject = "Weekly systems digest: queues, caches, latency, and more"
	preview := newCleanupDryRunPreview(report, models.RuleDryRunRequest{}, nil)

	rendered := stripANSI(preview.renderPanel(220, 50))
	rowLines := dryRunRenderedRowLines(rendered)
	if got, want := len(rowLines), len(report.Rows); got != want {
		t.Fatalf("rendered dry-run rows should not wrap, got %d row lines want %d:\n%s", got, want, rendered)
	}
	for _, line := range rowLines {
		if !strings.Contains(line, "archive") || !strings.Contains(line, "INBOX") {
			t.Fatalf("expected each rendered row line to contain row columns, got %q in:\n%s", line, rendered)
		}
	}
}

func dryRunRenderedRowLines(rendered string) []string {
	lines := strings.Split(rendered, "\n")
	headerIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Rule") && strings.Contains(line, "Action") && strings.Contains(line, "Subject") {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil
	}
	var rows []string
	for _, line := range lines[headerIdx+1:] {
		trimmed := strings.Trim(line, " │")
		if trimmed == "" {
			break
		}
		rows = append(rows, trimmed)
	}
	return rows
}

func visualIndex(s, sub string) int {
	index := strings.Index(s, sub)
	if index < 0 {
		return -1
	}
	return ansi.StringWidth(s[:index])
}

func TestRuleEditorDoneOpensDryRunPreviewBeforeSaving(t *testing.T) {
	backend := &dryRunPreviewBackend{automationReport: sampleDryRunReport(models.RuleDryRunKindAutomation)}
	m := New(backend, nil, "", nil, false)
	m.currentFolder = "INBOX"

	updated, cmd := m.Update(RuleEditorDoneMsg{Rule: &models.Rule{
		Name:         "Archive Packet Press",
		Enabled:      true,
		TriggerType:  models.TriggerSender,
		TriggerValue: "newsletter@packetpress.example",
		Actions:      []models.RuleAction{{Type: models.ActionArchive}},
	}})
	m = updated.(*Model)
	if cmd == nil {
		t.Fatal("expected dry-run preview command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(*Model)

	if len(backend.savedRules) != 0 {
		t.Fatalf("rule should not be saved before preview choice, saved %d", len(backend.savedRules))
	}
	rendered := stripANSI(m.View().Content)
	for _, want := range []string{"[DRY RUN]", "Archive Packet Press", "Packet Press", "Weekly systems digest", "s: save disabled", "E: enable"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected dry-run preview to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestCleanupManagerPreviewSelectedRule(t *testing.T) {
	backend := &dryRunPreviewBackend{cleanupReport: sampleDryRunReport(models.RuleDryRunKindCleanup)}
	m := New(backend, nil, "", nil, false)
	m.activeTab = tabCleanup
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*Model)
	m.cleanupManager = NewCleanupManager(backend, 120, 40)
	m.cleanupManager.rules = []*models.CleanupRule{
		{ID: 3, Name: "Archive Packet Press", MatchType: "sender", MatchValue: "newsletter@packetpress.example", Action: "archive", OlderThanDays: 10, Enabled: true},
	}
	m.showCleanupMgr = true

	updated, cmd := m.Update(keyRunes("p"))
	m = updated.(*Model)
	if cmd == nil {
		t.Fatal("expected cleanup dry-run preview command")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(*Model)
	if cmd == nil {
		t.Fatal("expected cleanup dry-run preview fetch command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(*Model)

	rendered := stripANSI(m.View().Content)
	for _, want := range []string{"[DRY RUN]", "Archive Packet Press", "Packet Press", "Weekly systems digest", "R: run live"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected cleanup dry-run preview to contain %q, got:\n%s", want, rendered)
		}
	}
}

func TestCleanupPreviewBlocksLiveRunInGlobalDryRun(t *testing.T) {
	backend := &dryRunPreviewBackend{cleanupReport: sampleDryRunReport(models.RuleDryRunKindCleanup)}
	m := New(backend, nil, "", nil, true)

	updated, cmd := m.Update(CleanupDryRunMsg{RuleID: 3})
	m = updated.(*Model)
	if cmd == nil {
		t.Fatal("expected cleanup dry-run preview command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(*Model)

	m = pressKey(t, m, "R")
	if !strings.Contains(m.statusMessage, "relaunch without --dry-run") {
		t.Fatalf("expected global dry-run live-run block, got %q", m.statusMessage)
	}
	if m.ruleDryRunPreview == nil {
		t.Fatal("preview should remain visible after blocked live run")
	}
}

func TestCleanupRulePreviewSavesDisabledBeforeEnable(t *testing.T) {
	backend := &dryRunPreviewBackend{cleanupReport: sampleDryRunReport(models.RuleDryRunKindCleanup)}
	m := New(backend, nil, "", nil, false)

	rule := &models.CleanupRule{
		Name:          "Archive Packet Press",
		MatchType:     "sender",
		MatchValue:    "newsletter@packetpress.example",
		Action:        "archive",
		OlderThanDays: 10,
		Enabled:       true,
	}
	updated, cmd := m.Update(CleanupDryRunMsg{CleanupRule: rule})
	m = updated.(*Model)
	if cmd == nil {
		t.Fatal("expected cleanup dry-run preview command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(*Model)

	if len(backend.savedCleanup) != 0 {
		t.Fatalf("cleanup rule should not save before preview choice, saved %d", len(backend.savedCleanup))
	}
	m = pressKey(t, m, "s")

	if len(backend.savedCleanup) != 1 {
		t.Fatalf("expected one saved cleanup rule, got %d", len(backend.savedCleanup))
	}
	if backend.savedCleanup[0].Enabled {
		t.Fatal("expected cleanup rule to be saved disabled from preview")
	}
	if !strings.Contains(m.statusMessage, "Reopen C") {
		t.Fatalf("expected cleanup save status to mention Reopen C, got %q", m.statusMessage)
	}
}

func TestCleanupRulePreviewRequiresConfirmationBeforeEnable(t *testing.T) {
	backend := &dryRunPreviewBackend{cleanupReport: sampleDryRunReport(models.RuleDryRunKindCleanup)}
	m := New(backend, nil, "", nil, false)

	rule := &models.CleanupRule{
		Name:          "Delete Packet Press",
		MatchType:     "sender",
		MatchValue:    "newsletter@packetpress.example",
		Action:        "delete",
		OlderThanDays: 10,
	}
	updated, cmd := m.Update(CleanupDryRunMsg{CleanupRule: rule})
	m = updated.(*Model)
	updated, _ = m.Update(cmd())
	m = updated.(*Model)

	m = pressKey(t, m, "E")
	if len(backend.savedCleanup) != 0 {
		t.Fatalf("cleanup rule should not enable before second confirmation, saved %d", len(backend.savedCleanup))
	}
	if !strings.Contains(m.statusMessage, "Press E again") {
		t.Fatalf("expected confirmation status, got %q", m.statusMessage)
	}

	m = pressKey(t, m, "E")
	if len(backend.savedCleanup) != 1 {
		t.Fatalf("expected cleanup rule to save after confirmation, got %d", len(backend.savedCleanup))
	}
	if !backend.savedCleanup[0].Enabled {
		t.Fatal("expected cleanup rule to be enabled after confirmation")
	}
}

func TestCleanupManagerNewRuleDefaultsDisabled(t *testing.T) {
	mgr := NewCleanupManager(&dryRunPreviewBackend{}, 120, 40)

	updated, _ := mgr.Update(keyRunes("n"))
	mgr = updated

	if mgr.editing == nil {
		t.Fatal("expected new cleanup rule edit state")
	}
	if mgr.formEnabled {
		t.Fatal("new cleanup rules should default disabled until preview")
	}
	if mgr.editing.Enabled {
		t.Fatal("new cleanup rule model should default disabled until preview")
	}
}
