package app

import (
	"encoding/json"
	"fmt"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// chatToolRegistry returns the list of tools available in the chat panel
// and a dispatch function to execute a named tool call.
// It uses m.currentFolder at call time; prefer chatToolRegistryWithFolder
// when a snapshot of the folder is needed (e.g. inside a goroutine).
func (m *Model) chatToolRegistry() (tools []ai.Tool, dispatch func(name string, args json.RawMessage) (string, error)) {
	return m.chatToolRegistryWithFolder(m.currentFolder)
}

// chatToolRegistryWithFolder is like chatToolRegistry but uses the provided
// folder snapshot instead of m.currentFolder, avoiding data races in goroutines.
func (m *Model) chatToolRegistryWithFolder(currentFolder string) (tools []ai.Tool, dispatch func(name string, args json.RawMessage) (string, error)) {
	tools = []ai.Tool{
		{
			Name:        "search_emails",
			Description: "Search emails by keyword in subject, sender, or body preview",
			Parameters: ai.ToolParams{
				Type: "object",
				Properties: map[string]ai.ToolProp{
					"query": {Type: "string", Description: "Search query"},
				},
				Required: []string{"query"},
			},
		},
		{
			Name:        "list_emails_by_sender",
			Description: "Get all emails from a specific sender",
			Parameters: ai.ToolParams{
				Type: "object",
				Properties: map[string]ai.ToolProp{
					"sender": {Type: "string", Description: "Sender email address"},
				},
				Required: []string{"sender"},
			},
		},
		{
			Name:        "get_thread",
			Description: "Get all emails in a thread by subject",
			Parameters: ai.ToolParams{
				Type: "object",
				Properties: map[string]ai.ToolProp{
					"subject": {Type: "string", Description: "Thread subject"},
					"folder":  {Type: "string", Description: "Folder name (default: INBOX)"},
				},
				Required: []string{"subject"},
			},
		},
		{
			Name:        "get_sender_stats",
			Description: "Get email volume statistics per sender",
			Parameters:  ai.ToolParams{Type: "object", Properties: map[string]ai.ToolProp{}},
		},
	}

	dispatch = func(name string, args json.RawMessage) (string, error) {
		switch name {
		case "search_emails":
			var params struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			emails, err := m.backend.SearchEmails(currentFolder, params.Query, false)
			if err != nil {
				return "", fmt.Errorf("search_emails: %w", err)
			}
			if len(emails) > 20 {
				emails = emails[:20]
			}
			return marshalEmailList(emails)

		case "list_emails_by_sender":
			var params struct {
				Sender string `json:"sender"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			emails, err := m.backend.SearchEmails(currentFolder, params.Sender, false)
			if err != nil {
				return "", fmt.Errorf("list_emails_by_sender: %w", err)
			}
			// Filter to only matching senders
			var filtered []*models.EmailData
			for _, e := range emails {
				if e.Sender == params.Sender {
					filtered = append(filtered, e)
				}
			}
			if len(filtered) > 20 {
				filtered = filtered[:20]
			}
			return marshalEmailList(filtered)

		case "get_thread":
			var params struct {
				Subject string `json:"subject"`
				Folder  string `json:"folder"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			folder := params.Folder
			if folder == "" {
				folder = currentFolder
			}
			emails, err := m.backend.GetEmailsByThread(folder, params.Subject)
			if err != nil {
				return "", fmt.Errorf("get_thread: %w", err)
			}
			if len(emails) > 20 {
				emails = emails[:20]
			}
			return marshalEmailList(emails)

		case "get_sender_stats":
			stats, err := m.backend.GetSenderStatistics(currentFolder)
			if err != nil {
				return "", fmt.Errorf("get_sender_stats: %w", err)
			}
			type statEntry struct {
				Sender string `json:"sender"`
				Total  int    `json:"total_emails"`
			}
			var entries []statEntry
			for sender, s := range stats {
				entries = append(entries, statEntry{Sender: sender, Total: s.TotalEmails})
				if len(entries) >= 20 {
					break
				}
			}
			out, err := json.Marshal(entries)
			if err != nil {
				return "", fmt.Errorf("marshal sender stats: %w", err)
			}
			return string(out), nil
		}
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return
}

// marshalEmailList serialises a slice of EmailData to a compact JSON string.
func marshalEmailList(emails []*models.EmailData) (string, error) {
	type emailSummary struct {
		MessageID string `json:"message_id"`
		Sender    string `json:"sender"`
		Subject   string `json:"subject"`
		Date      string `json:"date"`
		Folder    string `json:"folder"`
	}
	var summaries []emailSummary
	for _, e := range emails {
		summaries = append(summaries, emailSummary{
			MessageID: e.MessageID,
			Sender:    e.Sender,
			Subject:   e.Subject,
			Date:      e.Date.Format("2006-01-02"),
			Folder:    e.Folder,
		})
	}
	out, err := json.Marshal(summaries)
	if err != nil {
		return "", fmt.Errorf("marshal emails: %w", err)
	}
	return string(out), nil
}
