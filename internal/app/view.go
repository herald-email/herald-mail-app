package app

import tea "charm.land/bubbletea/v2"

func newHeraldView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.KeyboardEnhancements.ReportAlternateKeys = true
	v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	v.KeyboardEnhancements.ReportAssociatedText = true
	return v
}

func viewContent(v tea.View) string {
	return v.Content
}
