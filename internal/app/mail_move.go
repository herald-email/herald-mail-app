package app

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type mailMoveTarget struct {
	ref models.MessageRef
	key string
}

type mailMovePicker struct {
	targets     []mailMoveTarget
	folders     []string
	input       textinput.Model
	cursor      int
	moving      bool
	destination string
}

type mailMoveResultMsg struct {
	moved       []string
	destination string
	failed      int
	firstErr    error
}

type moveEmailByRefBackend interface {
	MoveEmailByRef(models.MessageRef, string) error
}

func (m *Model) openMailMovePicker() tea.Cmd {
	if m.loading || m.deleting || m.mailMove != nil || m.timelineIsReadOnlyDiagnostic() || m.backend == nil {
		return nil
	}
	if m.focusedPanel != panelTimeline && m.focusedPanel != panelPreview {
		return nil
	}
	m.finishTimelineRangeSelection()
	emails := m.selectedTimelineEmails(true)
	if len(emails) == 0 {
		if m.focusedPanel == panelPreview && m.timeline.selectedEmail != nil {
			emails = []*models.EmailData{m.timeline.selectedEmail}
		} else {
			emails = m.currentTimelineRowEmails()
		}
	}
	var targets []mailMoveTarget
	seen := make(map[string]bool)
	var source models.SourceID
	for _, email := range emails {
		if email == nil || email.MessageID == "" {
			continue
		}
		if email.IsDraft {
			m.statusMessage = "Drafts cannot be moved; select received messages"
			return nil
		}
		ref := email.MessageRef()
		if email.SourceID == "" && m.activeSourceID != "" {
			ref.SourceID = m.activeSourceID
		}
		if len(targets) == 0 {
			source = ref.SourceID
		} else if ref.SourceID != source {
			m.statusMessage = "Select messages from one account to move them"
			return nil
		}
		if ref.Folder == "" {
			ref.Folder = m.currentFolder
		}
		key := timelineSelectionKey(email)
		if !seen[key] {
			targets = append(targets, mailMoveTarget{ref: ref, key: key})
			seen[key] = true
		}
	}
	if len(targets) == 0 {
		return nil
	}
	folders := m.folders
	if m.hasMultipleAccounts() {
		folders = nil
		for _, snapshot := range m.accountFolderSnapshots {
			if snapshot.Account.SourceID == source {
				folders = snapshot.Folders
				break
			}
		}
	}
	destinations := make([]string, 0, len(folders))
	seenFolders := make(map[string]bool)
	for _, folder := range folders {
		if strings.TrimSpace(folder) == "" || isVirtualAllMailOnlyFolder(folder) || seenFolders[folder] {
			continue
		}
		allAlreadyThere := true
		for _, target := range targets {
			if target.ref.Folder != folder {
				allAlreadyThere = false
				break
			}
		}
		if allAlreadyThere {
			continue
		}
		seenFolders[folder] = true
		destinations = append(destinations, folder)
	}
	if len(destinations) == 0 {
		m.statusMessage = "No destination folders available; refresh this account's folders"
		return nil
	}
	sort.Strings(destinations)
	input := textinput.New()
	input.Placeholder = "Filter folders"
	input.CharLimit = 200
	m.mailMove = &mailMovePicker{targets: targets, folders: destinations, input: input}
	return tea.Batch(m.timelineNativeImageClearCmd(), m.mailMove.input.Focus())
}

func (p *mailMovePicker) matches() []string {
	var out []string
	query := strings.ToLower(p.input.Value())
	for _, folder := range p.folders {
		if strings.Contains(strings.ToLower(folder), query) {
			out = append(out, folder)
		}
	}
	return out
}

// The picker is a text-entry modal: letters always filter folder names.
func (m *Model) handleMailMoveKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.mailMove
	if p.moving {
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.mailMove = nil
		return m, nil
	case "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return m, nil
	case "down":
		if p.cursor+1 < len(p.matches()) {
			p.cursor++
		}
		return m, nil
	case "enter":
		matches := p.matches()
		if len(matches) == 0 {
			return m, nil
		}
		p.destination = matches[p.cursor]
		p.moving = true
		p.input.Blur()
		targets := append([]mailMoveTarget(nil), p.targets...)
		destination, b := p.destination, m.backend
		return m, func() tea.Msg {
			result := mailMoveResultMsg{destination: destination}
			for _, target := range targets {
				var err error
				if target.ref.Folder != destination {
					if scoped, ok := b.(moveEmailByRefBackend); ok {
						err = scoped.MoveEmailByRef(target.ref, destination)
					} else {
						err = b.MoveEmail(target.ref.MessageID, target.ref.Folder, destination)
					}
				}
				if err != nil {
					result.failed++
					if result.firstErr == nil {
						result.firstErr = err
					}
					continue
				}
				result.moved = append(result.moved, target.key)
			}
			return result
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.cursor = 0
	return m, cmd
}

func (m *Model) finishMailMove(msg mailMoveResultMsg) tea.Cmd {
	m.mailMove = nil
	moved := make(map[string]bool)
	for _, key := range msg.moved {
		delete(m.timeline.selectedMessageIDs, key)
		moved[key] = true
	}
	// Cached search slices can share backing arrays. Allocate each filtered
	// slice rather than pruning in place and duplicating unrelated rows.
	prune := func(emails []*models.EmailData) []*models.EmailData {
		if emails == nil {
			return nil
		}
		out := make([]*models.EmailData, 0, len(emails))
		for _, email := range emails {
			if !moved[timelineSelectionKey(email)] {
				out = append(out, email)
			}
		}
		return out
	}
	m.timeline.searchToken++
	m.timeline.emails = prune(m.timeline.emails)
	m.timeline.emailsCache = prune(m.timeline.emailsCache)
	m.timeline.searchResults = prune(m.timeline.searchResults)
	m.timeline.chatFilteredEmails = prune(m.timeline.chatFilteredEmails)
	m.clearTimelinePreviewIfDeleted(moved)
	m.statusMessage = fmt.Sprintf("Moved %s to %s", countLabel(len(msg.moved), "email", "emails"), msg.destination)
	if msg.failed > 0 {
		m.statusMessage += fmt.Sprintf("; %d failed: %v", msg.failed, msg.firstErr)
	}
	m.updateTimelineTable()
	return tea.Batch(m.loadTimelineEmails(), m.loadFolderStatusCmd(m.folders, 0))
}

func (m *Model) renderMailMovePicker() string {
	p := m.mailMove
	width := min(68, m.windowWidth-6)
	contentW := max(20, width-4)
	p.input.SetWidth(contentW - 2)
	lines := []string{m.theme.Text.Primary.Style().Bold(true).Render("Move " + countLabel(len(p.targets), "email", "emails")), ""}
	if p.moving {
		lines = append(lines, safeChromeLine("Moving to "+p.destination+"…", contentW))
	} else {
		lines = append(lines, p.input.View(), "")
		matches := p.matches()
		rows := max(1, min(8, m.windowHeight-14))
		start := max(0, p.cursor-rows+1)
		for i := start; i < min(len(matches), start+rows); i++ {
			prefix := "  "
			if i == p.cursor {
				prefix = "> "
			}
			line := safeChromeLine(prefix+matches[i], contentW)
			if i == p.cursor {
				line = m.theme.Focus.SelectionActive.Style().Render(line)
			}
			lines = append(lines, line)
		}
		if len(matches) == 0 {
			lines = append(lines, "No matching folders")
		}
		lines = append(lines, "", "↑/↓ choose • Enter move • Esc cancel")
	}
	panel := m.baseStyle.Width(width).BorderForeground(m.theme.PanelBorderColor(true)).Render(strings.Join(lines, "\n"))
	return m.renderCompactOverlayView(panel)
}
