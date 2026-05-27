package backend

import (
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type fakeCalendarSummaryAI struct {
	response string
	err      error
	calls    int
	messages []ai.ChatMessage
}

func (f *fakeCalendarSummaryAI) Chat(messages []ai.ChatMessage) (string, error) {
	f.calls++
	f.messages = append([]ai.ChatMessage(nil), messages...)
	return f.response, f.err
}

func TestBuildCalendarAISummaryUsesCachedCrossSourceContextAndAI(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref:       models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "launch"},
		Title:     "Launch planning",
		Organizer: "Mina Park",
		Start:     start,
		End:       start.Add(time.Hour),
		Attendees: []models.CalendarAttendee{
			{Name: "Ari Lane", Email: "ari@example.com"},
		},
	}
	event.Ref = event.Ref.WithDefaults()
	mail := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "mail-planning",
		Folder:    "INBOX",
		Sender:    "mina@example.com",
		Subject:   "Launch planning risks",
		Date:      start.Add(-time.Hour),
	}
	nearby := event
	nearby.Ref.EventID = "launch-retro"
	nearby.Ref.LocalID = ""
	nearby.Ref = nearby.Ref.WithDefaults()
	nearby.Title = "Launch retro"
	nearby.Start = start.Add(2 * time.Hour)
	search := &fakeMeetingPrepSearch{resultsByQuery: map[string][]models.CrossSourceSearchResult{
		"Launch planning": {
			models.NewMailCrossSourceSearchResult(mail, "Launch planning"),
			models.NewEventCrossSourceSearchResult(event, "Launch planning"),
			models.NewEventCrossSourceSearchResult(nearby, "Launch planning"),
		},
	}}
	summarizer := &fakeCalendarSummaryAI{response: "Summary:\n- Mina flagged launch risk in cached mail.\n- Retro follows the launch meeting.\nActions:\n- Review Launch planning risks before the event."}

	summary, err := buildCalendarAISummary(search, summarizer, event)
	if err != nil {
		t.Fatalf("buildCalendarAISummary: %v", err)
	}
	if summary.GeneratedBy != models.CalendarAISummaryGeneratedByAI {
		t.Fatalf("GeneratedBy = %q, want AI", summary.GeneratedBy)
	}
	if len(summary.RelatedMail) != 1 || summary.RelatedMail[0].Subject != "Launch planning risks" {
		t.Fatalf("RelatedMail = %#v, want cached related mail", summary.RelatedMail)
	}
	if len(summary.NearbyEvents) != 1 || summary.NearbyEvents[0].Title != "Launch retro" {
		t.Fatalf("NearbyEvents = %#v, want related cached event only", summary.NearbyEvents)
	}
	if summarizer.calls != 1 {
		t.Fatalf("AI calls = %d, want 1", summarizer.calls)
	}
	prompt := strings.TrimSpace(summarizer.messages[len(summarizer.messages)-1].Content)
	for _, want := range []string{"Launch planning", "Launch planning risks", "Launch retro"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("AI prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"provider-event-id", "ETag", "syncToken"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("AI prompt leaked provider internals %q:\n%s", forbidden, prompt)
		}
	}
	if got := strings.Join(summary.Bullets, " "); !strings.Contains(got, "Mina flagged launch risk") {
		t.Fatalf("summary bullets = %#v", summary.Bullets)
	}
	if got := strings.Join(summary.ActionItems, " "); !strings.Contains(got, "Review Launch planning risks") {
		t.Fatalf("summary action items = %#v", summary.ActionItems)
	}
}

func TestBuildCalendarAISummaryFallsBackWhenAINotConfigured(t *testing.T) {
	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	event := models.CalendarEvent{
		Ref:   models.EventRef{SourceID: "work-calendar", AccountID: "work", CalendarID: "primary", EventID: "launch"}.WithDefaults(),
		Title: "Launch planning",
		Start: start,
		End:   start.Add(time.Hour),
	}
	mail := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "mail-planning",
		Folder:    "INBOX",
		Sender:    "mina@example.com",
		Subject:   "Launch planning risks",
		Date:      start.Add(-time.Hour),
	}
	search := &fakeMeetingPrepSearch{resultsByQuery: map[string][]models.CrossSourceSearchResult{
		"Launch planning": {
			models.NewMailCrossSourceSearchResult(mail, "Launch planning"),
		},
	}}

	summary, err := buildCalendarAISummary(search, nil, event)
	if err != nil {
		t.Fatalf("buildCalendarAISummary: %v", err)
	}
	if summary.GeneratedBy != models.CalendarAISummaryGeneratedByFallback {
		t.Fatalf("GeneratedBy = %q, want fallback", summary.GeneratedBy)
	}
	if len(summary.Bullets) == 0 || len(summary.ActionItems) == 0 {
		t.Fatalf("fallback summary missing content: %#v", summary)
	}
	if len(summary.RelatedMail) != 1 {
		t.Fatalf("RelatedMail = %#v, want cached mail preserved", summary.RelatedMail)
	}
}
