package app

import (
	"context"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/iterm2"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// --- Thread grouping types ---

// threadGroup holds all emails that share the same normalised subject.
type threadGroup struct {
	normalizedSubject string
	emails            []*models.EmailData // newest first (inherited from sorted input)
	label             string
	groupingMode      timelineGroupingMode
}

type timelineGroupingMode int

const (
	timelineGroupingThread timelineGroupingMode = iota
	timelineGroupingSender
	timelineGroupingDomain
)

type timelineSortCriterion int

const (
	timelineSortCriterionWhen timelineSortCriterion = iota
	timelineSortCriterionSender
	timelineSortCriterionCount
)

type timelineSortMode int

const (
	timelineSortWhenDesc timelineSortMode = iota
	timelineSortWhenAsc
	timelineSortSenderAsc
	timelineSortSenderDesc
	timelineSortCountDesc
	timelineSortCountAsc
)

func (mode timelineSortMode) criterion() timelineSortCriterion {
	switch mode {
	case timelineSortSenderAsc, timelineSortSenderDesc:
		return timelineSortCriterionSender
	case timelineSortCountDesc, timelineSortCountAsc:
		return timelineSortCriterionCount
	default:
		return timelineSortCriterionWhen
	}
}

func (mode timelineSortMode) ascending() bool {
	switch mode {
	case timelineSortWhenAsc, timelineSortSenderAsc, timelineSortCountAsc:
		return true
	default:
		return false
	}
}

func (mode timelineSortMode) directionIndicator() string {
	if mode.ascending() {
		return "↑"
	}
	return "↓"
}

func (mode timelineSortMode) directionLabel() string {
	if mode.ascending() {
		return "ascending"
	}
	return "descending"
}

func (mode timelineSortMode) criterionLabel() string {
	switch mode.criterion() {
	case timelineSortCriterionSender:
		return "Sender"
	case timelineSortCriterionCount:
		return "Count"
	default:
		return "When"
	}
}

func (mode timelineSortMode) shortLabel() string {
	return mode.criterionLabel() + " " + mode.directionIndicator()
}

func (mode timelineSortMode) statusLabel() string {
	return strings.ToLower(mode.criterionLabel()) + " " + mode.directionLabel()
}

func (mode timelineSortMode) next() timelineSortMode {
	switch mode {
	case timelineSortWhenDesc:
		return timelineSortWhenAsc
	case timelineSortWhenAsc:
		return timelineSortSenderAsc
	case timelineSortSenderAsc:
		return timelineSortSenderDesc
	case timelineSortSenderDesc:
		return timelineSortCountDesc
	case timelineSortCountDesc:
		return timelineSortCountAsc
	default:
		return timelineSortWhenDesc
	}
}

func (mode timelineSortMode) flipped() timelineSortMode {
	switch mode {
	case timelineSortWhenDesc:
		return timelineSortWhenAsc
	case timelineSortWhenAsc:
		return timelineSortWhenDesc
	case timelineSortSenderAsc:
		return timelineSortSenderDesc
	case timelineSortSenderDesc:
		return timelineSortSenderAsc
	case timelineSortCountDesc:
		return timelineSortCountAsc
	default:
		return timelineSortCountDesc
	}
}

func defaultTimelineSortModeForCriterion(criterion timelineSortCriterion) timelineSortMode {
	switch criterion {
	case timelineSortCriterionSender:
		return timelineSortSenderAsc
	case timelineSortCriterionCount:
		return timelineSortCountDesc
	default:
		return timelineSortWhenDesc
	}
}

func (mode timelineGroupingMode) Label() string {
	switch mode {
	case timelineGroupingSender:
		return "Sender"
	case timelineGroupingDomain:
		return "Domain"
	default:
		return "Thread"
	}
}

func (mode timelineGroupingMode) next() timelineGroupingMode {
	switch mode {
	case timelineGroupingThread:
		return timelineGroupingSender
	case timelineGroupingSender:
		return timelineGroupingDomain
	default:
		return timelineGroupingThread
	}
}

// timelineRowKind distinguishes collapsed thread headers from individual email rows.
type timelineRowKind int

const (
	rowKindThread timelineRowKind = iota // collapsed thread header (>1 email, not expanded)
	rowKindEmail                         // individual email row
)

const (
	threadCollapsedPrefix  = "▸ "
	threadExpandedPrefix   = "▾ "
	threadReplyPrefix      = "↩ "
	threadNestedPrefix     = "  ↳ "
	timelineAttachmentMark = "📎 "
)

func draftLabel(count int) string {
	if count <= 1 {
		return "Draft"
	}
	return fmt.Sprintf("Draft %d", count)
}

func draftKindLabel(email *models.EmailData) string {
	if email != nil && email.IsDraft && isReplySubject(email.Subject) {
		return "Draft reply"
	}
	return "Draft"
}

func draftStateText(email *models.EmailData) string {
	return draftKindLabel(email) + " - E edit draft - Ctrl+S send"
}

func threadDraftCount(emails []*models.EmailData) int {
	count := 0
	for _, email := range emails {
		if email != nil && email.IsDraft {
			count++
		}
	}
	return count
}

// timelineRowRef maps a table-cursor position to a thread group and email.
type timelineRowRef struct {
	kind     timelineRowKind
	group    *threadGroup
	emailIdx int // index into group.emails; meaningful only for rowKindEmail
}

func (m *Model) loadTimelineEmails() tea.Cmd {
	folder := m.currentFolder
	sourceID := m.activeSourceID
	return func() tea.Msg {
		if isVirtualAllMailOnlyFolder(folder) {
			view, err := m.backend.GetAllMailOnlyView()
			if err != nil {
				return TimelineLoadedMsg{
					SourceID: sourceID,
					Folder:   folder,
					Emails:   []*models.EmailData{},
					Notice:   "All Mail only inspection failed: " + err.Error(),
					ReadOnly: true,
				}
			}
			if view == nil {
				return TimelineLoadedMsg{
					SourceID: sourceID,
					Folder:   folder,
					Emails:   []*models.EmailData{},
					Notice:   "All Mail only inspector returned no data",
					ReadOnly: true,
				}
			}
			emails := view.Emails
			if emails == nil {
				emails = []*models.EmailData{}
			}
			return TimelineLoadedMsg{
				SourceID: sourceID,
				Folder:   folder,
				Emails:   emails,
				Notice:   view.Reason,
				ReadOnly: true,
			}
		}
		emails, err := m.backend.GetTimelineEmails(folder)
		if err != nil {
			logger.Error("Failed to load timeline emails: %v", err)
			return TimelineLoadedMsg{SourceID: sourceID, Folder: folder, Emails: nil}
		}
		return TimelineLoadedMsg{SourceID: sourceID, Folder: folder, Emails: emails}
	}
}

func (m *Model) hydrateCachedTimelineForCurrentFolder() bool {
	if m.backend == nil || isVirtualAllMailOnlyFolder(m.currentFolder) {
		m.reflowCurrentLayout()
		return false
	}
	emails, err := m.backend.GetTimelineEmails(m.currentFolder)
	if err != nil {
		logger.Warn("Failed to hydrate cached timeline for %s: %v", m.currentFolder, err)
		m.reflowCurrentLayout()
		return false
	}
	m.timeline.emails = emails
	m.timeline.virtualNotice = ""
	m.loadClassifications()
	m.reflowCurrentLayout()
	return true
}

// normalizeSubject strips common reply/forward prefixes (case-insensitive) so that
// "Re: Re: Hello" and "Fwd: Hello" both map to "hello".
func normalizeSubject(s string) string {
	prefixes := []string{"re:", "fwd:", "fw:", "aw:", "tr:"}
	s = strings.TrimSpace(strings.ToLower(s))
	for {
		changed := false
		for _, p := range prefixes {
			if strings.HasPrefix(s, p) {
				s = strings.TrimSpace(s[len(p):])
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return s
}

func isReplySubject(s string) bool {
	replyPrefixes := []string{"re:", "aw:", "tr:"}
	s = strings.TrimSpace(strings.ToLower(s))
	for _, p := range replyPrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func senderAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if addr, err := mail.ParseAddress(raw); err == nil && addr.Address != "" {
		return strings.ToLower(strings.TrimSpace(addr.Address))
	}
	start := strings.LastIndex(raw, "<")
	end := strings.LastIndex(raw, ">")
	if start >= 0 && end > start {
		return strings.ToLower(strings.TrimSpace(raw[start+1 : end]))
	}
	return strings.ToLower(raw)
}

func senderDisplayLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "(unknown)"
	}
	if addr, err := mail.ParseAddress(raw); err == nil {
		if name := sanitizeText(addr.Name); name != "" {
			return name
		}
		if addr.Address != "" {
			return addr.Address
		}
	}
	if lt := strings.Index(raw, " <"); lt > 0 {
		if name := sanitizeText(raw[:lt]); name != "" {
			return name
		}
	}
	if cleaned := sanitizeText(raw); cleaned != "" {
		return cleaned
	}
	return raw
}

func threadParticipantLabels(emails []*models.EmailData, fromAddress string) []string {
	labels := make([]string, 0, len(emails))
	seen := make(map[string]bool)
	from := strings.ToLower(strings.TrimSpace(fromAddress))

	for _, email := range emails {
		if email == nil {
			continue
		}
		addr := senderAddress(email.Sender)
		label := senderDisplayLabel(email.Sender)
		key := addr
		if from != "" && addr == from {
			label = "me"
			key = "me"
		}
		if key == "" {
			key = strings.ToLower(label)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		labels = append(labels, label)
	}

	return labels
}

func styledThreadParticipants(theme Theme, labels []string, maxWidth int) string {
	if len(labels) == 0 {
		labels = []string{"(unknown)"}
	}
	joined := truncate(strings.Join(labels, ", "), maxWidth)
	return lipgloss.NewStyle().Foreground(theme.Text.Primary.ForegroundColor()).Render(joined)
}

func formatTimelineListDate(date time.Time) string {
	if date.IsZero() {
		return "N/A"
	}
	return formatTimelineListDateAt(date.Local(), time.Now().Local())
}

func formatTimelineListDateAt(date, now time.Time) string {
	if date.IsZero() {
		return "N/A"
	}
	loc := now.Location()
	localDate := date.In(loc)
	localNow := now.In(loc)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	day := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, loc)
	daysAgo := int(today.Sub(day).Hours() / 24)

	switch {
	case daysAgo == 0:
		return localDate.Format("3:04 PM")
	case daysAgo == 1:
		return "Yesterday"
	case daysAgo > 1 && daysAgo < 7:
		return localDate.Format("Mon 3:04 PM")
	case localDate.Year() == localNow.Year():
		return localDate.Format("Jan 2")
	default:
		return localDate.Format("Jan 2 2006")
	}
}

func formatPreviewHeaderDate(date time.Time) string {
	if date.IsZero() {
		return "N/A"
	}
	return date.Local().Format("Mon, Jan 2, 2006 at 3:04 PM")
}

func timelineSubjectText(subject string, hasAttachments bool) string {
	subject = sanitizeText(subject)
	if subject == "" {
		subject = "(no subject)"
	}
	if hasAttachments {
		return timelineAttachmentMark + subject
	}
	return subject
}

// buildThreadGroups groups emails by normalised subject.
// emails must already be sorted newest-first; group order is determined by each
// group's most-recent email, so groups are also implicitly newest-first.
func buildThreadGroups(emails []*models.EmailData) []threadGroup {
	return buildTimelineGroups(emails, timelineGroupingThread)
}

func buildTimelineGroups(emails []*models.EmailData, mode timelineGroupingMode) []threadGroup {
	if mode != timelineGroupingSender && mode != timelineGroupingDomain {
		return buildThreadSubjectGroups(emails)
	}

	var groups []threadGroup
	seen := make(map[string]int)
	for _, e := range emails {
		key, label := timelineGroupingKeyAndLabel(e, mode)
		if key == "" {
			key = "(unknown)"
		}
		groupKey := fmt.Sprintf("%s:%s", strings.ToLower(mode.Label()), key)
		if idx, ok := seen[groupKey]; ok {
			groups[idx].emails = append(groups[idx].emails, e)
			continue
		}
		seen[groupKey] = len(groups)
		groups = append(groups, threadGroup{
			normalizedSubject: groupKey,
			emails:            []*models.EmailData{e},
			label:             label,
			groupingMode:      mode,
		})
	}
	return groups
}

func buildThreadSubjectGroups(emails []*models.EmailData) []threadGroup {
	var groups []threadGroup
	seen := make(map[string]int) // normalised subject → index in groups

	for _, e := range emails {
		ns := normalizeSubject(e.Subject)
		if ns == "" {
			// Empty subjects are never grouped; each stands alone.
			groups = append(groups, threadGroup{
				normalizedSubject: ns,
				emails:            []*models.EmailData{e},
				groupingMode:      timelineGroupingThread,
			})
			continue
		}
		if idx, ok := seen[ns]; ok {
			groups[idx].emails = append(groups[idx].emails, e)
		} else {
			seen[ns] = len(groups)
			groups = append(groups, threadGroup{
				normalizedSubject: ns,
				emails:            []*models.EmailData{e},
				groupingMode:      timelineGroupingThread,
			})
		}
	}
	return groups
}

func timelineGroupingKeyAndLabel(email *models.EmailData, mode timelineGroupingMode) (string, string) {
	if email == nil {
		return "", "(unknown)"
	}
	switch mode {
	case timelineGroupingSender:
		key := senderAddress(email.Sender)
		label := senderDisplayLabel(email.Sender)
		if key == "" {
			key = strings.ToLower(label)
		}
		return key, label
	case timelineGroupingDomain:
		domain := timelineSenderDomain(email.Sender)
		return domain, domain
	default:
		subject := normalizeSubject(email.Subject)
		return subject, subject
	}
}

func timelineSenderDomain(raw string) string {
	addr := senderAddress(raw)
	if at := strings.LastIndex(addr, "@"); at >= 0 && at < len(addr)-1 {
		return timelineRootDomain(addr[at+1:])
	}
	if addr != "" {
		return addr
	}
	return "(unknown)"
}

func timelineRootDomain(host string) string {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "."))
	if host == "" {
		return "(unknown)"
	}
	parts := strings.Split(host, ".")
	clean := parts[:0]
	for _, part := range parts {
		if part != "" {
			clean = append(clean, part)
		}
	}
	parts = clean
	if len(parts) == 0 {
		return "(unknown)"
	}
	if len(parts) > 2 {
		switch parts[len(parts)-2] {
		case "co", "com", "org", "gov", "edu", "net":
			return strings.Join(parts[len(parts)-3:], ".")
		}
	}
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return parts[0]
}

func (m *Model) ensureTimelineSelection() {
	if m.timeline.selectedMessageIDs == nil {
		m.timeline.selectedMessageIDs = make(map[string]bool)
	}
}

func timelineSelectionKey(email *models.EmailData) string {
	if email == nil {
		return ""
	}
	if email.SourceID != "" && email.SourceID != models.DefaultMailSourceID {
		return email.MessageRef().LocalID
	}
	if email.AccountID != "" && email.AccountID != models.DefaultAccountID {
		return email.MessageRef().LocalID
	}
	if strings.TrimSpace(email.LocalID) != "" && !strings.HasPrefix(email.LocalID, "mail:default-mail:default:") {
		return email.LocalID
	}
	return email.MessageID
}

func timelineEmailMatchesSelectionKey(email *models.EmailData, key string) bool {
	if email == nil || key == "" {
		return false
	}
	if timelineSelectionKey(email) == key {
		return true
	}
	return email.MessageID != "" && email.MessageID == key
}

func timelineEmailsSameIdentity(a, b *models.EmailData) bool {
	if a == nil || b == nil {
		return false
	}
	aKey := timelineSelectionKey(a)
	bKey := timelineSelectionKey(b)
	if aKey != "" || bKey != "" {
		return aKey == bKey
	}
	return a == b
}

func (m *Model) pruneTimelineSelection(displayEmails []*models.EmailData) {
	if len(m.timeline.selectedMessageIDs) == 0 {
		return
	}
	visible := make(map[string]bool, len(displayEmails))
	for _, email := range displayEmails {
		if key := timelineSelectionKey(email); key != "" {
			visible[key] = true
		}
	}
	for key := range m.timeline.selectedMessageIDs {
		if !visible[key] {
			delete(m.timeline.selectedMessageIDs, key)
		}
	}
}

func (m *Model) clearTimelineSelection() {
	m.timeline.selectedMessageIDs = make(map[string]bool)
	m.finishTimelineRangeSelection()
}

func cloneTimelineSelectedMessageIDs(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for id, selected := range src {
		if selected {
			dst[id] = true
		}
	}
	return dst
}

func (m *Model) finishTimelineRangeSelection() {
	m.timeline.rangeMode = false
	m.timeline.rangeShiftMode = false
	m.timeline.rangeAnchorRow = -1
	m.timeline.rangeCursorRow = -1
	m.timeline.rangeBaseSelection = nil
}

func (m *Model) timelineRowEmails(ref timelineRowRef) []*models.EmailData {
	if ref.group == nil {
		return nil
	}
	if ref.kind == rowKindThread {
		return ref.group.emails
	}
	if ref.emailIdx < 0 || ref.emailIdx >= len(ref.group.emails) {
		return nil
	}
	return []*models.EmailData{ref.group.emails[ref.emailIdx]}
}

func (m *Model) currentTimelineRowInRange() bool {
	cursor := m.timelineTable.Cursor()
	return cursor >= 0 && cursor < len(m.timeline.threadRowMap)
}

func (m *Model) beginTimelineRangeSelection(shiftMode bool) bool {
	if m.activeTab != tabTimeline || m.timelineIsReadOnlyDiagnostic() || m.focusedPanel != panelTimeline || !m.currentTimelineRowInRange() {
		return false
	}
	m.ensureTimelineSelection()
	cursor := m.timelineTable.Cursor()
	m.timeline.rangeMode = true
	m.timeline.rangeShiftMode = shiftMode
	m.timeline.rangeAnchorRow = cursor
	m.timeline.rangeCursorRow = cursor
	m.timeline.rangeBaseSelection = cloneTimelineSelectedMessageIDs(m.timeline.selectedMessageIDs)
	m.applyTimelineRangeSelection()
	return true
}

func (m *Model) applyTimelineRangeSelection() {
	if !m.timeline.rangeMode {
		return
	}
	if len(m.timeline.threadRowMap) == 0 {
		m.finishTimelineRangeSelection()
		return
	}
	anchor := m.timeline.rangeAnchorRow
	cursor := m.timeline.rangeCursorRow
	if anchor < 0 {
		anchor = 0
	}
	if cursor < 0 {
		cursor = 0
	}
	last := len(m.timeline.threadRowMap) - 1
	if anchor > last {
		anchor = last
	}
	if cursor > last {
		cursor = last
	}
	m.timeline.rangeAnchorRow = anchor
	m.timeline.rangeCursorRow = cursor

	lo, hi := anchor, cursor
	if lo > hi {
		lo, hi = hi, lo
	}

	selected := cloneTimelineSelectedMessageIDs(m.timeline.rangeBaseSelection)
	for row := lo; row <= hi; row++ {
		for _, email := range m.timelineRowEmails(m.timeline.threadRowMap[row]) {
			if key := timelineSelectionKey(email); key != "" {
				selected[key] = true
			}
		}
	}
	m.timeline.selectedMessageIDs = selected
	m.updateTimelineTable()
	m.timelineTable.SetCursor(cursor)
}

func (m *Model) extendTimelineRangeSelection(delta int, shiftMode bool) tea.Cmd {
	if m.activeTab != tabTimeline || m.timelineIsReadOnlyDiagnostic() || m.focusedPanel != panelTimeline {
		return nil
	}
	if len(m.timeline.threadRowMap) == 0 {
		return nil
	}
	if !m.timeline.rangeMode && !m.beginTimelineRangeSelection(shiftMode) {
		return nil
	}
	cursor := m.timeline.rangeCursorRow + delta
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(m.timeline.threadRowMap) {
		cursor = len(m.timeline.threadRowMap) - 1
	}
	m.timeline.rangeCursorRow = cursor
	m.timelineTable.SetCursor(cursor)
	m.applyTimelineRangeSelection()
	return m.maybeUpdatePreview()
}

func timelineShiftRangeDelta(msg tea.KeyPressMsg, key string) (int, bool) {
	switch key {
	case "shift+up":
		return -1, true
	case "shift+down":
		return 1, true
	}
	k := msg.Key()
	if !k.Mod.Contains(tea.ModShift) {
		return 0, false
	}
	switch k.Code {
	case tea.KeyUp:
		return -1, true
	case tea.KeyDown:
		return 1, true
	}
	return 0, false
}

func (m *Model) timelineSelectionMark(emails []*models.EmailData) string {
	if len(emails) == 0 || len(m.timeline.selectedMessageIDs) == 0 {
		return ""
	}
	selectable := 0
	selected := 0
	for _, email := range emails {
		key := timelineSelectionKey(email)
		if key == "" {
			continue
		}
		selectable++
		if m.timeline.selectedMessageIDs[key] {
			selected++
		}
	}
	if selectable == 0 || selected == 0 {
		return ""
	}
	if selected == selectable {
		return "✓"
	}
	return "•"
}

func (m *Model) currentTimelineRowEmails() []*models.EmailData {
	ref, ok := m.currentTimelineRowRef()
	if !ok {
		return nil
	}
	return m.timelineRowEmails(ref)
}

func (m *Model) currentTimelineFocusedDraftEmail() *models.EmailData {
	if m.focusedPanel == panelPreview && m.timeline.selectedEmail != nil && m.timeline.selectedEmail.IsDraft {
		return m.timeline.selectedEmail
	}
	emails := m.currentTimelineRowEmails()
	if len(emails) == 1 && emails[0] != nil && emails[0].IsDraft {
		return emails[0]
	}
	return nil
}

func (m *Model) selectedTimelineEmails(includeDrafts bool) []*models.EmailData {
	if len(m.timeline.selectedMessageIDs) == 0 {
		return nil
	}
	var out []*models.EmailData
	for _, email := range m.timelineDisplayEmails() {
		key := timelineSelectionKey(email)
		if key == "" || !m.timeline.selectedMessageIDs[key] {
			continue
		}
		if !includeDrafts && email.IsDraft {
			continue
		}
		out = append(out, email)
	}
	return out
}

func (m *Model) timelineSelectedCount() int {
	return len(m.selectedTimelineEmails(true))
}

func (m *Model) selectedTimelineArchiveEmails() []*models.EmailData {
	return m.selectedTimelineEmails(false)
}

func (m *Model) timelineShowsAccountBadges() bool {
	if !m.hasMultipleAccounts() {
		return false
	}
	if m.allAccountsScopeActive() {
		return true
	}
	var seen models.SourceID
	for _, email := range m.timelineDisplayEmails() {
		if email == nil || email.SourceID == "" {
			continue
		}
		if seen == "" {
			seen = email.SourceID
			continue
		}
		if seen != email.SourceID {
			return true
		}
	}
	return false
}

func (m *Model) accountLabelForSource(sourceID models.SourceID) string {
	if sourceID == backend.AllAccountsSourceID {
		return "All Accounts"
	}
	for _, account := range m.accounts {
		if account.SourceID == sourceID {
			if strings.TrimSpace(account.DisplayName) != "" {
				return account.DisplayName
			}
			break
		}
	}
	if sourceID == "" {
		return ""
	}
	return string(sourceID)
}

func (m *Model) accountBadgeForEmail(email *models.EmailData) string {
	if email == nil {
		return ""
	}
	return m.accountLabelForSource(email.SourceID)
}

func (m *Model) accountBadgeForEmails(emails []*models.EmailData) string {
	var label string
	for _, email := range emails {
		current := m.accountBadgeForEmail(email)
		if current == "" {
			continue
		}
		if label == "" {
			label = current
			continue
		}
		if label != current {
			return "Mixed"
		}
	}
	return label
}

func timelineCollapsedGroupLabel(theme Theme, g *threadGroup, fromAddress string, maxWidth int) string {
	if g == nil {
		return styledThreadParticipants(theme, nil, maxWidth)
	}
	if g.groupingMode == timelineGroupingSender || g.groupingMode == timelineGroupingDomain {
		label := sanitizeText(g.label)
		if label == "" {
			label = "(unknown)"
		}
		return lipgloss.NewStyle().
			Foreground(theme.Text.Primary.ForegroundColor()).
			Render(truncate(label, maxWidth))
	}
	return styledThreadParticipants(theme, threadParticipantLabels(g.emails, fromAddress), maxWidth)
}

func timelineGroupStarred(g *threadGroup) bool {
	return g != nil && len(g.emails) > 0 && g.emails[0] != nil && g.emails[0].IsStarred
}

func timelineGroupNewestDate(g *threadGroup) time.Time {
	if g == nil || len(g.emails) == 0 || g.emails[0] == nil {
		return time.Time{}
	}
	return g.emails[0].Date
}

func timelineGroupSortSenderLabel(g *threadGroup, fromAddress string) string {
	if g == nil {
		return ""
	}
	if g.groupingMode == timelineGroupingSender || g.groupingMode == timelineGroupingDomain {
		label := sanitizeText(g.label)
		if label != "" {
			return label
		}
		return "(unknown)"
	}
	return strings.Join(threadParticipantLabels(g.emails, fromAddress), ", ")
}

func compareTimelineGroupStrings(a, b string, ascending bool) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == b {
		return false
	}
	if ascending {
		return a < b
	}
	return a > b
}

func compareTimelineGroupTimes(a, b time.Time, ascending bool) bool {
	if a.Equal(b) {
		return false
	}
	if ascending {
		return a.Before(b)
	}
	return a.After(b)
}

func compareTimelineGroupCounts(a, b int, ascending bool) bool {
	if a == b {
		return false
	}
	if ascending {
		return a < b
	}
	return a > b
}

func (m *Model) sortTimelineGroups(groups []threadGroup) {
	sortMode := m.timeline.sortMode
	sort.SliceStable(groups, func(i, j int) bool {
		left := &groups[i]
		right := &groups[j]
		leftStarred := timelineGroupStarred(left)
		rightStarred := timelineGroupStarred(right)
		if leftStarred != rightStarred {
			return leftStarred
		}
		ascending := sortMode.ascending()
		switch sortMode.criterion() {
		case timelineSortCriterionSender:
			return compareTimelineGroupStrings(
				timelineGroupSortSenderLabel(left, m.fromAddress),
				timelineGroupSortSenderLabel(right, m.fromAddress),
				ascending,
			)
		case timelineSortCriterionCount:
			return compareTimelineGroupCounts(len(left.emails), len(right.emails), ascending)
		default:
			return compareTimelineGroupTimes(timelineGroupNewestDate(left), timelineGroupNewestDate(right), ascending)
		}
	})
}

func timelineExpandedRowPrefix(g *threadGroup, email *models.EmailData, idx int) string {
	if g == nil || g.groupingMode == timelineGroupingThread {
		if idx == 0 {
			prefix := threadExpandedPrefix
			if email != nil && isReplySubject(email.Subject) {
				prefix += threadReplyPrefix
			}
			return prefix
		}
		if email != nil && isReplySubject(email.Subject) {
			return threadReplyPrefix
		}
		if idx > 0 {
			return threadNestedPrefix
		}
		return ""
	}
	if idx == 0 {
		return threadExpandedPrefix
	}
	return threadNestedPrefix
}

// updateTimelineTable rebuilds the timeline table rows from m.timeline.emails,
// grouping them into collapsed threads where appropriate.
func (m *Model) updateTimelineTable() {
	m.ensureTimelineSelection()
	showAccount := m.timelineShowsAccountBadges() && m.timeline.accountColumnVisible
	maxSubj := m.timeline.subjectWidth
	if maxSubj <= 0 {
		maxSubj = 40
	}
	maxSend := m.timeline.senderWidth
	if maxSend <= 0 {
		maxSend = 20
	}

	trunc := func(s string, n int) string {
		if n <= 0 {
			return ""
		}
		r := []rune(s)
		if len(r) <= n {
			return s
		}
		if n <= 3 {
			return string(r[:n])
		}
		return string(r[:n-3]) + "..."
	}

	emailRow := func(email *models.EmailData, senderPrefix string) table.Row {
		dateStr := formatTimelineListDate(email.Date)
		subject := timelineSubjectText(email.Subject, email.HasAttachments)
		if email.IsDraft {
			subject = draftKindLabel(email) + ": " + subject
			senderPrefix += draftLabel(1) + " "
		}
		// Prepend similarity score badge for semantic search results
		if m.timeline.semanticScores != nil {
			if score, ok := m.timeline.semanticScores[email.MessageID]; ok {
				pct := int(score * 100)
				subject = fmt.Sprintf("[%d%%] %s", pct, subject)
			}
		}
		unreadDot := " "
		if !email.IsRead {
			unreadDot = "●"
		}
		starDot := " "
		if email.IsStarred {
			starDot = "★"
		}
		indicatorWidth := len([]rune(unreadDot)) + len([]rune(starDot)) + len([]rune(senderPrefix))
		senderAvail := maxSend - indicatorWidth
		if senderAvail < 1 {
			senderAvail = 1
		}
		sender := unreadDot + starDot + senderPrefix + styledSenderWithTheme(m.theme, email.Sender, senderAvail)
		tag := ""
		tag = m.classificationForEmail(email)
		row := table.Row{
			m.timelineSelectionMark([]*models.EmailData{email}),
		}
		if showAccount {
			row = append(row, m.accountBadgeForEmail(email))
		}
		row = append(row, sender, trunc(subject, maxSubj), dateStr, tag)
		return row
	}

	displayEmails := m.timelineDisplayEmails()
	m.pruneTimelineSelection(displayEmails)

	// Build groups from the full display list. Thread mode keeps the original
	// subject-based behavior; sender/domain modes reuse the same row machinery.
	m.timeline.threadGroups = buildTimelineGroups(displayEmails, m.timeline.groupingMode)
	m.timeline.threadRowMap = m.timeline.threadRowMap[:0]
	m.sortTimelineGroups(m.timeline.threadGroups)

	var rows []table.Row
	for gi := range m.timeline.threadGroups {
		g := &m.timeline.threadGroups[gi]
		expanded := m.timeline.expandedThreads[g.normalizedSubject]

		if len(g.emails) == 1 {
			// Single-email thread: show as a plain row
			rows = append(rows, emailRow(g.emails[0], ""))
			m.timeline.threadRowMap = append(m.timeline.threadRowMap, timelineRowRef{
				kind: rowKindEmail, group: g, emailIdx: 0,
			})
			continue
		}

		if !expanded {
			// Collapsed thread header: newest email's sender, subject with [N] prefix
			newest := g.emails[0]
			dateStr := formatTimelineListDate(newest.Date)
			anyAtt := false
			for _, e := range g.emails {
				if e.HasAttachments {
					anyAtt = true
				}
			}
			tag := ""
			tag = m.classificationForEmail(newest)
			threadSubj := timelineSubjectText(fmt.Sprintf("[%d] %s", len(g.emails), sanitizeText(newest.Subject)), anyAtt)
			if drafts := threadDraftCount(g.emails); drafts > 0 {
				threadSubj = draftLabel(drafts) + " " + threadSubj
			}
			// Build sender cell with the same indicators as single-email rows
			// so columns stay aligned across all timeline rows.
			unreadDot := " "
			if !newest.IsRead {
				unreadDot = "●"
			}
			starDot := " "
			if newest.IsStarred {
				starDot = "★"
			}
			indicatorWidth := len([]rune(unreadDot)) + len([]rune(starDot)) + len([]rune(threadCollapsedPrefix))
			senderAvail := maxSend - indicatorWidth
			if senderAvail < 1 {
				senderAvail = 1
			}
			threadSender := unreadDot + starDot + threadCollapsedPrefix + timelineCollapsedGroupLabel(m.theme, g, m.fromAddress, senderAvail)
			row := table.Row{
				m.timelineSelectionMark(g.emails),
			}
			if showAccount {
				row = append(row, m.accountBadgeForEmails(g.emails))
			}
			row = append(row, threadSender, trunc(threadSubj, maxSubj), dateStr, tag)
			rows = append(rows, row)
			m.timeline.threadRowMap = append(m.timeline.threadRowMap, timelineRowRef{
				kind: rowKindThread, group: g,
			})
		} else {
			// Expanded: thread mode marks replies explicitly; sender/domain
			// grouping keeps a simpler nested shape under the grouped row.
			for ei, email := range g.emails {
				rows = append(rows, emailRow(email, timelineExpandedRowPrefix(g, email, ei)))
				m.timeline.threadRowMap = append(m.timeline.threadRowMap, timelineRowRef{
					kind: rowKindEmail, group: g, emailIdx: ei,
				})
			}
		}
	}

	m.timelineTable.SetRows(rows)
	if len(rows) == 0 {
		m.timelineTable.SetCursor(0)
		return
	}
	cursor := m.timelineTable.Cursor()
	if cursor < 0 {
		m.timelineTable.SetCursor(0)
		return
	}
	if cursor >= len(rows) {
		m.timelineTable.SetCursor(len(rows) - 1)
	}
}

type timelineViewRender struct {
	Content         string
	NativeImageTail string
}

// renderTimelineView renders the timeline tab content.
// When an email is selected, it splits into a list on the left and preview on the right.
func (m *Model) renderTimelineView() string {
	return m.renderTimelineViewFrame(0).Content
}

func (m *Model) renderTimelineViewFrame(mainTopRow int) timelineViewRender {
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	chrome := m.chromeState(plan)

	var tableView string
	if m.timeline.emails != nil && len(m.timeline.emails) == 0 {
		notice := "No emails in this folder  •  press r to refresh"
		if m.timelineIsReadOnlyDiagnostic() {
			if m.timeline.virtualNotice != "" {
				notice = m.timeline.virtualNotice
			} else {
				notice = "No messages matched the All Mail only diagnostic"
			}
		}
		tableView = m.emptyStateView(notice)
	} else {
		style := m.baseStyle.
			Width(plan.Timeline.TableWidth + 2).
			BorderForeground(m.theme.Focus.PanelBorder.ForegroundColor())
		tableStyles := m.inactiveTableStyle
		if chrome.FocusedPanel == panelTimeline {
			style = style.BorderForeground(m.theme.Focus.PanelBorderFocused.ForegroundColor())
			tableStyles = m.activeTableStyle
		}
		tableView = m.renderTimelineGroupingNotice(style.Render(renderStyledTableViewWithStyles(&m.timelineTable, tableStyles)))
	}

	var mainContent string
	var previewFrame emailPreviewRender
	if m.timeline.selectedEmail != nil {
		previewFrame = m.renderEmailPreviewFrame()
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, tableView, panelGap, previewFrame.Panel)
	} else {
		mainContent = tableView
	}

	contentLeftCol := 1
	if plan.SidebarVisible {
		sidebarStyle := m.baseStyle.Width(sidebarContentWidth + 2)
		if chrome.FocusedPanel == panelSidebar {
			sidebarStyle = sidebarStyle.BorderForeground(m.theme.Focus.PanelBorderFocused.ForegroundColor())
		} else {
			sidebarStyle = sidebarStyle.BorderForeground(m.theme.Focus.PanelBorder.ForegroundColor())
		}
		sidebarView := sidebarStyle.Render(m.renderSidebar())
		contentLeftCol += lipgloss.Width(sidebarView) + panelGapWidth
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, panelGap, mainContent)
	}

	nativeTail := ""
	if mainTopRow > 0 && len(previewFrame.NativeOverlays) > 0 {
		originRow := mainTopRow + 1 + previewFrame.DocumentStartLine
		originCol := contentLeftCol + lipgloss.Width(tableView) + panelGapWidth + 2
		maxBottomRow := mainTopRow + previewFrame.PanelInnerHeight
		overlays := filterNativeOverlaysWithinBottomRow(previewFrame.NativeOverlays, originRow, maxBottomRow)
		nativeTail = renderNativeImageOverlayTail(overlays, originRow, originCol)
	}
	return timelineViewRender{Content: mainContent, NativeImageTail: nativeTail}
}

func (m *Model) timelineGroupingNoticeText(maxWidth int) string {
	if maxWidth < 16 {
		return ""
	}
	key := displayShortcutKey(m.commandKey("timeline", CommandTimelineGroupCycle), keyDisplayHint)
	if key == "" {
		key = "G"
	}
	text := fmt.Sprintf("Grouped by: %s (%s to change)", m.timeline.groupingMode.Label(), key)
	if ansi.StringWidth(text) > maxWidth {
		text = truncateVisual(text, maxWidth)
	}
	return text
}

func (m *Model) renderTimelineGroupingNotice(view string) string {
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		return view
	}
	lineWidth := ansi.StringWidth(lines[0])
	maxTextWidth := lineWidth - 4
	text := m.timelineGroupingNoticeText(maxTextWidth)
	if text == "" {
		return view
	}

	notice := defaultTheme.Text.Dim.Style().Render(" " + text + " ")
	noticeWidth := ansi.StringWidth(notice)
	startX := lineWidth - noticeWidth - 2
	if startX < 1 {
		startX = 1
	}

	line := lines[0]
	left := padANSIToWidth(ansi.Cut(line, 0, startX), startX)
	right := ansi.Cut(line, startX+noticeWidth, lineWidth)
	lines[0] = ansi.Cut(left+notice+right, 0, lineWidth)
	return strings.Join(lines, "\n")
}

func (m *Model) timelineSortColumnTitle(title string, criterion timelineSortCriterion) string {
	if m.timeline.sortMode.criterion() != criterion {
		return title
	}
	return title + " " + m.timeline.sortMode.directionIndicator()
}

func (m *Model) clearTimelineSearch() {
	m.finishTimelineRangeSelection()
	m.timeline.searchToken++
	m.timeline.searchMode = false
	m.timeline.searchFocus = timelineSearchFocusInput
	m.timeline.searchAutoFocusResults = false
	m.timeline.searchInput.Blur()
	m.timeline.searchInput.SetValue("")
	m.timeline.searchResults = nil
	m.timeline.searchResultsQuery = ""
	m.timeline.semanticScores = nil
	m.timeline.searchError = ""
	if m.timeline.emailsCache != nil {
		m.timeline.emails = m.timeline.emailsCache
	}
	if origin := m.timeline.searchOrigin; origin != nil {
		m.timeline.expandedThreads = cloneTimelineExpandedThreads(origin.expandedThreads)
		m.timeline.selectedEmail = origin.selectedEmail
		m.timeline.body = origin.body
		m.timeline.bodyMessageID = origin.bodyMessageID
		m.timeline.bodyLoading = origin.bodyLoading
		m.timeline.inlineImageDescs = cloneInlineImageDescs(origin.inlineImageDescs)
		m.timeline.remoteImageLoads = cloneRemoteImageStates(origin.remoteImageLoads)
		m.timeline.remoteImageRevision = origin.remoteImageRevision
		m.timeline.fullScreen = origin.fullScreen
		m.timeline.bodyScrollOffset = origin.bodyScrollOffset
		m.timeline.bodyWrappedLines = nil
		m.clearTimelinePreviewDocumentCache()
		m.clearPreviewSelection()
		m.timeline.quickReplyOpen = false
		m.timeline.quickReplyPending = false
		m.timeline.quickReplyIdx = 0
		m.timeline.attachmentSavePrompt = false
		m.timeline.attachmentSaveWarning = ""
		m.timeline.attachmentSaveInput.Blur()
		m.updateTimelineTable()
		maxCursor := len(m.timeline.threadRowMap) - 1
		cursor := origin.cursor
		if maxCursor >= 0 {
			if cursor < 0 {
				cursor = 0
			}
			if cursor > maxCursor {
				cursor = maxCursor
			}
			m.timelineTable.SetCursor(cursor)
		}
		m.setFocusedPanel(origin.focusedPanel)
	} else {
		m.updateTimelineTable()
		m.setFocusedPanel(panelTimeline)
	}
	m.timeline.searchOrigin = nil
	m.timeline.emailsCache = nil
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
}

func (m *Model) clearTimelineQuickReply() {
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyPending = false
	m.timeline.quickReplyIdx = 0
}

func (m *Model) clearTimelineFullScreen() {
	m.clearPreviewSelection()
	m.timeline.fullScreen = false
	m.timeline.bodyWrappedLines = nil
	m.clearTimelinePreviewDocumentCache()
}

func (m *Model) clearTimelinePreview() {
	m.clearPreviewSelection()
	m.revokeImagePreviews()
	m.previewPrintPending = false
	m.timeline.selectedEmail = nil
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyLoading = false
	m.timeline.bodyWrappedLines = nil
	m.clearTimelinePreviewDocumentCache()
	m.timeline.bodyScrollOffset = 0
	m.clearPreviewSelection()
	m.setFocusedPanel(panelTimeline)
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
}

func (m *Model) cycleTimelineGrouping() {
	m.finishTimelineRangeSelection()
	m.timeline.groupingMode = m.timeline.groupingMode.next()
	if m.timeline.selectedEmail != nil || m.timeline.body != nil || m.timeline.bodyMessageID != "" {
		m.clearTimelinePreview()
	} else {
		m.updateTimelineTable()
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
	if len(m.timeline.threadRowMap) == 0 {
		m.timelineTable.SetCursor(0)
	}
	m.statusMessage = "Grouped by " + strings.ToLower(m.timeline.groupingMode.Label())
}

func (m *Model) timelineSortAnchor() string {
	if m.timeline.selectedEmail != nil {
		if key := timelineSelectionKey(m.timeline.selectedEmail); key != "" {
			return "email:" + key
		}
	}
	cursor := m.timelineTable.Cursor()
	if cursor < 0 || cursor >= len(m.timeline.threadRowMap) {
		return ""
	}
	ref := m.timeline.threadRowMap[cursor]
	if ref.kind == rowKindThread && ref.group != nil {
		return "group:" + ref.group.normalizedSubject
	}
	for _, email := range m.timelineRowEmails(ref) {
		if key := timelineSelectionKey(email); key != "" {
			return "email:" + key
		}
	}
	return ""
}

func (m *Model) restoreTimelineSortAnchor(anchor string) {
	if anchor == "" {
		return
	}
	groupKey, wantGroup := strings.CutPrefix(anchor, "group:")
	emailKey, wantEmail := strings.CutPrefix(anchor, "email:")
	for idx, ref := range m.timeline.threadRowMap {
		if wantGroup && ref.group != nil && ref.group.normalizedSubject == groupKey {
			m.timelineTable.SetCursor(idx)
			return
		}
		if wantEmail {
			for _, email := range m.timelineRowEmails(ref) {
				if timelineEmailMatchesSelectionKey(email, emailKey) {
					m.timelineTable.SetCursor(idx)
					return
				}
			}
		}
	}
}

func (m *Model) refreshTimelineSort(anchor string) {
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	} else {
		m.updateTimelineTable()
	}
	m.restoreTimelineSortAnchor(anchor)
}

func (m *Model) cycleTimelineSort() {
	m.finishTimelineRangeSelection()
	anchor := m.timelineSortAnchor()
	m.timeline.sortMode = m.timeline.sortMode.next()
	m.refreshTimelineSort(anchor)
	m.statusMessage = "Sorted by " + m.timeline.sortMode.statusLabel()
}

func (m *Model) setTimelineSortCriterion(criterion timelineSortCriterion) {
	m.finishTimelineRangeSelection()
	anchor := m.timelineSortAnchor()
	if m.timeline.sortMode.criterion() == criterion {
		m.timeline.sortMode = m.timeline.sortMode.flipped()
	} else {
		m.timeline.sortMode = defaultTimelineSortModeForCriterion(criterion)
	}
	m.refreshTimelineSort(anchor)
	m.statusMessage = "Sorted by " + m.timeline.sortMode.statusLabel()
}

func (m *Model) clearTimelineChatFilter() {
	m.finishTimelineRangeSelection()
	m.timeline.chatFilterMode = false
	m.timeline.chatFilteredEmails = nil
	m.timeline.chatFilterLabel = ""
	m.updateTimelineTable()
}

func (m *Model) openTimelineSearch() {
	m.finishTimelineRangeSelection()
	if m.timeline.searchOrigin == nil {
		m.timeline.searchOrigin = &timelineSearchOrigin{
			cursor:              m.timelineTable.Cursor(),
			expandedThreads:     cloneTimelineExpandedThreads(m.timeline.expandedThreads),
			focusedPanel:        m.focusedPanel,
			selectedEmail:       m.timeline.selectedEmail,
			body:                m.timeline.body,
			bodyMessageID:       m.timeline.bodyMessageID,
			bodyLoading:         m.timeline.bodyLoading,
			inlineImageDescs:    cloneInlineImageDescs(m.timeline.inlineImageDescs),
			remoteImageLoads:    cloneRemoteImageStates(m.timeline.remoteImageLoads),
			remoteImageRevision: m.timeline.remoteImageRevision,
			fullScreen:          m.timeline.fullScreen,
			bodyScrollOffset:    m.timeline.bodyScrollOffset,
		}
	}
	m.timeline.searchToken++
	m.timeline.searchMode = true
	m.timeline.searchFocus = timelineSearchFocusInput
	m.timeline.searchAutoFocusResults = false
	if m.timeline.emailsCache == nil {
		m.timeline.emailsCache = m.timeline.emails
	}
	m.timeline.selectedEmail = nil
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyLoading = false
	m.timeline.inlineImageDescs = nil
	m.timeline.remoteImageLoads = nil
	m.timeline.remoteImageRevision++
	m.timeline.fullScreen = false
	m.timeline.bodyWrappedLines = nil
	m.clearTimelinePreviewDocumentCache()
	m.timeline.bodyScrollOffset = 0
	m.clearPreviewSelection()
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyPending = false
	m.timeline.quickReplyIdx = 0
	m.timeline.attachmentSavePrompt = false
	m.timeline.attachmentSaveWarning = ""
	m.timeline.attachmentSaveInput.Blur()
	m.timeline.searchInput.SetValue("")
	m.timeline.searchResults = nil
	m.timeline.searchResultsQuery = ""
	m.timeline.semanticScores = nil
	m.timeline.searchError = ""
	m.timeline.searchInput.Focus()
	m.setFocusedPanel(panelTimeline)
	m.updateTimelineTable()
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
}

func (m *Model) openTimelineSemanticSearch() {
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("? ")
	m.timeline.searchError = ""
	m.timeline.searchResultsQuery = ""
}

func (m *Model) currentTimelineRowEmail() *models.EmailData {
	cursor := m.timelineTable.Cursor()
	if cursor < 0 || cursor >= len(m.timeline.threadRowMap) {
		return nil
	}
	ref := m.timeline.threadRowMap[cursor]
	if ref.kind == rowKindThread {
		return ref.group.emails[0]
	}
	return ref.group.emails[ref.emailIdx]
}

func (m *Model) currentTimelinePreviewTarget() *models.EmailData {
	ref, ok := m.currentTimelineRowRef()
	if !ok || ref.group == nil || len(ref.group.emails) == 0 {
		return nil
	}
	if ref.kind == rowKindThread {
		return ref.group.emails[0]
	}
	if ref.emailIdx < 0 || ref.emailIdx >= len(ref.group.emails) {
		return nil
	}
	return ref.group.emails[ref.emailIdx]
}

func (m *Model) previewCurrentTimelineRow() tea.Cmd {
	if m.focusedPanel == panelSidebar {
		m.setFocusedPanel(panelTimeline)
		return nil
	}
	if m.focusedPanel == panelPreview {
		return nil
	}
	if m.timeline.selectedEmail != nil {
		m.setFocusedPanel(panelPreview)
		return nil
	}
	return m.openTimelineEmail(m.currentTimelinePreviewTarget())
}

func (m *Model) foldCurrentTimelineThreadIfOpen() bool {
	ref, ok := m.currentTimelineRowRef()
	if !ok || ref.group == nil {
		return false
	}
	key := ref.group.normalizedSubject
	isExpandedThreadHeader := ref.kind == rowKindEmail &&
		ref.emailIdx == 0 &&
		len(ref.group.emails) > 1 &&
		m.timeline.expandedThreads[key]
	isExpandedThreadRow := ref.kind == rowKindThread && m.timeline.expandedThreads[key]
	if !isExpandedThreadHeader && !isExpandedThreadRow {
		return false
	}
	if !m.timeline.expandedThreads[key] {
		return false
	}
	savedCursor := m.timelineTable.Cursor()
	m.timeline.expandedThreads[key] = false
	m.updateTimelineTable()
	if savedCursor >= len(m.timeline.threadRowMap) {
		savedCursor = len(m.timeline.threadRowMap) - 1
	}
	if savedCursor >= 0 {
		m.timelineTable.SetCursor(savedCursor)
	}
	return true
}

func (m *Model) focusTimelineFolders() {
	if !m.showSidebar {
		m.showSidebar = true
		if m.windowWidth > 0 {
			m.updateTableDimensions(m.windowWidth, m.windowHeight)
		}
	}
	plan := m.buildLayoutPlan(m.windowWidth, m.windowHeight)
	if plan.SidebarVisible {
		m.setFocusedPanel(panelSidebar)
	} else {
		m.statusMessage = "Folders hidden at this size — widen terminal"
	}
}

func (m *Model) closeTimelinePreviewOrFocusFolders() tea.Cmd {
	if m.focusedPanel == panelPreview {
		m.setFocusedPanel(panelTimeline)
		return nil
	}
	if m.focusedPanel == panelTimeline && m.foldCurrentTimelineThreadIfOpen() {
		return nil
	}
	clearCmd := m.timelineNativeImageClearCmd()
	if m.timeline.selectedEmail != nil {
		m.clearTimelinePreview()
	}
	m.focusTimelineFolders()
	return clearCmd
}

func (m *Model) setTimelineEmailReadState(email *models.EmailData, read bool) bool {
	if email == nil {
		return false
	}
	changed := false
	visit := func(candidate *models.EmailData) {
		if candidate == nil {
			return
		}
		same := candidate == email || timelineEmailsSameIdentity(candidate, email)
		if !same || candidate.IsRead == read {
			return
		}
		candidate.IsRead = read
		changed = true
	}
	visit(email)
	for _, emails := range [][]*models.EmailData{
		m.timeline.emails,
		m.timeline.emailsCache,
		m.timeline.searchResults,
		m.timeline.chatFilteredEmails,
	} {
		for _, candidate := range emails {
			visit(candidate)
		}
	}
	visit(m.timeline.selectedEmail)
	if changed && m.folderStatus != nil {
		folder := email.Folder
		if folder == "" {
			folder = m.currentFolder
		}
		if st, ok := m.folderStatus[folder]; ok {
			if read {
				if st.Unseen > 0 {
					st.Unseen--
				}
			} else {
				st.Unseen++
			}
			m.folderStatus[folder] = st
		}
	}
	return changed
}

func (m *Model) currentTimelineUnreadTarget() *models.EmailData {
	if m.timeline.selectedEmail != nil {
		return m.timeline.selectedEmail
	}
	return m.currentTimelinePreviewTarget()
}

func (m *Model) markCurrentTimelineUnread() tea.Cmd {
	email := m.currentTimelineUnreadTarget()
	if email == nil {
		return nil
	}
	if !email.IsRead {
		m.statusMessage = "Already unread"
		return nil
	}
	if m.setTimelineEmailReadState(email, false) {
		m.updateTimelineTable()
	}
	m.statusMessage = "Marked unread"
	return markUnreadEmailCmd(m.backend, email)
}

func (m *Model) currentTimelineDraftEmail() *models.EmailData {
	if m.focusedPanel == panelPreview && m.timeline.selectedEmail != nil && m.timeline.selectedEmail.IsDraft {
		return m.timeline.selectedEmail
	}
	cursor := m.timelineTable.Cursor()
	if cursor < 0 || cursor >= len(m.timeline.threadRowMap) {
		return nil
	}
	ref := m.timeline.threadRowMap[cursor]
	if ref.kind == rowKindThread {
		for _, email := range ref.group.emails {
			if email != nil && email.IsDraft {
				return email
			}
		}
		return nil
	}
	email := ref.group.emails[ref.emailIdx]
	if email != nil && email.IsDraft {
		return email
	}
	return nil
}

func (m *Model) removeTimelineEmail(messageID string) {
	if strings.TrimSpace(messageID) == "" {
		return
	}
	m.finishTimelineRangeSelection()
	remove := func(emails []*models.EmailData) []*models.EmailData {
		out := emails[:0]
		for _, email := range emails {
			if email == nil || email.MessageID != messageID {
				out = append(out, email)
			}
		}
		return out
	}
	m.timeline.emails = remove(m.timeline.emails)
	if m.timeline.emailsCache != nil {
		m.timeline.emailsCache = remove(m.timeline.emailsCache)
	}
	if m.timeline.searchResults != nil {
		m.timeline.searchResults = remove(m.timeline.searchResults)
	}
	if m.timeline.chatFilteredEmails != nil {
		m.timeline.chatFilteredEmails = remove(m.timeline.chatFilteredEmails)
	}
	if m.timeline.selectedMessageIDs != nil {
		delete(m.timeline.selectedMessageIDs, messageID)
	}
}

func timelineSelectionKeyForRef(ref models.MessageRef) string {
	ref = ref.WithDefaults()
	if ref.SourceID != "" && ref.SourceID != models.DefaultMailSourceID {
		return ref.LocalID
	}
	if ref.AccountID != "" && ref.AccountID != models.DefaultAccountID {
		return ref.LocalID
	}
	if strings.TrimSpace(ref.LocalID) != "" && !strings.HasPrefix(ref.LocalID, "mail:default-mail:default:") {
		return ref.LocalID
	}
	return ref.MessageID
}

func timelineEmailMatchesMessageRef(email *models.EmailData, ref models.MessageRef, fallbackMessageID string) bool {
	if email == nil {
		return false
	}
	if key := timelineSelectionKeyForRef(ref); key != "" {
		if timelineEmailMatchesSelectionKey(email, key) {
			return true
		}
		if key != ref.WithDefaults().MessageID {
			return false
		}
	}
	return strings.TrimSpace(fallbackMessageID) != "" && email.MessageID == fallbackMessageID
}

func (m *Model) removeTimelineEmailByRef(ref models.MessageRef, fallbackMessageID string) {
	if strings.TrimSpace(fallbackMessageID) == "" && strings.TrimSpace(ref.MessageID) == "" && strings.TrimSpace(ref.LocalID) == "" {
		return
	}
	m.finishTimelineRangeSelection()
	remove := func(emails []*models.EmailData) []*models.EmailData {
		out := emails[:0]
		for _, email := range emails {
			if !timelineEmailMatchesMessageRef(email, ref, fallbackMessageID) {
				out = append(out, email)
			}
		}
		return out
	}
	m.timeline.emails = remove(m.timeline.emails)
	if m.timeline.emailsCache != nil {
		m.timeline.emailsCache = remove(m.timeline.emailsCache)
	}
	if m.timeline.searchResults != nil {
		m.timeline.searchResults = remove(m.timeline.searchResults)
	}
	if m.timeline.chatFilteredEmails != nil {
		m.timeline.chatFilteredEmails = remove(m.timeline.chatFilteredEmails)
	}
	if m.timeline.selectedMessageIDs != nil {
		if key := timelineSelectionKeyForRef(ref); key != "" {
			delete(m.timeline.selectedMessageIDs, key)
		}
		delete(m.timeline.selectedMessageIDs, fallbackMessageID)
	}
}

type cachedTimelineEmailDeleter interface {
	DeleteCachedEmail(models.MessageRef) error
}

func (m *Model) timelineBodyLoadRef(msg EmailBodyMsg) models.MessageRef {
	ref := msg.MessageRef
	if m.timeline.selectedEmail != nil {
		selectedRef := m.timeline.selectedEmail.MessageRef()
		if strings.TrimSpace(ref.MessageID) == "" {
			ref.MessageID = selectedRef.MessageID
		}
		if strings.TrimSpace(ref.Folder) == "" {
			ref.Folder = selectedRef.Folder
		}
		if ref.UID == 0 {
			ref.UID = selectedRef.UID
		}
		if strings.TrimSpace(ref.LocalID) == "" {
			ref.LocalID = selectedRef.LocalID
		}
		if ref.SourceID == "" {
			ref.SourceID = selectedRef.SourceID
		}
		if ref.AccountID == "" {
			ref.AccountID = selectedRef.AccountID
		}
		if ref.UIDValidity == 0 {
			ref.UIDValidity = selectedRef.UIDValidity
		}
	}
	if strings.TrimSpace(ref.MessageID) == "" {
		ref.MessageID = msg.MessageID
	}
	if strings.TrimSpace(ref.Folder) == "" {
		ref.Folder = msg.Folder
	}
	if ref.UID == 0 {
		ref.UID = msg.UID
	}
	return ref.WithDefaults()
}

func (m *Model) recoverFromStaleTimelineBodyLoad(msg EmailBodyMsg) tea.Cmd {
	ref := m.timelineBodyLoadRef(msg)
	messageID := strings.TrimSpace(msg.MessageID)
	if messageID == "" {
		messageID = ref.MessageID
	}
	if messageID == "" && m.timeline.selectedEmail != nil {
		messageID = m.timeline.selectedEmail.MessageID
	}
	if deleter, ok := m.backend.(cachedTimelineEmailDeleter); ok {
		if err := deleter.DeleteCachedEmail(ref); err != nil {
			logger.Warn("Failed to delete stale cached email %s: %v", ref.LocalID, err)
		}
	} else {
		logger.Warn("Backend does not expose stale cached email deletion for %s", ref.LocalID)
	}

	cursor := m.timelineTable.Cursor()
	m.timeline.bodyLoading = false
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyWrappedLines = nil
	m.timeline.inlineImageDescs = nil
	m.timeline.quickReplies = nil
	m.timeline.quickRepliesReady = false
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyIdx = 0
	m.clearTimelinePreviewDocumentCache()
	m.removeTimelineEmailByRef(ref, messageID)
	m.updateTimelineTable()
	if len(m.timeline.threadRowMap) == 0 {
		m.timeline.selectedEmail = nil
		m.statusMessage = "Removed stale cached email"
		return nil
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(m.timeline.threadRowMap) {
		cursor = len(m.timeline.threadRowMap) - 1
	}
	m.timelineTable.SetCursor(cursor)
	next := m.currentTimelinePreviewTarget()
	if next == nil {
		m.timeline.selectedEmail = nil
		m.statusMessage = "Removed stale cached email"
		return nil
	}
	m.statusMessage = "Removed stale cached email; loading next message"
	return m.openTimelineEmail(next)
}

func buildForwardSubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), "fwd:") {
		return subject
	}
	return "Fwd: " + subject
}

func buildForwardBody(email *models.EmailData, bodyText string) string {
	if email == nil {
		return bodyText
	}
	forwarded := fmt.Sprintf("\n\n--- Forwarded message ---\nFrom: %s\nDate: %s\nSubject: %s\n\n",
		email.Sender, email.Date.Format("Mon, 02 Jan 2006 15:04"), email.Subject)
	return forwarded + bodyText
}

func buildReplySubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

func newComposePreservedContext(kind models.PreservedMessageKind, email *models.EmailData, body *models.EmailBody, warning string) *composePreservedContext {
	if body == nil {
		body = &models.EmailBody{}
	}
	if body.MessageID == "" && email != nil {
		body.MessageID = email.MessageID
	}
	ctx := &composePreservedContext{
		kind:        kind,
		mode:        models.PreservationModeSafe,
		email:       email,
		body:        body,
		loadWarning: warning,
	}
	if kind == models.PreservedMessageKindForward {
		ctx.forwardedAttachments = make([]models.ForwardedAttachment, 0, len(body.Attachments))
		for _, att := range body.Attachments {
			ctx.forwardedAttachments = append(ctx.forwardedAttachments, models.ForwardedAttachment{
				Attachment: att,
				Include:    len(att.Data) > 0,
			})
		}
	}
	return ctx
}

func (m *Model) timelineBodyLoadedFor(email *models.EmailData) bool {
	return email != nil &&
		m.timeline.body != nil &&
		m.timeline.bodyMessageID == email.MessageID
}

func (m *Model) openTimelineForwardCompose(email *models.EmailData, body *models.EmailBody, composeStatus string) {
	m.rememberComposeReturn()
	m.activeTab = tabCompose
	m.composeTo.SetValue("")
	m.composeSubject.SetValue(buildForwardSubject(email.Subject))
	m.composeBody.SetValue("")
	m.setComposeSourceForEmail(email)
	m.applyConfiguredSignatureToComposeBody()
	m.composeStatus = composeStatus
	m.statusMessage = ""
	m.replyContextEmail = nil
	m.composeAIThread = false
	m.resetComposeAIBar()
	m.composePreserved = newComposePreservedContext(models.PreservedMessageKindForward, email, body, composeStatus)
	m.composeField = composeFieldTo
	m.composeTo.Focus()
	m.composeSubject.Blur()
	m.composeBody.Blur()
	m.resetFieldKeyMode()
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
}

func (m *Model) openTimelineReplyCompose(email *models.EmailData, body *models.EmailBody, composeStatus string, replyAll bool) {
	m.rememberComposeReturn()
	m.activeTab = tabCompose
	m.replyContextEmail = email
	m.composeAIThread = true
	to := email.Sender
	cc := ""
	if replyAll {
		to, cc = m.replyAllRecipientFields(email, body)
	}
	m.composeTo.SetValue(to)
	m.composeCC.SetValue(cc)
	m.composeBCC.SetValue("")
	m.composeSubject.SetValue(buildReplySubject(email.Subject))
	m.composeBody.SetValue("")
	m.setComposeSourceForEmail(email)
	m.applyConfiguredSignatureToComposeBody()
	m.composeStatus = composeStatus
	m.statusMessage = ""
	m.composePreserved = newComposePreservedContext(models.PreservedMessageKindReply, email, body, composeStatus)
	m.resetComposeAIBar()
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeSubject.Blur()
	m.composeBody.Focus()
	m.resetFieldKeyMode()
	if m.windowWidth > 0 {
		m.updateTableDimensions(m.windowWidth, m.windowHeight)
	}
}

func (m *Model) openTimelineDraftCompose(email *models.EmailData, body *models.EmailBody, composeStatus string) {
	if body == nil {
		body = &models.EmailBody{}
	}
	subject := strings.TrimSpace(body.Subject)
	if subject == "" && email != nil {
		subject = email.Subject
	}
	m.rememberComposeReturn()
	m.activeTab = tabCompose
	m.replyContextEmail = nil
	m.composeAIThread = false
	m.composePreserved = nil
	m.composePreview = false
	m.composeTo.SetValue(body.To)
	m.composeCC.SetValue(body.CC)
	m.composeBCC.SetValue(body.BCC)
	m.composeSubject.SetValue(subject)
	m.composeBody.SetValue(body.TextPlain)
	m.composeAttachments = nil
	m.setComposeSourceForEmail(email)
	m.resetComposeAIBar()
	m.composeStatus = composeStatus
	if m.composeStatus == "" {
		m.composeStatus = "Editing draft"
	}
	m.statusMessage = ""
	if email != nil {
		m.lastDraftUID = email.UID
		m.lastDraftFolder = email.Folder
		m.lastDraftSourceID = email.SourceID
		m.lastDraftReplaceable = draftFolderIsReplaceable(email.Folder)
	}
	m.draftSaving = false
	m.composeField = composeFieldBody
	m.composeTo.Blur()
	m.composeCC.Blur()
	m.composeBCC.Blur()
	m.composeSubject.Blur()
	m.composeBody.Focus()
	m.resetFieldKeyMode()
}

func (m *Model) startTimelineForward(email *models.EmailData) tea.Cmd {
	if email == nil {
		return nil
	}
	if m.timelineBodyLoadedFor(email) {
		m.openTimelineForwardCompose(email, m.timeline.body, "")
		return nil
	}
	m.timeline.forwardRequestID++
	requestID := m.timeline.forwardRequestID
	m.timeline.forwardPendingMessage = email.MessageID
	m.statusMessage = "Loading forwarded message body..."
	return m.loadTimelineForwardBodyCmd(email, requestID)
}

func (m *Model) loadTimelineForwardBodyCmd(email *models.EmailData, requestID int) tea.Cmd {
	emailCopy := *email
	b := m.backend
	return func() tea.Msg {
		if emailCopy.UID == 0 {
			return TimelineForwardBodyMsg{
				Email: &emailCopy,
				Body: &models.EmailBody{
					TextPlain: "(Body unavailable: this cached email has no server UID yet, so Herald cannot safely load its full contents. Re-sync the folder or use server search to refresh it.)",
				},
				MessageID: emailCopy.MessageID,
				RequestID: requestID,
			}
		}
		var body *models.EmailBody
		var err error
		if serviceBackend, ok := b.(messageBodyServiceBackend); ok {
			result, serviceErr := serviceBackend.GetMessage(context.Background(), emailCopy.MessageRef())
			body, err = result.Body, serviceErr
		} else {
			body, err = b.FetchEmailBody(emailCopy.Folder, emailCopy.UID)
		}
		return TimelineForwardBodyMsg{
			Email:     &emailCopy,
			Body:      body,
			Err:       err,
			MessageID: emailCopy.MessageID,
			RequestID: requestID,
		}
	}
}

func (m *Model) startTimelineDraft(email *models.EmailData) tea.Cmd {
	if email == nil || !email.IsDraft {
		return nil
	}
	if m.timelineBodyLoadedFor(email) {
		m.openTimelineDraftCompose(email, m.timeline.body, "")
		return nil
	}
	m.timeline.draftRequestID++
	requestID := m.timeline.draftRequestID
	m.timeline.draftPendingMessage = email.MessageID
	m.statusMessage = "Loading draft body..."
	return m.loadTimelineDraftBodyCmd(email, requestID)
}

func (m *Model) loadTimelineDraftBodyCmd(email *models.EmailData, requestID int) tea.Cmd {
	emailCopy := *email
	b := m.backend
	return func() tea.Msg {
		if emailCopy.UID == 0 {
			return TimelineDraftBodyMsg{
				Email: &emailCopy,
				Body: &models.EmailBody{
					Subject:   emailCopy.Subject,
					TextPlain: "(Draft body unavailable: this cached draft has no server UID yet, so Herald cannot safely edit it. Re-sync the folder to refresh it.)",
				},
				MessageID: emailCopy.MessageID,
				RequestID: requestID,
			}
		}
		var body *models.EmailBody
		var err error
		if serviceBackend, ok := b.(messageBodyServiceBackend); ok {
			result, serviceErr := serviceBackend.GetMessage(context.Background(), emailCopy.MessageRef())
			body, err = result.Body, serviceErr
		} else {
			body, err = b.FetchEmailBody(emailCopy.Folder, emailCopy.UID)
		}
		return TimelineDraftBodyMsg{
			Email:     &emailCopy,
			Body:      body,
			Err:       err,
			MessageID: emailCopy.MessageID,
			RequestID: requestID,
		}
	}
}

func (m *Model) buildSendDraftDesc(email *models.EmailData) string {
	if email == nil {
		return ""
	}
	subj := email.Subject
	if len(subj) > 50 {
		subj = subj[:47] + "..."
	}
	return fmt.Sprintf("Send draft \"%s\"?", subj)
}

func (m *Model) startTimelineSendDraft(email *models.EmailData) tea.Cmd {
	if email == nil || !email.IsDraft {
		return nil
	}
	m.statusMessage = "Sending draft..."
	return m.sendTimelineDraftCmd(email)
}

func (m *Model) sendTimelineDraftCmd(email *models.EmailData) tea.Cmd {
	emailCopy := *email
	b := m.backend
	return func() tea.Msg {
		if emailCopy.UID == 0 {
			return TimelineDraftSentMsg{
				Email:     &emailCopy,
				MessageID: emailCopy.MessageID,
				Err:       fmt.Errorf("cached draft has no server UID; re-sync the folder to refresh it"),
			}
		}
		var err error
		if scoped, ok := b.(backend.AccountComposeBackend); ok && emailCopy.SourceID != "" {
			err = scoped.SendDraftForAccount(emailCopy.SourceID, emailCopy.UID, emailCopy.Folder)
		} else {
			err = b.SendDraft(emailCopy.UID, emailCopy.Folder)
		}
		return TimelineDraftSentMsg{
			Email:     &emailCopy,
			MessageID: emailCopy.MessageID,
			Err:       err,
		}
	}
}

func (m *Model) replyAllRecipientFields(email *models.EmailData, body *models.EmailBody) (string, string) {
	if email == nil {
		return "", ""
	}
	own := m.ownAddressSet()
	seen := map[string]bool{}
	add := func(values []string, out *[]string) {
		for _, value := range values {
			for _, addr := range parseHeaderAddressValues(value) {
				key := strings.ToLower(addr.Address)
				if key == "" || own[key] || seen[key] {
					continue
				}
				seen[key] = true
				*out = append(*out, addr.String())
			}
		}
	}

	to := []string{}
	cc := []string{}
	add([]string{email.Sender}, &to)
	if body != nil {
		add([]string{body.To}, &to)
		add([]string{body.CC}, &cc)
	}
	if len(to) == 0 {
		to = append(to, email.Sender)
	}
	return strings.Join(to, ", "), strings.Join(cc, ", ")
}

func (m *Model) ownAddressSet() map[string]bool {
	values := []string{m.fromAddress}
	if m.cfg != nil {
		values = append(values, m.cfg.Credentials.Username, m.cfg.Gmail.Email)
	}
	own := map[string]bool{}
	for _, value := range values {
		for _, addr := range parseHeaderAddressValues(value) {
			own[strings.ToLower(addr.Address)] = true
		}
	}
	return own
}

func parseHeaderAddressValues(value string) []*mail.Address {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(value)
	if err == nil {
		return addrs
	}
	addr, err := mail.ParseAddress(value)
	if err == nil {
		return []*mail.Address{addr}
	}
	return []*mail.Address{{Address: value}}
}

func (m *Model) startTimelineReply(email *models.EmailData, replyAll bool) tea.Cmd {
	if email == nil {
		return nil
	}
	if m.timelineBodyLoadedFor(email) {
		m.openTimelineReplyCompose(email, m.timeline.body, "", replyAll)
		return nil
	}
	m.timeline.replyRequestID++
	requestID := m.timeline.replyRequestID
	m.timeline.replyPendingMessage = email.MessageID
	m.statusMessage = "Loading reply message body..."
	return m.loadTimelineReplyBodyCmd(email, requestID, replyAll)
}

func (m *Model) loadTimelineReplyBodyCmd(email *models.EmailData, requestID int, replyAll bool) tea.Cmd {
	emailCopy := *email
	b := m.backend
	return func() tea.Msg {
		if emailCopy.UID == 0 {
			return TimelineReplyBodyMsg{
				Email: &emailCopy,
				Body: &models.EmailBody{
					TextPlain: "(Body unavailable: this cached email has no server UID yet, so Herald cannot safely load its full contents. Re-sync the folder or use server search to refresh it.)",
				},
				MessageID: emailCopy.MessageID,
				RequestID: requestID,
				ReplyAll:  replyAll,
			}
		}
		var body *models.EmailBody
		var err error
		if serviceBackend, ok := b.(messageBodyServiceBackend); ok {
			result, serviceErr := serviceBackend.GetMessage(context.Background(), emailCopy.MessageRef())
			body, err = result.Body, serviceErr
		} else {
			body, err = b.FetchEmailBody(emailCopy.Folder, emailCopy.UID)
		}
		return TimelineReplyBodyMsg{
			Email:     &emailCopy,
			Body:      body,
			Err:       err,
			MessageID: emailCopy.MessageID,
			RequestID: requestID,
			ReplyAll:  replyAll,
		}
	}
}

func (m *Model) currentTimelineRowRef() (timelineRowRef, bool) {
	cursor := m.timelineTable.Cursor()
	if cursor < 0 || cursor >= len(m.timeline.threadRowMap) {
		return timelineRowRef{}, false
	}
	return m.timeline.threadRowMap[cursor], true
}

func (m *Model) openCurrentTimelineEmail() tea.Cmd {
	ref, ok := m.currentTimelineRowRef()
	if !ok {
		return nil
	}
	if ref.kind == rowKindThread {
		key := ref.group.normalizedSubject
		savedCursor := m.timelineTable.Cursor()
		m.timeline.expandedThreads[key] = !m.timeline.expandedThreads[key]
		m.updateTimelineTable()
		m.timelineTable.SetCursor(savedCursor)
		return nil
	}
	if ref.emailIdx == 0 && len(ref.group.emails) > 1 && m.timeline.expandedThreads[ref.group.normalizedSubject] {
		savedCursor := m.timelineTable.Cursor()
		m.timeline.expandedThreads[ref.group.normalizedSubject] = false
		m.updateTimelineTable()
		m.timelineTable.SetCursor(savedCursor)
		return nil
	}
	email := ref.group.emails[ref.emailIdx]
	return m.openTimelineEmail(email)
}

func (m *Model) activateCurrentTimelineRowFromMouse() tea.Cmd {
	ref, ok := m.currentTimelineRowRef()
	if !ok || ref.group == nil || len(ref.group.emails) == 0 {
		return nil
	}

	toggleThread := func(key string) {
		savedCursor := m.timelineTable.Cursor()
		m.timeline.expandedThreads[key] = !m.timeline.expandedThreads[key]
		m.updateTimelineTable()
		m.timelineTable.SetCursor(savedCursor)
	}
	isSelected := func(email *models.EmailData) bool {
		return email != nil &&
			m.timeline.selectedEmail != nil &&
			email.MessageID == m.timeline.selectedEmail.MessageID
	}

	if ref.kind == rowKindThread {
		email := ref.group.emails[0]
		if isSelected(email) {
			toggleThread(ref.group.normalizedSubject)
			return nil
		}
		return m.openTimelineEmail(email)
	}

	if ref.emailIdx < 0 || ref.emailIdx >= len(ref.group.emails) {
		return nil
	}
	email := ref.group.emails[ref.emailIdx]
	if ref.emailIdx == 0 && len(ref.group.emails) > 1 &&
		m.timeline.expandedThreads[ref.group.normalizedSubject] &&
		isSelected(email) {
		toggleThread(ref.group.normalizedSubject)
		return nil
	}
	return m.openTimelineEmail(email)
}

func (m *Model) openTimelineEmail(email *models.EmailData) tea.Cmd {
	if email == nil {
		return nil
	}
	clearCmd := m.timelineNativeImageClearCmd()
	m.revokeImagePreviews()
	m.timeline.selectedEmail = email
	m.timeline.body = nil
	m.timeline.bodyMessageID = ""
	m.timeline.bodyLoading = true
	m.timeline.previewLoad = previewLoadTelemetry{}
	m.timeline.inlineImageDescs = nil
	m.timeline.remoteImageLoads = nil
	m.timeline.remoteImageRevision++
	m.timeline.bodyScrollOffset = 0
	m.timeline.bodyWrappedLines = nil
	m.clearTimelinePreviewDocumentCache()
	m.timeline.quickReplies = nil
	m.timeline.quickRepliesReady = false
	m.timeline.quickReplyOpen = false
	m.timeline.quickReplyIdx = 0
	m.timeline.quickRepliesAIFetched = false
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	loadCmd := m.loadEmailBodyForRefCmd(email.MessageRef())
	if clearCmd != nil {
		return tea.Sequence(clearCmd, loadCmd)
	}
	return loadCmd
}

func (m *Model) openTimelineQuickReply() tea.Cmd {
	if m.timelineIsReadOnlyDiagnostic() {
		return nil
	}
	if m.timeline.selectedEmail == nil || m.timeline.body == nil {
		return nil
	}
	m.timeline.quickReplyPending = false
	m.timeline.quickReplyOpen = true
	m.setFocusedPanel(panelPreview)
	if m.timeline.quickReplyIdx >= len(m.timeline.quickReplies) {
		m.timeline.quickReplyIdx = 0
	}
	if m.classifier != nil && m.timeline.body.TextPlain != "" && m.timeline.selectedEmail != nil && !m.timeline.quickRepliesAIFetched {
		m.timeline.quickRepliesAIFetched = true
		email := m.timeline.selectedEmail
		bodyPreview := m.timeline.body.TextPlain
		if len([]rune(bodyPreview)) > 500 {
			bodyPreview = string([]rune(bodyPreview)[:500])
		}
		return generateQuickRepliesCmd(m.classifier, email.Sender, email.Subject, bodyPreview)
	}
	return nil
}

func (m *Model) toggleTimelineQuickReply() tea.Cmd {
	if m.timelineIsReadOnlyDiagnostic() {
		return nil
	}
	if m.timeline.selectedEmail == nil || m.timeline.body == nil {
		if email := m.currentTimelineRowEmail(); email != nil {
			m.timeline.quickReplyPending = true
			return m.openTimelineEmail(email)
		}
		return nil
	}
	m.timeline.quickReplyOpen = !m.timeline.quickReplyOpen
	if !m.timeline.quickReplyOpen {
		m.timeline.quickReplyPending = false
		return nil
	}
	return m.openTimelineQuickReply()
}

func (m *Model) timelineFilterPrefix() string {
	if m.activeTab != tabTimeline || !m.timeline.chatFilterMode {
		return ""
	}
	filterLabel := m.timeline.chatFilterLabel
	if filterLabel == "" {
		filterLabel = "filtered"
	}
	return lipgloss.NewStyle().
		Foreground(m.theme.Severity.Info.ForegroundColor()).
		Bold(true).
		Render(fmt.Sprintf("⬡ filter: %s (%d emails)  ", filterLabel, len(m.timeline.chatFilteredEmails)))
}

func (m *Model) appendTimelineStatusParts(parts []string) []string {
	if m.activeTab == tabTimeline {
		if !(m.windowWidth <= 80 && m.sidebarTooWide) {
			parts = append(parts, "Group: "+m.timeline.groupingMode.Label())
			parts = append(parts, "Sort: "+m.timeline.sortMode.shortLabel())
		}
		if m.timelineIsReadOnlyDiagnostic() {
			parts = append(parts, fmt.Sprintf("%d emails", len(m.timeline.emails)))
			parts = append(parts, "diagnostic read-only")
		} else if _, ok := m.folderStatus[m.currentFolder]; !ok {
			parts = append(parts, fmt.Sprintf("%d emails", len(m.timeline.emails)))
		}
		if m.timeline.rangeMode {
			parts = append(parts, "range select")
		}
		if selected := m.timelineSelectedCount(); selected > 0 {
			parts = append(parts, countLabel(selected, "message selected", "messages selected"))
		}
	}
	if m.timeline.searchMode {
		if m.timeline.searchFocus == timelineSearchFocusResults {
			parts = append(parts, fmt.Sprintf("Search results: %d", len(m.timeline.searchResults)))
		} else if m.timeline.searchResults != nil {
			parts = append(parts, fmt.Sprintf("Search: %d results", len(m.timeline.searchResults)))
		} else {
			parts = append(parts, "Search input")
		}
	}
	if m.activeTab == tabTimeline && m.timeline.body != nil && !m.timelineIsReadOnlyDiagnostic() {
		if !m.timeline.quickRepliesReady && m.classifier != nil {
			parts = append(parts, "⚡ generating replies…")
		} else if m.timeline.quickRepliesReady && !m.timeline.quickReplyOpen {
			parts = append(parts, "ctrl+q: quick reply")
		}
	}
	if m.timeline.mouseMode {
		parts = append([]string{"[mouse] select mode — m: restore TUI"}, parts...)
	}
	return parts
}

func (m *Model) timelineKeyHints(chrome ChromeState) (string, bool) {
	if m.activeTab != tabTimeline {
		return "", false
	}
	if m.timeline.quickReplyOpen {
		return joinHintSegments(m.movementHint("timeline", "navigate replies"), "enter: compose", "1-8: select", "esc: close picker", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit")), true
	}
	if m.timeline.searchMode {
		q := m.timeline.searchInput.View()
		if m.timeline.searchFocus == timelineSearchFocusResults {
			if m.timeline.selectedEmail != nil && chrome.FocusedPanel == panelPreview {
				if m.timelineIsReadOnlyDiagnostic() {
					return timelineReadOnlyPreviewHintText("tab: back to results", "esc: back to results"), true
				}
				if selectionHints := previewSelectionHintSegments(m.previewSelection, previewSelectionTimeline); len(selectionHints) > 0 {
					segments := append([]string{"tab: back to results", m.commandHint("timeline", CommandComposeNew, "compose")}, selectionHints...)
					segments = append(segments, m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
					return joinHintSegments(segments...), true
				}
				revealHint := ""
				if m.timelineRemoteRevealAvailable() {
					revealHint = m.commandHint("timeline", CommandPreviewRevealRemoteImages, "reveal images")
				}
				previewSegments := append(
					[]string{"tab: back to results", m.commandHint("timeline", CommandComposeNew, "compose")},
					append(m.timelineMessageActionHintSegments(), m.movementHint("timeline", "scroll"), revealHint)...,
				)
				if m.timelinePrintablePreviewLoaded() {
					if printHint := m.previewPrintHint("timeline"); printHint != "" {
						previewSegments = append(previewSegments, printHint)
					}
				}
				previewSegments = append(previewSegments, "z: full-screen", "v: cursor", "yy: copy line", "Y: copy all", problemReportShortcutHint, "m: mouse mode", "esc: back to results", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
				return joinHintSegments(previewSegments...), true
			}
			return joinHintSegments(append(
				[]string{fmt.Sprintf("%d results", len(m.timeline.threadRowMap)), m.commandHint("timeline", CommandComposeNew, "compose"), m.commandHint("timeline", CommandTimelineGroupCycle, "group")},
				append(m.timelineMessageActionHintSegments(),
					fmt.Sprintf("%s %s", displayShortcutKey(m.commandKey("timeline", CommandHelpSearch), keyDisplayHint), q), m.movementHint("timeline", "results"), "space: select", "enter: open", "esc: back to search")...,
			)...), true
		}
		if m.timeline.searchError != "" {
			return joinHintSegments(fmt.Sprintf("%s %s", displayShortcutKey(m.commandKey("timeline", CommandHelpSearch), keyDisplayHint), q), "Error: "+m.timeline.searchError, "esc: back"), true
		}
		query := m.timeline.searchInput.Value()
		if !m.timelineIsReadOnlyDiagnostic() && m.timeline.searchResults != nil && len(m.timeline.searchResults) == 0 && query != "" && !strings.HasPrefix(query, "/*") {
			return joinHintSegments(fmt.Sprintf("%s %s", displayShortcutKey(m.commandKey("timeline", CommandHelpSearch), keyDisplayHint), q), fmt.Sprintf("No results in this folder — try: /* %s", query), "ctrl+i: server search", "esc: back"), true
		}
		if m.timelineIsReadOnlyDiagnostic() {
			return joinHintSegments(fmt.Sprintf("%s %s", displayShortcutKey(m.commandKey("timeline", CommandHelpSearch), keyDisplayHint), q), "read-only local search", "enter: results", "esc: back"), true
		}
		return joinHintSegments(fmt.Sprintf("%s %s", displayShortcutKey(m.commandKey("timeline", CommandHelpSearch), keyDisplayHint), q), "current-folder hybrid search", "enter: results", "ctrl+i: server search", "esc: back"), true
	}
	if m.timeline.chatFilterMode {
		return joinHintSegments(append(
			[]string{m.primaryTabShortcutHint(), m.commandHint("timeline", CommandComposeNew, "compose"), m.commandHint("timeline", CommandTimelineGroupCycle, "group")},
			append(m.timelinePrimaryMessageActionHintSegments(),
				"esc: clear filter", m.movementHint("timeline", "navigate"), "space: select", "enter: open", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))...,
		)...), true
	}
	if m.timelineIsReadOnlyDiagnostic() && chrome.FocusedPanel == panelPreview {
		return timelineReadOnlyPreviewHintText("tab/shift+tab: panels", "esc: close"), true
	}
	if m.timelineIsReadOnlyDiagnostic() && m.timeline.selectedEmail != nil {
		return joinHintSegments(m.timelinePanelSwitchHint(), m.movementHint("timeline", "navigate"), "enter: open", "esc: close", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only"), true
	}
	if m.timelineIsReadOnlyDiagnostic() {
		return joinHintSegments(m.primaryTabShortcutHint(), m.commandHint("timeline", CommandTimelineGroupCycle, "group"), m.commandHint("timeline", CommandTimelineSortCycle, "sort"), m.movementHint("timeline", "navigate"), "enter: open", m.commandHint("timeline", CommandHelpSearch, "local search"), m.commandHint(keyboardScopeGlobal, CommandSidebarToggle, "sidebar"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"), "read-only"), true
	}
	if m.timeline.rangeMode && chrome.FocusedPanel == panelTimeline {
		segments := []string{m.rangeExtendHint("timeline"), "V/Esc: done"}
		if m.timeline.rangeShiftMode {
			segments = []string{"shift+↑/↓: extend range", "plain ↑/↓: done"}
		}
		segments = append(segments, m.timelinePrimaryMessageActionHintSegments()...)
		return joinHintSegments(segments...), true
	}
	if chrome.FocusedPanel == panelPreview {
		hasAttachments := m.timeline.body != nil && len(m.timeline.body.Attachments) > 0
		hasMultipleAttachments := m.timeline.body != nil && len(m.timeline.body.Attachments) > 1
		hasUnsub := m.timeline.selectedEmail != nil && m.timeline.bodyMessageID == m.timeline.selectedEmail.MessageID && previewHasUnsubscribe(m.timeline.body)
		if selectionHints := previewSelectionHintSegments(m.previewSelection, previewSelectionTimeline); len(selectionHints) > 0 {
			return joinHintSegments(selectionHints...), true
		}
		if m.usesDefaultKeyboardProfile() && (m.timeline.selectedEmail == nil || !m.timeline.selectedEmail.IsDraft) {
			return joinHintSegments("Esc: close", "A: archive", "Del: delete", "Ctrl+R: reply", "Y: copy"), true
		}
		segments := []string{m.timelinePanelSwitchHint(), m.commandHint("timeline", CommandComposeNew, "compose")}
		segments = append(segments, m.timelineMessageActionHintSegments()...)
		segments = append(segments, m.commandHint("timeline", CommandTimelineGroupCycle, "group"))
		if hasAttachments {
			if hasMultipleAttachments {
				segments = append(segments, "[ and ]: attachments")
			}
			segments = append(segments, "s: save attachment")
		}
		if calendarBodyHasInvitation(m.timeline.body) {
			segments = append(segments, "i: create event")
		}
		segments = append(segments, "U: unread", m.previewActionHintText("timeline", hasUnsub), m.leftFocusHint("timeline", "Timeline"), m.movementHint("timeline", "scroll"))
		if m.timelineRemoteRevealAvailable() {
			segments = append(segments, m.commandHint("timeline", CommandPreviewRevealRemoteImages, "reveal images"))
		}
		if m.timelinePrintAvailableFromTimeline() {
			if printHint := m.previewPrintHint("timeline"); printHint != "" {
				segments = append(segments, printHint)
			}
		}
		segments = append(segments, "z: full-screen", "drag: select", "v: cursor", "yy: copy line", "Y: copy all", problemReportShortcutHint, "m: mouse mode", "esc: close", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
		return joinHintSegments(segments...), true
	}
	if m.timeline.selectedEmail != nil {
		if m.usesDefaultKeyboardProfile() {
			return joinHintSegments("Enter: open", "Ctrl+N: new", "Ctrl+R: reply", "Del: delete", "/: search"), true
		}
		segments := append([]string{m.timelinePanelSwitchHint(), m.commandHint("timeline", CommandComposeNew, "compose")}, m.timelineMessageActionHintSegments()...)
		segments = append(segments, m.commandHint("timeline", CommandTimelineGroupCycle, "group"), "V: range", "U: unread")
		if m.timelinePrintAvailableFromTimeline() {
			if printHint := m.previewPrintHint("timeline"); printHint != "" {
				segments = append(segments, printHint)
			}
		}
		segments = append(segments, m.previewFocusHint("timeline"), "h/left/[: fold/folders", m.movementHint("timeline", "navigate"), "space: select", "shift+↑/↓: range", "enter: open", "esc: close", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
		return joinHintSegments(segments...), true
	}
	if m.timelineSelectedCount() > 0 {
		segments := []string{m.primaryTabShortcutHint(), m.commandHint("timeline", CommandComposeNew, "compose")}
		segments = append(segments, m.timelinePrimaryMessageActionHintSegments()...)
		segments = append(segments, m.commandHint("timeline", CommandTimelineGroupCycle, "group"), m.commandHint("timeline", CommandTimelineSortCycle, "sort"), "V: range", "space: select", m.commandHint(keyboardScopeGlobal, CommandAppSettings, "settings"), m.movementHint("timeline", "navigate"), "shift+↑/↓: range", m.timelineOpenPreviewHint(), m.foldersFocusHint("timeline"), "enter: open", m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
		return joinHintSegments(segments...), true
	}
	if m.usesDefaultKeyboardProfile() && m.currentTimelineRowEmail() != nil {
		return joinHintSegments("Enter: open", "Ctrl+N: new", "Ctrl+R: reply", "Del: delete", "/: search"), true
	}
	if m.usesDefaultKeyboardProfile() && chrome.FocusedPanel == panelTimeline {
		return joinHintSegments("Enter: open", "Ctrl+N: new", "Ctrl+R: reply", "Del: delete", "/: search"), true
	}
	segments := []string{m.primaryTabShortcutHint(), m.timelinePanelSwitchHint(), m.commandHint("timeline", CommandComposeNew, "compose")}
	if m.hasMultipleAccounts() {
		segments = append(segments, "A: accounts")
	}
	if m.currentTimelineRowEmail() != nil {
		if m.currentTimelineDraftEmail() != nil {
			segments = append(segments, m.commandHint(keyboardScopeGlobal, CommandAppSettings, "settings"))
			if m.windowWidth == 0 || m.windowWidth > 80 {
				segments = append(segments, "U: unread")
			}
			segments = append(segments, m.timelinePrimaryMessageActionHintSegments()...)
		} else if m.windowWidth > 0 && m.windowWidth <= 80 {
			segments = []string{
				m.primaryTabShortcutHint(),
				m.commandHint("timeline", CommandComposeNew, "compose"),
				m.commandHint("timeline", CommandMailReplyAll, "all"),
				m.commandHint(keyboardScopeGlobal, CommandAppSettings, "settings"),
				m.commandHint("timeline", CommandMailDeleteConfirm, "delete"),
				m.commandHint("timeline", CommandMailDeleteImmediate, "delete now"),
				m.timelinePanelSwitchHint(),
				m.commandHint("timeline", CommandMailReplySender, "sender"),
				m.commandHint("timeline", CommandMailForward, "forward"),
				m.commandHint("timeline", CommandMailArchiveCurrent, "archive"),
				m.commandHint("timeline", CommandMailReclassify, "re-classify"),
			}
		} else {
			segments = append(segments, m.commandHint(keyboardScopeGlobal, CommandAppSettings, "settings"), "U: unread")
			segments = append(segments, m.commandHint("timeline", CommandMailReplyAll, "all"), m.commandHint("timeline", CommandMailReplySender, "sender"), m.commandHint("timeline", CommandMailForward, "forward"), m.commandHint("timeline", CommandMailDeleteConfirm, "delete"), m.commandHint("timeline", CommandMailDeleteImmediate, "delete now"), m.commandHint("timeline", CommandMailArchiveCurrent, "archive"), m.commandHint("timeline", CommandMailReclassify, "re-classify"), "V: range", "*: star")
		}
	} else {
		segments = append(segments, m.commandHint(keyboardScopeGlobal, CommandAppSettings, "settings"))
	}
	segments = append(segments, m.foldersFocusHint("timeline"), m.timelineOpenPreviewHint(), m.commandHint("timeline", CommandTimelineGroupCycle, "group"), m.commandHint("timeline", CommandTimelineSortCycle, "sort"), m.movementHint("timeline", "navigate"), "ctrl+d/u: half-page", "space: select", "shift+↑/↓: range", "enter: open")
	if m.timelineSelectedCount() == 0 {
		segments = append(segments, m.commandHint("timeline", CommandHelpSearch, "hybrid search"))
	}
	segments = append(segments, m.commandHint(keyboardScopeGlobal, CommandSidebarToggle, "sidebar"), m.commandHint(keyboardScopeGlobal, CommandAppQuit, "quit"))
	return joinHintSegments(segments...), true
}

func (m *Model) timelinePanelSwitchHint() string {
	if m.usesDefaultKeyboardProfile() {
		if m.windowWidth > 0 && m.windowWidth <= 80 {
			return "F6: panels"
		}
		return "F6/Shift+F6: panels"
	}
	if m.windowWidth > 0 && m.windowWidth <= 80 {
		return "tab: panels"
	}
	return "tab/shift+tab: panels"
}

func joinHintSegments(segments ...string) string {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment = strings.TrimSpace(segment); segment != "" {
			parts = append(parts, segment)
		}
	}
	return strings.Join(parts, "  │  ")
}

func (m *Model) timelineMessageActionHintSegments() []string {
	segments := m.timelinePrimaryMessageActionHintSegments()
	if m.timelineSelectedCount() > 0 || m.currentTimelineFocusedDraftEmail() != nil {
		return segments
	}
	return append(segments, m.commandHint("timeline", CommandMailReclassify, "re-classify"))
}

func (m *Model) timelinePrimaryMessageActionHintSegments() []string {
	if m.usesDefaultKeyboardProfile() {
		if m.timelineSelectedCount() > 0 {
			segments := []string{"Del: delete selected", "Shift+Del: delete now"}
			if len(m.selectedTimelineArchiveEmails()) > 0 {
				segments = append(segments, "A: archive selected")
			}
			return segments
		}
		if m.currentTimelineDraftEmail() != nil {
			segments := []string{"E: edit draft", "Ctrl+S: send draft"}
			if m.currentTimelineFocusedDraftEmail() != nil {
				segments = append(segments, "Del: discard draft")
			} else {
				segments = append(segments, "Del: delete")
			}
			segments = append(segments, "Shift+Del: delete now")
			return segments
		}
		return []string{"Ctrl+R: reply", "Ctrl+Shift+R: reply all", "Ctrl+F: forward", "Del: delete", "Shift+Del: delete now", "A: archive"}
	}
	if m.timelineSelectedCount() > 0 {
		segments := []string{
			m.commandHint("timeline", CommandMailDeleteConfirm, "delete selected"),
			m.commandHint("timeline", CommandMailDeleteImmediate, "delete now"),
		}
		if len(m.selectedTimelineArchiveEmails()) > 0 {
			segments = append(segments, m.commandHint("timeline", CommandMailArchiveCurrent, "archive selected"))
		}
		return segments
	}
	if m.currentTimelineDraftEmail() != nil {
		segments := []string{"E: edit draft", "ctrl+s: send draft"}
		if m.currentTimelineFocusedDraftEmail() != nil {
			segments = append(segments, m.commandHint("timeline", CommandMailDeleteConfirm, "discard draft"))
		} else {
			segments = append(segments, m.commandHint("timeline", CommandMailDeleteConfirm, "delete"))
		}
		segments = append(segments, m.commandHint("timeline", CommandMailDeleteImmediate, "delete now"))
		return segments
	}
	return []string{"*: star", m.commandHint("timeline", CommandMailReplyAll, "all"), m.commandHint("timeline", CommandMailReplySender, "sender"), m.commandHint("timeline", CommandMailForward, "forward"), m.commandHint("timeline", CommandMailDeleteConfirm, "delete"), m.commandHint("timeline", CommandMailDeleteImmediate, "delete now"), m.commandHint("timeline", CommandMailArchiveCurrent, "archive")}
}

func timelineReadOnlyPreviewHintText(backHint, closeHint string) string {
	return joinHintSegments(backHint, "↑/k ↓/j: scroll", "read-only", "z: full-screen", "v: cursor", "yy: copy line", "Y: copy all", problemReportShortcutHint, "m: mouse mode", closeHint, "q: quit")
}

func (m *Model) handleTimelineMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case TimelineLoadedMsg:
		if !m.scopedResultMatchesActive(msg.SourceID) || (msg.Folder != "" && msg.Folder != m.currentFolder) {
			logger.Debug("TimelineLoadedMsg: ignoring stale source=%s active=%s folder=%s current=%s", msg.SourceID, m.activeSourceID, msg.Folder, m.currentFolder)
			return m, nil, true
		}
		m.finishTimelineRangeSelection()
		m.timeline.emails = msg.Emails
		m.timeline.virtualNotice = msg.Notice
		if msg.ReadOnly {
			m.loading = false
			unseen := 0
			for _, email := range msg.Emails {
				if email != nil && !email.IsRead {
					unseen++
				}
			}
			m.folderStatus[m.currentFolder] = models.FolderStatus{Unseen: unseen, Total: len(msg.Emails)}
		}
		m.reflowCurrentLayout()
		if m.timeline.selectedEmail != nil {
			targetID := m.timeline.selectedEmail.MessageID
			for rowIdx, ref := range m.timeline.threadRowMap {
				if ref.kind == rowKindEmail &&
					ref.group != nil &&
					ref.emailIdx < len(ref.group.emails) &&
					ref.group.emails[ref.emailIdx].MessageID == targetID {
					m.timelineTable.SetCursor(rowIdx)
					break
				}
			}
		}
		pendingDeepLinkCmd := m.consumePendingDeepLinkCmd()
		if !msg.ReadOnly {
			cmds := make([]tea.Cmd, 0, 4)
			if pendingDeepLinkCmd != nil {
				cmds = append(cmds, pendingDeepLinkCmd)
			}
			if cmd := m.startPreviewPrewarmerIfNeeded(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			if m.classifier != nil {
				if cmd := m.startEmbeddingBatchIfNeeded(); cmd != nil {
					cmds = append(cmds, cmd)
				}
				if cmd := m.startContactEnrichmentIfNeeded(); cmd != nil {
					cmds = append(cmds, cmd)
				}
				if !m.demoMode {
					if cmd := m.startClassificationIfNeeded(); cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
			}
			if len(cmds) > 0 {
				return m, tea.Batch(cmds...), true
			}
		}
		if pendingDeepLinkCmd != nil {
			return m, pendingDeepLinkCmd, true
		}
		return m, nil, true

	case EmailBodyMsg:
		if m.contactPreviewLoading {
			return m, nil, false
		}
		telemetry := previewTelemetryFromEmailBodyMsg(msg)
		if msg.MessageID != "" {
			if m.timeline.selectedEmail == nil || msg.MessageID != m.timeline.selectedEmail.MessageID {
				logPreviewLoad("timeline", telemetry, true)
				return m, nil, true
			}
		}
		m.timeline.previewLoad = telemetry
		logPreviewLoad("timeline", telemetry, false)
		m.timeline.bodyLoading = false
		m.timeline.selectedAttachment = 0
		m.timeline.quickReplies = nil
		m.timeline.quickRepliesReady = false
		m.timeline.quickReplyOpen = false
		m.timeline.quickReplyIdx = 0
		m.revokeImagePreviews()
		openInviteAfterLoad := m.calendarInvitation.OpenAfterLoad
		var cmds []tea.Cmd
		if msg.Err != nil {
			if backend.IsStaleMessageNotFoundError(msg.Err) {
				logger.Warn("Timeline body ref is stale; pruning cached email: %v", msg.Err)
				if openInviteAfterLoad {
					m.calendarInvitation = calendarInvitationPromptState{}
				}
				return m, m.recoverFromStaleTimelineBodyLoad(msg), true
			}
			logger.Warn("Failed to fetch email body: %v", msg.Err)
			m.statusMessage = "Body load failed: " + previewFailureHint(msg.Err.Error())
			if openInviteAfterLoad {
				m.calendarInvitation = calendarInvitationPromptState{}
			}
			m.timeline.body = &models.EmailBody{TextPlain: previewFailureBodyText(msg, m.timeline.selectedEmail)}
			if m.timeline.selectedEmail != nil {
				m.timeline.bodyMessageID = m.timeline.selectedEmail.MessageID
			}
		} else {
			m.timeline.body = msg.Body
			m.timeline.bodyMessageID = msg.MessageID
			if msg.Body != nil && msg.Body.TextPlain != "" && m.timeline.selectedEmail != nil {
				msgID := m.timeline.selectedEmail.MessageID
				bodyText := msg.Body.TextPlain
				go func() {
					if err := m.backend.CacheBodyText(msgID, bodyText); err != nil {
						logger.Warn("Failed to cache body text: %v", err)
					}
				}()
			}
			if m.timeline.selectedEmail != nil {
				email := m.timeline.selectedEmail
				m.timeline.quickReplies = buildCannedReplies(email.Sender)
				body := msg.Body
				if !email.IsRead && !m.timelineIsReadOnlyDiagnostic() {
					m.setTimelineEmailReadState(email, true)
					m.updateTimelineTable()
					cmds = append(cmds, markReadEmailCmd(m.backend, email))
				}
				if body != nil && (body.ListUnsubscribe != "" || body.ListUnsubscribePost != "") && !m.timelineIsReadOnlyDiagnostic() {
					cmds = append(cmds, cacheUnsubscribeHeadersCmd(m.backend, email.MessageID, body.ListUnsubscribe, body.ListUnsubscribePost))
				}
				if body != nil && len(body.InlineImages) > 0 && m.classifier != nil && m.classifier.HasVisionModel() && !iterm2.IsSupported() {
					cmds = append(cmds, describeImagesCmd(m.classifier, body.InlineImages)...)
				}
				m.timeline.quickRepliesReady = true
				if m.timeline.quickReplyPending {
					cmds = append(cmds, m.openTimelineQuickReply())
				}
				if openInviteAfterLoad {
					m.calendarInvitation.OpenAfterLoad = false
					if cmd := m.openCalendarInvitationPrompt(); cmd != nil {
						cmds = append(cmds, cmd)
					}
					openInviteAfterLoad = false
				}
				if len(cmds) > 0 && !m.previewPrintPending {
					m.timeline.bodyWrappedLines = nil
					m.clearTimelinePreviewDocumentCache()
					return m, tea.Batch(cmds...), true
				}
			} else {
				m.timeline.quickRepliesReady = true
				if m.timeline.quickReplyPending {
					cmd := m.openTimelineQuickReply()
					m.timeline.bodyWrappedLines = nil
					m.clearTimelinePreviewDocumentCache()
					return m, cmd, true
				}
			}
		}
		if openInviteAfterLoad && msg.Err == nil {
			m.calendarInvitation.OpenAfterLoad = false
			if cmd := m.openCalendarInvitationPrompt(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.previewPrintPending {
			m.previewPrintPending = false
			if msg.Err != nil || m.timeline.body == nil {
				m.statusMessage = "Print unavailable: preview failed to load"
			} else {
				model, cmd, _ := m.openPreviewPrintChooser(previewPrintSurfaceTimeline)
				if next, ok := model.(*Model); ok {
					m = next
				}
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
		m.timeline.bodyWrappedLines = nil
		m.clearTimelinePreviewDocumentCache()
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...), true
		}
		return m, nil, true

	case QuickRepliesMsg:
		if msg.Err != nil {
			logger.Warn("Quick reply generation failed: %v", msg.Err)
		} else if len(msg.Replies) > 0 {
			for _, reply := range msg.Replies {
				reply = strings.TrimSpace(reply)
				if reply == "" {
					continue
				}
				if !strings.HasPrefix(reply, "[AI] ") {
					reply = "[AI] " + reply
				}
				m.timeline.quickReplies = append(m.timeline.quickReplies, reply)
			}
		}
		m.timeline.quickRepliesReady = true
		return m, nil, true

	case ImageDescMsg:
		if msg.Err == nil && msg.Description != "" {
			if m.timeline.inlineImageDescs == nil {
				m.timeline.inlineImageDescs = make(map[string]string)
			}
			m.timeline.inlineImageDescs[msg.ContentID] = msg.Description
			m.clearTimelinePreviewDocumentCache()
		}
		return m, nil, true

	case RemoteImageRevealMsg:
		if m.timeline.selectedEmail == nil ||
			msg.MessageID == "" ||
			msg.MessageID != m.timeline.selectedEmail.MessageID ||
			msg.MessageID != m.timeline.bodyMessageID {
			return m, nil, true
		}
		if m.timeline.remoteImageLoads == nil {
			m.timeline.remoteImageLoads = make(map[string]previewRemoteImageState, len(msg.Results))
		}
		revealed := 0
		failed := 0
		for _, result := range msg.Results {
			key := result.Key
			if key == "" {
				key = remoteImageDocumentKey(result.URL)
			}
			state := m.timeline.remoteImageLoads[key]
			state.Loading = false
			if result.Err != nil {
				state.Err = previewFailureHint(result.Err.Error())
				state.Image = models.InlineImage{}
				failed++
			} else {
				state.Err = ""
				state.Image = result.Image
				if state.Image.ContentID == "" {
					state.Image.ContentID = key
				}
				revealed++
			}
			m.timeline.remoteImageLoads[key] = state
		}
		m.timeline.remoteImageRevision++
		m.timeline.bodyWrappedLines = nil
		m.clearTimelinePreviewDocumentCache()
		switch {
		case revealed > 0 && failed > 0:
			m.statusMessage = fmt.Sprintf("Revealed %d linked image(s); %d failed", revealed, failed)
		case revealed > 0:
			m.statusMessage = fmt.Sprintf("Revealed %d linked image(s)", revealed)
		case failed > 0:
			m.statusMessage = fmt.Sprintf("Linked image reveal failed (%d)", failed)
		}
		return m, nil, true

	case SearchResultMsg:
		if m.timeline.searchMode && msg.Token != 0 && msg.Token != m.timeline.searchToken {
			return m, nil, true
		}
		if msg.Err != nil {
			m.timeline.searchError = msg.Err.Error()
			m.timeline.searchAutoFocusResults = false
			return m, nil, true
		}
		m.timeline.searchError = ""
		if msg.Query == "" {
			m.timeline.searchResults = nil
			m.timeline.searchResultsQuery = ""
			m.timeline.semanticScores = nil
			if m.timeline.emailsCache != nil {
				m.timeline.emails = m.timeline.emailsCache
			}
			m.updateTimelineTable()
		} else {
			m.timeline.searchResults = msg.Emails
			m.timeline.searchResultsQuery = msg.Query
			m.timeline.semanticScores = msg.Scores
			m.updateTimelineTable()
			if len(m.timeline.threadRowMap) > 0 {
				m.timelineTable.SetCursor(0)
			}
		}
		if m.timeline.searchAutoFocusResults {
			if len(m.timeline.threadRowMap) > 0 {
				m.timeline.searchFocus = timelineSearchFocusResults
				m.timeline.searchInput.Blur()
				m.setFocusedPanel(panelTimeline)
			}
			m.timeline.searchAutoFocusResults = false
		}
		return m, nil, true

	case TimelineSearchDebounceMsg:
		if !m.timeline.searchMode || m.timeline.searchFocus != timelineSearchFocusInput {
			return m, nil, true
		}
		if msg.Token != m.timeline.searchToken {
			return m, nil, true
		}
		return m, m.performSearchWithToken(msg.Query, msg.Token), true

	case NewEmailsMsg:
		if msg.Folder == m.currentFolder {
			existing := make(map[string]struct{}, len(m.timeline.emails))
			for _, e := range m.timeline.emails {
				existing[e.MessageID] = struct{}{}
			}
			var fresh []*models.EmailData
			for _, e := range msg.Emails {
				if _, dup := existing[e.MessageID]; !dup {
					fresh = append(fresh, e)
				}
			}
			if len(fresh) > 0 {
				m.finishTimelineRangeSelection()
				m.timeline.emails = append(fresh, m.timeline.emails...)
				if m.timeline.emailsCache != nil {
					m.timeline.emailsCache = append(fresh, m.timeline.emailsCache...)
				}
				m.updateTimelineTable()
			}
		}
		var cmds []tea.Cmd
		cmds = append(cmds, m.listenForNewEmails())
		cmds = append(cmds, m.notifyNewMailCmd(msg))
		for _, email := range msg.Emails {
			if m.classifier != nil && m.classificationForEmail(email) == "" {
				cmds = append(cmds, m.autoClassifyEmailCmd(email))
			} else if cat := m.classificationForEmail(email); cat != "" {
				select {
				case m.ruleRequestCh <- models.NewMailAutomationEvent(email, cat):
				default:
				}
			}
			if ok, _ := m.backend.IsUnsubscribedSender(email.Sender); ok {
				m.statusMessage = fmt.Sprintf("⚠ Email from unsubscribed sender: %s", email.Sender)
			}
		}
		return m, tea.Batch(cmds...), true

	case EmailExpungedMsg:
		if msg.Folder == m.currentFolder {
			m.finishTimelineRangeSelection()
			filtered := m.timeline.emails[:0]
			for _, e := range m.timeline.emails {
				if e.MessageID != msg.MessageID {
					filtered = append(filtered, e)
				}
			}
			m.timeline.emails = filtered
			if m.timeline.emailsCache != nil {
				filtered2 := m.timeline.emailsCache[:0]
				for _, e := range m.timeline.emailsCache {
					if e.MessageID != msg.MessageID {
						filtered2 = append(filtered2, e)
					}
				}
				m.timeline.emailsCache = filtered2
			}
			m.updateTimelineTable()
		}
		return m, m.listenForExpunged(), true

	case TimelineForwardBodyMsg:
		if msg.RequestID != m.timeline.forwardRequestID || msg.MessageID != m.timeline.forwardPendingMessage {
			return m, nil, true
		}
		m.timeline.forwardPendingMessage = ""
		if m.activeTab != tabTimeline {
			return m, nil, true
		}
		email := msg.Email
		if email == nil {
			return m, nil, true
		}
		body := msg.Body
		composeStatus := ""
		if msg.Err != nil {
			logger.Warn("Failed to fetch forwarded email body: %v", msg.Err)
			composeStatus = "Forward body failed to load: " + msg.Err.Error()
			body = &models.EmailBody{TextPlain: "(" + composeStatus + ")"}
		}
		m.openTimelineForwardCompose(email, body, composeStatus)
		return m, nil, true

	case TimelineReplyBodyMsg:
		if msg.RequestID != m.timeline.replyRequestID || msg.MessageID != m.timeline.replyPendingMessage {
			return m, nil, true
		}
		m.timeline.replyPendingMessage = ""
		if m.activeTab != tabTimeline {
			return m, nil, true
		}
		email := msg.Email
		if email == nil {
			return m, nil, true
		}
		body := msg.Body
		composeStatus := ""
		if msg.Err != nil {
			logger.Warn("Failed to fetch reply email body: %v", msg.Err)
			composeStatus = "Reply body failed to load: " + msg.Err.Error()
			body = &models.EmailBody{TextPlain: "(" + composeStatus + ")"}
		}
		m.openTimelineReplyCompose(email, body, composeStatus, msg.ReplyAll)
		return m, nil, true

	case TimelineDraftBodyMsg:
		if msg.RequestID != m.timeline.draftRequestID || msg.MessageID != m.timeline.draftPendingMessage {
			return m, nil, true
		}
		m.timeline.draftPendingMessage = ""
		if m.activeTab != tabTimeline {
			return m, nil, true
		}
		email := msg.Email
		if email == nil {
			return m, nil, true
		}
		body := msg.Body
		composeStatus := ""
		if msg.Err != nil {
			logger.Warn("Failed to fetch draft email body: %v", msg.Err)
			composeStatus = "Draft body failed to load: " + msg.Err.Error()
			body = &models.EmailBody{Subject: email.Subject, TextPlain: "(" + composeStatus + ")"}
		}
		m.openTimelineDraftCompose(email, body, composeStatus)
		return m, nil, true

	case TimelineDraftSentMsg:
		if msg.Err != nil {
			m.statusMessage = "Send draft failed: " + msg.Err.Error()
			return m, nil, true
		}
		m.statusMessage = "Draft sent"
		m.removeTimelineEmail(msg.MessageID)
		if m.timeline.selectedEmail != nil && m.timeline.selectedEmail.MessageID == msg.MessageID {
			m.clearTimelinePreview()
		}
		m.updateTimelineTable()
		return m, nil, true

	case StarResultMsg:
		if msg.Err != nil {
			m.statusMessage = "Star failed: " + msg.Err.Error()
		} else {
			for _, e := range m.timeline.emails {
				if e.MessageID == msg.MessageID {
					e.IsStarred = msg.Starred
					break
				}
			}
			m.updateTimelineTable()
			if msg.Starred {
				m.statusMessage = "★ Starred"
			} else {
				m.statusMessage = "☆ Unstarred"
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) handleTimelineKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.activeTab != tabTimeline {
		return m, nil, false
	}
	key := shortcutKey(msg)
	if delta, ok := timelineShiftRangeDelta(msg, key); ok {
		if m.focusedPanel != panelTimeline {
			return m, nil, false
		}
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			return m, m.extendTimelineRangeSelection(delta, true), true
		}
		return m, nil, true
	}
	switch key {
	case " ", "space":
		if m.focusedPanel != panelTimeline {
			return m, nil, false
		}
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			m.finishTimelineRangeSelection()
			m.toggleTimelineSelection()
		}
		return m, nil, true
	case "V":
		if m.focusedPanel != panelTimeline {
			return m, nil, false
		}
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			if m.timeline.rangeMode {
				m.finishTimelineRangeSelection()
			} else {
				m.beginTimelineRangeSelection(false)
			}
		}
		return m, nil, true
	case "G":
		if m.timeline.quickReplyOpen {
			return m, nil, false
		}
		if m.canInteractWithVisibleData() {
			cmd := m.timelineNativeImageClearCmd()
			m.cycleTimelineGrouping()
			return m, cmd, true
		}
		return m, nil, true
	case "O":
		if m.timeline.quickReplyOpen {
			return m, nil, false
		}
		if m.canInteractWithVisibleData() {
			m.cycleTimelineSort()
		}
		return m, nil, true
	case "esc":
		if m.timeline.rangeMode {
			m.finishTimelineRangeSelection()
			return m, nil, true
		}
	case "*":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			if email := m.currentTimelineRowEmail(); email != nil {
				return m, m.toggleStarCmd(email), true
			}
		}
		return m, nil, true
	case "u":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.timeline.body != nil && m.timeline.selectedEmail != nil && m.timeline.body.ListUnsubscribe != "" {
			sender := m.timeline.selectedEmail.Sender
			body := m.timeline.body
			m.pendingUnsubscribe = true
			m.pendingUnsubscribeDesc = fmt.Sprintf("Unsubscribe from %s?", sender)
			m.pendingUnsubscribeAction = func() tea.Cmd { return unsubscribeCmd(body) }
		}
		return m, nil, true
	case "H":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.timeline.selectedEmail != nil {
			return m, createHideFutureMailCmd(m.backend, m.timeline.selectedEmail.Sender), true
		}
		return m, nil, true
	case "E":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			if email := m.currentTimelineDraftEmail(); email != nil {
				return m, m.startTimelineDraft(email), true
			}
		}
		return m, nil, false
	case "ctrl+s":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() && !m.pendingDeleteConfirm {
			if email := m.currentTimelineDraftEmail(); email != nil {
				if desc := m.buildSendDraftDesc(email); desc != "" {
					m.pendingDeleteConfirm = true
					m.pendingDeleteDesc = desc
					m.pendingArchive = false
					m.pendingDeleteAction = func() tea.Cmd {
						return m.startTimelineSendDraft(email)
					}
				}
			}
		}
		return m, nil, true
	case "F", "f", "ctrl+f":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			if email := m.currentTimelineRowEmail(); email != nil {
				return m, m.startTimelineForward(email), true
			}
		}
		return m, nil, true
	case "c", "ctrl+n":
		if m.canInteractWithVisibleData() {
			return m, m.openBlankComposeFromCurrent(), true
		}
		return m, nil, true
	case "right", "l":
		if m.previewSelection.activeOn(previewSelectionTimeline) {
			m.moveActivePreviewSelection(0, 1)
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			return m, m.previewCurrentTimelineRow(), true
		}
		return m, nil, true
	case "left", "h":
		if m.previewSelection.activeOn(previewSelectionTimeline) {
			m.moveActivePreviewSelection(0, -1)
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			return m, m.closeTimelinePreviewOrFocusFolders(), true
		}
		return m, nil, true
	case "/", "ctrl+k":
		if !m.loading && !m.timeline.searchMode {
			cmd := m.timelineNativeImageClearCmd()
			m.openTimelineSearch()
			return m, cmd, true
		}
		return m, nil, true
	case "enter":
		if m.canInteractWithVisibleData() {
			if m.timeline.searchMode && m.timeline.searchFocus == timelineSearchFocusResults {
				if m.focusedPanel == panelPreview {
					m.setFocusedPanel(panelTimeline)
					return m, nil, true
				}
				return m, m.openCurrentTimelineEmail(), true
			}
			if m.timeline.quickReplyOpen && len(m.timeline.quickReplies) > 0 {
				model, cmd := m.openQuickReply(m.timeline.quickReplies[m.timeline.quickReplyIdx])
				return model, cmd, true
			}
			if m.focusedPanel == panelSidebar {
				if cmd, handledAccount := m.selectSidebarFolder(); handledAccount {
					m.clearTimelineChatFilter()
					return m, cmd, true
				}
				m.clearTimelineChatFilter()
				return m, m.activateCurrentFolder(), true
			}
			if m.focusedPanel != panelSidebar {
				return m, m.openCurrentTimelineEmail(), true
			}
		}
		return m, nil, true
	case "tab", "shift+tab":
		if m.canInteractWithVisibleData() && m.timeline.searchMode && m.timeline.searchFocus == timelineSearchFocusResults &&
			m.timeline.selectedEmail != nil && m.focusedPanel == panelPreview {
			m.setFocusedPanel(panelTimeline)
			return m, nil, true
		}
	case "ctrl+q":
		return m, m.toggleTimelineQuickReply(), true
	case "ctrl+d", "ctrl+u":
		if m.canInteractWithVisibleData() {
			down := key == "ctrl+d"
			return m, m.timelineHalfPageScroll(down), true
		}
		return m, nil, true
	case "z":
		if !m.loading && m.timeline.selectedEmail != nil {
			m.timeline.fullScreen = !m.timeline.fullScreen
			m.timeline.bodyWrappedLines = nil
			m.clearTimelinePreviewDocumentCache()
		}
		return m, m.timelineIterm2NativeImageRepaintCmd(), true
	case remoteImageRevealCommandKey:
		if m.timelineRemoteRevealAvailable() {
			return m, m.revealTimelineRemoteImages(), true
		}
		return m, nil, true
	case "p":
		if m.timeline.fullScreen || m.focusedPanel == panelPreview || m.focusedPanel == panelTimeline || m.timeline.selectedEmail != nil {
			return m.openTimelinePrintChooserOrLoad()
		}
		return m, nil, false
	case "s":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if !m.loading && m.focusedPanel == panelPreview && m.timeline.body != nil &&
			len(m.timeline.body.Attachments) > 0 && !m.timeline.attachmentSavePrompt {
			att := m.timeline.body.Attachments[m.timeline.selectedAttachment]
			defaultPath := expandTilde("~/Downloads/" + att.Filename)
			savePath, warning, _ := attachmentSaveCollision(defaultPath)
			m.timeline.attachmentSaveInput.SetValue(savePath)
			m.timeline.attachmentSaveWarning = warning
			m.timeline.attachmentSaveInput.Focus()
			m.timeline.attachmentSavePrompt = true
		}
		return m, nil, true
	case "i":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		inviteShortcutPanel := m.focusedPanel == panelPreview ||
			m.timeline.fullScreen ||
			(m.focusedPanel == panelTimeline && m.timelineBodyLoadedFor(m.timeline.selectedEmail))
		if !m.loading && inviteShortcutPanel && m.timeline.body != nil {
			return m, m.openCalendarInvitationPrompt(), true
		}
		return m, nil, true
	case "U":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			return m, m.markCurrentTimelineUnread(), true
		}
		return m, nil, true
	case "]":
		if !m.loading && (m.focusedPanel == panelPreview || m.timeline.fullScreen) &&
			m.timeline.body != nil && m.timeline.selectedAttachment < len(m.timeline.body.Attachments)-1 {
			m.timeline.selectedAttachment++
			return m, nil, true
		}
		if (m.focusedPanel == panelPreview || m.timeline.fullScreen) &&
			m.timeline.body != nil && len(m.timeline.body.Attachments) > 1 {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			return m, m.previewCurrentTimelineRow(), true
		}
		return m, nil, true
	case "[":
		if !m.loading && (m.focusedPanel == panelPreview || m.timeline.fullScreen) &&
			m.timeline.body != nil && m.timeline.selectedAttachment > 0 {
			m.timeline.selectedAttachment--
			return m, nil, true
		}
		if (m.focusedPanel == panelPreview || m.timeline.fullScreen) &&
			m.timeline.body != nil && len(m.timeline.body.Attachments) > 1 {
			return m, nil, true
		}
		if m.canInteractWithVisibleData() {
			return m, m.closeTimelinePreviewOrFocusFolders(), true
		}
		return m, nil, true
	case "r", "R", "ctrl+r", "ctrl+R", "ctrl+shift+r", "ctrl+shift+R":
		if m.timelineIsReadOnlyDiagnostic() {
			return m, nil, true
		}
		if !m.loading {
			if email := m.currentTimelineRowEmail(); email != nil {
				replyAll := key == "r" || key == "ctrl+R" || key == "ctrl+shift+r" || key == "ctrl+shift+R"
				return m, m.startTimelineReply(email, replyAll), true
			}
		}
		return m, nil, true
	case "up", "k":
		if m.canInteractWithVisibleData() {
			if m.timeline.rangeMode && m.focusedPanel == panelTimeline {
				if !m.timeline.rangeShiftMode {
					return m, m.extendTimelineRangeSelection(-1, false), true
				}
				m.finishTimelineRangeSelection()
			}
			if m.timeline.quickReplyOpen {
				if m.timeline.quickReplyIdx > 0 {
					m.timeline.quickReplyIdx--
				}
				return m, nil, true
			}
			if m.timeline.fullScreen {
				if m.previewSelection.activeOn(previewSelectionTimeline) {
					m.moveActivePreviewSelection(-1, 0)
				} else if m.timeline.bodyScrollOffset > 0 {
					m.timeline.bodyScrollOffset--
				}
				return m, m.timelineIterm2NativeImageRepaintCmd(), true
			}
			if m.focusedPanel == panelPreview {
				if m.previewSelection.activeOn(previewSelectionTimeline) {
					m.moveActivePreviewSelection(-1, 0)
				} else if m.timeline.bodyScrollOffset > 0 {
					m.timeline.bodyScrollOffset--
				}
				return m, m.timelineIterm2NativeImageRepaintCmd(), true
			}
			if m.focusedPanel == panelSidebar {
				model, cmd := m.handleNavigation(-1)
				return model, cmd, true
			}
			m.timelineTable.MoveUp(1)
			return m, m.maybeUpdatePreview(), true
		}
		return m, nil, true
	case "down", "j":
		if m.canInteractWithVisibleData() {
			if m.timeline.rangeMode && m.focusedPanel == panelTimeline {
				if !m.timeline.rangeShiftMode {
					return m, m.extendTimelineRangeSelection(1, false), true
				}
				m.finishTimelineRangeSelection()
			}
			if m.timeline.quickReplyOpen {
				if m.timeline.quickReplyIdx < len(m.timeline.quickReplies)-1 {
					m.timeline.quickReplyIdx++
				}
				return m, nil, true
			}
			if m.timeline.fullScreen {
				if m.previewSelection.activeOn(previewSelectionTimeline) {
					m.moveActivePreviewSelection(1, 0)
				} else {
					m.timeline.bodyScrollOffset++
				}
				return m, m.timelineIterm2NativeImageRepaintCmd(), true
			}
			if m.focusedPanel == panelPreview {
				if m.previewSelection.activeOn(previewSelectionTimeline) {
					m.moveActivePreviewSelection(1, 0)
				} else {
					m.timeline.bodyScrollOffset++
				}
				return m, m.timelineIterm2NativeImageRepaintCmd(), true
			}
			if m.focusedPanel == panelSidebar {
				model, cmd := m.handleNavigation(1)
				return model, cmd, true
			}
			m.timelineTable.MoveDown(1)
			return m, m.maybeUpdatePreview(), true
		}
		return m, nil, true
	case "v":
		if m.timeline.fullScreen || m.focusedPanel == panelPreview {
			m.togglePreviewSelectionForSurface(previewSelectionTimeline)
		}
		return m, nil, true
	case "m":
		return m, m.toggleMouseCaptureMode(), true
	case "y":
		if cmd, handled := m.handlePreviewCopyKey(previewSelectionTimeline, key); handled {
			return m, cmd, true
		}
		return m, nil, true
	case "Y":
		if cmd, handled := m.handlePreviewCopyKey(previewSelectionTimeline, key); handled {
			return m, cmd, true
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) timelineHalfPageScroll(down bool) tea.Cmd {
	step := m.timelineTable.Height() / 2
	if m.focusedPanel == panelPreview || m.timeline.fullScreen {
		step = m.windowHeight / 2
	}
	if step < 1 {
		step = 1
	}
	if !down {
		step = -step
	}
	if m.timeline.fullScreen || m.focusedPanel == panelPreview {
		m.timeline.bodyScrollOffset += step
		if m.timeline.bodyScrollOffset < 0 {
			m.timeline.bodyScrollOffset = 0
		}
		if m.timeline.fullScreen || m.focusedPanel == panelPreview {
			return m.timelineIterm2NativeImageRepaintCmd()
		}
		return nil
	}
	if m.focusedPanel == panelSidebar {
		if step > 0 {
			max := len(m.visibleSidebarItems()) - 1
			if max < 0 {
				max = 0
			}
			for i := 0; i < step; i++ {
				m.sidebarCursor++
				if m.sidebarCursor > max {
					m.sidebarCursor = max
					break
				}
			}
		} else {
			m.sidebarCursor += step
			if m.sidebarCursor < 0 {
				m.sidebarCursor = 0
			}
		}
		m.normalizeSidebarCursor(step)
		return nil
	}
	if step > 0 {
		m.timelineTable.MoveDown(step)
	} else {
		m.timelineTable.MoveUp(-step)
	}
	return m.maybeUpdatePreview()
}

func cloneTimelineExpandedThreads(src map[string]bool) map[string]bool {
	if len(src) == 0 {
		return make(map[string]bool)
	}
	dst := make(map[string]bool, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneInlineImageDescs(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneRemoteImageStates(src map[string]previewRemoteImageState) map[string]previewRemoteImageState {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]previewRemoteImageState, len(src))
	for key, value := range src {
		if len(value.Image.Data) > 0 {
			value.Image.Data = append([]byte(nil), value.Image.Data...)
		}
		dst[key] = value
	}
	return dst
}

func (m *Model) handleNavigation(direction int) (tea.Model, tea.Cmd) {
	if m.focusedPanel == panelSidebar {
		max := len(m.visibleSidebarItems()) - 1
		if max < 0 {
			max = 0
		}
		if direction > 0 {
			if m.sidebarCursor < max {
				m.sidebarCursor++
			}
		} else {
			if m.sidebarCursor > 0 {
				m.sidebarCursor--
			}
		}
		m.normalizeSidebarCursor(direction)
		return m, nil
	}
	return m, nil
}
