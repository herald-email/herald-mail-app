package config

import (
	"path/filepath"
	"testing"
)

func TestThemeConfigDefaultsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := minimalOAuthConfig()
	original.Theme.Name = "herald-light"
	original.Theme.Overrides = map[string]ThemeOverride{
		"chrome.tab_active": {
			Foreground: "#ffffff",
			Background: "#1a73e8",
			Bold:       true,
		},
		"focus.selection_active": {
			Foreground: "inherit",
			Background: "xterm:25",
		},
	}

	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if got := loaded.Theme.Name; got != "herald-light" {
		t.Fatalf("Theme.Name = %q, want herald-light", got)
	}
	tab := loaded.Theme.Overrides["chrome.tab_active"]
	if tab.Foreground != "#ffffff" || tab.Background != "#1a73e8" || !tab.Bold {
		t.Fatalf("chrome.tab_active override = %#v, want configured colors and bold", tab)
	}
	selection := loaded.Theme.Overrides["focus.selection_active"]
	if selection.Foreground != "inherit" || selection.Background != "xterm:25" {
		t.Fatalf("focus.selection_active override = %#v, want inherited fg and xterm bg", selection)
	}
}

func TestThemeConfigDefaultsToInherited(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := minimalOAuthConfig()
	if err := original.Save(path); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if got := loaded.Theme.Name; got != "inherited" {
		t.Fatalf("default Theme.Name = %q, want inherited", got)
	}
	if loaded.Theme.Overrides == nil {
		t.Fatalf("default Theme.Overrides should be an empty map for settings edits")
	}
}
