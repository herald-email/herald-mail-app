package app

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const panelGap = " "
const panelGapWidth = 1

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
	cursor              int
	expandedThreads     map[string]bool
	focusedPanel        int
	selectedEmail       *models.EmailData
	body                *models.EmailBody
	bodyMessageID       string
	bodyLoading         bool
	inlineImageDescs    map[string]string
	remoteImageLoads    map[string]previewRemoteImageState
	remoteImageRevision int
	fullScreen          bool
	bodyScrollOffset    int
}

type TimelineState struct {
	emails               []*models.EmailData
	senderWidth          int
	subjectWidth         int
	accountColumnVisible bool
	virtualNotice        string
	groupingMode         timelineGroupingMode
	sortMode             timelineSortMode
	selectedMessageIDs   map[string]bool
	rangeMode            bool
	rangeShiftMode       bool
	rangeAnchorRow       int
	rangeCursorRow       int
	rangeBaseSelection   map[string]bool

	threadGroups    []threadGroup
	threadRowMap    []timelineRowRef
	expandedThreads map[string]bool

	selectedEmail            *models.EmailData
	body                     *models.EmailBody
	bodyMessageID            string
	bodyLoading              bool
	previewLoad              previewLoadTelemetry
	inlineImageDescs         map[string]string
	remoteImageLoads         map[string]previewRemoteImageState
	remoteImageRevision      int
	previewWidth             int
	fullScreen               bool
	bodyFetchCancel          context.CancelFunc
	bodyWrappedLines         []string
	bodyWrappedWidth         int
	bodyScrollOffset         int
	previewDocLayout         *previewDocumentLayout
	previewDocWidth          int
	previewDocRows           int
	previewDocMode           previewImageMode
	previewDocMessageID      string
	previewDocRemoteRevision int

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
	draftRequestID        int
	draftPendingMessage   string

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
	Compose        ComposeLayoutPlan
	Contacts       ContactsLayoutPlan
}

type TimelineLayoutPlan struct {
	TableWidth   int
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
	return width-m.effectiveChatOuterWidth(width)-panelGapWidth >= chatMainMinWidth
}

func (m *Model) effectiveChatPanelWidth(width int) int {
	if width <= 0 {
		return chatPanelMinWidth
	}
	maxWidthForMain := width - chatMainMinWidth - 2 - panelGapWidth
	if maxWidthForMain < chatPanelMinWidth {
		return chatPanelMinWidth
	}
	desired := width / 3
	if desired < chatPanelMinWidth+8 {
		desired = chatPanelMinWidth
	}
	if desired > chatPanelMaxWidth {
		desired = chatPanelMaxWidth
	}
	if desired > maxWidthForMain {
		desired = maxWidthForMain
	}
	if desired < chatPanelMinWidth {
		return chatPanelMinWidth
	}
	return desired
}

func (m *Model) effectiveChatOuterWidth(width int) int {
	return m.effectiveChatPanelWidth(width) + 2
}

func (m *Model) chatLayoutDeduction(width int) int {
	return m.effectiveChatOuterWidth(width) + panelGapWidth
}

func (m *Model) chromeState(plan LayoutPlan) ChromeState {
	focused := m.focusedPanel
	if focused == panelSidebar && !plan.SidebarVisible {
		if m.activeTab == tabTimeline {
			focused = panelTimeline
		} else {
			focused = panelTimeline
		}
	}
	if focused == panelChat && !plan.ChatVisible {
		if m.activeTab == tabTimeline {
			focused = panelTimeline
		} else {
			focused = panelTimeline
		}
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

func (m *Model) defaultFocusPanel() int {
	if m.activeTab == tabTimeline {
		return panelTimeline
	}
	return panelTimeline
}

func (m *Model) normalizeFocusForLayout(plan LayoutPlan) {
	if m.focusedPanel == panelSidebar && !plan.SidebarVisible {
		m.setFocusedPanel(m.defaultFocusPanel())
	}
	if m.focusedPanel == panelChat && !plan.ChatVisible {
		m.setFocusedPanel(m.defaultFocusPanel())
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

func (m *Model) reflowCurrentLayout() {
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
		return
	}
	m.updateTimelineTable()
}

func (m *Model) reflowIfTopSyncStripChanged(wasVisible bool) {
	if wasVisible != m.hasTopSyncStrip() {
		m.reflowCurrentLayout()
	}
}

func (m *Model) contentHeightForLayout(width, height int, plan LayoutPlan) int {
	if height <= 0 {
		return 5
	}
	chromeRows := 3 // title-tabs row, status bar, status/key-hint divider
	if m.hasTopSyncStrip() {
		chromeRows++
	}
	chromeRows += len(m.keyHintRows(width, m.chromeState(plan)))
	return clamp(height-chromeRows-2, 5)
}

func (m *Model) normalizeTimelineFocus() {
	if m.activeTab == tabTimeline && m.timeline.selectedEmail == nil && m.focusedPanel == panelPreview {
		m.setFocusedPanel(panelTimeline)
	}
}

func (m *Model) buildLayoutPlan(width, height int) LayoutPlan {
	plan := LayoutPlan{
		Width:          width,
		Height:         height,
		SidebarVisible: false,
		ChatVisible:    m.showChat,
	}

	canShowSidebar := m.showSidebar &&
		m.activeTab == tabTimeline &&
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
	chatDeduction := 0
	if plan.ChatVisible {
		chatDeduction = m.chatLayoutDeduction(width)
		contentWidth -= chatDeduction
	}

	// Compose: label width + bordered field.
	const composeLabelW = 10
	composeWidth := width
	if plan.ChatVisible {
		composeWidth -= chatDeduction
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
		contactsWidth -= chatDeduction
	}
	contactsAvailable := clamp(contactsWidth-panelGapWidth, 20)
	leftPreferred := contactsAvailable * 35 / 100
	left, right := splitWidth(contactsAvailable, 0, 20, 20, leftPreferred)
	plan.Contacts = ContactsLayoutPlan{ListWidth: left, DetailWidth: right}

	// Timeline: table inner width plus optional preview outer width.
	timelineWidth := width
	if plan.ChatVisible {
		timelineWidth -= chatDeduction
	}
	if plan.SidebarVisible {
		timelineWidth -= sidebarContentWidth + 2 + panelGapWidth
	}
	if m.timeline.selectedEmail != nil {
		tableOuter, previewOuter := splitWidth(clamp(timelineWidth, 20), panelGapWidth, 26, 25, timelineWidth/2)
		plan.Timeline = TimelineLayoutPlan{
			TableWidth:   clamp(tableOuter-3, 20),
			PreviewWidth: previewOuter,
		}
	} else {
		plan.Timeline = TimelineLayoutPlan{TableWidth: clamp(timelineWidth-3, 20)}
	}

	plan.ContentHeight = m.contentHeightForLayout(width, height, plan)

	return plan
}
