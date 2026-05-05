package app

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type Theme struct {
	TabActiveBg    color.Color
	TabActiveFg    color.Color
	TabInactiveFg  color.Color
	BorderActive   color.Color
	BorderInactive color.Color
	StatusBg       color.Color
	StatusFg       color.Color
	HintBg         color.Color
	HintFg         color.Color
	HeaderFg       color.Color
	HeaderBg       color.Color
	DimFg          color.Color
	TextFg         color.Color
	MutedFg        color.Color
	InfoFg         color.Color
	WarningFg      color.Color
	ErrorFg        color.Color
	ConfirmFg      color.Color
	ConfirmBg      color.Color
	UnsubBg        color.Color
	DemoFg         color.Color
	DryRunFg       color.Color
	LogInfoFg      color.Color
	LogWarnFg      color.Color
	LogErrorFg     color.Color
	LogDebugFg     color.Color
}

var defaultTheme = Theme{
	TabActiveBg:    lipgloss.Color("57"),
	TabActiveFg:    lipgloss.Color("229"),
	TabInactiveFg:  lipgloss.Color("245"),
	BorderActive:   lipgloss.Color("39"),
	BorderInactive: lipgloss.Color("240"),
	StatusBg:       lipgloss.Color("0"),
	StatusFg:       lipgloss.Color("252"),
	HintBg:         lipgloss.Color("235"),
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
