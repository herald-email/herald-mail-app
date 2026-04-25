package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"mail-processor/internal/demo"
	"mail-processor/internal/models"
)

func newDemoMCPServer() *server.MCPServer {
	s := server.NewMCPServer(
		"herald-demo",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	s.AddTool(
		mcp.NewTool("list_recent_emails",
			mcp.WithDescription("List deterministic demo emails in a folder"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder", mcp.Required(), mcp.Description("Demo folder name, e.g. INBOX")),
			mcp.WithNumber("limit", mcp.Description("Maximum emails to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			limit := req.GetInt("limit", 20)
			emails := demoMCPEmails(folder)
			if limit > 0 && len(emails) > limit {
				emails = emails[:limit]
			}
			if len(emails) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No demo emails found in %s", folder)), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Recent demo emails in %s (%d results):\n\n", folder, len(emails)))
			for _, email := range emails {
				flags := ""
				if email.HasAttachments {
					flags += " [attach]"
				}
				if !email.IsRead {
					flags += " [unread]"
				}
				sb.WriteString(fmt.Sprintf("  %s  %-46s  %s%s\n",
					email.Date.Format("2006-01-02 15:04"), email.Sender, email.Subject, flags))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	s.AddTool(
		mcp.NewTool("search_emails",
			mcp.WithDescription("Search deterministic demo emails by sender, subject, or body"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder", mcp.Required(), mcp.Description("Demo folder name")),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search term")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			query, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			q := strings.ToLower(query)
			var matches []*models.EmailData
			for _, email := range demoMCPEmails(folder) {
				bodyText := ""
				if body, ok := demo.BodyByMessageID(email.MessageID); ok {
					bodyText = body.TextPlain
				}
				if strings.Contains(strings.ToLower(email.Sender), q) ||
					strings.Contains(strings.ToLower(email.Subject), q) ||
					strings.Contains(strings.ToLower(bodyText), q) {
					matches = append(matches, email)
				}
			}
			if len(matches) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No demo emails found matching %q in %s", query, folder)), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d demo emails matching %q in %s:\n\n", len(matches), query, folder))
			for _, email := range matches {
				sb.WriteString(fmt.Sprintf("  [%s] %-46s %s\n",
					email.Date.Format("2006-01-02"), email.Sender, email.Subject))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_sender_stats",
			mcp.WithDescription("Get deterministic demo sender counts"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder", mcp.Required(), mcp.Description("Demo folder name")),
			mcp.WithNumber("top_n", mcp.Description("Return top N senders")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			topN := req.GetInt("top_n", 20)
			type row struct {
				sender string
				count  int
			}
			counts := make(map[string]int)
			for _, email := range demoMCPEmails(folder) {
				counts[email.Sender]++
			}
			rows := make([]row, 0, len(counts))
			for sender, count := range counts {
				rows = append(rows, row{sender: sender, count: count})
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].count == rows[j].count {
					return rows[i].sender < rows[j].sender
				}
				return rows[i].count > rows[j].count
			})
			if topN > 0 && len(rows) > topN {
				rows = rows[:topN]
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Top %d demo senders in %s:\n\n", len(rows), folder))
			for i, r := range rows {
				sb.WriteString(fmt.Sprintf("  %2d. %-50s %d emails\n", i+1, r.sender, r.count))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	s.AddTool(
		mcp.NewTool("get_email_classifications",
			mcp.WithDescription("Get deterministic demo classification counts"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder", mcp.Required(), mcp.Description("Demo folder name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			counts := make(map[string]int)
			cats := demo.Classifications()
			for _, email := range demoMCPEmails(folder) {
				cat := cats[email.MessageID]
				if cat == "" {
					cat = "unclassified"
				}
				counts[cat]++
			}
			out, _ := json.MarshalIndent(counts, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf("Demo classification summary for %s:\n%s", folder, out)), nil
		},
	)

	s.AddTool(
		mcp.NewTool("list_contacts",
			mcp.WithDescription("List deterministic demo contacts"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("limit", mcp.Description("Maximum contacts to return")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			contacts := demo.Contacts()
			limit := req.GetInt("limit", 20)
			if limit > 0 && len(contacts) > limit {
				contacts = contacts[:limit]
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Demo contacts (%d):\n\n", len(contacts)))
			for _, contact := range contacts {
				sb.WriteString(fmt.Sprintf("  %-24s %-38s %s\n", contact.DisplayName, contact.Email, strings.Join(contact.Topics, ", ")))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	return s
}

func demoMCPEmails(folder string) []*models.EmailData {
	folder = strings.TrimSpace(folder)
	var emails []*models.EmailData
	for _, msg := range demo.Mailbox().Messages {
		if folder == "" || msg.Email.Folder == folder {
			email := msg.Email
			emails = append(emails, &email)
		}
	}
	sort.SliceStable(emails, func(i, j int) bool {
		return emails[i].Date.After(emails[j].Date)
	})
	return emails
}
