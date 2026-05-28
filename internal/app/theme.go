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
	"github.com/charmbracelet/x/ansi"
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

func dynamicForegroundStyle(value string) lipgloss.Style {
	value = strings.TrimSpace(value)
	if value == "" {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(value))
}

func dynamicCalendarBlockStyle(foreground, background string) lipgloss.Style {
	style := lipgloss.NewStyle()
	if strings.TrimSpace(foreground) != "" {
		style = style.Foreground(lipgloss.Color(foreground))
	}
	if strings.TrimSpace(background) != "" {
		style = style.Background(lipgloss.Color(background))
	}
	return style
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
	FormHelp     ThemeStyle
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
		FormHelp:     ThemeStyle{Foreground: lipgloss.Color("8"), Faint: true},
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
		FormHelp:     ThemeStyle{Foreground: lipgloss.Color("243"), Faint: true},
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
		FormHelp:     ThemeStyle{Foreground: lipgloss.Color("244"), Faint: true},
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

type terminalThemePalette struct {
	Name          string
	Text          string
	Muted         string
	Dim           string
	Surface       string
	SurfaceStrong string
	Accent        string
	Accent2       string
	Accent3       string
	Accent4       string
	SelectionFG   string
	SelectionBG   string
	Border        string
	FocusedBorder string
	Success       string
	Warning       string
	Error         string
	DestructiveFG string
	DestructiveBG string
}

func themeColor(value string) color.Color {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return lipgloss.Color(value)
}

func themeStyle(fg, bg string, attrs ...string) ThemeStyle {
	style := ThemeStyle{
		Foreground: themeColor(fg),
		Background: themeColor(bg),
	}
	for _, attr := range attrs {
		switch attr {
		case "bold":
			style.Bold = true
		case "faint":
			style.Faint = true
		case "underline":
			style.Underline = true
		case "reverse":
			style.Reverse = true
		case "strike":
			style.Strikethrough = true
		}
	}
	return style
}

func terminalTheme(p terminalThemePalette) Theme {
	background := p.SurfaceStrong
	return Theme{
		Name: p.Name,
		Text: ThemeTextRoles{
			Primary:  themeStyle(p.Text, background),
			Muted:    themeStyle(p.Muted, background),
			Dim:      themeStyle(p.Dim, background),
			Disabled: themeStyle(p.Dim, background, "faint"),
		},
		Chrome: ThemeChromeRoles{
			TitleBar:     themeStyle(p.Accent, p.Surface, "bold"),
			TabActive:    themeStyle(p.SelectionFG, p.SelectionBG, "bold"),
			TabInactive:  themeStyle(p.Muted, ""),
			StatusBar:    themeStyle(p.Text, p.SurfaceStrong),
			HintBar:      themeStyle(p.Muted, p.Surface),
			FormHelp:     themeStyle(p.Dim, "", "faint"),
			TopSyncStrip: themeStyle(p.Warning, p.SurfaceStrong, "bold"),
			TableHeader:  themeStyle(p.Accent, p.Surface, "bold"),
			Loading:      themeStyle(p.Accent2, p.Surface, "bold"),
			Progress:     themeStyle(p.Dim, ""),
		},
		Focus: ThemeFocusRoles{
			PanelBorder:        themeStyle(p.Border, ""),
			PanelBorderFocused: themeStyle(p.FocusedBorder, ""),
			SelectionActive:    themeStyle(p.SelectionFG, p.SelectionBG, "bold"),
			SelectionInactive:  themeStyle(p.Text, p.Surface, "underline"),
			VisualSelection:    themeStyle(p.SelectionFG, p.SelectionBG),
		},
		Metadata: ThemeMetadataRoles{
			Label:   themeStyle(p.Muted, ""),
			Sender:  themeStyle(p.Accent2, "", "bold"),
			Date:    themeStyle(p.Muted, ""),
			Subject: themeStyle(p.Accent3, "", "bold"),
			Tag:     themeStyle(p.Accent4, "", "bold"),
			Action:  themeStyle(p.Text, "", "bold"),
		},
		Severity: ThemeSeverityRoles{
			Info:        themeStyle(p.Accent2, ""),
			Success:     themeStyle(p.Success, ""),
			Warning:     themeStyle(p.Warning, ""),
			Error:       themeStyle(p.Error, ""),
			Caution:     themeStyle(p.SurfaceStrong, p.Warning, "bold"),
			Destructive: themeStyle(p.DestructiveFG, p.DestructiveBG, "bold"),
		},
		Badges: ThemeBadgeRoles{
			Demo:   themeStyle(p.Warning, "", "bold"),
			DryRun: themeStyle(p.Error, "", "bold"),
		},
		Logs: ThemeLogRoles{
			Info:  themeStyle(p.Accent2, ""),
			Warn:  themeStyle(p.Warning, ""),
			Error: themeStyle(p.Error, ""),
			Debug: themeStyle(p.Dim, ""),
		},
		Overlay: ThemeOverlayRoles{
			CompactBorder: themeStyle(p.FocusedBorder, ""),
			DemoKeyBadge:  themeStyle(p.Text, p.SurfaceStrong, "bold"),
		},
		Setup: ThemeSetupRoles{
			Title:        themeStyle(p.Accent, "", "bold"),
			Spinner:      themeStyle(p.Accent, ""),
			Border:       themeStyle(p.FocusedBorder, ""),
			SummaryLabel: themeStyle(p.Text, "", "bold"),
			SummaryBody:  themeStyle(p.Muted, ""),
			Link:         themeStyle(p.Accent2, "", "bold"),
		},
		Compose: ThemeComposeRoles{
			Accent:           themeStyle(p.Accent2, ""),
			Attachment:       themeStyle(p.Accent4, ""),
			StatusInfo:       themeStyle(p.Accent2, ""),
			StatusWarning:    themeStyle(p.Warning, ""),
			StatusError:      themeStyle(p.Error, ""),
			AITitle:          themeStyle(p.Accent2, "", "bold"),
			AILabel:          themeStyle(p.Muted, ""),
			AIToggleActive:   themeStyle(p.SelectionFG, p.SelectionBG),
			AIToggleInactive: themeStyle(p.Dim, ""),
			AIAction:         themeStyle(p.Text, p.Surface),
			AIAccept:         themeStyle(p.SelectionFG, p.Success),
			AIDiscard:        themeStyle(p.Muted, p.Surface),
			AIDim:            themeStyle(p.Dim, ""),
			AIBorder:         themeStyle(p.Accent2, ""),
		},
		Diff: ThemeDiffRoles{
			Delete: themeStyle(p.Error, p.DestructiveBG, "strike"),
			Add:    themeStyle(p.Success, p.SurfaceStrong),
		},
		Contacts: ThemeContactsRoles{
			KeywordSearch:   themeStyle(p.Accent2, ""),
			SelectedEmail:   themeStyle(p.Accent4, ""),
			SelectedCompany: themeStyle(p.Warning, ""),
			Company:         themeStyle(p.Muted, ""),
		},
		Rules: ThemeRuleRoles{
			Title:      themeStyle(p.Accent, "", "bold"),
			Note:       themeStyle(p.Dim, ""),
			Selected:   themeStyle(p.SelectionFG, p.SelectionBG),
			DryRun:     themeStyle(p.SelectionFG, p.SelectionBG),
			Row:        themeStyle(p.Text, ""),
			Error:      themeStyle(p.Error, ""),
			GuideLabel: themeStyle(p.FocusedBorder, "", "bold"),
		},
	}
}

var selectedBuiltInThemes = []Theme{
	terminalTheme(terminalThemePalette{Name: "red-black", Text: "#f4e8e8", Muted: "#b88787", Dim: "#765151", Surface: "#211010", SurfaceStrong: "#070303", Accent: "#ff4d5d", Accent2: "#ff6b6b", Accent3: "#ffd0d0", Accent4: "#f5a524", SelectionFG: "#fff5f5", SelectionBG: "#8f1f2d", Border: "#5b2b31", FocusedBorder: "#ff4d5d", Success: "#8bdc9a", Warning: "#f5a524", Error: "#ff4d5d", DestructiveFG: "#fff5f5", DestructiveBG: "#a9152a"}),
	terminalTheme(terminalThemePalette{Name: "crimson", Text: "#ffe9f0", Muted: "#d2899d", Dim: "#845464", Surface: "#2a101a", SurfaceStrong: "#12050b", Accent: "#ff3d7f", Accent2: "#ff6a9d", Accent3: "#ffd35a", Accent4: "#c792ea", SelectionFG: "#fff7fb", SelectionBG: "#9b174d", Border: "#693043", FocusedBorder: "#ff6a9d", Success: "#7ee787", Warning: "#ffd35a", Error: "#ff3d7f", DestructiveFG: "#fff7fb", DestructiveBG: "#b31349"}),
	terminalTheme(terminalThemePalette{Name: "ember", Text: "#fff0df", Muted: "#c7986c", Dim: "#7b5d47", Surface: "#271408", SurfaceStrong: "#100701", Accent: "#ff6a00", Accent2: "#ff9e40", Accent3: "#ffd27f", Accent4: "#e85d75", SelectionFG: "#1a0900", SelectionBG: "#ff9e40", Border: "#6f3b18", FocusedBorder: "#ff8a1c", Success: "#a6e3a1", Warning: "#ffd166", Error: "#ff5c57", DestructiveFG: "#fff0df", DestructiveBG: "#a33a00"}),
	terminalTheme(terminalThemePalette{Name: "ruby-noir", Text: "#f7e6ec", Muted: "#ad7d8b", Dim: "#68505a", Surface: "#1c0d14", SurfaceStrong: "#080306", Accent: "#e84a6a", Accent2: "#ff7a90", Accent3: "#f3c969", Accent4: "#8bd5ca", SelectionFG: "#fff8fa", SelectionBG: "#731b2e", Border: "#4b2733", FocusedBorder: "#e84a6a", Success: "#8bd5ca", Warning: "#f3c969", Error: "#ff5d73", DestructiveFG: "#fff8fa", DestructiveBG: "#931733"}),
	terminalTheme(terminalThemePalette{Name: "garnet-console", Text: "#eadfdd", Muted: "#b17d78", Dim: "#704d4a", Surface: "#261312", SurfaceStrong: "#0b0404", Accent: "#d74b4b", Accent2: "#f26d6d", Accent3: "#f6c177", Accent4: "#9ccfd8", SelectionFG: "#fff4ef", SelectionBG: "#783232", Border: "#5a3230", FocusedBorder: "#f26d6d", Success: "#a6da95", Warning: "#f6c177", Error: "#ed8796", DestructiveFG: "#fff4ef", DestructiveBG: "#8f2424"}),
	terminalTheme(terminalThemePalette{Name: "jade-signal", Text: "#def7e8", Muted: "#87baa0", Dim: "#577865", Surface: "#0b2017", SurfaceStrong: "#03110b", Accent: "#35e89b", Accent2: "#50fa7b", Accent3: "#c7f59b", Accent4: "#5ccfe6", SelectionFG: "#00140c", SelectionBG: "#35e89b", Border: "#244b39", FocusedBorder: "#50fa7b", Success: "#50fa7b", Warning: "#f1fa8c", Error: "#ff6e6e", DestructiveFG: "#fff5f5", DestructiveBG: "#9b1c31"}),
	terminalTheme(terminalThemePalette{Name: "viridian-glass", Text: "#d9fff2", Muted: "#88c7b5", Dim: "#547c70", Surface: "#10261f", SurfaceStrong: "#06120f", Accent: "#00d7a7", Accent2: "#38f2c2", Accent3: "#a6ffcb", Accent4: "#7aa2f7", SelectionFG: "#001510", SelectionBG: "#38f2c2", Border: "#2a5f50", FocusedBorder: "#00d7a7", Success: "#a6ffcb", Warning: "#e0af68", Error: "#f7768e", DestructiveFG: "#fff3f3", DestructiveBG: "#8c1d40"}),
	terminalTheme(terminalThemePalette{Name: "forest-crt", Text: "#d8f5c7", Muted: "#8fb17f", Dim: "#60735a", Surface: "#12200f", SurfaceStrong: "#050d04", Accent: "#9ece6a", Accent2: "#73daca", Accent3: "#c3e88d", Accent4: "#e0af68", SelectionFG: "#071003", SelectionBG: "#9ece6a", Border: "#34472e", FocusedBorder: "#73daca", Success: "#9ece6a", Warning: "#e0af68", Error: "#f7768e", DestructiveFG: "#fff8f0", DestructiveBG: "#873934"}),
	terminalTheme(terminalThemePalette{Name: "pine-mail", Text: "#e4f3dd", Muted: "#98b18d", Dim: "#63715e", Surface: "#172315", SurfaceStrong: "#080f07", Accent: "#88c070", Accent2: "#a6da95", Accent3: "#eed49f", Accent4: "#8bd5ca", SelectionFG: "#071005", SelectionBG: "#88c070", Border: "#3a4c34", FocusedBorder: "#a6da95", Success: "#a6da95", Warning: "#eed49f", Error: "#ed8796", DestructiveFG: "#fff5f5", DestructiveBG: "#983a45"}),
	terminalTheme(terminalThemePalette{Name: "amber-furnace", Text: "#fff2d6", Muted: "#c9a56b", Dim: "#7f6744", Surface: "#241806", SurfaceStrong: "#0e0801", Accent: "#ffb000", Accent2: "#ffc857", Accent3: "#ff8c42", Accent4: "#7dd3fc", SelectionFG: "#190d00", SelectionBG: "#ffc857", Border: "#69460e", FocusedBorder: "#ffb000", Success: "#a6e3a1", Warning: "#ffb000", Error: "#ff5d5d", DestructiveFG: "#fff6e6", DestructiveBG: "#9a3d00"}),
	terminalTheme(terminalThemePalette{Name: "copper-ash", Text: "#f4e4d0", Muted: "#b99170", Dim: "#76604e", Surface: "#24170f", SurfaceStrong: "#0d0704", Accent: "#d99058", Accent2: "#f2a65a", Accent3: "#e0c097", Accent4: "#88c0d0", SelectionFG: "#140804", SelectionBG: "#d99058", Border: "#60402e", FocusedBorder: "#f2a65a", Success: "#a3be8c", Warning: "#ebcb8b", Error: "#bf616a", DestructiveFG: "#fff4ed", DestructiveBG: "#874421"}),
	terminalTheme(terminalThemePalette{Name: "magma-core", Text: "#ffe7d6", Muted: "#c78769", Dim: "#7b5546", Surface: "#2a100b", SurfaceStrong: "#100301", Accent: "#ff4b1f", Accent2: "#ff7a30", Accent3: "#ffd166", Accent4: "#ff4d8d", SelectionFG: "#170300", SelectionBG: "#ff7a30", Border: "#673124", FocusedBorder: "#ff4b1f", Success: "#b8f28b", Warning: "#ffd166", Error: "#ff4d6d", DestructiveFG: "#fff3ed", DestructiveBG: "#b22b16"}),
	terminalTheme(terminalThemePalette{Name: "peacock-ink", Text: "#dff7f8", Muted: "#85b8c4", Dim: "#587982", Surface: "#0c1d26", SurfaceStrong: "#031017", Accent: "#00c2d1", Accent2: "#5eead4", Accent3: "#b9fbc0", Accent4: "#c4b5fd", SelectionFG: "#021015", SelectionBG: "#00c2d1", Border: "#29515d", FocusedBorder: "#5eead4", Success: "#8bd450", Warning: "#f4d35e", Error: "#ff6b8a", DestructiveFG: "#fff5fa", DestructiveBG: "#8f1d3f"}),
	terminalTheme(terminalThemePalette{Name: "ultramarine-desk", Text: "#e6edff", Muted: "#94a8d8", Dim: "#64749a", Surface: "#101a3a", SurfaceStrong: "#060b1e", Accent: "#6ea8fe", Accent2: "#7dcfff", Accent3: "#f7d774", Accent4: "#bb9af7", SelectionFG: "#061022", SelectionBG: "#7dcfff", Border: "#2e4076", FocusedBorder: "#6ea8fe", Success: "#9ece6a", Warning: "#f7d774", Error: "#ff7a93", DestructiveFG: "#fff5fa", DestructiveBG: "#95284d"}),
	terminalTheme(terminalThemePalette{Name: "amethyst-night", Text: "#f2e9ff", Muted: "#b69ad2", Dim: "#756584", Surface: "#1d142e", SurfaceStrong: "#0b0614", Accent: "#bd93f9", Accent2: "#ff79c6", Accent3: "#f1fa8c", Accent4: "#8be9fd", SelectionFG: "#14081d", SelectionBG: "#bd93f9", Border: "#514067", FocusedBorder: "#ff79c6", Success: "#50fa7b", Warning: "#f1fa8c", Error: "#ff5555", DestructiveFG: "#fff4fb", DestructiveBG: "#8b2358"}),
	terminalTheme(terminalThemePalette{Name: "graphite-rose", Text: "#eee7ea", Muted: "#ab9aa2", Dim: "#70686c", Surface: "#1f1c20", SurfaceStrong: "#0c0a0d", Accent: "#eb6f92", Accent2: "#f6c177", Accent3: "#c4a7e7", Accent4: "#9ccfd8", SelectionFG: "#1b0b10", SelectionBG: "#eb6f92", Border: "#4d444b", FocusedBorder: "#c4a7e7", Success: "#9ccfd8", Warning: "#f6c177", Error: "#eb6f92", DestructiveFG: "#fff8fa", DestructiveBG: "#92324d"}),
	terminalTheme(terminalThemePalette{Name: "olive-circuit", Text: "#edf3d4", Muted: "#a9b278", Dim: "#70774e", Surface: "#1b2210", SurfaceStrong: "#080d03", Accent: "#b8bb26", Accent2: "#8ec07c", Accent3: "#fabd2f", Accent4: "#83a598", SelectionFG: "#101400", SelectionBG: "#b8bb26", Border: "#4d5727", FocusedBorder: "#8ec07c", Success: "#8ec07c", Warning: "#fabd2f", Error: "#fb4934", DestructiveFG: "#fff4ee", DestructiveBG: "#9d2f21"}),
	terminalTheme(terminalThemePalette{Name: "arctic-signal", Text: "#e8f1ff", Muted: "#9eb4cc", Dim: "#6b7c91", Surface: "#17212f", SurfaceStrong: "#07101b", Accent: "#88c0d0", Accent2: "#8fbcbb", Accent3: "#ebcb8b", Accent4: "#b48ead", SelectionFG: "#08121c", SelectionBG: "#88c0d0", Border: "#3b4c60", FocusedBorder: "#81a1c1", Success: "#a3be8c", Warning: "#ebcb8b", Error: "#bf616a", DestructiveFG: "#fff5f5", DestructiveBG: "#8f3840"}),
	terminalTheme(terminalThemePalette{Name: "sepia-debug", Text: "#f1e4c8", Muted: "#b9a47d", Dim: "#7c6e55", Surface: "#211a10", SurfaceStrong: "#0d0904", Accent: "#d6b16d", Accent2: "#e6c384", Accent3: "#a3be8c", Accent4: "#8fbcbb", SelectionFG: "#140e04", SelectionBG: "#d6b16d", Border: "#55472e", FocusedBorder: "#e6c384", Success: "#a3be8c", Warning: "#d6b16d", Error: "#d08770", DestructiveFG: "#fff7ed", DestructiveBG: "#8b4b2f"}),
	terminalTheme(terminalThemePalette{Name: "ayu-courier", Text: "#e6e1cf", Muted: "#8a9199", Dim: "#5c6773", Surface: "#14191f", SurfaceStrong: "#0a0e14", Accent: "#ffcc66", Accent2: "#5ccfe6", Accent3: "#bae67e", Accent4: "#ffae57", SelectionFG: "#0a0e14", SelectionBG: "#ffcc66", Border: "#39424d", FocusedBorder: "#5ccfe6", Success: "#bae67e", Warning: "#ffcc66", Error: "#f07178", DestructiveFG: "#fff4f0", DestructiveBG: "#8f2f36"}),
	terminalTheme(terminalThemePalette{Name: "cobalt-dispatch", Text: "#edf4ff", Muted: "#9bb8df", Dim: "#6f86a8", Surface: "#071b3a", SurfaceStrong: "#030b1c", Accent: "#3ddcff", Accent2: "#7aa2ff", Accent3: "#ffe66d", Accent4: "#ff9e64", SelectionFG: "#031021", SelectionBG: "#3ddcff", Border: "#234a7d", FocusedBorder: "#7aa2ff", Success: "#9ece6a", Warning: "#ffe66d", Error: "#ff667d", DestructiveFG: "#fff5f8", DestructiveBG: "#9b2446"}),
	terminalTheme(terminalThemePalette{Name: "kanagawa-post", Text: "#dcd7ba", Muted: "#938aa9", Dim: "#727169", Surface: "#1f1f28", SurfaceStrong: "#0d0d12", Accent: "#c8c093", Accent2: "#7e9cd8", Accent3: "#e6c384", Accent4: "#98bb6c", SelectionFG: "#101016", SelectionBG: "#c8c093", Border: "#54546d", FocusedBorder: "#7e9cd8", Success: "#98bb6c", Warning: "#e6c384", Error: "#e82424", DestructiveFG: "#fff3ee", DestructiveBG: "#8a2f2f"}),
	terminalTheme(terminalThemePalette{Name: "rose-pine-desk", Text: "#e0def4", Muted: "#908caa", Dim: "#6e6a86", Surface: "#26233a", SurfaceStrong: "#191724", Accent: "#ebbcba", Accent2: "#9ccfd8", Accent3: "#f6c177", Accent4: "#c4a7e7", SelectionFG: "#191724", SelectionBG: "#ebbcba", Border: "#403d52", FocusedBorder: "#9ccfd8", Success: "#31748f", Warning: "#f6c177", Error: "#eb6f92", DestructiveFG: "#fff8fa", DestructiveBG: "#9b3155"}),
	terminalTheme(terminalThemePalette{Name: "solar-paper", Text: "#073642", Muted: "#657b83", Dim: "#93a1a1", Surface: "#eee8d5", SurfaceStrong: "#fdf6e3", Accent: "#268bd2", Accent2: "#2aa198", Accent3: "#b58900", Accent4: "#d33682", SelectionFG: "#fdf6e3", SelectionBG: "#268bd2", Border: "#93a1a1", FocusedBorder: "#2aa198", Success: "#859900", Warning: "#b58900", Error: "#dc322f", DestructiveFG: "#fdf6e3", DestructiveBG: "#dc322f"}),
	terminalTheme(terminalThemePalette{Name: "tokyo-dusk", Text: "#c0caf5", Muted: "#9aa5ce", Dim: "#565f89", Surface: "#1a1b2d", SurfaceStrong: "#11121d", Accent: "#bb9af7", Accent2: "#7dcfff", Accent3: "#e0af68", Accent4: "#f7768e", SelectionFG: "#11121d", SelectionBG: "#7dcfff", Border: "#3b4261", FocusedBorder: "#bb9af7", Success: "#9ece6a", Warning: "#e0af68", Error: "#f7768e", DestructiveFG: "#fff5f7", DestructiveBG: "#9d2f55"}),
	terminalTheme(terminalThemePalette{Name: "iceberg-queue", Text: "#d2d9f4", Muted: "#8389a3", Dim: "#5b6078", Surface: "#17171f", SurfaceStrong: "#0f1117", Accent: "#84a0c6", Accent2: "#89b8c2", Accent3: "#e2a478", Accent4: "#a093c7", SelectionFG: "#0f1117", SelectionBG: "#84a0c6", Border: "#3e445e", FocusedBorder: "#89b8c2", Success: "#b4be82", Warning: "#e2a478", Error: "#e27878", DestructiveFG: "#fff5f5", DestructiveBG: "#8a3d4d"}),
	terminalTheme(terminalThemePalette{Name: "panda-packet", Text: "#e6e6e6", Muted: "#bbbbbb", Dim: "#676b79", Surface: "#1d1e27", SurfaceStrong: "#101116", Accent: "#ff2c6d", Accent2: "#19f9d8", Accent3: "#ffb86c", Accent4: "#ff75b5", SelectionFG: "#101116", SelectionBG: "#19f9d8", Border: "#414554", FocusedBorder: "#ff75b5", Success: "#6fcf97", Warning: "#ffb86c", Error: "#ff2c6d", DestructiveFG: "#fff6fa", DestructiveBG: "#9d2049"}),
	terminalTheme(terminalThemePalette{Name: "sonokai-signal", Text: "#e2e2e3", Muted: "#7f8490", Dim: "#5a5f6a", Surface: "#2c2e34", SurfaceStrong: "#181819", Accent: "#e2b86b", Accent2: "#76cce0", Accent3: "#a7df78", Accent4: "#fc5d7c", SelectionFG: "#181819", SelectionBG: "#76cce0", Border: "#4b5059", FocusedBorder: "#e2b86b", Success: "#a7df78", Warning: "#e2b86b", Error: "#fc5d7c", DestructiveFG: "#fff7fa", DestructiveBG: "#9b2d4a"}),
	terminalTheme(terminalThemePalette{Name: "zenbones-light", Text: "#353535", Muted: "#6f6f6f", Dim: "#9a9a9a", Surface: "#ededed", SurfaceStrong: "#fafafa", Accent: "#286486", Accent2: "#3b8992", Accent3: "#b86e28", Accent4: "#8f5f87", SelectionFG: "#fafafa", SelectionBG: "#286486", Border: "#c5c5c5", FocusedBorder: "#3b8992", Success: "#4f7d3f", Warning: "#b86e28", Error: "#a8334c", DestructiveFG: "#fafafa", DestructiveBG: "#a8334c"}),
	terminalTheme(terminalThemePalette{Name: "tomorrow-desk", Text: "#c5c8c6", Muted: "#969896", Dim: "#6b6d70", Surface: "#25282c", SurfaceStrong: "#1d1f21", Accent: "#81a2be", Accent2: "#8abeb7", Accent3: "#f0c674", Accent4: "#b294bb", SelectionFG: "#1d1f21", SelectionBG: "#81a2be", Border: "#373b41", FocusedBorder: "#8abeb7", Success: "#b5bd68", Warning: "#f0c674", Error: "#cc6666", DestructiveFG: "#fff6f6", DestructiveBG: "#8c3434"}),
}

var builtInThemeOrder = append([]string{"inherited", "herald-dark", "herald-light"}, selectedBuiltInThemeNames()...)

var builtInThemes = func() map[string]Theme {
	themes := map[string]Theme{
		"inherited":    inheritedTheme,
		"herald-dark":  heraldDarkTheme,
		"herald-light": heraldLightTheme,
	}
	for _, theme := range selectedBuiltInThemes {
		themes[theme.Name] = theme
	}
	return themes
}()

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
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "", "adaptive":
		return inheritedTheme
	case "legacy", "legacy-dark", "legacy_dark", "herald-dark":
		return heraldDarkTheme
	case "crymsom":
		normalized = "crimson"
	}
	if theme, ok := builtInThemes[normalized]; ok {
		return theme
	}
	return inheritedTheme
}

func builtInThemeNames() []string {
	return append([]string(nil), builtInThemeOrder...)
}

func selectedBuiltInThemeNames() []string {
	names := make([]string, 0, len(selectedBuiltInThemes))
	for _, theme := range selectedBuiltInThemes {
		names = append(names, theme.Name)
	}
	return names
}

func isBuiltInThemeName(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "inherited", "adaptive", "legacy", "legacy-dark", "legacy_dark", "crymsom":
		return true
	}
	_, ok := builtInThemes[normalized]
	return ok
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

func ResolveLaunchTheme(value, themeDir string) (Theme, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return inheritedTheme, nil
	}
	if looksLikeThemePath(trimmed) {
		path, err := config.ExpandPath(trimmed)
		if err != nil {
			return Theme{}, fmt.Errorf("resolve theme file path: %w", err)
		}
		doc, err := LoadThemeFromFile(path)
		if err != nil {
			return Theme{}, fmt.Errorf("load theme file: %w", err)
		}
		return themeFromDocument(doc)
	}
	theme, warning := resolveThemeName(trimmed, themeDir)
	if warning != "" {
		return Theme{}, fmt.Errorf("%s", warning)
	}
	return theme, nil
}

func looksLikeThemePath(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.ContainsAny(value, `/\`) ||
		strings.HasPrefix(value, ".") ||
		strings.HasPrefix(value, "~") ||
		strings.HasSuffix(lower, ".yaml") ||
		strings.HasSuffix(lower, ".yml")
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
	theme, err := themeFromDocument(doc)
	if err != nil {
		return inheritedTheme, fmt.Sprintf("Theme %q failed to apply: %v; using inherited.", normalized, err)
	}
	return theme, ""
}

func themeFromDocument(doc ThemeDocument) (Theme, error) {
	theme := ThemeByName(doc.Inherits)
	theme.Name = doc.Name
	if err := applyThemeOverrides(&theme, doc.Roles); err != nil {
		return Theme{}, err
	}
	return theme, nil
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

func themeXtermColor(index int) color.Color {
	return lipgloss.Color(strconv.Itoa(index))
}

func themeXtermContrastColor(index int) color.Color {
	rgb := xterm256ToRGB(index)
	luminance := 0.2126*float64(rgb[0]) + 0.7152*float64(rgb[1]) + 0.0722*float64(rgb[2])
	if luminance > 140 {
		return lipgloss.Color("#000000")
	}
	return lipgloss.Color("#ffffff")
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
		"chrome.form_help":           &t.Chrome.FormHelp,
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

func (t Theme) ScreenStyle(width, height int) lipgloss.Style {
	if t.Text.Primary.Background == nil {
		return lipgloss.NewStyle()
	}
	return t.Text.Primary.Style().
		Width(width).
		Height(height)
}

func (t Theme) RenderScreen(content string, width, height int) string {
	if t.Text.Primary.Background == nil || width <= 0 || height <= 0 {
		return content
	}
	prefix, suffix := t.screenStyleSequences()
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		if isNativeImageOverlayTailLine(line) {
			lines[i] = line
			continue
		}
		line = reapplyScreenStyleAfterReset(line, prefix)
		if missing := width - ansi.StringWidth(line); missing > 0 {
			line += suffix + prefix + strings.Repeat(" ", missing)
		}
		lines[i] = prefix + line + suffix
	}
	return strings.Join(lines, "\n")
}

func (t Theme) screenStyleSequences() (string, string) {
	const marker = "__HERALD_SCREEN_STYLE_MARKER__"
	rendered := t.Text.Primary.Style().Render(marker)
	index := strings.Index(rendered, marker)
	if index < 0 {
		return "", ""
	}
	return rendered[:index], rendered[index+len(marker):]
}

func reapplyScreenStyleAfterReset(line, prefix string) string {
	if prefix == "" || !strings.Contains(line, "\x1b[") {
		return line
	}
	replacer := strings.NewReplacer(
		"\x1b[0m", "\x1b[0m"+prefix,
		"\x1b[m", "\x1b[m"+prefix,
		"\x1b[39m", "\x1b[39m"+prefix,
		"\x1b[49m", "\x1b[49m"+prefix,
		"\x1b[39;49m", "\x1b[39;49m"+prefix,
		"\x1b[49;39m", "\x1b[49;39m"+prefix,
	)
	return replacer.Replace(line)
}

func isNativeImageOverlayTailLine(line string) bool {
	return strings.Contains(line, "\x1b7") &&
		strings.HasSuffix(line, "\x1b8") &&
		(strings.Contains(line, "\x1b]1337;File=") || strings.Contains(line, "\x1b_G"))
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
