package mcpserver

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addCalendarTools(s *server.MCPServer, c *cache.Cache) {
	s.AddTool(
		mcp.NewTool("list_calendar_events",
			mcp.WithDescription("List cached calendar events for a scoped calendar source"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("source_id", mcp.Description("Calendar source ID (default default-calendar)")),
			mcp.WithString("account_id", mcp.Description("Account ID (default default)")),
			mcp.WithString("start", mcp.Description("Inclusive start time, RFC3339 or YYYY-MM-DD")),
			mcp.WithString("end", mcp.Description("Exclusive end time, RFC3339 or YYYY-MM-DD")),
			mcp.WithNumber("limit", mcp.Description("Maximum events to return (default 20, max 100)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sourceID := models.SourceID(req.GetString("source_id", ""))
			accountID := models.AccountID(req.GetString("account_id", ""))
			start, err := parseMCPCalendarTime(req.GetString("start", ""))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			end, err := parseMCPCalendarTime(req.GetString("end", ""))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			limit := clampMCPResultLimit(req.GetInt("limit", 20))
			events, err := c.ListCalendarAgendaEvents(sourceID, accountID, start, end)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("calendar cache error: %v", err)), nil
			}
			return mcp.NewToolResultText(formatMCPCalendarEventList("Calendar events", events, limit)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("search_calendar_events",
			mcp.WithDescription("Search cached calendar events by visible title, notes, location, attendees, organizer, or recurrence text"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search text")),
			mcp.WithString("source_id", mcp.Description("Calendar source ID (default default-calendar)")),
			mcp.WithString("account_id", mcp.Description("Account ID (default default)")),
			mcp.WithString("start", mcp.Description("Inclusive start time, RFC3339 or YYYY-MM-DD")),
			mcp.WithString("end", mcp.Description("Exclusive end time, RFC3339 or YYYY-MM-DD")),
			mcp.WithNumber("limit", mcp.Description("Maximum events to return (default 20, max 100)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			sourceID := models.SourceID(req.GetString("source_id", ""))
			accountID := models.AccountID(req.GetString("account_id", ""))
			start, err := parseMCPCalendarTime(req.GetString("start", ""))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			end, err := parseMCPCalendarTime(req.GetString("end", ""))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			limit := clampMCPResultLimit(req.GetInt("limit", 20))
			events, err := c.SearchCalendarEvents(sourceID, accountID, query, start, end)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("calendar search error: %v", err)), nil
			}
			return mcp.NewToolResultText(formatMCPCalendarEventList(fmt.Sprintf("Calendar events matching %q", query), events, limit)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_calendar_event",
			mcp.WithDescription("Read one cached calendar event by scoped EventRef fields"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("local_id", mcp.Description("Scoped Herald event local ID from list/search results")),
			mcp.WithString("source_id", mcp.Description("Calendar source ID")),
			mcp.WithString("account_id", mcp.Description("Account ID")),
			mcp.WithString("calendar_id", mcp.Description("Calendar collection ID")),
			mcp.WithString("event_id", mcp.Description("Provider event ID")),
			mcp.WithString("instance_id", mcp.Description("Recurrence instance ID, when present")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ref := models.EventRef{
				SourceID:   models.SourceID(req.GetString("source_id", "")),
				AccountID:  models.AccountID(req.GetString("account_id", "")),
				CalendarID: req.GetString("calendar_id", ""),
				EventID:    req.GetString("event_id", ""),
				InstanceID: req.GetString("instance_id", ""),
				LocalID:    req.GetString("local_id", ""),
			}
			if strings.TrimSpace(ref.LocalID) == "" && (strings.TrimSpace(ref.CalendarID) == "" || strings.TrimSpace(ref.EventID) == "") {
				return mcp.NewToolResultError("local_id or calendar_id plus event_id is required"), nil
			}
			event, err := c.GetCalendarEventByRef(ref)
			if err != nil {
				if err == sql.ErrNoRows {
					return mcp.NewToolResultText("No cached calendar event found for the provided scoped ref."), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("calendar cache error: %v", err)), nil
			}
			return mcp.NewToolResultText(formatMCPCalendarEventDetail(*event)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("create_calendar_event",
			mcp.WithDescription("Create a calendar event through the running herald daemon"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("calendar_id", mcp.Required(), mcp.Description("Calendar collection ID")),
			mcp.WithString("title", mcp.Required(), mcp.Description("Event title")),
			mcp.WithString("start", mcp.Required(), mcp.Description("Start time, RFC3339 or YYYY-MM-DD HH:MM")),
			mcp.WithString("end", mcp.Required(), mcp.Description("End time, RFC3339 or YYYY-MM-DD HH:MM")),
			mcp.WithString("source_id", mcp.Description("Calendar source ID")),
			mcp.WithString("account_id", mcp.Description("Account ID")),
			mcp.WithString("event_id", mcp.Description("Provider event ID to request")),
			mcp.WithString("timezone", mcp.Description("IANA timezone, for non-RFC3339 start/end values")),
			mcp.WithString("start_timezone", mcp.Description("IANA timezone for the start wall time")),
			mcp.WithString("end_timezone", mcp.Description("IANA timezone for the end wall time")),
			mcp.WithString("location", mcp.Description("Event location")),
			mcp.WithString("description", mcp.Description("Event notes/description")),
			mcp.WithBoolean("all_day", mcp.Description("Whether this is an all-day event")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			for _, key := range []string{"calendar_id", "title", "start", "end"} {
				if _, err := req.RequireString(key); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}
			}
			body := calendarDaemonBody(req)
			respBody, status, err := daemonPost("/v1/calendar/events", body)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusCreated && status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			return mcp.NewToolResultText("Calendar event created: " + strings.TrimSpace(string(respBody))), nil
		},
	)

	s.AddTool(
		mcp.NewTool("update_calendar_event",
			mcp.WithDescription("Update a calendar event through the running herald daemon"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("local_id", mcp.Description("Scoped Herald event local ID from list/search results")),
			mcp.WithString("source_id", mcp.Description("Calendar source ID")),
			mcp.WithString("account_id", mcp.Description("Account ID")),
			mcp.WithString("calendar_id", mcp.Description("Calendar collection ID")),
			mcp.WithString("event_id", mcp.Description("Provider event ID")),
			mcp.WithString("instance_id", mcp.Description("Recurrence instance ID, when present")),
			mcp.WithString("title", mcp.Description("Event title")),
			mcp.WithString("start", mcp.Description("Start time, RFC3339 or YYYY-MM-DD HH:MM")),
			mcp.WithString("end", mcp.Description("End time, RFC3339 or YYYY-MM-DD HH:MM")),
			mcp.WithString("timezone", mcp.Description("IANA timezone, for non-RFC3339 start/end values")),
			mcp.WithString("start_timezone", mcp.Description("IANA timezone for the start wall time")),
			mcp.WithString("end_timezone", mcp.Description("IANA timezone for the end wall time")),
			mcp.WithString("location", mcp.Description("Event location")),
			mcp.WithString("description", mcp.Description("Event notes/description")),
			mcp.WithBoolean("all_day", mcp.Description("Whether this is an all-day event")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !calendarToolHasRef(req) {
				return mcp.NewToolResultError("local_id or calendar_id plus event_id is required"), nil
			}
			body := calendarDaemonBody(req)
			respBody, status, err := daemonPatch(calendarDaemonPath(req), body)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			return mcp.NewToolResultText("Calendar event updated: " + strings.TrimSpace(string(respBody))), nil
		},
	)

	s.AddTool(
		mcp.NewTool("delete_calendar_event",
			mcp.WithDescription("Delete a calendar event through the running herald daemon"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("local_id", mcp.Description("Scoped Herald event local ID from list/search results")),
			mcp.WithString("source_id", mcp.Description("Calendar source ID")),
			mcp.WithString("account_id", mcp.Description("Account ID")),
			mcp.WithString("calendar_id", mcp.Description("Calendar collection ID")),
			mcp.WithString("event_id", mcp.Description("Provider event ID")),
			mcp.WithString("instance_id", mcp.Description("Recurrence instance ID, when present")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if !calendarToolHasRef(req) {
				return mcp.NewToolResultError("local_id or calendar_id plus event_id is required"), nil
			}
			status, err := daemonDelete(calendarDaemonPath(req))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusNoContent {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d", status)), nil
			}
			return mcp.NewToolResultText("Calendar event deleted"), nil
		},
	)
}

func calendarDaemonBody(req mcp.CallToolRequest) map[string]any {
	args := req.GetArguments()
	body := make(map[string]any)
	for _, key := range []string{
		"source_id",
		"account_id",
		"calendar_id",
		"event_id",
		"instance_id",
		"local_id",
		"provider_uid",
		"title",
		"description",
		"location",
		"start",
		"end",
		"timezone",
		"start_timezone",
		"end_timezone",
		"status",
	} {
		if value := strings.TrimSpace(req.GetString(key, "")); value != "" {
			body[key] = value
		}
	}
	if _, ok := args["all_day"]; ok {
		body["all_day"] = req.GetBool("all_day", false)
	}
	return body
}

func calendarToolHasRef(req mcp.CallToolRequest) bool {
	if strings.TrimSpace(req.GetString("local_id", "")) != "" {
		return true
	}
	return strings.TrimSpace(req.GetString("calendar_id", "")) != "" && strings.TrimSpace(calendarToolEventID(req)) != ""
}

func calendarToolEventID(req mcp.CallToolRequest) string {
	if eventID := strings.TrimSpace(req.GetString("event_id", "")); eventID != "" {
		return eventID
	}
	if parsed, ok := models.EventRefFromLocalID(req.GetString("local_id", "")); ok {
		return parsed.EventID
	}
	return ""
}

func calendarDaemonPath(req mcp.CallToolRequest) string {
	eventID := calendarToolEventID(req)
	if eventID == "" {
		eventID = "event"
	}
	values := url.Values{}
	for _, key := range []string{"source_id", "account_id", "calendar_id", "event_id", "instance_id", "local_id"} {
		if value := strings.TrimSpace(req.GetString(key, "")); value != "" {
			values.Set(key, value)
		}
	}
	if values.Get("event_id") == "" && eventID != "event" {
		values.Set("event_id", eventID)
	}
	path := "/v1/calendar/events/" + url.PathEscape(eventID)
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

func clampMCPResultLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func parseMCPCalendarTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid calendar time %q; use RFC3339 or YYYY-MM-DD", value)
	}
	return t, nil
}

func formatMCPCalendarEventList(title string, events []models.CalendarEvent, limit int) string {
	if len(events) == 0 {
		return title + ": no cached events found."
	}
	if len(events) > limit {
		events = events[:limit]
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s (%d results):\n\n", title, len(events)))
	for _, event := range events {
		sb.WriteString(fmt.Sprintf("  %s  %-32s  %s  %s\n",
			mcpCalendarTimeRange(event),
			event.Title,
			mcpCalendarPlace(event),
			mcpEventIDRef(event.EventRef()),
		))
	}
	return sb.String()
}

func formatMCPCalendarEventDetail(event models.CalendarEvent) string {
	ref := event.EventRef()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s\n", event.Title))
	sb.WriteString(fmt.Sprintf("When: %s\n", mcpCalendarTimeRange(event)))
	if event.Location != "" {
		sb.WriteString(fmt.Sprintf("Location: %s\n", event.Location))
	}
	if event.Status != "" {
		sb.WriteString(fmt.Sprintf("Status: %s\n", event.Status))
	}
	if event.Organizer != "" || event.OrganizerEmail != "" {
		sb.WriteString(fmt.Sprintf("Organizer: %s\n", formatMCPPerson(event.Organizer, event.OrganizerEmail)))
	}
	if len(event.Attendees) > 0 {
		attendees := make([]string, 0, len(event.Attendees))
		for _, attendee := range event.Attendees {
			label := formatMCPPerson(attendee.Name, attendee.Email)
			if attendee.RSVP != "" {
				label += fmt.Sprintf(" (%s)", attendee.RSVP)
			}
			attendees = append(attendees, label)
		}
		sb.WriteString(fmt.Sprintf("Attendees: %s\n", strings.Join(attendees, ", ")))
	}
	if event.RecurrenceSummary != "" {
		sb.WriteString(fmt.Sprintf("Recurrence: %s\n", event.RecurrenceSummary))
	}
	if event.Description != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", event.Description))
	}
	sb.WriteString("\n")
	sb.WriteString(mcpEventIDRef(ref))
	return sb.String()
}

func mcpCalendarTimeRange(event models.CalendarEvent) string {
	if event.Start.IsZero() && event.End.IsZero() {
		return "(time unknown)"
	}
	if event.AllDay {
		return event.Start.Format("2006-01-02")
	}
	if event.End.IsZero() {
		return event.Start.Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("%s-%s", event.Start.Format("2006-01-02 15:04"), event.End.Format("15:04"))
}

func mcpCalendarPlace(event models.CalendarEvent) string {
	if event.Location != "" {
		return event.Location
	}
	if event.Status != "" {
		return event.Status
	}
	return "(no location)"
}

func formatMCPPerson(name, email string) string {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	switch {
	case name != "" && email != "":
		return fmt.Sprintf("%s <%s>", name, email)
	case name != "":
		return name
	default:
		return email
	}
}

func mcpEventIDRef(ref models.EventRef) string {
	ref = ref.WithDefaults()
	if ref.EventID == "" && ref.LocalID == "" {
		return "event_id=(missing)"
	}
	return fmt.Sprintf("event_id=%s source_id=%s account_id=%s calendar_id=%s local_id=%s", ref.EventID, ref.SourceID, ref.AccountID, ref.CalendarID, ref.LocalID)
}
