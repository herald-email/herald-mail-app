package app

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

func heraldHuhTheme(isDark bool) *huh.Styles {
	styles := huh.ThemeCharm(isDark)
	unsetForeground := func(style lipgloss.Style) lipgloss.Style {
		return style.UnsetForeground().Faint(false)
	}
	dimHelp := func(style lipgloss.Style) lipgloss.Style {
		return defaultTheme.Chrome.FormHelp.Apply(style)
	}

	styles.Help.Ellipsis = dimHelp(styles.Help.Ellipsis)
	styles.Help.ShortKey = dimHelp(styles.Help.ShortKey)
	styles.Help.ShortDesc = dimHelp(styles.Help.ShortDesc)
	styles.Help.ShortSeparator = dimHelp(styles.Help.ShortSeparator)
	styles.Help.FullKey = dimHelp(styles.Help.FullKey)
	styles.Help.FullDesc = dimHelp(styles.Help.FullDesc)
	styles.Help.FullSeparator = dimHelp(styles.Help.FullSeparator)

	styles.Focused.Description = unsetForeground(styles.Focused.Description)
	styles.Blurred.Description = unsetForeground(styles.Blurred.Description)
	styles.Group.Description = unsetForeground(styles.Group.Description)
	styles.Focused.UnselectedPrefix = unsetForeground(styles.Focused.UnselectedPrefix)
	styles.Blurred.UnselectedPrefix = unsetForeground(styles.Blurred.UnselectedPrefix)
	styles.Focused.TextInput.Placeholder = unsetForeground(styles.Focused.TextInput.Placeholder)
	styles.Blurred.TextInput.Placeholder = unsetForeground(styles.Blurred.TextInput.Placeholder)

	return styles
}
