package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/memory"
)

const composeMemoryRadarDebounce = 350 * time.Millisecond

const (
	composeMemoryActionSource          = "source"
	composeMemoryActionInsert          = "insert"
	composeMemoryActionDismiss         = "dismiss"
	composeMemoryActionResolve         = "resolve"
	composeMemoryActionSave            = "save"
	composeMemoryActionResearchPerson  = "research_person"
	composeMemoryActionResearchCompany = "research_company"

	composeMemoryInsertPhraseLimit = 220
	composeMemoryStatusLimit       = 180
)

type composeMemoryBackend interface {
	BuildReplyMemoryContext(context.Context, memory.ReplyPrepQuery) (memory.ReplyPrep, error)
}

func (m *Model) resetComposeMemoryRadar() {
	m.composeMemoryToken++
	m.composeMemoryDebounceToken++
	m.composeMemoryLoading = false
	m.composeMemoryPrep = memory.ReplyPrep{}
	m.composeMemoryError = ""
}

func (m *Model) startComposeMemoryRadar() tea.Cmd {
	source, ok := m.backend.(composeMemoryBackend)
	if !ok || source == nil || !m.composeMemoryRadarEligible() {
		m.resetComposeMemoryRadar()
		return nil
	}
	query := memory.ReplyPrepQuery{
		Recipient:    strings.TrimSpace(m.composeTo.Value()),
		Subject:      strings.TrimSpace(m.composeSubject.Value()),
		DraftExcerpt: chatAgentBoundedText(m.composeBody.Value(), 1000),
		Limit:        8,
	}
	if m.replyContextEmail != nil {
		query.MessageID = m.replyContextEmail.MessageID
	}
	if query.Recipient == "" && query.Subject == "" && query.MessageID == "" {
		m.resetComposeMemoryRadar()
		return nil
	}
	m.composeMemoryToken++
	token := m.composeMemoryToken
	m.composeMemoryLoading = true
	m.composeMemoryPrep = memory.ReplyPrep{}
	m.composeMemoryError = ""
	m.refreshComposeLayout()
	return func() tea.Msg {
		prep, err := source.BuildReplyMemoryContext(context.Background(), query)
		return ComposeMemoryRadarMsg{Token: token, Prep: prep, Err: err}
	}
}

func (m *Model) composeMemoryRadarEligible() bool {
	return m.activeTab == tabCompose && m.replyContextEmail != nil
}

func (m *Model) composeMemoryRadarSignature() string {
	messageID := ""
	if m.replyContextEmail != nil {
		messageID = m.replyContextEmail.MessageID
	}
	return strings.Join([]string{
		strings.TrimSpace(m.composeTo.Value()),
		strings.TrimSpace(m.composeSubject.Value()),
		chatAgentBoundedText(m.composeBody.Value(), 1000),
		messageID,
	}, "\x1f")
}

func (m *Model) scheduleComposeMemoryRadarRefresh() tea.Cmd {
	if !m.composeMemoryRadarEligible() {
		return nil
	}
	m.composeMemoryDebounceToken++
	token := m.composeMemoryDebounceToken
	signature := m.composeMemoryRadarSignature()
	return tea.Tick(composeMemoryRadarDebounce, func(_ time.Time) tea.Msg {
		return ComposeMemoryRadarDebounceMsg{Token: token, Signature: signature}
	})
}

func (m *Model) composeMemoryRadarContextChanged(before string) bool {
	return m.composeMemoryRadarEligible() && before != m.composeMemoryRadarSignature()
}

func (m *Model) composeMemoryRadarRows(tableHeight int) int {
	if m.composeMemoryLoading || m.composeMemoryError != "" {
		return 1
	}
	if len(m.composeMemoryPrep.Nudges) == 0 {
		return 0
	}
	if tableHeight <= 24 {
		return 1
	}
	rows := len(m.composeMemoryPrep.Nudges) + 1
	if rows > 4 {
		return 4
	}
	return rows
}

func (m *Model) renderComposeMemoryRadar(width int) string {
	if width <= 0 {
		return ""
	}
	dimStyle := m.theme.Compose.AIDim.Style()
	accentStyle := m.theme.Compose.Accent.Style()
	warnStyle := m.theme.Compose.StatusWarning.Style()
	if m.composeMemoryLoading {
		return dimStyle.Render(truncateVisual("Radar: checking memories...", width))
	}
	if m.composeMemoryError != "" {
		return warnStyle.Render(truncateVisual("Radar unavailable: "+m.composeMemoryError, width))
	}
	nudges := m.composeMemoryPrep.Nudges
	if len(nudges) == 0 {
		return ""
	}
	if m.windowHeight > 0 && m.windowHeight <= 24 {
		return accentStyle.Render(truncateVisual("Radar: "+nudgeLine(nudges[0])+" | alt+o/i/d", width))
	}
	header := fmt.Sprintf("Radar: %d memory nudge(s) | alt+o source alt+i insert alt+d dismiss alt+r/s resolve/save alt+p/c research", len(nudges))
	lines := []string{accentStyle.Render(truncateVisual(header, width))}
	for i, nudge := range nudges {
		if i >= 3 {
			break
		}
		lines = append(lines, dimStyle.Render(truncateVisual("  - "+nudgeLine(nudge), width)))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) performComposeMemoryRadarAction(action string) tea.Cmd {
	if len(m.composeMemoryPrep.Nudges) == 0 {
		m.composeStatus = "Radar has no memory nudge to act on"
		m.refreshComposeLayout()
		return nil
	}
	nudge := &m.composeMemoryPrep.Nudges[0]
	switch action {
	case composeMemoryActionSource:
		source := composeMemorySourceInspectionLabel(*nudge)
		if source == "" {
			m.composeStatus = "Radar source unavailable for this nudge"
		} else {
			m.composeStatus = "Radar source: " + chatAgentBoundedText(source, composeMemoryStatusLimit)
		}
	case composeMemoryActionInsert:
		before := m.composeMemoryRadarSignature()
		phrase := composeMemoryInsertPhrase(*nudge)
		if phrase == "" {
			m.composeStatus = "Radar has no phrase to insert"
			break
		}
		m.composeBody.SetValue(composeMemoryAppendParagraph(m.composeBody.Value(), phrase))
		m.composeBody.CursorEnd()
		nudge.ActionState = memory.NudgeActionInserted
		m.composeStatus = "Radar phrase inserted"
		m.refreshComposeLayout()
		if m.composeMemoryRadarContextChanged(before) {
			return m.scheduleComposeMemoryRadarRefresh()
		}
	case composeMemoryActionDismiss:
		nudge.ActionState = memory.NudgeActionDismissed
		m.composeMemoryPrep.Nudges = append([]memory.Nudge{}, m.composeMemoryPrep.Nudges[1:]...)
		m.composeStatus = "Radar nudge dismissed for this draft"
	case composeMemoryActionResolve:
		nudge.ActionState = memory.NudgeActionResolved
		m.composeStatus = "Radar nudge marked resolved locally"
	case composeMemoryActionSave:
		nudge.ActionState = memory.NudgeActionSaved
		m.composeStatus = "Radar nudge saved for review"
	case composeMemoryActionResearchPerson:
		target := composeMemoryResearchTarget("person", m.composeTo.Value(), *nudge)
		m.composeStatus = "Radar research intent recorded for person: " + target + " (Research Mode is opt-in)"
	case composeMemoryActionResearchCompany:
		target := composeMemoryResearchTarget("company", m.composeTo.Value(), *nudge)
		m.composeStatus = "Radar research intent recorded for company: " + target + " (Research Mode is opt-in)"
	default:
		m.composeStatus = "Radar action unavailable"
	}
	m.refreshComposeLayout()
	return nil
}

func nudgeLine(nudge memory.Nudge) string {
	message := strings.TrimSpace(nudge.Message)
	if message == "" {
		message = strings.TrimSpace(nudge.Type)
	}
	source := ""
	if len(nudge.Evidence) > 0 {
		source = nudgeEvidenceLabel(nudge.Evidence[0])
	}
	if source != "" {
		message = "[" + source + "] " + message
	}
	if state := composeMemoryActionStateLabel(nudge.ActionState); state != "" {
		message += " (" + state + ")"
	}
	return message
}

func nudgeEvidenceLabel(evidence memory.Evidence) string {
	switch evidence.SourceType {
	case memory.SourceEmail, memory.SourceSentEmail:
		return firstNonEmptyString(evidence.MessageID, evidence.LocalID, evidence.ID)
	case memory.SourceObsidian:
		return evidence.Path
	case memory.SourceResearch:
		return evidence.URL
	default:
		return firstNonEmptyString(evidence.ID, evidence.MessageID, evidence.Path, evidence.URL)
	}
}

func composeMemoryActionStateLabel(state string) string {
	switch strings.TrimSpace(state) {
	case "", memory.NudgeActionNew:
		return ""
	case memory.NudgeActionDismissed:
		return "dismissed"
	case memory.NudgeActionInserted:
		return "inserted"
	case memory.NudgeActionResolved:
		return "resolved"
	case memory.NudgeActionSaved:
		return "saved"
	default:
		return state
	}
}

func composeMemorySourceInspectionLabel(nudge memory.Nudge) string {
	if len(nudge.Evidence) == 0 {
		return ""
	}
	evidence := nudge.Evidence[0]
	switch evidence.SourceType {
	case memory.SourceEmail:
		return composeMemoryJoinSourceParts("email", firstNonEmptyString(evidence.MessageID, evidence.LocalID, evidence.SourceID, evidence.ID), evidence.Folder)
	case memory.SourceSentEmail:
		return composeMemoryJoinSourceParts("sent email", firstNonEmptyString(evidence.MessageID, evidence.LocalID, evidence.SourceID, evidence.ID), evidence.Folder)
	case memory.SourceObsidian:
		return composeMemoryJoinSourceParts("Obsidian note", evidence.Path, evidence.ID)
	case memory.SourceResearch:
		return composeMemoryJoinSourceParts("research URL", evidence.URL, evidence.ID)
	case memory.SourceCalendar:
		return composeMemoryJoinSourceParts("calendar event", firstNonEmptyString(evidence.SourceID, evidence.ID), evidence.Date.Format("2006-01-02"))
	case memory.SourceAttachment:
		return composeMemoryJoinSourceParts("attachment", firstNonEmptyString(evidence.Path, evidence.ID, evidence.SourceID), evidence.MessageID)
	default:
		return composeMemoryJoinSourceParts(firstNonEmptyString(evidence.SourceType, "source"), firstNonEmptyString(evidence.ID, evidence.MessageID, evidence.Path, evidence.URL), evidence.Folder)
	}
}

func composeMemoryJoinSourceParts(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = composeMemoryOneLine(part)
		if part != "" && part != "0001-01-01" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " ")
}

func composeMemoryInsertPhrase(nudge memory.Nudge) string {
	phrase := firstNonEmptyString(nudge.Message, nudge.Why, nudge.Type)
	phrase = composeMemoryOneLine(phrase)
	return chatAgentBoundedText(phrase, composeMemoryInsertPhraseLimit)
}

func composeMemoryAppendParagraph(body, phrase string) string {
	body = strings.TrimRight(body, " \t\r\n")
	phrase = strings.TrimSpace(phrase)
	if phrase == "" {
		return body
	}
	if body == "" {
		return phrase
	}
	return body + "\n\n" + phrase
}

func composeMemoryResearchTarget(scope, recipient string, nudge memory.Nudge) string {
	recipient = composeMemoryOneLine(recipient)
	if scope == "person" && recipient != "" {
		return chatAgentBoundedText(recipient, 80)
	}
	for _, evidence := range nudge.Evidence {
		if scope == "company" {
			if target := composeMemoryDomainFromEmail(firstNonEmptyString(evidence.SourceID, evidence.ID, evidence.MessageID)); target != "" {
				return target
			}
		}
	}
	if scope == "company" {
		if target := composeMemoryDomainFromEmail(recipient); target != "" {
			return target
		}
	}
	return scope
}

func composeMemoryOneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func composeMemoryDomainFromEmail(value string) string {
	value = strings.TrimSpace(value)
	at := strings.LastIndex(value, "@")
	if at < 0 || at+1 >= len(value) {
		return ""
	}
	domain := value[at+1:]
	domain = strings.Trim(domain, " <>.,;")
	return strings.ToLower(domain)
}
