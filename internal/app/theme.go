package app

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
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

var adaptiveTheme = Theme{
	Name: "adaptive",
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

var legacyDarkTheme = Theme{
	Name: "legacy-dark",
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

var defaultTheme = adaptiveTheme

func ThemeByName(name string) Theme {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "adaptive":
		return adaptiveTheme
	case "legacy", "legacy-dark", "legacy_dark":
		return legacyDarkTheme
	default:
		return adaptiveTheme
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
