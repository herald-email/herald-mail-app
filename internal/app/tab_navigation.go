package app

const primaryTabShortcutHint = "1-2: tabs"

type tabNavigationItem struct {
	tab     int
	key     string
	label   string
	command string
}

var topLevelTabNavigation = []tabNavigationItem{
	{tab: tabTimeline, key: "1", label: "Timeline", command: CommandTabTimeline},
	{tab: tabContacts, key: "2", label: "Contacts", command: CommandTabContacts},
}

func tabBarLabel(item tabNavigationItem) string {
	return item.key + "  " + item.label
}

func (m *Model) tabBarLabel(item tabNavigationItem) string {
	key := item.key
	if item.command != "" {
		if resolved := m.commandKey(keyboardScopeGlobal, item.command); resolved != "" {
			key = displayShortcutKey(resolved, keyDisplayHint)
		}
	}
	return key + "  " + item.label
}

func tabMouseWidth(item tabNavigationItem) int {
	return len(tabBarLabel(item)) + 4
}

func (m *Model) tabMouseWidth(item tabNavigationItem) int {
	return len(m.tabBarLabel(item)) + 4
}
