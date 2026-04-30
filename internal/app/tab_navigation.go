package app

const primaryTabShortcutHint = "F1-F3: tabs"

type tabNavigationItem struct {
	tab   int
	key   string
	label string
}

var topLevelTabNavigation = []tabNavigationItem{
	{tab: tabTimeline, key: "F1", label: "Timeline"},
	{tab: tabCleanup, key: "F2", label: "Cleanup"},
	{tab: tabContacts, key: "F3", label: "Contacts"},
}

func tabBarLabel(item tabNavigationItem) string {
	return item.key + "  " + item.label
}

func tabMouseWidth(item tabNavigationItem) int {
	return len(tabBarLabel(item)) + 4
}
