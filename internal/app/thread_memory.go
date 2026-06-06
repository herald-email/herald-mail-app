package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/memory"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func (m *Model) resetTimelineThreadMemoryDossier() {
	m.threadMemoryToken++
	m.threadMemoryDossier = memory.Dossier{}
	m.threadMemoryLoading = false
	m.threadMemoryError = ""
	m.threadMemoryMessageID = ""
	m.threadMemorySubject = ""
}

func (m *Model) loadTimelineThreadMemoryDossier(email *models.EmailData) tea.Cmd {
	if email == nil {
		m.resetTimelineThreadMemoryDossier()
		return nil
	}
	source, ok := m.backend.(contactMemorySource)
	if !ok || source == nil {
		m.resetTimelineThreadMemoryDossier()
		return nil
	}
	settings := memory.DefaultSettings()
	if m.cfg != nil {
		settings = m.cfg.Memories
	}
	settings.ApplyDefaults()
	if !settings.Enabled {
		m.resetTimelineThreadMemoryDossier()
		return nil
	}
	subject := threadDossierSubject(email.Subject)
	if subject == "" {
		m.resetTimelineThreadMemoryDossier()
		return nil
	}
	m.threadMemoryToken++
	token := m.threadMemoryToken
	m.threadMemoryDossier = memory.Dossier{}
	m.threadMemoryLoading = true
	m.threadMemoryError = ""
	m.threadMemoryMessageID = email.MessageID
	m.threadMemorySubject = subject
	return func() tea.Msg {
		ctx := context.Background()
		memories, err := searchThreadDossierMemories(ctx, source, subject, settings)
		dossier := memory.BuildThreadDossier(subject, memories, settings, time.Now())
		return ThreadMemoryDossierMsg{
			Token:     token,
			MessageID: email.MessageID,
			Subject:   subject,
			Dossier:   dossier,
			Err:       err,
		}
	}
}

func searchThreadDossierMemories(ctx context.Context, source contactMemorySource, subject string, settings memory.Settings) ([]memory.Memory, error) {
	subject = threadDossierSubject(subject)
	if source == nil || subject == "" {
		return nil, nil
	}
	minConfidence := settings.Thresholds.Dossier
	limit := 12
	queries := []memory.Query{
		{Topic: subject, MinConfidence: minConfidence, Limit: limit},
		{Text: subject, MinConfidence: minConfidence, Limit: limit},
	}
	seen := make(map[string]bool)
	out := make([]memory.Memory, 0)
	for _, query := range queries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		results, err := source.SearchMemories(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, item := range results {
			key := strings.TrimSpace(item.ID)
			if key == "" {
				key = memory.DeterministicID(item)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	memory.SortMemoriesNewestFirst(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func threadDossierSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	for {
		lower := strings.ToLower(subject)
		matched := false
		for _, prefix := range []string{"re:", "fw:", "fwd:", "aw:", "tr:"} {
			if strings.HasPrefix(lower, prefix) {
				subject = strings.TrimSpace(subject[len(prefix):])
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	return subject
}

func (m *Model) timelineThreadMemoryDossierLines(width, maxLines int) []string {
	if maxLines <= 0 || width <= 0 || m.timeline.selectedEmail == nil {
		return nil
	}
	boldStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Chrome.TitleBar.ForegroundColor())
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Dim.ForegroundColor())
	normalStyle := lipgloss.NewStyle().Foreground(m.theme.Text.Primary.ForegroundColor())
	if m.threadMemoryLoading {
		return []string{
			boldStyle.Render(truncateVisual("Herald Memories", width)),
			dimStyle.Render(truncateVisual("  Loading thread memory...", width)),
		}
	}
	if m.threadMemoryError != "" {
		return []string{
			boldStyle.Render(truncateVisual("Herald Memories", width)),
			dimStyle.Render(truncateVisual("  Memories unavailable: "+m.threadMemoryError, width)),
		}
	}
	dossier := m.threadMemoryDossier
	if !contactMemoryDossierHasContent(dossier) {
		return nil
	}
	lines := []string{boldStyle.Render(truncateVisual("Herald Memories", width))}
	if dossier.Subject != "" {
		lines = append(lines, normalStyle.Render(truncateVisual("  Thread: "+dossier.Subject, width)))
	}
	if len(dossier.ActiveTracks) > 0 {
		lines = append(lines, dimStyle.Render(truncateVisual("  Track: "+contactTrackLine(dossier.ActiveTracks[0]), width)))
	}
	if len(dossier.OpenLoops) > 0 {
		lines = append(lines, dimStyle.Render(truncateVisual("  Open loop: "+contactMemorySummary(dossier.OpenLoops[0]), width)))
	}
	if len(dossier.VaultLinks) > 0 {
		lines = append(lines, dimStyle.Render(truncateVisual("  Vault: "+dossier.VaultLinks[0], width)))
	}
	if len(dossier.Evidence) > 0 {
		lines = append(lines, dimStyle.Render(truncateVisual("  Evidence: "+nudgeEvidenceLabel(dossier.Evidence[0]), width)))
	}
	if len(lines) > maxLines {
		return lines[:maxLines]
	}
	return lines
}
