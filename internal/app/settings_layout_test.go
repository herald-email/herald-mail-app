package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
)

func renderSettingsRawViewForTest(t *testing.T, s *Settings, width, height int) string {
	t.Helper()
	updated, _ := s.Update(tea.WindowSizeMsg{Width: width, Height: height})
	s = updated.(*Settings)
	rendered := s.View().Content
	assertFitsWidth(t, width, rendered)
	assertFitsHeight(t, height, rendered)
	return rendered
}

func renderSettingsViewForTest(t *testing.T, s *Settings, width, height int) string {
	t.Helper()
	return ansi.Strip(renderSettingsRawViewForTest(t, s, width, height))
}

func TestSettingsWizardView_RendersHeraldChrome(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if !strings.Contains(rendered, "Herald Setup") {
		t.Fatalf("expected Herald setup title, got:\n%s", rendered)
	}
	for _, want := range []string{"Recommended", "Supported", "Experimental"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected setup messaging to include %q, got:\n%s", want, rendered)
		}
	}
	if !strings.Contains(rendered, "╭") || !strings.Contains(rendered, "╯") {
		t.Fatalf("expected bordered wizard shell, got:\n%s", rendered)
	}
}

func TestSettingsWizardView_MinimumSizeGuardAt50x15(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)

	rendered := renderSettingsViewForTest(t, s, 50, 15)

	if !strings.Contains(rendered, "Terminal too narrow") {
		t.Fatalf("expected minimum-size guard, got:\n%s", rendered)
	}
}

func TestSettingsWizard_GmailIMAPStepIncludesGuidance(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.provider = "gmail"
	s.buildForm()
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	for _, want := range []string{
		"App Password",
		"imap.gmail.com",
		"smtp.gmail.com",
		"[click] App passwords",
		"Workspace",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected Gmail guidance to include %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsWizardEscBackBypassesRequiredFieldValidation(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.form.NextGroup()

	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyEscape})

	rendered := renderSettingsViewForTest(t, s, 80, 24)
	if !strings.Contains(rendered, "Account Type") {
		t.Fatalf("expected Esc to return to account type without validating empty credentials, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "email address is required") {
		t.Fatalf("expected Esc back navigation to bypass required-field error, got:\n%s", rendered)
	}
}

func TestSettingsWizardShiftTabBackBypassesRequiredFieldValidation(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.form.NextGroup()

	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	rendered := renderSettingsViewForTest(t, s, 80, 24)
	if !strings.Contains(rendered, "Account Type") {
		t.Fatalf("expected Shift+Tab from first credentials control to return to account type, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "email address is required") {
		t.Fatalf("expected Shift+Tab back navigation to bypass required-field error, got:\n%s", rendered)
	}
}

func TestSettingsWizard_ProtonMailPresetFieldsArePrepopulated(t *testing.T) {
	s := NewSettingsWithOptions(SettingsModeWizard, nil, SettingsOptions{FirstRunAccountOnly: true})
	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyDown})
	s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyDown})
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 80, 24)
	for _, want := range []string{"127.0.0.1", "1143", "1025"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected ProtonMail Bridge preset field %q to be visible, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsWizardPreferencesOmitAdvancedSyncCleanupControls(t *testing.T) {
	s := NewSettingsWithOptions(SettingsModeWizard, nil, SettingsOptions{FirstRunPreferencesOnly: true})
	var views []string
	for i := 0; i < 12 && s.form.State != huh.StateCompleted; i++ {
		views = append(views, renderSettingsViewForTest(t, s, 120, 40))
		s.form.NextGroup()
	}
	rendered := strings.Join(views, "\n")
	for _, notWant := range []string{
		"Poll Interval",
		"Enable IMAP IDLE",
		"Reclaim offline cache storage",
		"Auto-Cleanup Schedule",
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected first-run preferences to omit advanced sync/cleanup control %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsPanelSyncCleanupKeepsAdvancedControls(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s = openSettingsPanelCategoryForTest(t, s, "Sync & Cleanup")

	var views []string
	for i := 0; i < 8; i++ {
		views = append(views, renderSettingsViewForTest(t, s, 120, 40))
		s = updateSettingsForTest(t, s, huh.NextField())
	}
	rendered := strings.Join(views, "\n")
	for _, want := range []string{
		"Poll Interval",
		"Enable IMAP IDLE",
		"Lightweight previews",
		"Message bodies without attachments",
		"Full offline archive",
		"Reclaim offline cache storage",
		"Auto-Cleanup Schedule",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected in-app Sync & Cleanup to include %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsWizard_SyncCleanupUsesCompactOfflineCachePolicyLabels(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	focusSyncCleanupSettingsGroupForTest(t, s)

	rendered := renderSettingsViewForTest(t, s, 120, 40)
	normalized := strings.Join(strings.Fields(rendered), " ")
	for _, oldCopy := range []string{
		"First open/prewarm",
		"media fetches on demand",
		"No attachments - preview data",
		"Preserve all data - attachments too",
	} {
		if strings.Contains(normalized, oldCopy) {
			t.Fatalf("expected setup wizard to omit distracting copy %q, got:\n%s", oldCopy, rendered)
		}
	}
}

func TestSettingsWizard_DefaultHidesGmailOAuthAndShowsIMAPPresets(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)

	var labels []string
	for _, option := range s.accountTypeOptions() {
		labels = append(labels, option.Key)
	}
	rendered := strings.Join(labels, "\n")

	for _, want := range []string{
		"Gmail (IMAP + App Password)",
		"ProtonMail Bridge",
		"Fastmail",
		"iCloud",
		"Outlook",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected default wizard account choices to include %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Gmail OAuth") {
		t.Fatalf("expected default wizard account choices to hide Gmail OAuth, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "(Experimental)") {
		t.Fatalf("expected IMAP-based wizard account choices to avoid experimental labels, got:\n%s", rendered)
	}
}

func TestSettingsWizard_ExperimentalShowsGmailOAuthMarked(t *testing.T) {
	s := NewSettingsWithOptions(SettingsModeWizard, nil, SettingsOptions{ShowExperimentalEmailServices: true})

	var labels []string
	for _, option := range s.accountTypeOptions() {
		labels = append(labels, option.Key)
	}
	rendered := strings.Join(labels, "\n")

	if !strings.Contains(rendered, "Gmail OAuth (Experimental)") {
		t.Fatalf("expected experimental wizard account choices to include marked Gmail OAuth, got:\n%s", rendered)
	}
}

func TestSettingsPanel_StillShowsGmailOAuth(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)

	var labels []string
	for _, option := range s.accountTypeOptions() {
		labels = append(labels, option.Key)
	}
	rendered := strings.Join(labels, "\n")

	if !strings.Contains(rendered, "Gmail OAuth") {
		t.Fatalf("expected in-app settings panel to keep Gmail OAuth visible, got:\n%s", rendered)
	}
}

func TestSettingsPanelOpensTopLevelCategoryMenu(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	for _, want := range []string{"Account setup", "AI", "Sync & Cleanup", "Keyboard", "Theme Selection", "Theme Editor", "Signature"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected settings menu to include %q, got:\n%s", want, rendered)
		}
	}
	for _, notWant := range []string{"Account Type", "Email address", "AI Provider", "Keyboard Profile", "Email Signature"} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected settings menu not to show category form field %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsPanelSignatureCategorySkipsUnrelatedFields(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s = openSettingsPanelCategoryForTest(t, s, "Signature")

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if !strings.Contains(rendered, "Email Signature") {
		t.Fatalf("expected Signature category to show signature field, got:\n%s", rendered)
	}
	for _, notWant := range []string{"Account Type", "AI Provider", "Poll Interval"} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected Signature category to skip unrelated field %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsPanelAICategorySkipsAccountValidation(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s = openSettingsPanelCategoryForTest(t, s, "AI")

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if !strings.Contains(rendered, "AI Provider") {
		t.Fatalf("expected AI category to show AI provider without account fields, got:\n%s", rendered)
	}
	for _, notWant := range []string{"Email address", "Password", "IMAP Host", "Email Signature"} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected AI category to skip unrelated field %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsPanelKeyboardCategorySkipsUnrelatedFields(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s = openSettingsPanelCategoryForTest(t, s, "Keyboard")

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if !strings.Contains(rendered, "Keyboard Profile") {
		t.Fatalf("expected Keyboard category to show keyboard profile, got:\n%s", rendered)
	}
	for _, notWant := range []string{"Account Type", "AI Provider", "Email Signature"} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected Keyboard category to skip unrelated field %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsPanelKeyboardDefaultHidesCustomKeymap(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s = openSettingsPanelCategoryForTest(t, s, "Keyboard")

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if !strings.Contains(rendered, "Keyboard Profile") {
		t.Fatalf("expected Keyboard category to show keyboard profile, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Custom Keymap") {
		t.Fatalf("expected default keyboard profile to hide custom keymap, got:\n%s", rendered)
	}
}

func TestSettingsPanelKeyboardCustomShowsCustomKeymap(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s.keyboardProfile = keyboardProfileCustom
	s.customKeymap = "~/.config/herald/keymaps/work.yaml"
	s = openSettingsPanelCategoryForTest(t, s, "Keyboard")

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	for _, want := range []string{"Keyboard Profile", "Custom YAML", "Custom Keymap", "~/.config/herald/keymaps/work.yaml"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected custom keyboard profile to show %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsPanelKeyboardSelectingCustomShowsCustomKeymap(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s.customKeymap = "~/.config/herald/keymaps/work.yaml"
	s = openSettingsPanelCategoryForTest(t, s, "Keyboard")

	for i := 0; i < 3; i++ {
		s = updateSettingsForTest(t, s, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	rendered := renderSettingsViewForTest(t, s, 80, 24)

	for _, want := range []string{"Custom YAML", "Custom Keymap", "~/.config/herald/keymaps/work.yaml"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected selected custom keyboard profile to show %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsPanelThemeSelectionCategoryShowsPickerAndInstallOnly(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s = openSettingsPanelCategoryForTest(t, s, "Theme Selection")

	var views []string
	for i := 0; i < 4; i++ {
		views = append(views, renderSettingsViewForTest(t, s, 100, 32))
		s = updateSettingsForTest(t, s, huh.NextField())
	}
	rendered := strings.Join(views, "\n--- next field ---\n")

	for _, want := range []string{
		"Current Theme",
		"Install local theme YAML",
		"Save changes",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected Theme Selection category to include %q, got:\n%s", want, rendered)
		}
	}
	for _, notWant := range []string{
		"Theme Role",
		"Foreground",
		"Background",
		"Foreground Picker",
		"Background Picker",
		"Live preview",
		"Save As New Theme",
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected Theme Selection category to skip editor field %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsPanelThemeEditorCategoryShowsRoleEditorOnly(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	s = openSettingsPanelCategoryForTest(t, s, "Theme Editor")

	var views []string
	for i := 0; i < 10; i++ {
		views = append(views, renderSettingsViewForTest(t, s, 100, 32))
		s = updateSettingsForTest(t, s, huh.NextField())
	}
	rendered := strings.Join(views, "\n--- next field ---\n")

	for _, want := range []string{
		"Theme Role",
		"Foreground",
		"Background",
		"Foreground Picker",
		"Background Picker",
		"xterm-256",
		"RGB",
		"Live preview",
		"Save As New Theme",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected Theme Editor category to include %q, got:\n%s", want, rendered)
		}
	}
	for _, notWant := range []string{"Current Theme", "Install local theme YAML", "Account Type", "AI Provider", "Email Signature"} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected Theme Editor category to skip unrelated field %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsWizardThemeStepOnlyShowsThemePicker(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)

	var views []string
	for i := 0; i < 12; i++ {
		view := renderSettingsViewForTest(t, s, 100, 32)
		if strings.Contains(view, "Current Theme") {
			for j := 0; j < 8; j++ {
				views = append(views, renderSettingsViewForTest(t, s, 100, 32))
				s = updateSettingsForTest(t, s, huh.NextField())
			}
			break
		}
		s.form.NextGroup()
	}
	if len(views) == 0 {
		t.Fatalf("expected setup wizard to include a Theme step")
	}
	rendered := strings.Join(views, "\n--- next field ---\n")

	for _, want := range []string{"Current Theme", "Inherited", "Herald dark", "Herald light"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected setup wizard Theme step to include %q, got:\n%s", want, rendered)
		}
	}
	for _, notWant := range []string{
		"Install local theme YAML",
		"Theme Role",
		"Foreground",
		"Background",
		"Live preview",
		"Reset selected role",
		"Reset all theme overrides",
		"Save As New Theme",
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected setup wizard Theme step to hide advanced field %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsWizardPreferencesThemeStepOnlyShowsThemePicker(t *testing.T) {
	s := NewSettingsWithOptions(SettingsModeWizard, nil, SettingsOptions{FirstRunPreferencesOnly: true})

	var views []string
	for i := 0; i < 12; i++ {
		view := renderSettingsViewForTest(t, s, 100, 32)
		if strings.Contains(view, "Current Theme") {
			for j := 0; j < 8; j++ {
				views = append(views, renderSettingsViewForTest(t, s, 100, 32))
				s = updateSettingsForTest(t, s, huh.NextField())
			}
			break
		}
		s.form.NextGroup()
	}
	if len(views) == 0 {
		t.Fatalf("expected first-run preferences to include a Theme step")
	}
	rendered := strings.Join(views, "\n--- next field ---\n")

	for _, want := range []string{"Current Theme", "Inherited", "Herald dark", "Herald light"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected first-run preferences Theme step to include %q, got:\n%s", want, rendered)
		}
	}
	for _, notWant := range []string{
		"Install local theme YAML",
		"Theme Role",
		"Foreground",
		"Background",
		"Live preview",
		"Reset selected role",
		"Reset all theme overrides",
		"Save As New Theme",
	} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("expected first-run preferences Theme step to hide advanced field %q, got:\n%s", notWant, rendered)
		}
	}
}

func TestSettingsWizardDoesNotShowPanelCategoryMenu(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if strings.Contains(rendered, "Account setup") || strings.Contains(rendered, "Sync & Cleanup") {
		t.Fatalf("expected first-run wizard to remain linear without panel category menu, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Account Type") {
		t.Fatalf("expected first-run wizard to still start at account type, got:\n%s", rendered)
	}
}

func TestSettingsPanelMenuHintsExplainOpenFilterAndExit(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)

	rendered := renderSettingsViewForTest(t, s, 80, 24)
	normalized := strings.Join(strings.Fields(rendered), " ")

	for _, want := range []string{"enter open", "/ filter", "esc exit"} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("expected settings menu hints to include %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(normalized, "enter submit") {
		t.Fatalf("expected settings menu hints to avoid submit wording, got:\n%s", rendered)
	}
}

func TestModelSettingsBottomHintsReflectSettingsMenuAndCategory(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressSettingsPanelForTest(t, m)
	menuHints := stripANSI(updated.renderKeyHints())
	for _, want := range []string{"enter: open category", "/: filter", "esc: exit settings"} {
		if !strings.Contains(menuHints, want) {
			t.Fatalf("expected settings menu bottom hints to include %q, got:\n%s", want, menuHints)
		}
	}
	if strings.Contains(menuHints, "S: settings") {
		t.Fatalf("expected settings menu bottom hints to replace the underlying tab hints, got:\n%s", menuHints)
	}

	opened := openSettingsPanelCategoryForTest(t, updated.settingsPanel, "Signature")
	updated.settingsPanel = opened
	categoryHints := stripANSI(updated.renderKeyHints())
	for _, want := range []string{"tab: fields", "enter: edit/select", "esc: back to settings menu"} {
		if !strings.Contains(categoryHints, want) {
			t.Fatalf("expected settings category bottom hints to include %q, got:\n%s", want, categoryHints)
		}
	}
}

func pressSettingsPanelForTest(t *testing.T, m *Model) *Model {
	t.Helper()
	model, _ := m.handleKeyMsg(keyRunes("S"))
	updated := model.(*Model)
	if !updated.showSettings || updated.settingsPanel == nil {
		t.Fatalf("expected S to open settings panel")
	}
	return updated
}

func findRenderedText(lines []string, needle string) (int, int) {
	for row, line := range lines {
		if col := strings.Index(line, needle); col >= 0 {
			return row, col
		}
	}
	return -1, -1
}

func lineHasHorizontalRule(line string) bool {
	return strings.Count(line, "─") >= 8
}

func TestSettingsPanelRendersCompactCenteredModalOverCurrentView(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressSettingsPanelForTest(t, m)

	rendered := updated.View().Content
	assertFitsWidth(t, 220, rendered)
	assertFitsHeight(t, 50, rendered)
	lines := strings.Split(stripANSI(rendered), "\n")
	if len(lines) < 40 {
		t.Fatalf("expected settings overlay to preserve full terminal view, got %d lines:\n%s", len(lines), stripANSI(rendered))
	}
	if !strings.Contains(lines[0], "Herald") {
		t.Fatalf("expected current view to remain visible behind settings, got first line %q", lines[0])
	}
	titleRow, titleCol := findRenderedText(lines, "Account setup")
	if titleRow < 8 {
		t.Fatalf("expected settings content to be vertically centered in a compact modal, row=%d:\n%s", titleRow, stripANSI(rendered))
	}
	if titleCol < 40 || titleCol > 100 {
		t.Fatalf("expected settings content to be horizontally centered in a compact modal, col=%d:\n%s", titleCol, stripANSI(rendered))
	}
}

func TestSettingsPanelInternalFooterHintHasDividerAndLowerRow(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressSettingsPanelForTest(t, m)

	rendered := stripANSI(updated.View().Content)
	lines := strings.Split(rendered, "\n")
	hintIdx := indexLineContaining(lines, "↑ up")
	if hintIdx < 1 {
		t.Fatalf("expected settings internal footer hint, got:\n%s", rendered)
	}
	if !lineHasHorizontalRule(lines[hintIdx-1]) {
		t.Fatalf("expected settings divider immediately above footer hint, got previous line %q:\n%s", lines[hintIdx-1], rendered)
	}
	if hintIdx+2 >= len(lines) || !strings.Contains(lines[hintIdx+2], "╰") {
		t.Fatalf("expected settings footer hint to move one row lower while preserving bottom padding; following lines=%q / %q:\n%s", lines[min(hintIdx+1, len(lines)-1)], lines[min(hintIdx+2, len(lines)-1)], rendered)
	}
}

func TestSettingsPanelFitsAt80ColsAsModal(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()

	updated := pressSettingsPanelForTest(t, m)

	rendered := updated.View().Content
	assertFitsWidth(t, 80, rendered)
	assertFitsHeight(t, 24, rendered)
	lines := strings.Split(strings.TrimRight(stripANSI(rendered), "\n"), "\n")
	if len(lines) > 24 {
		t.Fatalf("expected settings modal to fit 80x24 height, got %d lines:\n%s", len(lines), stripANSI(rendered))
	}
	if !strings.Contains(lines[0], "Herald") {
		t.Fatalf("expected settings modal to keep the current view visible at 80x24, got first line %q", lines[0])
	}
	titleRow, _ := findRenderedText(lines, "Account setup")
	if titleRow < 2 {
		t.Fatalf("expected settings modal to leave a vertical margin at 80x24, row=%d:\n%s", titleRow, stripANSI(rendered))
	}
}

func TestSettingsPanelSignatureFieldKeepsFooterAt80x24(t *testing.T) {
	s := NewSettings(SettingsModePanel, nil)
	focusSignatureSettingsGroup(t, s)

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	for _, want := range []string{"Email Signature", "enter new line", "tab next"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected settings signature panel to include %q at 80x24, got:\n%s", want, rendered)
		}
	}
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "╰") && strings.Contains(line, "╯") {
			return
		}
	}
	t.Fatalf("expected settings signature panel to keep the bottom border at 80x24, got:\n%s", rendered)
}

func TestSettingsPanelUsesMinimumSizeGuardWhenTooSmall(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	opened := pressSettingsPanelForTest(t, m)

	model, _ := opened.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	resized := model.(*Model)

	if resized.windowWidth != 50 || resized.windowHeight != 15 {
		t.Fatalf("expected parent model to track settings resize, got %dx%d", resized.windowWidth, resized.windowHeight)
	}
	rendered := resized.View().Content
	assertFitsWidth(t, 50, rendered)
	assertFitsHeight(t, 15, rendered)
	if !strings.Contains(stripANSI(rendered), "Terminal too narrow") {
		t.Fatalf("expected minimum-size guard instead of clipped settings form, got:\n%s", stripANSI(rendered))
	}
}

func TestSettingsPanelResizeKeepsBackdropAndModalInSync(t *testing.T) {
	m := makeSizedModel(t, 220, 50)
	m.activeTab = tabTimeline
	m.timeline.emails = mockEmails()
	m.updateTimelineTable()
	opened := pressSettingsPanelForTest(t, m)

	model, _ := opened.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	resized := model.(*Model)

	if resized.windowWidth != 80 || resized.windowHeight != 24 {
		t.Fatalf("expected parent model to track resize while settings is open, got %dx%d", resized.windowWidth, resized.windowHeight)
	}
	if resized.settingsPanel == nil || resized.settingsPanel.width != 80 || resized.settingsPanel.height != 24 {
		t.Fatalf("expected settings panel to receive resize, got %#v", resized.settingsPanel)
	}
	rendered := resized.View().Content
	assertFitsWidth(t, 80, rendered)
	assertFitsHeight(t, 24, rendered)
	lines := strings.Split(stripANSI(rendered), "\n")
	if !strings.Contains(lines[0], "Herald") {
		t.Fatalf("expected resized settings modal to keep backdrop aligned, got first line %q", lines[0])
	}
	if !strings.Contains(stripANSI(rendered), "Account setup") {
		t.Fatalf("expected settings content after resize, got:\n%s", stripANSI(rendered))
	}
}

func TestSettingsWizard_GmailSummaryUsesShortClickableLinks(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	updated, _ := s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	s = updated.(*Settings)

	updated, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	s = updated.(*Settings)
	rendered := s.View().Content
	plain := ansi.Strip(rendered)

	for _, want := range []string{
		"[click] App passwords",
		"[click] Add Gmail to another client",
		"[click] Workspace IMAP setup",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected short clickable Gmail docs label %q, got:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "https://myaccount.google.com/apppasswords") {
		t.Fatalf("expected raw Gmail docs URL to be hidden behind a short clickable label, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;https://myaccount.google.com/apppasswords") {
		t.Fatalf("expected OSC 8 hyperlink for Gmail docs, got raw view:\n%q", rendered)
	}
}

func TestSettingsWizard_AIProviderChoicesIncludeDefaultCustomAndDisabled(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.form.NextGroup()
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	for _, want := range []string{
		"Ollama (local default)",
		"Ollama (local custom)",
		"Claude API",
		"OpenAI / compatible",
		"AI features disabled",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected AI provider choices to include %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsWizard_OllamaDefaultDoesNotAskForCustomValues(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.aiProvider = "ollama-default"
	s.buildForm()
	s.form.NextGroup()
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 80, 24)

	if strings.Contains(rendered, "Ollama Host>") || strings.Contains(rendered, "Ollama Model>") {
		t.Fatalf("expected default Ollama path to hide custom fields, got:\n%s", rendered)
	}
}

func TestSettingsWizard_OllamaDefaultWarnsAboutMemoryAndShowsSafeDefaults(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.aiProvider = "ollama-default"
	s.buildForm()
	s.form.NextGroup()
	s.form.NextGroup()
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 100, 32)
	for _, want := range []string{
		"llama3.2:1b",
		"nomic-embed-text",
		"8GB",
		"larger models",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected Ollama default guidance to include %q, got:\n%s", want, rendered)
		}
	}
}

func TestSettingsWizard_CustomOllamaShowsCuratedChatAndEmbeddingChoices(t *testing.T) {
	s := NewSettings(SettingsModeWizard, nil)
	s.aiProvider = "ollama-custom"
	s.buildForm()
	s.form.NextGroup()
	s.form.NextGroup()
	s.form.NextGroup()

	rendered := renderSettingsViewForTest(t, s, 120, 40)
	for _, want := range []string{
		"Chat Model",
		"llama3.2:1b",
		"qwen3.5:0.8b",
		"llama3.2:3b",
		"gemma3:4b",
		"Embedding Model",
		"nomic-embed-text",
		"all-minilm",
		"nomic-embed-text-v2-moe",
		"mxbai-embed-large",
		"bge-m3",
		"Custom model name",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected custom Ollama choices to include %q, got:\n%s", want, rendered)
		}
	}
}
