package app

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"gopkg.in/yaml.v3"
)

type ThemeStyle struct {
	Foreground    color.Color
	Background    color.Color
	Bold          bool
	Faint         bool
	Underline     bool
	Reverse       bool
	Strikethrough bool
}

func (s ThemeStyle) Apply(style lipgloss.Style) lipgloss.Style {
	if s.Foreground != nil {
		style = style.Foreground(s.Foreground)
	} else {
		style = style.UnsetForeground()
	}
	if s.Background != nil {
		style = style.Background(s.Background)
	} else {
		style = style.UnsetBackground()
	}
	return style.
		Bold(s.Bold).
		Faint(s.Faint).
		Underline(s.Underline).
		Reverse(s.Reverse).
		Strikethrough(s.Strikethrough)
}

func (s ThemeStyle) Style() lipgloss.Style {
	return s.Apply(lipgloss.NewStyle())
}

func (s ThemeStyle) ForegroundColor() color.Color {
	if s.Foreground != nil {
		return s.Foreground
	}
	return lipgloss.NoColor{}
}

func (s ThemeStyle) BackgroundColor() color.Color {
	if s.Background != nil {
		return s.Background
	}
	return lipgloss.NoColor{}
}

type ThemeTextRoles struct {
	Primary  ThemeStyle
	Muted    ThemeStyle
	Dim      ThemeStyle
	Disabled ThemeStyle
}

type ThemeChromeRoles struct {
	TitleBar     ThemeStyle
	TabActive    ThemeStyle
	TabInactive  ThemeStyle
	StatusBar    ThemeStyle
	HintBar      ThemeStyle
	TopSyncStrip ThemeStyle
	TableHeader  ThemeStyle
	Loading      ThemeStyle
	Progress     ThemeStyle
}

type ThemeFocusRoles struct {
	PanelBorder        ThemeStyle
	PanelBorderFocused ThemeStyle
	SelectionActive    ThemeStyle
	SelectionInactive  ThemeStyle
	VisualSelection    ThemeStyle
}

type ThemeMetadataRoles struct {
	Label   ThemeStyle
	Sender  ThemeStyle
	Date    ThemeStyle
	Subject ThemeStyle
	Tag     ThemeStyle
	Action  ThemeStyle
}

type ThemeSeverityRoles struct {
	Info        ThemeStyle
	Success     ThemeStyle
	Warning     ThemeStyle
	Error       ThemeStyle
	Caution     ThemeStyle
	Destructive ThemeStyle
}

type ThemeBadgeRoles struct {
	Demo   ThemeStyle
	DryRun ThemeStyle
}

type ThemeLogRoles struct {
	Info  ThemeStyle
	Warn  ThemeStyle
	Error ThemeStyle
	Debug ThemeStyle
}

type ThemeOverlayRoles struct {
	CompactBorder ThemeStyle
	DemoKeyBadge  ThemeStyle
}

type ThemeSetupRoles struct {
	Title        ThemeStyle
	Spinner      ThemeStyle
	Border       ThemeStyle
	SummaryLabel ThemeStyle
	SummaryBody  ThemeStyle
	Link         ThemeStyle
}

type ThemeComposeRoles struct {
	Accent           ThemeStyle
	Attachment       ThemeStyle
	StatusInfo       ThemeStyle
	StatusWarning    ThemeStyle
	StatusError      ThemeStyle
	AITitle          ThemeStyle
	AILabel          ThemeStyle
	AIToggleActive   ThemeStyle
	AIToggleInactive ThemeStyle
	AIAction         ThemeStyle
	AIAccept         ThemeStyle
	AIDiscard        ThemeStyle
	AIDim            ThemeStyle
	AIBorder         ThemeStyle
}

type ThemeDiffRoles struct {
	Delete ThemeStyle
	Add    ThemeStyle
}

type ThemeContactsRoles struct {
	KeywordSearch   ThemeStyle
	SelectedEmail   ThemeStyle
	SelectedCompany ThemeStyle
	Company         ThemeStyle
}

type ThemeRuleRoles struct {
	Title      ThemeStyle
	Note       ThemeStyle
	Selected   ThemeStyle
	DryRun     ThemeStyle
	Row        ThemeStyle
	Error      ThemeStyle
	GuideLabel ThemeStyle
}

type Theme struct {
	Name     string
	Text     ThemeTextRoles
	Chrome   ThemeChromeRoles
	Focus    ThemeFocusRoles
	Metadata ThemeMetadataRoles
	Severity ThemeSeverityRoles
	Badges   ThemeBadgeRoles
	Logs     ThemeLogRoles
	Overlay  ThemeOverlayRoles
	Setup    ThemeSetupRoles
	Compose  ThemeComposeRoles
	Diff     ThemeDiffRoles
	Contacts ThemeContactsRoles
	Rules    ThemeRuleRoles
}

var inheritedTheme = Theme{
	Name: "inherited",
	Text: ThemeTextRoles{
		Primary:  ThemeStyle{},
		Muted:    ThemeStyle{},
		Dim:      ThemeStyle{},
		Disabled: ThemeStyle{},
	},
	Chrome: ThemeChromeRoles{
		TitleBar:     ThemeStyle{Bold: true, Reverse: true},
		TabActive:    ThemeStyle{Bold: true, Reverse: true},
		TabInactive:  ThemeStyle{},
		StatusBar:    ThemeStyle{Reverse: true},
		HintBar:      ThemeStyle{},
		TopSyncStrip: ThemeStyle{Foreground: lipgloss.Color("3"), Bold: true},
		TableHeader:  ThemeStyle{Bold: true, Reverse: true},
		Loading:      ThemeStyle{Foreground: lipgloss.Color("6"), Bold: true, Reverse: true},
		Progress:     ThemeStyle{Foreground: lipgloss.Color("8"), Faint: true},
	},
	Focus: ThemeFocusRoles{
		PanelBorder:        ThemeStyle{Foreground: lipgloss.Color("8")},
		PanelBorderFocused: ThemeStyle{Foreground: lipgloss.Color("6")},
		SelectionActive:    ThemeStyle{Bold: true, Reverse: true},
		SelectionInactive:  ThemeStyle{Underline: true},
		VisualSelection:    ThemeStyle{Reverse: true},
	},
	Metadata: ThemeMetadataRoles{
		Label:   ThemeStyle{},
		Sender:  ThemeStyle{Foreground: lipgloss.Color("6"), Bold: true},
		Date:    ThemeStyle{},
		Subject: ThemeStyle{Bold: true},
		Tag:     ThemeStyle{Foreground: lipgloss.Color("5"), Bold: true},
		Action:  ThemeStyle{Foreground: lipgloss.Color("3"), Bold: true},
	},
	Severity: ThemeSeverityRoles{
		Info:        ThemeStyle{Foreground: lipgloss.Color("6")},
		Success:     ThemeStyle{Foreground: lipgloss.Color("2")},
		Warning:     ThemeStyle{Foreground: lipgloss.Color("3")},
		Error:       ThemeStyle{Foreground: lipgloss.Color("1")},
		Caution:     ThemeStyle{Foreground: lipgloss.Color("0"), Background: lipgloss.Color("3"), Bold: true},
		Destructive: ThemeStyle{Foreground: lipgloss.Color("15"), Background: lipgloss.Color("1"), Bold: true},
	},
	Badges: ThemeBadgeRoles{
		Demo:   ThemeStyle{Foreground: lipgloss.Color("3"), Bold: true},
		DryRun: ThemeStyle{Foreground: lipgloss.Color("3"), Bold: true},
	},
	Logs: ThemeLogRoles{
		Info:  ThemeStyle{Foreground: lipgloss.Color("6")},
		Warn:  ThemeStyle{Foreground: lipgloss.Color("3")},
		Error: ThemeStyle{Foreground: lipgloss.Color("1")},
		Debug: ThemeStyle{Foreground: lipgloss.Color("8")},
	},
	Overlay: ThemeOverlayRoles{
		CompactBorder: ThemeStyle{Foreground: lipgloss.Color("62")},
		DemoKeyBadge:  ThemeStyle{Foreground: lipgloss.Color("230"), Background: lipgloss.Color("238"), Bold: true},
	},
	Setup: ThemeSetupRoles{
		Title:        ThemeStyle{Foreground: lipgloss.Color("205"), Bold: true},
		Spinner:      ThemeStyle{Foreground: lipgloss.Color("205")},
		Border:       ThemeStyle{Foreground: lipgloss.Color("62")},
		SummaryLabel: ThemeStyle{Foreground: lipgloss.Color("252"), Bold: true},
		SummaryBody:  ThemeStyle{Foreground: lipgloss.Color("250")},
		Link:         ThemeStyle{Foreground: lipgloss.Color("75"), Bold: true},
	},
	Compose: ThemeComposeRoles{
		Accent:           ThemeStyle{Foreground: lipgloss.Color("86")},
		Attachment:       ThemeStyle{Foreground: lipgloss.Color("111")},
		StatusInfo:       ThemeStyle{Foreground: lipgloss.Color("86")},
		StatusWarning:    ThemeStyle{Foreground: lipgloss.Color("214")},
		StatusError:      ThemeStyle{Foreground: lipgloss.Color("196")},
		AITitle:          ThemeStyle{Foreground: lipgloss.Color("86"), Bold: true},
		AILabel:          ThemeStyle{Foreground: lipgloss.Color("245")},
		AIToggleActive:   ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("25")},
		AIToggleInactive: ThemeStyle{Foreground: lipgloss.Color("240")},
		AIAction:         ThemeStyle{Foreground: lipgloss.Color("252"), Background: lipgloss.Color("236")},
		AIAccept:         ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("28")},
		AIDiscard:        ThemeStyle{Foreground: lipgloss.Color("245"), Background: lipgloss.Color("236")},
		AIDim:            ThemeStyle{Foreground: lipgloss.Color("240")},
		AIBorder:         ThemeStyle{Foreground: lipgloss.Color("86")},
	},
	Diff: ThemeDiffRoles{
		Delete: ThemeStyle{Foreground: lipgloss.Color("196"), Background: lipgloss.Color("52"), Strikethrough: true},
		Add:    ThemeStyle{Foreground: lipgloss.Color("46"), Background: lipgloss.Color("22")},
	},
	Contacts: ThemeContactsRoles{
		KeywordSearch:   ThemeStyle{Foreground: lipgloss.Color("33")},
		SelectedEmail:   ThemeStyle{Foreground: lipgloss.Color("183")},
		SelectedCompany: ThemeStyle{Foreground: lipgloss.Color("223")},
		Company:         ThemeStyle{Foreground: lipgloss.Color("249")},
	},
	Rules: ThemeRuleRoles{
		Title:      ThemeStyle{Foreground: lipgloss.Color("205"), Bold: true},
		Note:       ThemeStyle{Foreground: lipgloss.Color("243")},
		Selected:   ThemeStyle{Foreground: lipgloss.Color("229"), Background: lipgloss.Color("57")},
		DryRun:     ThemeStyle{Foreground: lipgloss.Color("229"), Background: lipgloss.Color("57")},
		Row:        ThemeStyle{Foreground: lipgloss.Color("252")},
		Error:      ThemeStyle{Foreground: lipgloss.Color("196")},
		GuideLabel: ThemeStyle{Foreground: lipgloss.Color("99"), Bold: true},
	},
}

var heraldDarkTheme = Theme{
	Name: "herald-dark",
	Text: ThemeTextRoles{
		Primary:  ThemeStyle{Foreground: lipgloss.Color("252")},
		Muted:    ThemeStyle{Foreground: lipgloss.Color("244")},
		Dim:      ThemeStyle{Foreground: lipgloss.Color("241")},
		Disabled: ThemeStyle{Foreground: lipgloss.Color("241"), Faint: true},
	},
	Chrome: ThemeChromeRoles{
		TitleBar:     ThemeStyle{Foreground: lipgloss.Color("205"), Background: lipgloss.Color("235"), Bold: true},
		TabActive:    ThemeStyle{Foreground: lipgloss.Color("229"), Background: lipgloss.Color("57"), Bold: true},
		TabInactive:  ThemeStyle{Foreground: lipgloss.Color("245")},
		StatusBar:    ThemeStyle{Foreground: lipgloss.Color("252"), Background: lipgloss.Color("0")},
		HintBar:      ThemeStyle{Foreground: lipgloss.Color("243"), Background: lipgloss.Color("235")},
		TopSyncStrip: ThemeStyle{Foreground: lipgloss.Color("214"), Background: lipgloss.Color("0")},
		TableHeader:  ThemeStyle{Foreground: lipgloss.Color("205"), Background: lipgloss.Color("235"), Bold: true},
		Loading:      ThemeStyle{Foreground: lipgloss.Color("86"), Background: lipgloss.Color("235"), Bold: true},
		Progress:     ThemeStyle{Foreground: lipgloss.Color("241")},
	},
	Focus: ThemeFocusRoles{
		PanelBorder:        ThemeStyle{Foreground: lipgloss.Color("240")},
		PanelBorderFocused: ThemeStyle{Foreground: lipgloss.Color("39")},
		SelectionActive:    ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("57"), Bold: true},
		SelectionInactive:  ThemeStyle{Foreground: lipgloss.Color("252"), Background: lipgloss.Color("240"), Underline: true},
		VisualSelection:    ThemeStyle{Foreground: lipgloss.Color("229"), Background: lipgloss.Color("57")},
	},
	Metadata: ThemeMetadataRoles{
		Label:   ThemeStyle{Foreground: lipgloss.Color("244")},
		Sender:  ThemeStyle{Foreground: lipgloss.Color("86"), Bold: true},
		Date:    ThemeStyle{Foreground: lipgloss.Color("245")},
		Subject: ThemeStyle{Foreground: lipgloss.Color("214"), Bold: true},
		Tag:     ThemeStyle{Foreground: lipgloss.Color("226"), Bold: true},
		Action:  ThemeStyle{Foreground: lipgloss.Color("255"), Bold: true},
	},
	Severity: ThemeSeverityRoles{
		Info:        ThemeStyle{Foreground: lipgloss.Color("86")},
		Success:     ThemeStyle{Foreground: lipgloss.Color("46")},
		Warning:     ThemeStyle{Foreground: lipgloss.Color("214")},
		Error:       ThemeStyle{Foreground: lipgloss.Color("196")},
		Caution:     ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("202")},
		Destructive: ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("160")},
	},
	Badges: ThemeBadgeRoles{
		Demo:   ThemeStyle{Foreground: lipgloss.Color("226"), Bold: true},
		DryRun: ThemeStyle{Foreground: lipgloss.Color("208"), Bold: true},
	},
	Logs: ThemeLogRoles{
		Info:  ThemeStyle{Foreground: lipgloss.Color("86")},
		Warn:  ThemeStyle{Foreground: lipgloss.Color("214")},
		Error: ThemeStyle{Foreground: lipgloss.Color("196")},
		Debug: ThemeStyle{Foreground: lipgloss.Color("241")},
	},
	Overlay: ThemeOverlayRoles{
		CompactBorder: ThemeStyle{Foreground: lipgloss.Color("62")},
		DemoKeyBadge:  ThemeStyle{Foreground: lipgloss.Color("230"), Background: lipgloss.Color("238"), Bold: true},
	},
	Setup: ThemeSetupRoles{
		Title:        ThemeStyle{Foreground: lipgloss.Color("205"), Bold: true},
		Spinner:      ThemeStyle{Foreground: lipgloss.Color("205")},
		Border:       ThemeStyle{Foreground: lipgloss.Color("62")},
		SummaryLabel: ThemeStyle{Foreground: lipgloss.Color("252"), Bold: true},
		SummaryBody:  ThemeStyle{Foreground: lipgloss.Color("250")},
		Link:         ThemeStyle{Foreground: lipgloss.Color("75"), Bold: true},
	},
	Compose: ThemeComposeRoles{
		Accent:           ThemeStyle{Foreground: lipgloss.Color("86")},
		Attachment:       ThemeStyle{Foreground: lipgloss.Color("111")},
		StatusInfo:       ThemeStyle{Foreground: lipgloss.Color("86")},
		StatusWarning:    ThemeStyle{Foreground: lipgloss.Color("214")},
		StatusError:      ThemeStyle{Foreground: lipgloss.Color("196")},
		AITitle:          ThemeStyle{Foreground: lipgloss.Color("86"), Bold: true},
		AILabel:          ThemeStyle{Foreground: lipgloss.Color("245")},
		AIToggleActive:   ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("25")},
		AIToggleInactive: ThemeStyle{Foreground: lipgloss.Color("240")},
		AIAction:         ThemeStyle{Foreground: lipgloss.Color("252"), Background: lipgloss.Color("236")},
		AIAccept:         ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("28")},
		AIDiscard:        ThemeStyle{Foreground: lipgloss.Color("245"), Background: lipgloss.Color("236")},
		AIDim:            ThemeStyle{Foreground: lipgloss.Color("240")},
		AIBorder:         ThemeStyle{Foreground: lipgloss.Color("86")},
	},
	Diff: ThemeDiffRoles{
		Delete: ThemeStyle{Foreground: lipgloss.Color("196"), Background: lipgloss.Color("52"), Strikethrough: true},
		Add:    ThemeStyle{Foreground: lipgloss.Color("46"), Background: lipgloss.Color("22")},
	},
	Contacts: ThemeContactsRoles{
		KeywordSearch:   ThemeStyle{Foreground: lipgloss.Color("33")},
		SelectedEmail:   ThemeStyle{Foreground: lipgloss.Color("183")},
		SelectedCompany: ThemeStyle{Foreground: lipgloss.Color("223")},
		Company:         ThemeStyle{Foreground: lipgloss.Color("249")},
	},
	Rules: ThemeRuleRoles{
		Title:      ThemeStyle{Foreground: lipgloss.Color("205"), Bold: true},
		Note:       ThemeStyle{Foreground: lipgloss.Color("243")},
		Selected:   ThemeStyle{Foreground: lipgloss.Color("229"), Background: lipgloss.Color("57")},
		DryRun:     ThemeStyle{Foreground: lipgloss.Color("229"), Background: lipgloss.Color("57")},
		Row:        ThemeStyle{Foreground: lipgloss.Color("252")},
		Error:      ThemeStyle{Foreground: lipgloss.Color("196")},
		GuideLabel: ThemeStyle{Foreground: lipgloss.Color("99"), Bold: true},
	},
}

var heraldLightTheme = Theme{
	Name: "herald-light",
	Text: ThemeTextRoles{
		Primary:  ThemeStyle{Foreground: lipgloss.Color("235")},
		Muted:    ThemeStyle{Foreground: lipgloss.Color("244")},
		Dim:      ThemeStyle{Foreground: lipgloss.Color("246")},
		Disabled: ThemeStyle{Foreground: lipgloss.Color("250"), Faint: true},
	},
	Chrome: ThemeChromeRoles{
		TitleBar:     ThemeStyle{Foreground: lipgloss.Color("25"), Background: lipgloss.Color("255"), Bold: true},
		TabActive:    ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("25"), Bold: true},
		TabInactive:  ThemeStyle{Foreground: lipgloss.Color("240")},
		StatusBar:    ThemeStyle{Foreground: lipgloss.Color("235"), Background: lipgloss.Color("254")},
		HintBar:      ThemeStyle{Foreground: lipgloss.Color("240"), Background: lipgloss.Color("255")},
		TopSyncStrip: ThemeStyle{Foreground: lipgloss.Color("130"), Background: lipgloss.Color("255"), Bold: true},
		TableHeader:  ThemeStyle{Foreground: lipgloss.Color("25"), Background: lipgloss.Color("254"), Bold: true},
		Loading:      ThemeStyle{Foreground: lipgloss.Color("25"), Background: lipgloss.Color("255"), Bold: true},
		Progress:     ThemeStyle{Foreground: lipgloss.Color("246")},
	},
	Focus: ThemeFocusRoles{
		PanelBorder:        ThemeStyle{Foreground: lipgloss.Color("250")},
		PanelBorderFocused: ThemeStyle{Foreground: lipgloss.Color("25")},
		SelectionActive:    ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("25"), Bold: true},
		SelectionInactive:  ThemeStyle{Foreground: lipgloss.Color("235"), Background: lipgloss.Color("253"), Underline: true},
		VisualSelection:    ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("31")},
	},
	Metadata: ThemeMetadataRoles{
		Label:   ThemeStyle{Foreground: lipgloss.Color("244")},
		Sender:  ThemeStyle{Foreground: lipgloss.Color("25"), Bold: true},
		Date:    ThemeStyle{Foreground: lipgloss.Color("244")},
		Subject: ThemeStyle{Foreground: lipgloss.Color("235"), Bold: true},
		Tag:     ThemeStyle{Foreground: lipgloss.Color("90"), Bold: true},
		Action:  ThemeStyle{Foreground: lipgloss.Color("130"), Bold: true},
	},
	Severity: ThemeSeverityRoles{
		Info:        ThemeStyle{Foreground: lipgloss.Color("25")},
		Success:     ThemeStyle{Foreground: lipgloss.Color("28")},
		Warning:     ThemeStyle{Foreground: lipgloss.Color("130")},
		Error:       ThemeStyle{Foreground: lipgloss.Color("160")},
		Caution:     ThemeStyle{Foreground: lipgloss.Color("235"), Background: lipgloss.Color("220"), Bold: true},
		Destructive: ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("160"), Bold: true},
	},
	Badges: ThemeBadgeRoles{
		Demo:   ThemeStyle{Foreground: lipgloss.Color("130"), Bold: true},
		DryRun: ThemeStyle{Foreground: lipgloss.Color("166"), Bold: true},
	},
	Logs: ThemeLogRoles{
		Info:  ThemeStyle{Foreground: lipgloss.Color("25")},
		Warn:  ThemeStyle{Foreground: lipgloss.Color("130")},
		Error: ThemeStyle{Foreground: lipgloss.Color("160")},
		Debug: ThemeStyle{Foreground: lipgloss.Color("246")},
	},
	Overlay: ThemeOverlayRoles{
		CompactBorder: ThemeStyle{Foreground: lipgloss.Color("61")},
		DemoKeyBadge:  ThemeStyle{Foreground: lipgloss.Color("235"), Background: lipgloss.Color("229"), Bold: true},
	},
	Setup: ThemeSetupRoles{
		Title:        ThemeStyle{Foreground: lipgloss.Color("25"), Bold: true},
		Spinner:      ThemeStyle{Foreground: lipgloss.Color("25")},
		Border:       ThemeStyle{Foreground: lipgloss.Color("61")},
		SummaryLabel: ThemeStyle{Foreground: lipgloss.Color("235"), Bold: true},
		SummaryBody:  ThemeStyle{Foreground: lipgloss.Color("240")},
		Link:         ThemeStyle{Foreground: lipgloss.Color("25"), Bold: true},
	},
	Compose: ThemeComposeRoles{
		Accent:           ThemeStyle{Foreground: lipgloss.Color("29")},
		Attachment:       ThemeStyle{Foreground: lipgloss.Color("25")},
		StatusInfo:       ThemeStyle{Foreground: lipgloss.Color("29")},
		StatusWarning:    ThemeStyle{Foreground: lipgloss.Color("130")},
		StatusError:      ThemeStyle{Foreground: lipgloss.Color("160")},
		AITitle:          ThemeStyle{Foreground: lipgloss.Color("29"), Bold: true},
		AILabel:          ThemeStyle{Foreground: lipgloss.Color("244")},
		AIToggleActive:   ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("25")},
		AIToggleInactive: ThemeStyle{Foreground: lipgloss.Color("244")},
		AIAction:         ThemeStyle{Foreground: lipgloss.Color("235"), Background: lipgloss.Color("253")},
		AIAccept:         ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("28")},
		AIDiscard:        ThemeStyle{Foreground: lipgloss.Color("240"), Background: lipgloss.Color("253")},
		AIDim:            ThemeStyle{Foreground: lipgloss.Color("246")},
		AIBorder:         ThemeStyle{Foreground: lipgloss.Color("29")},
	},
	Diff: ThemeDiffRoles{
		Delete: ThemeStyle{Foreground: lipgloss.Color("160"), Background: lipgloss.Color("224"), Strikethrough: true},
		Add:    ThemeStyle{Foreground: lipgloss.Color("22"), Background: lipgloss.Color("194")},
	},
	Contacts: ThemeContactsRoles{
		KeywordSearch:   ThemeStyle{Foreground: lipgloss.Color("25")},
		SelectedEmail:   ThemeStyle{Foreground: lipgloss.Color("54")},
		SelectedCompany: ThemeStyle{Foreground: lipgloss.Color("94")},
		Company:         ThemeStyle{Foreground: lipgloss.Color("240")},
	},
	Rules: ThemeRuleRoles{
		Title:      ThemeStyle{Foreground: lipgloss.Color("25"), Bold: true},
		Note:       ThemeStyle{Foreground: lipgloss.Color("244")},
		Selected:   ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("25")},
		DryRun:     ThemeStyle{Foreground: lipgloss.Color("255"), Background: lipgloss.Color("25")},
		Row:        ThemeStyle{Foreground: lipgloss.Color("235")},
		Error:      ThemeStyle{Foreground: lipgloss.Color("160")},
		GuideLabel: ThemeStyle{Foreground: lipgloss.Color("61"), Bold: true},
	},
}

var defaultTheme = inheritedTheme

type ThemeDocument struct {
	Version     int                             `yaml:"version"`
	Name        string                          `yaml:"name"`
	DisplayName string                          `yaml:"display_name,omitempty"`
	Inherits    string                          `yaml:"inherits,omitempty"`
	Roles       map[string]config.ThemeOverride `yaml:"roles,omitempty"`
}

var themeSlugRE = regexp.MustCompile(`^[a-z0-9_-]+$`)

func ThemeByName(name string) Theme {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "inherited", "adaptive":
		return inheritedTheme
	case "legacy", "legacy-dark", "legacy_dark", "herald-dark":
		return heraldDarkTheme
	case "herald-light":
		return heraldLightTheme
	default:
		return inheritedTheme
	}
}

func builtInThemeNames() []string {
	return []string{"inherited", "herald-dark", "herald-light"}
}

func isBuiltInThemeName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "inherited", "adaptive", "herald-dark", "legacy", "legacy-dark", "legacy_dark", "herald-light":
		return true
	default:
		return false
	}
}

func ThemeDisplayNames(themeDir string) []string {
	names := builtInThemeNames()
	var installed []string
	for _, doc := range loadInstalledThemeDocuments(themeDir) {
		installed = append(installed, doc.Name)
	}
	sort.Strings(installed)
	names = append(names, installed...)
	return names
}

func DefaultThemeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".herald", "themes")
}

func ResolveThemeForConfig(cfg *config.Config, themeDir string) (Theme, string) {
	name := "inherited"
	overrides := map[string]config.ThemeOverride(nil)
	if cfg != nil {
		if strings.TrimSpace(cfg.Theme.Name) != "" {
			name = cfg.Theme.Name
		}
		overrides = cfg.Theme.Overrides
	}
	theme, warning := resolveThemeName(name, themeDir)
	if warning != "" {
		return inheritedTheme, warning
	}
	if len(overrides) > 0 {
		if err := applyThemeOverrides(&theme, overrides); err != nil {
			return inheritedTheme, fmt.Sprintf("Theme override ignored: %v", err)
		}
	}
	return theme, ""
}

func resolveThemeName(name, themeDir string) (Theme, string) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" || isBuiltInThemeName(normalized) {
		return ThemeByName(normalized), ""
	}
	if !themeSlugRE.MatchString(normalized) {
		return inheritedTheme, fmt.Sprintf("Invalid theme name %q; using inherited.", name)
	}
	if themeDir == "" {
		themeDir = DefaultThemeDir()
	}
	doc, err := LoadThemeFromFile(filepath.Join(themeDir, normalized+".yaml"))
	if err != nil {
		return inheritedTheme, fmt.Sprintf("Theme %q failed to load: %v; using inherited.", normalized, err)
	}
	theme := ThemeByName(doc.Inherits)
	theme.Name = doc.Name
	if err := applyThemeOverrides(&theme, doc.Roles); err != nil {
		return inheritedTheme, fmt.Sprintf("Theme %q failed to apply: %v; using inherited.", normalized, err)
	}
	return theme, ""
}

func loadInstalledThemeDocuments(themeDir string) []ThemeDocument {
	if themeDir == "" {
		themeDir = DefaultThemeDir()
	}
	if themeDir == "" {
		return nil
	}
	entries, err := os.ReadDir(themeDir)
	if err != nil {
		return nil
	}
	var docs []ThemeDocument
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		doc, err := LoadThemeFromFile(filepath.Join(themeDir, entry.Name()))
		if err == nil {
			docs = append(docs, doc)
		}
	}
	return docs
}

func LoadThemeFromFile(path string) (ThemeDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ThemeDocument{}, err
	}
	var doc ThemeDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return ThemeDocument{}, fmt.Errorf("parse theme YAML: %w", err)
	}
	if err := validateThemeDocument(doc); err != nil {
		return ThemeDocument{}, err
	}
	return doc, nil
}

func validateThemeDocument(doc ThemeDocument) error {
	if doc.Version != 1 {
		return fmt.Errorf("unsupported theme version %d", doc.Version)
	}
	if !themeSlugRE.MatchString(doc.Name) {
		return fmt.Errorf("invalid theme slug %q", doc.Name)
	}
	if isBuiltInThemeName(doc.Name) {
		return fmt.Errorf("theme slug %q is reserved", doc.Name)
	}
	if strings.TrimSpace(doc.Inherits) != "" && !isBuiltInThemeName(doc.Inherits) {
		return fmt.Errorf("theme inherits %q is not a built-in theme", doc.Inherits)
	}
	for role, override := range doc.Roles {
		if _, ok := themeRoleMap(&inheritedTheme)[role]; !ok {
			return fmt.Errorf("unknown theme role %q", role)
		}
		if _, err := parseThemeColor(override.Foreground); err != nil {
			return fmt.Errorf("invalid color for %s.fg: %w", role, err)
		}
		if _, err := parseThemeColor(override.Background); err != nil {
			return fmt.Errorf("invalid color for %s.bg: %w", role, err)
		}
	}
	return nil
}

func InstallThemeFile(srcPath, destDir string) (string, error) {
	doc, err := LoadThemeFromFile(srcPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, doc.Name+".yaml")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", err
	}
	return dest, nil
}

func SaveThemeDocument(doc ThemeDocument, destDir string) (string, error) {
	if doc.Version == 0 {
		doc.Version = 1
	}
	if doc.Inherits == "" {
		doc.Inherits = "herald-dark"
	}
	if err := validateThemeDocument(doc); err != nil {
		return "", err
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, doc.Name+".yaml")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", err
	}
	return dest, nil
}

func applyThemeOverrides(theme *Theme, overrides map[string]config.ThemeOverride) error {
	roles := themeRoleMap(theme)
	for role, override := range overrides {
		target, ok := roles[role]
		if !ok {
			return fmt.Errorf("unknown theme role %q", role)
		}
		if err := applyThemeOverride(target, override); err != nil {
			return fmt.Errorf("%s: %w", role, err)
		}
	}
	return nil
}

func applyThemeOverride(target *ThemeStyle, override config.ThemeOverride) error {
	if strings.TrimSpace(override.Foreground) != "" {
		color, err := parseThemeColor(override.Foreground)
		if err != nil {
			return err
		}
		target.Foreground = color
	}
	if strings.TrimSpace(override.Background) != "" {
		color, err := parseThemeColor(override.Background)
		if err != nil {
			return err
		}
		target.Background = color
	}
	if override.Bold {
		target.Bold = true
	}
	if override.Faint {
		target.Faint = true
	}
	if override.Underline {
		target.Underline = true
	}
	if override.Reverse {
		target.Reverse = true
	}
	if override.Strikethrough {
		target.Strikethrough = true
	}
	return nil
}

func parseThemeColor(value string) (color.Color, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil, nil
	}
	if strings.EqualFold(v, "inherit") {
		return nil, nil
	}
	lower := strings.ToLower(v)
	for _, prefix := range []string{"ansi:", "xterm:"} {
		if strings.HasPrefix(lower, prefix) {
			n, err := strconv.Atoi(strings.TrimPrefix(lower, prefix))
			if err != nil || n < 0 || n > 255 {
				return nil, fmt.Errorf("invalid color %q", value)
			}
			return lipgloss.Color(strconv.Itoa(n)), nil
		}
	}
	if len(v) == 7 && strings.HasPrefix(v, "#") {
		for _, r := range v[1:] {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return nil, fmt.Errorf("invalid color %q", value)
			}
		}
		return lipgloss.Color(v), nil
	}
	return nil, fmt.Errorf("invalid color %q", value)
}

func themeRoleMap(t *Theme) map[string]*ThemeStyle {
	return map[string]*ThemeStyle{
		"text.primary":               &t.Text.Primary,
		"text.muted":                 &t.Text.Muted,
		"text.dim":                   &t.Text.Dim,
		"text.disabled":              &t.Text.Disabled,
		"chrome.title_bar":           &t.Chrome.TitleBar,
		"chrome.tab_active":          &t.Chrome.TabActive,
		"chrome.tab_inactive":        &t.Chrome.TabInactive,
		"chrome.status_bar":          &t.Chrome.StatusBar,
		"chrome.hint_bar":            &t.Chrome.HintBar,
		"chrome.top_sync_strip":      &t.Chrome.TopSyncStrip,
		"chrome.table_header":        &t.Chrome.TableHeader,
		"chrome.loading":             &t.Chrome.Loading,
		"chrome.progress":            &t.Chrome.Progress,
		"focus.panel_border":         &t.Focus.PanelBorder,
		"focus.panel_border_focused": &t.Focus.PanelBorderFocused,
		"focus.selection_active":     &t.Focus.SelectionActive,
		"focus.selection_inactive":   &t.Focus.SelectionInactive,
		"focus.visual_selection":     &t.Focus.VisualSelection,
		"metadata.label":             &t.Metadata.Label,
		"metadata.sender":            &t.Metadata.Sender,
		"metadata.date":              &t.Metadata.Date,
		"metadata.subject":           &t.Metadata.Subject,
		"metadata.tag":               &t.Metadata.Tag,
		"metadata.action":            &t.Metadata.Action,
		"severity.info":              &t.Severity.Info,
		"severity.success":           &t.Severity.Success,
		"severity.warning":           &t.Severity.Warning,
		"severity.error":             &t.Severity.Error,
		"severity.caution":           &t.Severity.Caution,
		"severity.destructive":       &t.Severity.Destructive,
		"badges.demo":                &t.Badges.Demo,
		"badges.dry_run":             &t.Badges.DryRun,
		"logs.info":                  &t.Logs.Info,
		"logs.warn":                  &t.Logs.Warn,
		"logs.error":                 &t.Logs.Error,
		"logs.debug":                 &t.Logs.Debug,
		"overlay.compact_border":     &t.Overlay.CompactBorder,
		"overlay.demo_key_badge":     &t.Overlay.DemoKeyBadge,
		"setup.title":                &t.Setup.Title,
		"setup.spinner":              &t.Setup.Spinner,
		"setup.border":               &t.Setup.Border,
		"setup.summary_label":        &t.Setup.SummaryLabel,
		"setup.summary_body":         &t.Setup.SummaryBody,
		"setup.link":                 &t.Setup.Link,
		"compose.accent":             &t.Compose.Accent,
		"compose.attachment":         &t.Compose.Attachment,
		"compose.status_info":        &t.Compose.StatusInfo,
		"compose.status_warning":     &t.Compose.StatusWarning,
		"compose.status_error":       &t.Compose.StatusError,
		"compose.ai_title":           &t.Compose.AITitle,
		"compose.ai_label":           &t.Compose.AILabel,
		"compose.ai_toggle_active":   &t.Compose.AIToggleActive,
		"compose.ai_toggle_inactive": &t.Compose.AIToggleInactive,
		"compose.ai_action":          &t.Compose.AIAction,
		"compose.ai_accept":          &t.Compose.AIAccept,
		"compose.ai_discard":         &t.Compose.AIDiscard,
		"compose.ai_dim":             &t.Compose.AIDim,
		"compose.ai_border":          &t.Compose.AIBorder,
		"diff.delete":                &t.Diff.Delete,
		"diff.add":                   &t.Diff.Add,
		"contacts.keyword_search":    &t.Contacts.KeywordSearch,
		"contacts.selected_email":    &t.Contacts.SelectedEmail,
		"contacts.selected_company":  &t.Contacts.SelectedCompany,
		"contacts.company":           &t.Contacts.Company,
		"rules.title":                &t.Rules.Title,
		"rules.note":                 &t.Rules.Note,
		"rules.selected":             &t.Rules.Selected,
		"rules.dry_run":              &t.Rules.DryRun,
		"rules.row":                  &t.Rules.Row,
		"rules.error":                &t.Rules.Error,
		"rules.guide_label":          &t.Rules.GuideLabel,
	}
}

func (t Theme) borderColor(role ThemeStyle) color.Color {
	if role.Foreground != nil {
		return role.Foreground
	}
	return lipgloss.NoColor{}
}

func (t Theme) PanelBorderColor(focused bool) color.Color {
	if focused {
		return t.borderColor(t.Focus.PanelBorderFocused)
	}
	return t.borderColor(t.Focus.PanelBorder)
}

func (t Theme) BasePanelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(t.PanelBorderColor(false))
}

func (t Theme) TitleBarStyle() lipgloss.Style {
	return t.Chrome.TitleBar.Style().Padding(0, 1)
}

func (t Theme) LoadingStyle() lipgloss.Style {
	return t.Chrome.Loading.Style().
		Padding(1, 3).
		Margin(1, 0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.borderColor(t.Severity.Info)).
		Align(lipgloss.Center)
}

func (t Theme) ProgressStyle() lipgloss.Style {
	return t.Chrome.Progress.Style().Margin(0, 2)
}

func (t Theme) TableStyles(active bool) table.Styles {
	styles := table.DefaultStyles()
	styles.Header = t.Chrome.TableHeader.Apply(styles.Header).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(t.PanelBorderColor(false)).
		BorderBottom(true)
	if active {
		styles.Selected = t.Focus.SelectionActive.Apply(styles.Selected)
	} else {
		styles.Selected = t.Focus.SelectionInactive.Apply(styles.Selected)
	}
	return styles
}
