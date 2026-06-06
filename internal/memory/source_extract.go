package memory

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func (e Extractor) ExtractCalendarEvents(events []models.CalendarEvent) []Memory {
	now := e.now()
	settings := e.Settings
	settings.ApplyDefaults()
	memories := make([]Memory, 0, len(events))
	for _, event := range events {
		topic := strings.TrimSpace(event.Title)
		if topic == "" {
			topic = "calendar event"
		}
		activity := event.Start
		if activity.IsZero() {
			activity = event.UpdatedAt
		}
		if activity.IsZero() {
			activity = now
		}
		people := peopleForCalendarEvent(event)
		person := "Calendar"
		if len(people) > 0 {
			person = people[0]
		}
		domain := domainForCalendarEvent(event)
		company := companyFromDomain(domain)
		snippet := calendarEventSnippet(event)
		ref := event.Ref.WithDefaults()
		claim := fmt.Sprintf("Calendar event %q is scheduled for %s.", topic, activity.Format("2006-01-02 15:04"))
		if snippet != "" {
			claim = fmt.Sprintf("Calendar event %q is scheduled for %s: %s", topic, activity.Format("2006-01-02 15:04"), snippet)
		}
		memories = append(memories, Memory{
			Kind:           KindRelationshipContext,
			Claim:          bounded(claim, 600),
			Summary:        bounded(firstNonEmpty(snippet, claim), 300),
			Topic:          topic,
			People:         people,
			Company:        company,
			Domain:         domain,
			Status:         StatusActive,
			Confidence:     0.72,
			LastActivityAt: activity,
			ObsidianTarget: defaultTargetFor(settings, person, company, topic),
			Tags:           tagsForMemory(KindRelationshipContext, settings),
			Evidence: []Evidence{{
				SourceType: SourceCalendar,
				SourceID:   string(ref.SourceID),
				AccountID:  string(ref.AccountID),
				ID:         firstNonEmpty(ref.LocalID, ref.EventID, event.ProviderUID),
				LocalID:    ref.LocalID,
				Date:       activity,
				Snippet:    snippet,
			}},
			Details: MemoryDetails{
				GeneratedSummary: bounded(firstNonEmpty(snippet, claim), 300),
				SourceQuote:      snippet,
				SourceCount:      1,
				ExtractionPrompt: PromptVersionHeuristicV1,
				SourceSignals:    []string{"from calendar", "cached_calendar_event"},
				LastValidatedAt:  now,
			},
		})
	}
	for i := range memories {
		memories[i] = PrepareMemoryForAppend(memories[i], now)
	}
	return dedupeMemories(memories)
}

func (e Extractor) ExtractObsidianNotes(notes []ObsidianNoteSnapshot) []Memory {
	now := e.now()
	settings := e.Settings
	settings.ApplyDefaults()
	memories := make([]Memory, 0, len(notes))
	for _, note := range notes {
		body := BoundSnapshotBodyText(note.BodyText)
		snippet := bounded(firstUsefulSentence(body, note.Title), 280)
		if strings.TrimSpace(snippet) == "" {
			continue
		}
		title := firstNonEmpty(note.Title, strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path)), "Obsidian note")
		person, company := noteIdentityFromPath(note.Path, title, settings)
		activity := note.ModifiedAt
		if activity.IsZero() {
			activity = now
		}
		claim := fmt.Sprintf("Obsidian note %q says: %s", title, snippet)
		memories = append(memories, Memory{
			Kind:           KindRelationshipContext,
			Claim:          bounded(claim, 600),
			Summary:        snippet,
			Topic:          title,
			People:         CompactStrings([]string{person}),
			Company:        company,
			Domain:         "",
			Status:         StatusActive,
			Confidence:     0.68,
			LastActivityAt: activity,
			ObsidianTarget: filepath.ToSlash(note.Path),
			Tags:           tagsForMemory(KindRelationshipContext, settings),
			Evidence: []Evidence{{
				SourceType: SourceObsidian,
				Path:       filepath.ToSlash(note.Path),
				Date:       activity,
				Snippet:    snippet,
			}},
			Details: MemoryDetails{
				GeneratedSummary: snippet,
				SourceQuote:      snippet,
				SourceCount:      1,
				ExtractionPrompt: PromptVersionHeuristicV1,
				ContactCompany:   company,
				SourceSignals:    []string{"from Obsidian", "configured_note"},
				LastValidatedAt:  now,
			},
		})
	}
	for i := range memories {
		memories[i] = PrepareMemoryForAppend(memories[i], now)
	}
	return dedupeMemories(memories)
}

func (e Extractor) ExtractResearchNotes(inputs []ResearchNoteInput) []Memory {
	now := e.now()
	settings := e.Settings
	settings.ApplyDefaults()
	memories := make([]Memory, 0, len(inputs))
	for _, input := range inputs {
		memory, err := BuildResearchMemory(input, settings)
		if err != nil {
			continue
		}
		memory.Details.SourceSignals = CompactStrings(append(memory.Details.SourceSignals, "explicit_saved_research_note"))
		memories = append(memories, PrepareMemoryForAppend(memory, now))
	}
	return dedupeMemories(memories)
}

func peopleForCalendarEvent(event models.CalendarEvent) []string {
	values := []string{event.Organizer, event.OrganizerEmail}
	for _, attendee := range event.Attendees {
		values = append(values, attendee.Name, attendee.Email)
	}
	return CompactStrings(values)
}

func domainForCalendarEvent(event models.CalendarEvent) string {
	if domain := domainFromSender(event.OrganizerEmail); domain != "" {
		return domain
	}
	for _, attendee := range event.Attendees {
		if domain := domainFromSender(attendee.Email); domain != "" {
			return domain
		}
	}
	return ""
}

func calendarEventSnippet(event models.CalendarEvent) string {
	parts := []string{event.Description}
	if event.Location != "" {
		parts = append(parts, "Location: "+event.Location)
	}
	if event.OrganizerEmail != "" {
		parts = append(parts, "Organizer: "+event.OrganizerEmail)
	}
	for _, attendee := range event.Attendees {
		if attendee.Email != "" {
			parts = append(parts, "Attendee: "+attendee.Email)
		}
	}
	return bounded(strings.Join(CompactStrings(parts), " "), 280)
}

func noteIdentityFromPath(path, title string, settings Settings) (string, string) {
	normalizedPath := strings.ToLower(filepath.ToSlash(path))
	peoplePrefix := strings.ToLower(strings.TrimRight(filepath.ToSlash(settings.Destinations.People), "/")) + "/"
	companiesPrefix := strings.ToLower(strings.TrimRight(filepath.ToSlash(settings.Destinations.Companies), "/")) + "/"
	jobPrefix := strings.ToLower(strings.TrimRight(filepath.ToSlash(settings.Destinations.JobSearch), "/")) + "/"
	switch {
	case strings.HasPrefix(normalizedPath, peoplePrefix):
		return title, ""
	case strings.HasPrefix(normalizedPath, companiesPrefix):
		return "", firstPathPartAfterPrefix(path, settings.Destinations.Companies, title)
	case strings.HasPrefix(normalizedPath, jobPrefix):
		return "", firstPathPartAfterPrefix(path, settings.Destinations.JobSearch, title)
	default:
		return title, ""
	}
}

func firstPathPartAfterPrefix(path, prefix, fallback string) string {
	rel := strings.TrimPrefix(filepath.ToSlash(path), strings.TrimRight(filepath.ToSlash(prefix), "/")+"/")
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return fallback
	}
	first := strings.Split(rel, "/")[0]
	first = strings.TrimSuffix(first, filepath.Ext(first))
	if first == "" {
		return fallback
	}
	return first
}
