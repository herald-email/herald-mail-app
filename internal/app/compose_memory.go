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
		return accentStyle.Render(truncateVisual("Radar: "+nudgeLine(nudges[0]), width))
	}
	lines := []string{accentStyle.Render(truncateVisual(fmt.Sprintf("Radar: %d memory nudge(s)", len(nudges)), width))}
	for i, nudge := range nudges {
		if i >= 3 {
			break
		}
		lines = append(lines, dimStyle.Render(truncateVisual("  - "+nudgeLine(nudge), width)))
	}
	return strings.Join(lines, "\n")
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
