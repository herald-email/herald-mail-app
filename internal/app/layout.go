package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/x/ansi"
	"mail-processor/internal/models"
)

const panelGap = "  "
const panelGapWidth = 2

type ChromeState struct {
	ActiveTab     int
	FocusedPanel  int
	ShowLogs      bool
	ShowChat      bool
	ShowSidebar   bool
	SidebarNarrow bool
	StatusMessage string
}

type timelineSearchFocus int

const (
	timelineSearchFocusInput timelineSearchFocus = iota
	timelineSearchFocusResults
)

type timelineSearchOrigin struct {
	cursor           int
	expandedThreads  map[string]bool
	focusedPanel     int
	selectedEmail    *models.EmailData
	body             *models.EmailBody
	bodyMessageID    string
	bodyLoading      bool
	inlineImageDescs map[string]string
	fullScreen       bool
	bodyScrollOffset int
}

type TimelineState struct {
	emails        []*models.EmailData
	senderWidth   int
	subjectWidth  int
	virtualNotice string

	threadGroups    []threadGroup
	threadRowMap    []timelineRowRef
	expandedThreads map[string]bool

	selectedEmail    *models.EmailData
	body             *models.EmailBody
	bodyMessageID    string
	bodyLoading      bool
	inlineImageDescs map[string]string
	previewWidth     int
	fullScreen       bool
	bodyFetchCancel  context.CancelFunc
	bodyWrappedLines []string
	bodyWrappedWidth int
	bodyScrollOffset int

	selectedAttachment    int
	attachmentSavePrompt  bool
	attachmentSaveInput   textinput.Model
	attachmentSaveWarning string

	quickReplies          []string
	quickRepliesReady     bool
	quickReplyOpen        bool
	quickReplyPending     bool
	quickReplyIdx         int
	quickRepliesAIFetched bool
	forwardRequestID      int
	forwardPendingMessage string
	replyRequestID        int
	replyPendingMessage   string

	searchMode             bool
	searchInput            textinput.Model
	searchResults          []*models.EmailData
	searchResultsQuery     string
	emailsCache            []*models.EmailData
	semanticScores         map[string]float64
	searchError            string
	searchFocus            timelineSearchFocus
	searchOrigin           *timelineSearchOrigin
	searchToken            int
	searchAutoFocusResults bool

	chatFilterMode     bool
	chatFilteredEmails []*models.EmailData
	chatFilterLabel    string

	mouseMode   bool
	visualMode  bool
	visualStart int
	visualEnd   int
	pendingY    bool
}

type CleanupState struct {
	PreviewOpen   bool
	FullScreen    bool
	GroupByDomain bool
}

type ComposeState struct {
	PreviewOpen bool
	AIPanelOpen bool
}

type ContactsState struct {
	PreviewOpen bool
}

type SyncState struct {
	Loading bool
}

type LayoutPlan struct {
	Width          int
	Height         int
	ContentHeight  int
	SidebarVisible bool
	ChatVisible    bool
	Timeline       TimelineLayoutPlan
	Cleanup        CleanupLayoutPlan
	Compose        ComposeLayoutPlan
	Contacts       ContactsLayoutPlan
}

type TimelineLayoutPlan struct {
	TableWidth   int
	PreviewWidth int
}

type CleanupLayoutPlan struct {
	SummaryWidth int
	DetailsWidth int
	PreviewWidth int
}

type ComposeLayoutPlan struct {
	LabelWidth      int
	FieldInnerWidth int
	BodyInnerWidth  int
}

type ContactsLayoutPlan struct {
	ListWidth   int
	DetailWidth int
}

func (m *Model) canRenderChat(width int) bool {
	if width <= 0 {
		return true
	}
	chatOuter := chatPanelWidth + 2
	return width-chatOuter >= 48
}

func (m *Model) chromeState(plan LayoutPlan) ChromeState {
	focused := m.focusedPanel
	if focused == panelSidebar && !plan.SidebarVisible {
		if m.activeTab == tabTimeline {
			focused = panelTimeline
		} else {
			focused = panelSummary
		}
	}
	if focused == panelChat && !plan.ChatVisible {
		if m.activeTab == tabTimeline {
			focused = panelTimeline
		} else {
			focused = panelSummary
		}
	}
	if m.activeTab == tabCleanup && m.showCleanupPreview {
		focused = panelDetails
	}
	if m.activeTab == tabTimeline && m.timeline.selectedEmail == nil && focused == panelPreview {
		focused = panelTimeline
	}
	return ChromeState{
		ActiveTab:     m.activeTab,
		FocusedPanel:  focused,
		ShowLogs:      m.showLogs,
		ShowChat:      plan.ChatVisible,
		ShowSidebar:   plan.SidebarVisible,
		SidebarNarrow: m.sidebarTooWide,
		StatusMessage: m.statusMessage,
	}
}

func (m *Model) timelineState() TimelineState {
	return m.timeline
}

func (m *Model) cleanupState() CleanupState {
	return CleanupState{
		PreviewOpen:   m.showCleanupPreview,
		FullScreen:    m.cleanupFullScreen,
		GroupByDomain: m.groupByDomain,
	}
}

func (m *Model) composeState() ComposeState {
	return ComposeState{
		PreviewOpen: m.composePreview,
		AIPanelOpen: m.composeAIPanel,
	}
}

func (m *Model) contactsState() ContactsState {
	return ContactsState{
		PreviewOpen: m.contactPreviewEmail != nil,
	}
}

func (m *Model) syncState() SyncState {
	return SyncState{
		Loading: m.loading,
	}
}

func clamp(n, min int) int {
	if n < min {
		return min
	}
	return n
}

func truncateVisual(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

func safeChromeLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Truncate(s, width, "")
}

func renderMinSizeMessage(width, height int) string {
	lines := []string{}
	if width < minTermWidth {
		lines = append(lines, fmt.Sprintf("Terminal too narrow (%d cols).", width))
	}
	if height < minTermHeight {
		lines = append(lines, fmt.Sprintf("Terminal too short (%d rows).", height))
	}
	lines = append(lines, fmt.Sprintf("Resize to at least %dx%d.", minTermWidth, minTermHeight))
	for i, line := range lines {
		lines[i] = "  " + safeChromeLine(line, clamp(width-2, 10))
	}
	return "\n" + strings.Join(lines, "\n")
}

func splitWidth(total, separator, leftMin, rightMin, leftPreferred int) (int, int) {
	available := total - separator
	if available < leftMin+rightMin {
		left := clamp((available-leftMin-rightMin)/2+leftMin, leftMin)
		right := available - left
		if right < rightMin {
			right = rightMin
			left = available - right
		}
		return clamp(left, 1), clamp(right, 1)
	}
	left := leftPreferred
	if left < leftMin {
		left = leftMin
	}
	if left > available-rightMin {
		left = available - rightMin
	}
	right := available - left
	return left, right
}

func splitThreeWidth(total, leftMin, middleMin, rightMin, rightPreferred int) (int, int, int) {
	available := total
	if available < leftMin+middleMin+rightMin {
		return leftMin, middleMin, rightMin
	}
	right := rightPreferred
	if right < rightMin {
		right = rightMin
	}
	if right > available-leftMin-middleMin {
		right = available - leftMin - middleMin
	}
	remaining := available - right
	leftPreferred := remaining * 45 / 100
	left, middle := splitWidth(remaining, 0, leftMin, middleMin, leftPreferred)
	return left, middle, right
}

func (m *Model) defaultFocusPanel() int {
	if m.activeTab == tabTimeline {
		return panelTimeline
	}
	return panelSummary
}

func (m *Model) normalizeFocusForLayout(plan LayoutPlan) {
	if m.focusedPanel == panelSidebar && !plan.SidebarVisible {
		m.setFocusedPanel(m.defaultFocusPanel())
	}
	if m.focusedPanel == panelChat && !plan.ChatVisible {
		m.setFocusedPanel(m.defaultFocusPanel())
	}
	if m.activeTab == tabCleanup && m.showCleanupPreview && plan.Cleanup.SummaryWidth == 0 && m.focusedPanel == panelSummary {
		m.setFocusedPanel(panelDetails)
	}
	if m.activeTab == tabTimeline && m.timeline.selectedEmail == nil && m.focusedPanel == panelPreview {
		m.setFocusedPanel(panelTimeline)
	}
}

func (t TimelineState) PreviewOpen() bool {
	return t.selectedEmail != nil
}

func (t TimelineState) SearchOpen() bool {
	return t.searchMode
}

func (t TimelineState) QuickReplyOpen() bool {
	return t.quickReplyOpen
}

func (m *Model) timelineDisplayEmails() []*models.EmailData {
	switch {
	case m.timeline.chatFilterMode:
		return m.timeline.chatFilteredEmails
	case m.timeline.searchMode && m.timeline.searchResults != nil:
		return m.timeline.searchResults
	default:
		return m.timeline.emails
	}
}

func (m *Model) hasTimelinePreview() bool {
	return m.timeline.selectedEmail != nil
}

func (m *Model) hasTopSyncStrip() bool {
	return m.loading && !m.syncCountsSettled && m.hasVisibleStartupData()
}

func (m *Model) normalizeTimelineFocus() {
	if m.activeTab == tabTimeline && m.timeline.selectedEmail == nil && m.focusedPanel == panelPreview {
		m.setFocusedPanel(panelTimeline)
	}
}

func (m *Model) buildLayoutPlan(width, height int) LayoutPlan {
	extraChromeRows := 0
	if m.hasTopSyncStrip() {
		extraChromeRows = 1
	}
	plan := LayoutPlan{
		Width:          width,
		Height:         height,
		ContentHeight:  clamp(height-(9+extraChromeRows), 5),
		SidebarVisible: false,
		ChatVisible:    m.showChat,
	}

	canShowSidebar := m.showSidebar &&
		(m.activeTab == tabTimeline || m.activeTab == tabCleanup) &&
		!m.showCleanupPreview &&
		!(m.activeTab == tabTimeline && m.timeline.selectedEmail != nil)
	if canShowSidebar {
		sidebarOuter := sidebarContentWidth + 2
		if width-sidebarOuter >= 60 && width >= 100 {
			plan.SidebarVisible = true
		}
	}
	if m.showChat && !m.canRenderChat(width) {
		plan.ChatVisible = false
	}

	contentWidth := width
	if plan.SidebarVisible {
		contentWidth -= sidebarContentWidth + 2 + panelGapWidth
	}
	if plan.ChatVisible {
		contentWidth -= chatPanelWidth + 2 + panelGapWidth
	}

	// Compose: label width + bordered field.
	const composeLabelW = 10
	composeWidth := width
	if plan.ChatVisible {
		composeWidth -= chatPanelWidth + 2 + panelGapWidth
	}
	fieldOuter := clamp(composeWidth-composeLabelW, 12)
	plan.Compose = ComposeLayoutPlan{
		LabelWidth:      composeLabelW,
		FieldInnerWidth: clamp(fieldOuter-2, 10),
		BodyInnerWidth:  clamp(composeWidth-2, 10),
	}

	// Contacts: two bordered panels plus separator.
	contactsWidth := width
	if plan.ChatVisible {
		contactsWidth -= chatPanelWidth + 2 + panelGapWidth
	}
	contactsAvailable := clamp(contactsWidth-6, 20)
	leftPreferred := contactsAvailable * 35 / 100
	left, right := splitWidth(contactsAvailable, 0, 20, 20, leftPreferred)
	plan.Contacts = ContactsLayoutPlan{ListWidth: left, DetailWidth: right}

	// Timeline: table inner width plus optional preview outer width.
	timelineWidth := width
	if plan.ChatVisible {
		timelineWidth -= chatPanelWidth + 2 + panelGapWidth
	}
	if plan.SidebarVisible {
		timelineWidth -= sidebarContentWidth + 2 + panelGapWidth
	}
	if m.timeline.selectedEmail != nil {
		tableOuter, previewOuter := splitWidth(clamp(timelineWidth, 20), panelGapWidth, 26, 25, timelineWidth/2)
		plan.Timeline = TimelineLayoutPlan{
			TableWidth:   clamp(tableOuter-2, 20),
			PreviewWidth: previewOuter,
		}
	} else {
		plan.Timeline = TimelineLayoutPlan{TableWidth: clamp(timelineWidth-2, 20)}
	}

	// Cleanup: summary/details inner widths with optional preview outer width.
	cleanupWidth := width
	if plan.ChatVisible {
		cleanupWidth -= chatPanelWidth + 2 + panelGapWidth
	}
	if plan.SidebarVisible {
		cleanupWidth -= sidebarContentWidth + 2 + panelGapWidth
	}
	if m.showCleanupPreview && !m.cleanupFullScreen {
		if cleanupWidth < 100 {
			previewWidth := 24
			if previewWidth > cleanupWidth-30 {
				previewWidth = clamp(cleanupWidth/3, 20)
			}
			plan.Cleanup = CleanupLayoutPlan{
				SummaryWidth: 0,
				DetailsWidth: clamp(cleanupWidth-previewWidth-6, 24),
				PreviewWidth: previewWidth,
			}
		} else {
			leftW, midW, rightW := splitThreeWidth(clamp(cleanupWidth-10, 30), 18, 24, 24, 24)
			plan.Cleanup = CleanupLayoutPlan{
				SummaryWidth: leftW,
				DetailsWidth: midW,
				PreviewWidth: rightW,
			}
		}
	} else {
		leftW, rightW := splitWidth(clamp(cleanupWidth-6, 24), 0, 20, 24, (cleanupWidth-6)*42/100)
		plan.Cleanup = CleanupLayoutPlan{
			SummaryWidth: leftW,
			DetailsWidth: rightW,
		}
	}

	return plan
}
