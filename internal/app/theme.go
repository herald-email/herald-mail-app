package app

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	TabActiveBg    lipgloss.Color
	TabActiveFg    lipgloss.Color
	TabInactiveFg  lipgloss.Color
	BorderActive   lipgloss.Color
	BorderInactive lipgloss.Color
	StatusBg       lipgloss.Color
	StatusFg       lipgloss.Color
	HintFg         lipgloss.Color
	HeaderFg       lipgloss.Color
	HeaderBg       lipgloss.Color
	DimFg          lipgloss.Color
	TextFg         lipgloss.Color
	MutedFg        lipgloss.Color
	InfoFg         lipgloss.Color
	WarningFg      lipgloss.Color
	ErrorFg        lipgloss.Color
	ConfirmFg      lipgloss.Color
	ConfirmBg      lipgloss.Color
	UnsubBg        lipgloss.Color
	DemoFg         lipgloss.Color
	DryRunFg       lipgloss.Color
	LogInfoFg      lipgloss.Color
	LogWarnFg      lipgloss.Color
	LogErrorFg     lipgloss.Color
	LogDebugFg     lipgloss.Color
}

var defaultTheme = Theme{
	TabActiveBg:    lipgloss.Color("57"),
	TabActiveFg:    lipgloss.Color("229"),
	TabInactiveFg:  lipgloss.Color("245"),
	BorderActive:   lipgloss.Color("39"),
	BorderInactive: lipgloss.Color("240"),
	StatusBg:       lipgloss.Color("0"),
	StatusFg:       lipgloss.Color("252"),
	HintFg:         lipgloss.Color("243"),
	HeaderFg:       lipgloss.Color("205"),
	HeaderBg:       lipgloss.Color("235"),
	DimFg:          lipgloss.Color("241"),
	TextFg:         lipgloss.Color("252"),
	MutedFg:        lipgloss.Color("244"),
	InfoFg:         lipgloss.Color("86"),
	WarningFg:      lipgloss.Color("214"),
	ErrorFg:        lipgloss.Color("196"),
	ConfirmFg:      lipgloss.Color("255"),
	ConfirmBg:      lipgloss.Color("160"),
	UnsubBg:        lipgloss.Color("202"),
	DemoFg:         lipgloss.Color("226"),
	DryRunFg:       lipgloss.Color("208"),
	LogInfoFg:      lipgloss.Color("86"),
	LogWarnFg:      lipgloss.Color("214"),
	LogErrorFg:     lipgloss.Color("196"),
	LogDebugFg:     lipgloss.Color("241"),
}
