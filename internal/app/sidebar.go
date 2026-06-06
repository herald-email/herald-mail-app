package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// --- Folder tree types ---

// folderNode represents one node in the folder tree
type folderNode struct {
	name     string
	fullPath string // IMAP path; empty for synthetic parent nodes
	children []*folderNode
	expanded bool
}

type sidebarItemKind int

const (
	sidebarItemFolder sidebarItemKind = iota
	sidebarItemAccount
	sidebarItemHeader
	sidebarItemAggregate
	sidebarItemAccountFolder
	sidebarItemGroup
)

// sidebarItem is a flattened entry used for navigation
type sidebarItem struct {
	node         *folderNode
	depth        int
	kind         sidebarItemKind
	account      models.SourceID
	label        string
	fullPath     string
	status       *models.FolderStatus
	syntheticKey string
	aggregate    string
}

// commonFolderPriority defines the sort order for well-known top-level folders
var commonFolderPriority = map[string]int{
	"INBOX":    0,
	"Sent":     1,
	"Drafts":   2,
	"Archive":  3,
	"Spam":     4,
	"Trash":    5,
	"Starred":  6,
	"All Mail": 7,
}

// buildFolderTree parses a flat IMAP folder list into a tree.
// Common folders (INBOX, Sent, …) are sorted to the top.
func buildFolderTree(folders []string) []*folderNode {
	sorted := make([]string, len(folders))
	copy(sorted, folders)
	sort.Strings(sorted)

	nodeMap := map[string]*folderNode{}
	var roots []*folderNode

	var getOrCreate func(path string) *folderNode
	getOrCreate = func(path string) *folderNode {
		if n, ok := nodeMap[path]; ok {
			return n
		}
		parts := strings.Split(path, "/")
		n := &folderNode{name: parts[len(parts)-1], expanded: true}
		nodeMap[path] = n
		if len(parts) == 1 {
			roots = append(roots, n)
		} else {
			parent := getOrCreate(strings.Join(parts[:len(parts)-1], "/"))
			parent.children = append(parent.children, n)
		}
		return n
	}

	for _, folder := range sorted {
		n := getOrCreate(folder)
		n.fullPath = folder
	}

	// Sort root nodes: common folders first (by priority), then alphabetical
	sort.SliceStable(roots, func(i, j int) bool {
		pi, oki := commonFolderPriority[roots[i].name]
		pj, okj := commonFolderPriority[roots[j].name]
		if oki && okj {
			return pi < pj
		}
		if oki {
			return true
		}
		if okj {
			return false
		}
		return roots[i].name < roots[j].name
	})

	virtualNode := &folderNode{name: "All Mail only", fullPath: virtualFolderAllMailOnly, expanded: true}
	insertAt := len(roots)
	for i, root := range roots {
		if root.name == "All Mail" {
			insertAt = i + 1
			break
		}
	}
	roots = append(roots, nil)
	copy(roots[insertAt+1:], roots[insertAt:])
	roots[insertAt] = virtualNode

	return roots
}

// flattenTree returns all currently visible nodes in display order
func flattenTree(roots []*folderNode) []sidebarItem {
	var items []sidebarItem
	var walk func(nodes []*folderNode, depth int)
	walk = func(nodes []*folderNode, depth int) {
		for _, node := range nodes {
			items = append(items, sidebarItem{node: node, depth: depth, kind: sidebarItemFolder})
			if node.expanded && len(node.children) > 0 {
				walk(node.children, depth+1)
			}
		}
	}
	walk(roots, 0)
	return items
}

type sidebarFolderRole struct {
	key   string
	label string
}

var sidebarFavoriteRoles = []struct {
	sidebarFolderRole
	defaultExpanded bool
	alwaysVisible   bool
}{
	{sidebarFolderRole{key: "inbox", label: "All Inboxes"}, true, true},
	{sidebarFolderRole{key: "drafts", label: "All Drafts"}, false, true},
	{sidebarFolderRole{key: "sent", label: "All Sent"}, true, true},
	{sidebarFolderRole{key: "flagged", label: "Flagged"}, true, false},
}

var sidebarStandardRoles = []sidebarFolderRole{
	{key: "inbox", label: "Inbox"},
	{key: "drafts", label: "Drafts"},
	{key: "sent", label: "Sent"},
	{key: "junk", label: "Junk"},
	{key: "trash", label: "Trash"},
	{key: "archive", label: "Archive"},
	{key: "all-mail", label: "All Mail"},
}

func (m *Model) visibleSidebarItems() []sidebarItem {
	items := flattenTree(m.folderTree)
	if !m.hasMultipleAccounts() {
		return items
	}
	if len(m.accountFolderSnapshots) > 0 {
		return m.accountSidebarItems()
	}
	accountItems := make([]sidebarItem, 0, len(m.accounts)+1+len(items))
	accountItems = append(accountItems, sidebarItem{kind: sidebarItemAccount, account: backend.AllAccountsSourceID})
	for _, account := range m.accounts {
		accountItems = append(accountItems, sidebarItem{kind: sidebarItemAccount, account: account.SourceID})
	}
	return append(accountItems, items...)
}

func (m *Model) accountSidebarItems() []sidebarItem {
	var items []sidebarItem
	items = append(items, sidebarItem{kind: sidebarItemHeader, label: "Favorites"})
	for _, role := range sidebarFavoriteRoles {
		children := m.favoriteAccountFolderItems(role.key)
		if len(children) == 0 && !role.alwaysVisible {
			continue
		}
		status := sumSidebarStatuses(children)
		key := "favorite:" + role.key
		fullPath := uniqueSidebarPath(children)
		if role.key == "inbox" && fullPath == "" {
			fullPath = "INBOX"
		}
		items = append(items, sidebarItem{
			kind:         sidebarItemAggregate,
			label:        role.label,
			account:      backend.AllAccountsSourceID,
			fullPath:     fullPath,
			status:       status,
			syntheticKey: key,
			aggregate:    role.key,
		})
		if len(children) > 0 && m.sidebarExpandedState(key, role.defaultExpanded) {
			items = append(items, children...)
		}
	}
	for _, snapshot := range m.accountFolderSnapshots {
		accountLabel := formatAccountLabel(snapshot.Account)
		accountKey := "account:" + string(snapshot.Account.SourceID)
		items = append(items, sidebarItem{
			kind:         sidebarItemGroup,
			label:        accountLabel,
			account:      snapshot.Account.SourceID,
			syntheticKey: accountKey,
		})
		if !m.sidebarExpandedState(accountKey, true) {
			continue
		}
		used := make(map[string]bool)
		for _, role := range sidebarStandardRoles {
			path := matchFolderRole(snapshot.Folders, role.key)
			if path == "" {
				continue
			}
			used[path] = true
			items = append(items, m.accountFolderSidebarItem(snapshot, role.label, path, 1))
		}
		items = append(items, sidebarItem{
			kind:     sidebarItemAccountFolder,
			label:    "All Mail only",
			account:  snapshot.Account.SourceID,
			fullPath: virtualFolderAllMailOnly,
			depth:    1,
		})
		custom := customSidebarFolders(snapshot.Folders, used)
		if len(custom) == 0 {
			continue
		}
		groupKey := "account:" + string(snapshot.Account.SourceID) + ":folders"
		items = append(items, sidebarItem{
			kind:         sidebarItemGroup,
			label:        "Folders",
			account:      snapshot.Account.SourceID,
			syntheticKey: groupKey,
			depth:        1,
		})
		if m.sidebarExpandedState(groupKey, true) {
			for _, folder := range custom {
				items = append(items, m.accountFolderSidebarItem(snapshot, folderDisplayName(folder), folder, 2+folderDepth(folder)))
			}
		}
	}
	return items
}

func (m *Model) favoriteAccountFolderItems(role string) []sidebarItem {
	var children []sidebarItem
	for _, snapshot := range m.accountFolderSnapshots {
		path := matchFolderRole(snapshot.Folders, role)
		if path == "" {
			continue
		}
		label := formatAccountLabel(snapshot.Account)
		children = append(children, m.accountFolderSidebarItem(snapshot, label, path, 1))
	}
	return children
}

func formatAccountLabel(account backend.AccountInfo) string {
	name := strings.TrimSpace(account.DisplayName)
	if name == "" {
		name = strings.TrimSpace(string(account.SourceID))
	}
	address := strings.TrimSpace(account.Address)
	if address == "" || strings.Contains(name, "("+address+")") {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, address)
}

func truncateSidebarLabel(label string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(label) <= width {
		return label
	}
	start := strings.LastIndex(label, " (")
	if start > 0 && strings.HasSuffix(label, ")") {
		name := label[:start]
		address := strings.TrimSuffix(label[start+2:], ")")
		nameWidth := ansi.StringWidth(name)
		if nameWidth+4 < width {
			addressWidth := width - nameWidth - 3
			return name + " (" + truncateVisual(address, addressWidth) + ")"
		}
	}
	return truncateVisual(label, width)
}

func (m *Model) accountFolderSidebarItem(snapshot backend.AccountFolderSnapshot, label, path string, depth int) sidebarItem {
	item := sidebarItem{
		kind:     sidebarItemAccountFolder,
		label:    label,
		account:  snapshot.Account.SourceID,
		fullPath: path,
		depth:    depth,
	}
	if st, ok := snapshot.Status[path]; ok {
		status := st
		item.status = &status
	}
	return item
}

func (m *Model) sidebarExpandedState(key string, defaultExpanded bool) bool {
	if key == "" {
		return defaultExpanded
	}
	if m.sidebarExpanded == nil {
		m.sidebarExpanded = make(map[string]bool)
	}
	expanded, ok := m.sidebarExpanded[key]
	if !ok {
		return defaultExpanded
	}
	return expanded
}

func (m *Model) setSidebarExpandedState(key string, expanded bool) {
	if key == "" {
		return
	}
	if m.sidebarExpanded == nil {
		m.sidebarExpanded = make(map[string]bool)
	}
	m.sidebarExpanded[key] = expanded
}

func folderDisplayName(folder string) string {
	if folder == virtualFolderAllMailOnly {
		return "All Mail only"
	}
	parts := strings.Split(strings.Trim(folder, "/"), "/")
	if len(parts) == 0 {
		return folder
	}
	return parts[len(parts)-1]
}

func folderDepth(folder string) int {
	folder = strings.Trim(folder, "/")
	if folder == "" {
		return 0
	}
	return strings.Count(folder, "/")
}

func matchFolderRole(folders []string, role string) string {
	for _, folder := range folders {
		if folderMatchesRole(folder, role) {
			return folder
		}
	}
	return ""
}

func folderMatchesRole(folder, role string) bool {
	base := strings.ToLower(strings.TrimSpace(folderDisplayName(folder)))
	full := strings.ToLower(strings.TrimSpace(folder))
	switch role {
	case "inbox":
		return full == "inbox" || base == "inbox"
	case "drafts":
		return strings.Contains(base, "draft")
	case "sent":
		return base == "sent" || strings.Contains(base, "sent mail")
	case "junk":
		return base == "junk" || base == "spam"
	case "trash":
		return base == "trash" || strings.Contains(base, "deleted")
	case "archive":
		return base == "archive" || base == "archives"
	case "all-mail":
		normalized := strings.ReplaceAll(base, " ", "")
		return normalized == "allmail"
	case "flagged":
		return base == "flagged" || base == "starred"
	default:
		return false
	}
}

func customSidebarFolders(folders []string, used map[string]bool) []string {
	var custom []string
	for _, folder := range folders {
		if used[folder] {
			continue
		}
		if folder == "" || folder == virtualFolderAllMailOnly {
			continue
		}
		custom = append(custom, folder)
	}
	sort.Strings(custom)
	return custom
}

func sumSidebarStatuses(items []sidebarItem) *models.FolderStatus {
	var total models.FolderStatus
	seen := false
	for _, item := range items {
		if item.status == nil {
			continue
		}
		seen = true
		total.Unseen += item.status.Unseen
		total.Total += item.status.Total
	}
	if !seen {
		return nil
	}
	return &total
}

func uniqueSidebarPath(items []sidebarItem) string {
	seen := ""
	for _, item := range items {
		if item.fullPath == "" {
			continue
		}
		if seen == "" {
			seen = item.fullPath
			continue
		}
		if seen != item.fullPath {
			return ""
		}
	}
	return seen
}

func (m *Model) sidebarItemExpandable(item sidebarItem) bool {
	switch item.kind {
	case sidebarItemFolder:
		return item.node != nil && len(item.node.children) > 0
	case sidebarItemAggregate:
		return len(m.favoriteAccountFolderItems(item.aggregate)) > 0
	case sidebarItemGroup:
		return item.syntheticKey != ""
	default:
		return false
	}
}

func (m *Model) sidebarItemExpanded(item sidebarItem) bool {
	switch item.kind {
	case sidebarItemFolder:
		return item.node != nil && item.node.expanded
	case sidebarItemAggregate:
		for _, role := range sidebarFavoriteRoles {
			if role.key == item.aggregate {
				return m.sidebarExpandedState(item.syntheticKey, role.defaultExpanded)
			}
		}
		return m.sidebarExpandedState(item.syntheticKey, true)
	case sidebarItemGroup:
		return m.sidebarExpandedState(item.syntheticKey, true)
	default:
		return false
	}
}

func (m *Model) toggleSidebarNode() {
	items := m.visibleSidebarItems()
	if m.sidebarCursor >= len(items) {
		return
	}
	m.normalizeSidebarCursor(1)
	items = m.visibleSidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return
	}
	item := items[m.sidebarCursor]
	switch item.kind {
	case sidebarItemFolder:
		node := item.node
		if node == nil || len(node.children) == 0 {
			return
		}
		node.expanded = !node.expanded
	case sidebarItemAggregate, sidebarItemGroup:
		if !m.sidebarItemExpandable(item) {
			return
		}
		m.setSidebarExpandedState(item.syntheticKey, !m.sidebarItemExpanded(item))
	default:
		return
	}
	newLen := len(m.visibleSidebarItems())
	if m.sidebarCursor >= newLen {
		m.sidebarCursor = newLen - 1
	}
	m.normalizeSidebarCursor(-1)
}

func (m *Model) sidebarItemSelectable(item sidebarItem) bool {
	return item.kind != sidebarItemHeader
}

func (m *Model) normalizeSidebarCursor(direction int) {
	items := m.visibleSidebarItems()
	if len(items) == 0 {
		m.sidebarCursor = 0
		return
	}
	if m.sidebarCursor < 0 {
		m.sidebarCursor = 0
	}
	if m.sidebarCursor >= len(items) {
		m.sidebarCursor = len(items) - 1
	}
	if m.sidebarItemSelectable(items[m.sidebarCursor]) {
		return
	}
	if direction == 0 {
		direction = 1
	}
	for i := m.sidebarCursor + direction; i >= 0 && i < len(items); i += direction {
		if m.sidebarItemSelectable(items[i]) {
			m.sidebarCursor = i
			return
		}
	}
	for i := m.sidebarCursor - direction; i >= 0 && i < len(items); i -= direction {
		if m.sidebarItemSelectable(items[i]) {
			m.sidebarCursor = i
			return
		}
	}
}

// selectSidebarFolder switches to the folder at sidebarCursor. It returns a
// command when the selected row performs an account-scoped switch directly.
func (m *Model) selectSidebarFolder() (tea.Cmd, bool) {
	m.normalizeSidebarCursor(1)
	items := m.visibleSidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return nil, false
	}
	item := items[m.sidebarCursor]
	if item.kind == sidebarItemAccount {
		return m.switchActiveAccount(item.account), true
	}
	if item.kind == sidebarItemAccountFolder {
		if item.fullPath == "" {
			return nil, false
		}
		return m.switchSidebarAccountFolder(item.account, item.fullPath), true
	}
	if item.kind == sidebarItemAggregate {
		if item.fullPath != "" {
			return m.switchSidebarAccountFolder(backend.AllAccountsSourceID, item.fullPath), true
		}
		m.toggleSidebarNode()
		return nil, false
	}
	if item.kind == sidebarItemGroup || item.kind == sidebarItemHeader {
		m.toggleSidebarNode()
		return nil, false
	}
	node := item.node
	if node == nil {
		return nil, false
	}
	if node.fullPath == "" {
		// Synthetic parent — toggle expand instead of navigating
		m.toggleSidebarNode()
		return nil, false
	}
	m.currentFolder = node.fullPath
	m.loading = true
	m.startTime = time.Now()
	m.timeline.virtualNotice = ""
	if m.activeTab == tabTimeline {
		m.setFocusedPanel(panelTimeline)
	} else {
		m.setFocusedPanel(panelTimeline)
	}
	logger.Info("Switching to folder: %s", m.currentFolder)
	return nil, false
}

func (m *Model) switchSidebarAccountFolder(sourceID models.SourceID, folder string) tea.Cmd {
	if folder == "" {
		return nil
	}
	if m.accountSelectedFolders == nil {
		m.accountSelectedFolders = make(map[models.SourceID]string)
	}
	m.accountSelectedFolders[sourceID] = folder
	if sourceID == m.activeSourceID {
		m.resetMailboxStateForFolder(folder)
		m.hydrateCachedTimelineForCurrentFolder()
		m.statusMessage = fmt.Sprintf("Switched to %s", m.activeAccountLabel())
		return tea.Batch(m.startLoading(), m.tickSpinner(), m.listenForSyncEvents())
	}
	return m.switchActiveAccount(sourceID)
}

func (m *Model) sidebarItemMatchesCurrentFolder(item sidebarItem) bool {
	switch item.kind {
	case sidebarItemFolder:
		return item.node != nil && item.node.fullPath == m.currentFolder
	case sidebarItemAggregate:
		return m.activeSourceID == backend.AllAccountsSourceID && item.fullPath != "" && item.fullPath == m.currentFolder
	case sidebarItemAccountFolder:
		return item.account == m.activeSourceID && item.fullPath == m.currentFolder
	default:
		return false
	}
}

// sidebarContentWidth is the fixed display width of sidebar content (excluding border)
const sidebarContentWidth = 26

// Chat panel widths are content widths, excluding the border.
const (
	chatPanelMinWidth = 36
	chatPanelMaxWidth = 72
	chatMainMinWidth  = 48
)

// renderSidebar renders the folder tree sidebar content (without border)
func (m *Model) renderSidebar() string {
	items := m.visibleSidebarItems()
	var sb strings.Builder

	// Limit rendered rows to tableHeight to prevent overflow at small terminal heights
	maxRows := m.buildLayoutPlan(m.windowWidth, m.windowHeight).ContentHeight
	if maxRows < 5 {
		maxRows = 5
	}
	startIdx := 0
	if len(items) > maxRows {
		startIdx = m.sidebarCursor - maxRows + 1
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx+maxRows > len(items) {
			startIdx = len(items) - maxRows
		}
	}

	for i, item := range items {
		if i < startIdx || i >= startIdx+maxRows {
			continue
		}
		switch item.kind {
		case sidebarItemHeader:
			label := truncateVisual(item.label, sidebarContentWidth)
			sb.WriteString(m.theme.Text.Primary.Style().Bold(true).Render(label) + "\n")
			continue
		case sidebarItemAccount:
			sb.WriteString(m.renderSidebarAccountLine(i, item.account) + "\n")
			continue
		}
		sb.WriteString(m.renderSidebarItemLine(i, item) + "\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func (m *Model) renderSidebarItemLine(index int, item sidebarItem) string {
	indent := strings.Repeat("  ", item.depth)
	icon := "  "
	if m.sidebarItemExpandable(item) {
		if m.sidebarItemExpanded(item) {
			icon = "▼ "
		} else {
			icon = "▶ "
		}
	}

	countSuffix := m.sidebarCountSuffix(item)
	selectionMarker := " "
	if index == m.sidebarCursor && m.focusedPanel != panelSidebar {
		selectionMarker = "›"
	}

	prefixLen := len([]rune(indent)) + 3 // selection marker (1) + icon (2)
	available := sidebarContentWidth - prefixLen - len([]rune(countSuffix))
	if available < 1 {
		available = 1
	}

	name := m.sidebarItemLabel(item)
	name = truncateSidebarLabel(name, available)
	line := fmt.Sprintf("%s%s%s%-*s%s", indent, selectionMarker, icon, available, name, countSuffix)

	if index == m.sidebarCursor {
		if m.focusedPanel == panelSidebar {
			line = m.theme.Focus.SelectionActive.Style().Render(line)
		} else {
			line = m.theme.Focus.SelectionInactive.Style().Render(line)
		}
	}
	return line
}

func (m *Model) sidebarItemLabel(item sidebarItem) string {
	if item.label != "" {
		return item.label
	}
	if item.node != nil {
		return item.node.name
	}
	return ""
}

func (m *Model) sidebarCountSuffix(item sidebarItem) string {
	var status *models.FolderStatus
	if item.status != nil {
		status = item.status
	} else if item.node != nil && item.node.fullPath != "" {
		if st, ok := m.folderStatus[item.node.fullPath]; ok {
			status = &st
		}
	} else if item.fullPath != "" && item.account == m.activeSourceID {
		if st, ok := m.folderStatus[item.fullPath]; ok {
			status = &st
		}
	}
	if status == nil {
		return ""
	}
	settledSuffix := ""
	if item.fullPath == m.currentFolder && m.loading && !m.syncCountsSettled {
		if item.account == "" || item.account == m.activeSourceID || item.account == backend.AllAccountsSourceID {
			settledSuffix = "…"
		}
	}
	return fmt.Sprintf(" %d/%d%s", status.Unseen, status.Total, settledSuffix)
}

func (m *Model) renderSidebarAccountLine(index int, sourceID models.SourceID) string {
	accountName := string(sourceID)
	if sourceID == backend.AllAccountsSourceID {
		accountName = "All Accounts"
	}
	for _, account := range m.accounts {
		if account.SourceID == sourceID {
			accountName = formatAccountLabel(account)
			break
		}
	}
	status := m.accountStatuses[sourceID]
	countSuffix := ""
	if status.Total > 0 {
		countSuffix = fmt.Sprintf(" %d/%d", status.Unread, status.Total)
	}
	if status.Error != "" {
		countSuffix = " auth"
	}
	selectionMarker := " "
	if index == m.sidebarCursor && m.focusedPanel != panelSidebar {
		selectionMarker = "›"
	}
	activeMarker := " "
	if sourceID == m.activeSourceID {
		activeMarker = ">"
	}
	prefixLen := 3
	available := sidebarContentWidth - prefixLen - len([]rune(countSuffix))
	if available < 1 {
		available = 1
	}
	label := accountName
	label = truncateSidebarLabel(label, available)
	line := fmt.Sprintf("%s%s %-*s%s", selectionMarker, activeMarker, available, label, countSuffix)
	if index == m.sidebarCursor {
		if m.focusedPanel == panelSidebar {
			line = m.theme.Focus.SelectionActive.Style().Render(line)
		} else {
			line = m.theme.Focus.SelectionInactive.Style().Render(line)
		}
	}
	return line
}

// startClassification starts background AI classification for unclassified emails.
// It closes the captured classifyCh when done so any outstanding
// listenForClassification cmd unblocks and returns ClassifyDoneMsg.
