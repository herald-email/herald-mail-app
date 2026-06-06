package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/agent"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func chatAgentAssistantContent(result agent.ChatResult, err error) string {
	if err != nil {
		return "Error: " + err.Error()
	}
	parts := make([]string, 0, 2)
	if reply := strings.TrimSpace(result.Reply); reply != "" {
		parts = append(parts, reply)
	}
	if summary := formatChatAgentSummary(result.Summary); summary != "" {
		parts = append(parts, summary)
	}
	content := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if content == "" {
		return "No agent response."
	}
	return content
}

func formatChatAgentSummary(summary *agent.EmailSummary) string {
	if summary == nil {
		return ""
	}
	var lines []string
	if value := strings.TrimSpace(summary.Summary); value != "" {
		lines = append(lines, "Summary: "+value)
	}
	if len(summary.People) > 0 {
		lines = append(lines, "People:")
		for _, person := range summary.People {
			label := strings.TrimSpace(person.NameOrEmail)
			if label == "" {
				continue
			}
			detail := strings.TrimSpace(person.Role)
			if person.EvidenceID != "" {
				if detail != "" {
					detail += ", "
				}
				detail += "source " + person.EvidenceID
			}
			if detail != "" {
				label = fmt.Sprintf("%s (%s)", label, detail)
			}
			lines = append(lines, "- "+label)
		}
	}
	if len(summary.Dates) > 0 {
		lines = append(lines, "Dates: "+strings.Join(summary.Dates, ", "))
	}
	if len(summary.ActionItems) > 0 {
		lines = append(lines, "Action items:")
		for _, item := range summary.ActionItems {
			if item = strings.TrimSpace(item); item != "" {
				lines = append(lines, "- "+item)
			}
		}
	}
	if len(summary.OpenQuestions) > 0 {
		lines = append(lines, "Open questions:")
		for _, question := range summary.OpenQuestions {
			if question = strings.TrimSpace(question); question != "" {
				lines = append(lines, "- "+question)
			}
		}
	}
	if len(summary.CitedIDs) > 0 {
		lines = append(lines, "Sources: "+strings.Join(summary.CitedIDs, ", "))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (m *Model) applyChatAgentTimelineIntent(intent *agent.TimelineIntent) tea.Cmd {
	if intent == nil {
		return nil
	}
	switch normalizeChatAgentTimelineMode(intent.Mode) {
	case agent.TimelineModeExplicitIDs:
		emails := m.chatAgentEmailsByIDs(intent.MessageIDs)
		if len(emails) == 0 {
			m.statusMessage = "No matching emails for agent Timeline results."
			return nil
		}
		m.finishTimelineRangeSelection()
		m.timeline.chatFilterMode = true
		m.timeline.chatFilteredEmails = emails
		m.timeline.chatFilterLabel = chatAgentTimelineLabel(intent)
		m.activeTab = tabTimeline
		m.updateTimelineTable()
		return nil
	case agent.TimelineModeKeyword, agent.TimelineModeSemantic, agent.TimelineModeHybrid:
		query := chatAgentSearchQueryForIntent(intent)
		if query == "" {
			m.statusMessage = "Agent Timeline search needs a query."
			return nil
		}
		m.activeTab = tabTimeline
		m.openTimelineSearch()
		m.timeline.searchInput.SetValue(query)
		m.timeline.searchFocus = timelineSearchFocusInput
		m.timeline.searchAutoFocusResults = true
		m.timeline.searchError = ""
		token := m.timeline.searchToken
		return m.performSearchWithToken(query, token)
	default:
		m.statusMessage = "Unsupported agent Timeline intent."
		return nil
	}
}

func (m *Model) applyChatAgentComposeIntent(intent *agent.ComposeIntent, composeWasActive bool) {
	if intent == nil {
		return
	}
	if !composeWasActive {
		m.statusMessage = "Open Compose to review the agent draft."
		return
	}
	if subject := strings.TrimSpace(intent.SubjectSuggestion); subject != "" {
		m.composeAISubjectHint = subject
	}
	if suggestion := strings.TrimSpace(intent.BodySuggestion); suggestion != "" {
		original := m.composeBody.Value()
		m.composeAIPanel = true
		m.composeAIMenu = ""
		m.composeAIDiff = wordDiffWithTheme(m.theme, original, suggestion)
		m.composeAIOriginal = original
		m.composeAIShowOriginal = false
		m.composeAIResponse.SetValue(suggestion)
		m.composeAIResponse.MoveToBegin()
		m.composeAIInput.Blur()
		m.composeBody.Blur()
		m.composeAIResponse.Focus()
		m.statusMessage = "Agent draft is ready for review."
		m.refreshComposeLayout()
		return
	}
	if m.composeAISubjectHint != "" {
		m.statusMessage = "Agent subject suggestion is ready for review."
		m.refreshComposeLayout()
	}
}

func (m *Model) chatAgentEmailsByIDs(ids []string) []*models.EmailData {
	if len(ids) == 0 {
		return nil
	}
	byID := make(map[string]*models.EmailData)
	for _, pool := range chatAgentEmailPools(m) {
		for _, row := range pool {
			if row == nil || strings.TrimSpace(row.MessageID) == "" {
				continue
			}
			if _, exists := byID[row.MessageID]; !exists {
				byID[row.MessageID] = row
			}
		}
	}
	out := make([]*models.EmailData, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if email := byID[id]; email != nil {
			out = append(out, email)
		}
	}
	return out
}

func chatAgentEmailPools(m *Model) [][]*models.EmailData {
	if m == nil {
		return nil
	}
	return [][]*models.EmailData{
		m.timeline.emails,
		m.timeline.emailsCache,
		m.timeline.searchResults,
		m.timeline.chatFilteredEmails,
	}
}

func chatAgentSearchQueryForIntent(intent *agent.TimelineIntent) string {
	if intent == nil {
		return ""
	}
	query := strings.TrimSpace(intent.Query)
	if query == "" {
		return ""
	}
	switch normalizeChatAgentTimelineMode(intent.Mode) {
	case agent.TimelineModeKeyword:
		return "/k " + query
	case agent.TimelineModeSemantic:
		return "?" + query
	default:
		return query
	}
}

func chatAgentTimelineLabel(intent *agent.TimelineIntent) string {
	if intent == nil {
		return "agent results"
	}
	if label := strings.TrimSpace(intent.Label); label != "" {
		return label
	}
	if query := strings.TrimSpace(intent.Query); query != "" {
		return query
	}
	return "agent results"
}

func normalizeChatAgentTimelineMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case agent.TimelineModeExplicitIDs:
		return agent.TimelineModeExplicitIDs
	case agent.TimelineModeKeyword:
		return agent.TimelineModeKeyword
	case agent.TimelineModeSemantic:
		return agent.TimelineModeSemantic
	case agent.TimelineModeHybrid:
		return agent.TimelineModeHybrid
	default:
		return ""
	}
}
