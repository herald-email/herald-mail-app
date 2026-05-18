package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
)

func TestThemeByNameBuiltInsAndAliases(t *testing.T) {
	tests := map[string]string{
		"":             "inherited",
		"inherited":    "inherited",
		"adaptive":     "inherited",
		"herald-dark":  "herald-dark",
		"legacy-dark":  "herald-dark",
		"legacy_dark":  "herald-dark",
		"herald-light": "herald-light",
	}
	for input, want := range tests {
		if got := ThemeByName(input).Name; got != want {
			t.Fatalf("ThemeByName(%q).Name = %q, want %q", input, got, want)
		}
	}
}

func TestResolveThemeForConfigAppliesOverrides(t *testing.T) {
	cfg := &config.Config{}
	cfg.Theme.Name = "herald-dark"
	cfg.Theme.Overrides = map[string]config.ThemeOverride{
		"chrome.tab_active": {
			Foreground: "#ffffff",
			Background: "xterm:25",
			Bold:       true,
		},
	}

	theme, warning := ResolveThemeForConfig(cfg, t.TempDir())
	if warning != "" {
		t.Fatalf("ResolveThemeForConfig warning = %q, want none", warning)
	}
	if theme.Name != "herald-dark" {
		t.Fatalf("resolved theme = %q, want herald-dark", theme.Name)
	}
	if !reflect.DeepEqual(theme.Chrome.TabActive.Foreground, lipgloss.Color("#ffffff")) {
		t.Fatalf("tab active fg = %#v, want #ffffff", theme.Chrome.TabActive.Foreground)
	}
	if !reflect.DeepEqual(theme.Chrome.TabActive.Background, lipgloss.Color("25")) {
		t.Fatalf("tab active bg = %#v, want xterm 25", theme.Chrome.TabActive.Background)
	}
	if !theme.Chrome.TabActive.Bold {
		t.Fatalf("tab active should preserve bold after override")
	}
}

func TestHeraldLightThemeHasExplicitReadableContrast(t *testing.T) {
	theme := ThemeByName("herald-light")

	if theme.Name != "herald-light" {
		t.Fatalf("theme name = %q, want herald-light", theme.Name)
	}
	if theme.Text.Primary.Foreground == nil {
		t.Fatalf("light theme primary text must not inherit terminal foreground")
	}
	if theme.Chrome.StatusBar.Background == nil || theme.Chrome.HintBar.Background == nil {
		t.Fatalf("light theme chrome must own readable status and hint backgrounds")
	}
	if theme.Focus.SelectionActive.Background == nil || theme.Focus.SelectionActive.Reverse {
		t.Fatalf("light theme active selection should use explicit foreground/background, not reverse-video")
	}
}

func TestLoadThemeFromFileValidatesRolesAndColors(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "quiet-slate.yaml")
	if err := os.WriteFile(validPath, []byte(`
version: 1
name: quiet-slate
display_name: Quiet Slate
inherits: herald-dark
roles:
  focus.panel_border_focused:
    fg: "#55c2ff"
  chrome.status_bar:
    bg: "ansi:4"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	doc, err := LoadThemeFromFile(validPath)
	if err != nil {
		t.Fatalf("LoadThemeFromFile(valid) failed: %v", err)
	}
	if doc.Name != "quiet-slate" || doc.DisplayName != "Quiet Slate" {
		t.Fatalf("loaded document = %#v, want quiet-slate metadata", doc)
	}

	invalidPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(invalidPath, []byte(`
version: 1
name: bad
roles:
  nope.not_a_role:
    fg: "#ffffff"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadThemeFromFile(invalidPath); err == nil || !strings.Contains(err.Error(), "unknown theme role") {
		t.Fatalf("LoadThemeFromFile(invalid role) err = %v, want unknown role error", err)
	}

	badColorPath := filepath.Join(dir, "bad-color.yaml")
	if err := os.WriteFile(badColorPath, []byte(`
version: 1
name: bad-color
roles:
  chrome.status_bar:
    fg: "tomato"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadThemeFromFile(badColorPath); err == nil || !strings.Contains(err.Error(), "invalid color") {
		t.Fatalf("LoadThemeFromFile(invalid color) err = %v, want invalid color error", err)
	}
}

func TestInstallThemeFileCopiesValidatedThemeWithPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "quiet-slate-source.yaml")
	if err := os.WriteFile(src, []byte(`
version: 1
name: quiet-slate
inherits: herald-dark
roles:
  focus.panel_border_focused:
    fg: "#55c2ff"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(dir, "themes")

	installed, err := InstallThemeFile(src, destDir)
	if err != nil {
		t.Fatalf("InstallThemeFile() failed: %v", err)
	}
	if filepath.Base(installed) != "quiet-slate.yaml" {
		t.Fatalf("installed path = %q, want quiet-slate.yaml", installed)
	}
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("installed theme permissions = %04o, want 0600", got)
	}
}

func TestModelThemeStateIsPerInstance(t *testing.T) {
	first := makeSizedModel(t, 80, 24)
	second := makeSizedModel(t, 80, 24)

	cfgDark := &config.Config{}
	cfgDark.Theme.Name = "herald-dark"
	cfgLight := &config.Config{}
	cfgLight.Theme.Name = "herald-light"

	first.SetConfig(cfgDark)
	second.SetConfig(cfgLight)

	if first.theme.Name != "herald-dark" {
		t.Fatalf("first model theme = %q, want herald-dark", first.theme.Name)
	}
	if second.theme.Name != "herald-light" {
		t.Fatalf("second model theme = %q, want herald-light", second.theme.Name)
	}
	if first.theme.Name == second.theme.Name {
		t.Fatalf("model theme state should be per instance, both are %q", first.theme.Name)
	}
}

func TestSettingsThemePreviewAppliesImmediatelyAndCancelRestores(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	cfg := &config.Config{}
	cfg.Theme.Name = "inherited"
	m.SetConfig(cfg)

	m.showSettings = true
	m.settingsPanel = NewSettings(SettingsModePanel, m.cfg)
	m.settingsPanel.panelSection = settingsPanelSectionTheme
	m.settingsPanel.themeName = "herald-light"
	m.settingsPanel.buildForm()
	m.settingsPanel.setSize(80, 24)

	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated := updatedModel.(*Model)
	if updated.theme.Name != "herald-light" {
		t.Fatalf("theme preview = %q, want herald-light", updated.theme.Name)
	}

	updatedModel, _ = updated.Update(SettingsCancelledMsg{})
	updated = updatedModel.(*Model)
	if updated.theme.Name != "inherited" {
		t.Fatalf("cancel should restore saved config theme, got %q", updated.theme.Name)
	}
}

func TestSettingsThemePickerPreviewDoesNotMutateSavedConfigBeforeSave(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	cfg := &config.Config{}
	cfg.Theme.Name = "inherited"
	m.SetConfig(cfg)

	m.showSettings = true
	m.settingsPanel = NewSettings(SettingsModePanel, m.cfg)
	m.settingsPanel.panelSection = settingsPanelSectionTheme
	m.settingsPanel.themeRole = "chrome.tab_active"
	m.settingsPanel.themeFG = "xterm:26"
	m.settingsPanel.storeThemeFieldsForRole(m.settingsPanel.themeRole, m.settingsPanel.themeFG, m.settingsPanel.themeBG)
	m.settingsPanel.buildForm()
	m.settingsPanel.setSize(80, 24)

	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated := updatedModel.(*Model)
	if got := updated.cfg.Theme.Overrides["chrome.tab_active"].Foreground; got != "" {
		t.Fatalf("saved config mutated before save, foreground = %q", got)
	}
	if updated.theme.Name != "inherited" {
		t.Fatalf("theme name = %q, want inherited preview with override", updated.theme.Name)
	}
	if got := fmt.Sprint(updated.theme.Chrome.TabActive.ForegroundColor()); got != "26" {
		t.Fatalf("preview foreground = %q, want xterm color 26", got)
	}
}
