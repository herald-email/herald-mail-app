package app

const primaryTabShortcutHint = "1-3: tabs"

type tabNavigationItem struct {
	tab   int
	key   string
	label string
}

var topLevelTabNavigation = []tabNavigationItem{
	{tab: tabTimeline, key: "1", label: "Timeline"},
	{tab: tabCleanup, key: "2", label: "Cleanup"},
	{tab: tabContacts, key: "3", label: "Contacts"},
}

func tabBarLabel(item tabNavigationItem) string {
	return item.key + "  " + item.label
}

func tabMouseWidth(item tabNavigationItem) int {
	return len(tabBarLabel(item)) + 4
}
