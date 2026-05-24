package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
)

// sidebarItem is a flattened entry used for navigation
type sidebarItem struct {
	node    *folderNode
	depth   int
	kind    sidebarItemKind
	account models.SourceID
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

func (m *Model) visibleSidebarItems() []sidebarItem {
	items := flattenTree(m.folderTree)
	if !m.hasMultipleAccounts() {
		return items
	}
	accountItems := make([]sidebarItem, 0, len(m.accounts)+len(items))
	for _, account := range m.accounts {
		accountItems = append(accountItems, sidebarItem{kind: sidebarItemAccount, account: account.SourceID})
	}
	return append(accountItems, items...)
}

func (m *Model) toggleSidebarNode() {
	items := m.visibleSidebarItems()
	if m.sidebarCursor >= len(items) {
		return
	}
	if items[m.sidebarCursor].kind != sidebarItemFolder {
		return
	}
	node := items[m.sidebarCursor].node
	if len(node.children) > 0 {
		node.expanded = !node.expanded
		// Clamp cursor if it fell outside the new visible range
		newLen := len(m.visibleSidebarItems())
		if m.sidebarCursor >= newLen {
			m.sidebarCursor = newLen - 1
		}
	}
}

// selectSidebarFolder switches to the folder at sidebarCursor. It returns a
// command when the selected row is an account rail item instead of a folder.
func (m *Model) selectSidebarFolder() (tea.Cmd, bool) {
	items := m.visibleSidebarItems()
	if m.sidebarCursor < 0 || m.sidebarCursor >= len(items) {
		return nil, false
	}
	item := items[m.sidebarCursor]
	if item.kind == sidebarItemAccount {
		return m.switchActiveAccount(item.account), true
	}
	node := item.node
	if node.fullPath == "" {
		// Synthetic parent — toggle expand instead of navigating
		m.toggleSidebarNode()
		return nil, false
	}
	m.currentFolder = node.fullPath
	m.loading = true
	m.startTime = time.Now()
	m.resetCleanupSelection()
	if isVirtualAllMailOnlyFolder(m.currentFolder) {
		m.clearCleanupData()
	} else {
		m.stats = nil
		m.selectedSender = ""
	}
	m.timeline.virtualNotice = ""
	if m.activeTab == tabTimeline {
		m.setFocusedPanel(panelTimeline)
	} else {
		m.setFocusedPanel(panelSummary)
	}
	logger.Info("Switching to folder: %s", m.currentFolder)
	return nil, false
}

// sidebarContentWidth is the fixed display width of sidebar content (excluding border)
const sidebarContentWidth = 26

// chatPanelWidth is the fixed display width of the chat panel content (excluding border)
const chatPanelWidth = 36

// renderSidebar renders the folder tree sidebar content (without border)
func (m *Model) renderSidebar() string {
	items := m.visibleSidebarItems()
	var sb strings.Builder

	// Limit rendered rows to tableHeight to prevent overflow at small terminal heights
	maxRows := m.windowHeight - 7
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

	accountHeaderWritten := false
	folderHeaderWritten := false
	for i, item := range items {
		if i < startIdx || i >= startIdx+maxRows {
			continue
		}
		if m.hasMultipleAccounts() && item.kind == sidebarItemAccount && !accountHeaderWritten {
			sb.WriteString(m.theme.Text.Primary.Style().Bold(true).Render("Accounts") + "\n")
			accountHeaderWritten = true
		}
		if m.hasMultipleAccounts() && item.kind == sidebarItemFolder && !folderHeaderWritten {
			if accountHeaderWritten {
				sb.WriteString("\n")
			}
			label := "Folders"
			if active := m.activeAccountLabel(); active != "" {
				label = "Folders - " + active
			}
			sb.WriteString(m.theme.Text.Primary.Style().Bold(true).Render(truncateVisual(label, sidebarContentWidth)) + "\n")
			folderHeaderWritten = true
		}
		if item.kind == sidebarItemAccount {
			sb.WriteString(m.renderSidebarAccountLine(i, item.account) + "\n")
			continue
		}
		indent := strings.Repeat("  ", item.depth)

		var icon string
		if len(item.node.children) > 0 {
			if item.node.expanded {
				icon = "▼ "
			} else {
				icon = "▶ "
			}
		} else {
			icon = "  "
		}

		// Build count suffix if status is available
		countSuffix := ""
		if item.node.fullPath != "" {
			if st, ok := m.folderStatus[item.node.fullPath]; ok {
				settledSuffix := ""
				if item.node.fullPath == m.currentFolder && m.loading && !m.syncCountsSettled {
					settledSuffix = "…"
				}
				countSuffix = fmt.Sprintf(" %d/%d%s", st.Unseen, st.Total, settledSuffix)
			}
		}

		selectionMarker := " "
		if i == m.sidebarCursor && m.focusedPanel != panelSidebar {
			selectionMarker = "›"
		}

		prefixLen := len([]rune(indent)) + 3 // selection marker (1) + icon (2)
		available := sidebarContentWidth - prefixLen - len([]rune(countSuffix))
		if available < 1 {
			available = 1
		}

		name := item.node.name
		runes := []rune(name)
		if len(runes) > available {
			if available > 3 {
				name = string(runes[:available-3]) + "..."
			} else {
				name = string(runes[:available])
			}
		}
		line := fmt.Sprintf("%s%s%s%-*s%s", indent, selectionMarker, icon, available, name, countSuffix)

		if i == m.sidebarCursor {
			if m.focusedPanel == panelSidebar {
				line = m.theme.Focus.SelectionActive.Style().Render(line)
			} else {
				line = m.theme.Focus.SelectionInactive.Style().Render(line)
			}
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func (m *Model) renderSidebarAccountLine(index int, sourceID models.SourceID) string {
	accountName := string(sourceID)
	for _, account := range m.accounts {
		if account.SourceID == sourceID {
			accountName = account.DisplayName
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
	runes := []rune(label)
	if len(runes) > available {
		if available > 3 {
			label = string(runes[:available-3]) + "..."
		} else {
			label = string(runes[:available])
		}
	}
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
