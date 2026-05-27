package backend

import (
	"fmt"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// CalendarAISummaryBackend is a read-only command-center capability assembled
// from existing cache-backed cross-source search results.
type CalendarAISummaryBackend interface {
	BuildCalendarAISummary(event models.CalendarEvent) (models.CalendarAISummary, error)
}

type calendarAIChatClient interface {
	Chat([]ai.ChatMessage) (string, error)
}

var _ CalendarAISummaryBackend = (*DemoBackend)(nil)
var _ CalendarAISummaryBackend = (*LocalBackend)(nil)
var _ CalendarAISummaryBackend = (*MultiBackend)(nil)

func (d *DemoBackend) BuildCalendarAISummary(event models.CalendarEvent) (models.CalendarAISummary, error) {
	return buildCalendarAISummary(d, nil, event)
}

func (b *LocalBackend) BuildCalendarAISummary(event models.CalendarEvent) (models.CalendarAISummary, error) {
	b.classifierMu.RLock()
	summarizer := b.classifier
	b.classifierMu.RUnlock()
	return buildCalendarAISummary(b, summarizer, event)
}

func (m *MultiBackend) BuildCalendarAISummary(event models.CalendarEvent) (models.CalendarAISummary, error) {
	if !m.allAccountsActive() {
		if active := m.activeBackend(); active != nil {
			if summary, ok := active.(CalendarAISummaryBackend); ok {
				return summary.BuildCalendarAISummary(event)
			}
		}
	}
	return buildCalendarAISummary(m, nil, event)
}

func buildCalendarAISummary(search CrossSourceSearchBackend, summarizer calendarAIChatClient, event models.CalendarEvent) (models.CalendarAISummary, error) {
	event.Ref = event.Ref.WithDefaults()
	terms := models.CalendarAISummaryQueries(event)
	base := models.CalendarAISummary{
		Event:       event,
		QueryTerms:  terms,
		GeneratedAt: time.Now().UTC(),
	}
	if search != nil {
		seenMail := make(map[string]struct{})
		seenEvents := make(map[string]struct{})
		selectedLocalID := event.Ref.LocalID
		for _, query := range terms {
			results, err := search.CrossSourceSearch(query)
			if err != nil {
				return base, err
			}
			for _, result := range results {
				switch result.Kind {
				case models.CrossSourceResultMail:
					if result.Email == nil {
						continue
					}
					key := result.Email.MessageRef().WithDefaults().LocalID
					if _, ok := seenMail[key]; ok {
						continue
					}
					seenMail[key] = struct{}{}
					base.RelatedMail = append(base.RelatedMail, models.CloneEmailData(result.Email))
				case models.CrossSourceResultEvent:
					if result.Event == nil {
						continue
					}
					related := *result.Event
					related.Ref = related.Ref.WithDefaults()
					if related.Ref.LocalID == selectedLocalID {
						continue
					}
					if _, ok := seenEvents[related.Ref.LocalID]; ok {
						continue
					}
					seenEvents[related.Ref.LocalID] = struct{}{}
					base.NearbyEvents = append(base.NearbyEvents, related)
				}
			}
		}
	}
	if len(base.RelatedMail) > 6 {
		base.RelatedMail = base.RelatedMail[:6]
	}
	if len(base.NearbyEvents) > 4 {
		base.NearbyEvents = base.NearbyEvents[:4]
	}
	if summarizer == nil {
		return models.CalendarAISummaryFallback(event, base.RelatedMail, base.NearbyEvents, terms, "AI unavailable"), nil
	}
	raw, err := summarizer.Chat(calendarAISummaryMessages(base))
	if err != nil {
		return models.CalendarAISummaryFallback(event, base.RelatedMail, base.NearbyEvents, terms, "AI unavailable: "+err.Error()), nil
	}
	bullets, actions := models.ParseCalendarAISummaryAIResponse(raw)
	if len(bullets) == 0 && len(actions) == 0 {
		return models.CalendarAISummaryFallback(event, base.RelatedMail, base.NearbyEvents, terms, "AI returned an empty summary"), nil
	}
	base.Bullets = bullets
	base.ActionItems = actions
	base.GeneratedBy = models.CalendarAISummaryGeneratedByAI
	base.AINote = "AI summary generated from cached context"
	base.GeneratedAt = time.Now().UTC()
	return base, nil
}

func calendarAISummaryMessages(summary models.CalendarAISummary) []ai.ChatMessage {
	return []ai.ChatMessage{
		{
			Role:    "system",
			Content: "Summarize cached calendar and mail context for a terminal email/calendar client. Return concise sections named Summary and Actions. Do not mention provider IDs, sync tokens, ETags, OAuth, or mutation controls.",
		},
		{
			Role:    "user",
			Content: calendarAISummaryPrompt(summary),
		},
	}
}

func calendarAISummaryPrompt(summary models.CalendarAISummary) string {
	event := summary.Event
	var b strings.Builder
	b.WriteString("Selected event\n")
	b.WriteString("Title: " + safeSummaryText(event.Title) + "\n")
	if !event.Start.IsZero() || !event.End.IsZero() {
		b.WriteString("Time: " + calendarSummaryTimeRange(event) + "\n")
	}
	if strings.TrimSpace(event.Location) != "" {
		b.WriteString("Location: " + safeSummaryText(event.Location) + "\n")
	}
	if strings.TrimSpace(event.Organizer) != "" || strings.TrimSpace(event.OrganizerEmail) != "" {
		b.WriteString("Organizer: " + safeSummaryText(strings.TrimSpace(event.Organizer+" "+event.OrganizerEmail)) + "\n")
	}
	if len(event.Attendees) > 0 {
		var attendees []string
		for _, attendee := range event.Attendees {
			label := strings.TrimSpace(attendee.Name + " " + attendee.Email)
			if label != "" {
				attendees = append(attendees, safeSummaryText(label))
			}
		}
		if len(attendees) > 0 {
			b.WriteString("Attendees: " + strings.Join(attendees, ", ") + "\n")
		}
	}
	if strings.TrimSpace(event.Description) != "" {
		b.WriteString("Notes: " + safeSummaryText(event.Description) + "\n")
	}

	b.WriteString("\nCached related mail\n")
	if len(summary.RelatedMail) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, email := range summary.RelatedMail {
			if email == nil {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s from %s in %s at %s\n",
				safeSummaryText(email.Subject),
				safeSummaryText(email.Sender),
				safeSummaryText(email.Folder),
				email.Date.Local().Format("Mon Jan 2 15:04"),
			))
		}
	}

	b.WriteString("\nCached nearby events\n")
	if len(summary.NearbyEvents) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, nearby := range summary.NearbyEvents {
			b.WriteString(fmt.Sprintf("- %s at %s\n", safeSummaryText(nearby.Title), calendarSummaryTimeRange(nearby)))
		}
	}
	return b.String()
}

func calendarSummaryTimeRange(event models.CalendarEvent) string {
	if event.Start.IsZero() && event.End.IsZero() {
		return ""
	}
	if event.End.IsZero() {
		return event.Start.Local().Format("Mon Jan 2 15:04")
	}
	return event.Start.Local().Format("Mon Jan 2 15:04") + " - " + event.End.Local().Format("15:04")
}

func safeSummaryText(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.Join(strings.Fields(value), " ")
}
