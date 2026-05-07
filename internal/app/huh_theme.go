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

	styles.Help.Ellipsis = unsetForeground(styles.Help.Ellipsis)
	styles.Help.ShortKey = unsetForeground(styles.Help.ShortKey)
	styles.Help.ShortDesc = unsetForeground(styles.Help.ShortDesc)
	styles.Help.ShortSeparator = unsetForeground(styles.Help.ShortSeparator)
	styles.Help.FullKey = unsetForeground(styles.Help.FullKey)
	styles.Help.FullDesc = unsetForeground(styles.Help.FullDesc)
	styles.Help.FullSeparator = unsetForeground(styles.Help.FullSeparator)

	styles.Focused.Description = unsetForeground(styles.Focused.Description)
	styles.Blurred.Description = unsetForeground(styles.Blurred.Description)
	styles.Group.Description = unsetForeground(styles.Group.Description)
	styles.Focused.UnselectedPrefix = unsetForeground(styles.Focused.UnselectedPrefix)
	styles.Blurred.UnselectedPrefix = unsetForeground(styles.Blurred.UnselectedPrefix)
	styles.Focused.TextInput.Placeholder = unsetForeground(styles.Focused.TextInput.Placeholder)
	styles.Blurred.TextInput.Placeholder = unsetForeground(styles.Blurred.TextInput.Placeholder)

	return styles
}
