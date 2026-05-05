package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/demo"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
				sb.WriteString(fmt.Sprintf("  %s  %-46s  %s%s  %s\n",
					email.Date.Format("2006-01-02 15:04"), email.Sender, email.Subject, flags, mcpMessageIDRef(email)))
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
				sb.WriteString(fmt.Sprintf("  [%s] %-46s %s  %s\n",
					email.Date.Format("2006-01-02"), email.Sender, email.Subject, mcpMessageIDRef(email)))
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

	s.AddTool(
		mcp.NewTool("dry_run_cleanup_rules",
			mcp.WithDescription("Preview deterministic demo cleanup rule matches without mutating mail"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("rule_id",
				mcp.Description("Optional cleanup rule ID to preview; omitted means all enabled demo cleanup rules"),
			),
			mcp.WithString("folder",
				mcp.Description("Optional folder filter; omitted means all demo folders"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ruleID := int64(req.GetInt("rule_id", 0))
			folder := req.GetString("folder", "")
			report := demoCleanupDryRunReport(ruleID, folder)
			out, _ := json.Marshal(report)
			return mcp.NewToolResultText(string(out)), nil
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

func demoMCPCleanupRules() []*models.CleanupRule {
	return []*models.CleanupRule{
		{
			ID:            1,
			Name:          "Archive old Packet Press",
			MatchType:     "sender",
			MatchValue:    "newsletter@packetpress.example",
			Action:        "archive",
			OlderThanDays: 10,
			Enabled:       true,
			CreatedAt:     time.Now().AddDate(0, 0, -10),
		},
		{
			ID:            2,
			Name:          "Delete old travel offers",
			MatchType:     "domain",
			MatchValue:    "trailpost.example",
			Action:        "delete",
			OlderThanDays: 7,
			Enabled:       true,
			CreatedAt:     time.Now().AddDate(0, 0, -10),
		},
	}
}

func demoCleanupDryRunReport(ruleID int64, folder string) *models.RuleDryRunReport {
	report := &models.RuleDryRunReport{
		Kind:        models.RuleDryRunKindCleanup,
		Scope:       "enabled demo cleanup rules / all folders",
		Folder:      folder,
		DryRun:      true,
		GeneratedAt: time.Now(),
	}
	if ruleID != 0 {
		report.Scope = "selected demo cleanup rules / all folders"
	}
	if folder != "" {
		report.Scope = strings.Replace(report.Scope, "all folders", folder, 1)
	}
	matches := map[string]bool{}
	for _, rule := range demoMCPCleanupRules() {
		if ruleID != 0 && rule.ID != ruleID {
			continue
		}
		if !rule.Enabled && ruleID == 0 {
			continue
		}
		report.RuleCount++
		cutoff := time.Now().AddDate(0, 0, -rule.OlderThanDays)
		for _, email := range demoMCPEmails(folder) {
			if !email.Date.Before(cutoff) || !demoCleanupRuleMatches(rule, email) {
				continue
			}
			matches[email.MessageID] = true
			report.Rows = append(report.Rows, models.RuleDryRunRow{
				RuleID:    rule.ID,
				RuleName:  rule.Name,
				MessageID: email.MessageID,
				Sender:    email.Sender,
				Domain:    demoEmailDomain(email.Sender),
				Folder:    email.Folder,
				Subject:   email.Subject,
				Date:      email.Date,
				Action:    rule.Action,
				Target:    demoCleanupActionTarget(rule.Action),
			})
		}
	}
	report.MatchCount = len(matches)
	report.ActionCount = len(report.Rows)
	return report
}

func demoCleanupRuleMatches(rule *models.CleanupRule, email *models.EmailData) bool {
	switch rule.MatchType {
	case "sender":
		return strings.EqualFold(demoEmailAddress(email.Sender), strings.TrimSpace(rule.MatchValue))
	case "domain":
		return strings.EqualFold(demoEmailDomain(email.Sender), strings.TrimSpace(rule.MatchValue))
	default:
		return false
	}
}

func demoEmailAddress(sender string) string {
	sender = strings.TrimSpace(sender)
	if start := strings.LastIndex(sender, "<"); start >= 0 {
		if end := strings.LastIndex(sender, ">"); end > start {
			return strings.ToLower(strings.TrimSpace(sender[start+1 : end]))
		}
	}
	return strings.ToLower(sender)
}

func demoEmailDomain(sender string) string {
	address := demoEmailAddress(sender)
	if at := strings.LastIndex(address, "@"); at >= 0 && at < len(address)-1 {
		return address[at+1:]
	}
	return ""
}

func demoCleanupActionTarget(action string) string {
	switch action {
	case "archive":
		return "Archive"
	case "delete":
		return "Trash"
	default:
		return ""
	}
}
