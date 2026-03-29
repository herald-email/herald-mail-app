// mcp-server exposes herald email operations as MCP tools over stdio.
// Usage: ./mcp-server [-config ~/.herald/conf.yaml]
// Add to Claude Code's MCP config to let Claude search and manage your email.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"mail-processor/internal/ai"
	"mail-processor/internal/cache"
	"mail-processor/internal/config"
)

func main() {
	configPath := flag.String("config", "~/.herald/conf.yaml", "Path to configuration file")
	flag.Parse()

	expanded, err := config.ExpandPath(*configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	cfg, err := config.Load(expanded)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	c, err := cache.New("email_cache.db")
	if err != nil {
		log.Fatalf("Failed to open cache: %v", err)
	}
	defer c.Close()

	// Classifier is optional — nil if Ollama is not configured
	var classifier *ai.Classifier
	if cfg.Ollama.Host != "" {
		classifier = ai.New(cfg.Ollama.Host, cfg.Ollama.Model)
	}

	s := server.NewMCPServer(
		"herald",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// Tool: list_recent_emails
	s.AddTool(
		mcp.NewTool("list_recent_emails",
			mcp.WithDescription("List the most recent emails in a folder, sorted newest first"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder",
				mcp.Required(),
				mcp.Description("IMAP folder name, e.g. INBOX"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum emails to return (default 20, max 100)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			limit := req.GetInt("limit", 20)
			if limit > 100 {
				limit = 100
			}

			emails, err := c.GetEmailsSortedByDate(folder)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("cache error: %v", err)), nil
			}
			if len(emails) > limit {
				emails = emails[:limit]
			}

			var sb strings.Builder
			if len(emails) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No emails found in %s (folder not found or empty)", folder)), nil
			}
			sb.WriteString(fmt.Sprintf("Recent emails in %s (%d results):\n\n", folder, len(emails)))
			for _, e := range emails {
				att := ""
				if e.HasAttachments {
					att = " [attach]"
				}
				unread := ""
				if !e.IsRead {
					unread = " [unread]"
				}
				sb.WriteString(fmt.Sprintf("  %s  %-40s  %s%s%s\n",
					e.Date.Format("2006-01-02 15:04"), e.Sender, e.Subject, att, unread))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: search_emails
	s.AddTool(
		mcp.NewTool("search_emails",
			mcp.WithDescription("Search emails by sender or subject keyword (case-insensitive, up to 100 results)"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder",
				mcp.Required(),
				mcp.Description("IMAP folder name"),
			),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Search term matched against sender address and subject"),
			),
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

			emails, err := c.SearchEmails(folder, query)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("search error: %v", err)), nil
			}
			if len(emails) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No emails found matching %q in %s", query, folder)), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d emails matching %q in %s:\n\n", len(emails), query, folder))
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("  [%s]  From: %-45s  %s\n",
					e.Date.Format("2006-01-02"), e.Sender, e.Subject))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: get_sender_stats
	s.AddTool(
		mcp.NewTool("get_sender_stats",
			mcp.WithDescription("Get per-sender email statistics for a folder (count, sorted by volume)"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder",
				mcp.Required(),
				mcp.Description("IMAP folder name"),
			),
			mcp.WithNumber("top_n",
				mcp.Description("Return only the top N senders by email count (default 20)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			topN := req.GetInt("top_n", 20)

			emailsBySender, err := c.GetAllEmails(folder, false)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("stats error: %v", err)), nil
			}

			type row struct {
				sender string
				count  int
			}
			rows := make([]row, 0, len(emailsBySender))
			for sender, emails := range emailsBySender {
				rows = append(rows, row{sender, len(emails)})
			}
			for i := 1; i < len(rows); i++ {
				for j := i; j > 0 && rows[j].count > rows[j-1].count; j-- {
					rows[j], rows[j-1] = rows[j-1], rows[j]
				}
			}
			if len(rows) > topN {
				rows = rows[:topN]
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Top %d senders in %s:\n\n", len(rows), folder))
			for i, r := range rows {
				sb.WriteString(fmt.Sprintf("  %3d. %-50s  %d emails\n", i+1, r.sender, r.count))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: get_email_classifications
	s.AddTool(
		mcp.NewTool("get_email_classifications",
			mcp.WithDescription("Get AI classification tag summary for emails in a folder"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder",
				mcp.Required(),
				mcp.Description("IMAP folder name"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			tags, err := c.GetClassifications(folder)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(tags) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf(
					"No classifications found for %s. Open the TUI and press 'a' to classify.", folder)), nil
			}

			counts := make(map[string]int)
			for _, cat := range tags {
				if cat == "" {
					cat = "unclassified"
				}
				counts[cat]++
			}
			out, _ := json.MarshalIndent(counts, "", "  ")
			return mcp.NewToolResultText(fmt.Sprintf(
				"Classification summary for %s (%d tagged):\n%s", folder, len(tags), string(out))), nil
		},
	)

	// Tool: get_email_body
	s.AddTool(
		mcp.NewTool("get_email_body",
			mcp.WithDescription("Get the cached plain-text body of a specific email by message ID"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id",
				mcp.Required(),
				mcp.Description("The Message-ID of the email (from list_recent_emails or search_emails)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, err := c.GetBodyText(msgID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if body == "" {
				return mcp.NewToolResultText("Body not cached. Open the email in the TUI to load its body."), nil
			}
			return mcp.NewToolResultText(body), nil
		},
	)

	// Tool: list_unread_emails
	s.AddTool(
		mcp.NewTool("list_unread_emails",
			mcp.WithDescription("List unread emails in a folder, newest first"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder",
				mcp.Required(),
				mcp.Description("IMAP folder name"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum emails to return (default 20, max 100)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			limit := req.GetInt("limit", 20)
			if limit > 100 {
				limit = 100
			}
			emails, err := c.GetUnreadEmails(folder, limit)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(emails) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No unread emails in %s", folder)), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Unread emails in %s (%d):\n\n", folder, len(emails)))
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("  %s  %-40s  %s\n",
					e.Date.Format("2006-01-02 15:04"), e.Sender, e.Subject))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: search_by_date
	s.AddTool(
		mcp.NewTool("search_by_date",
			mcp.WithDescription("Find emails in a folder within an optional date range (ISO 8601)"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder",
				mcp.Required(),
				mcp.Description("IMAP folder name"),
			),
			mcp.WithString("after",
				mcp.Description("Only include emails after this date, ISO 8601 (e.g. 2024-01-01)"),
			),
			mcp.WithString("before",
				mcp.Description("Only include emails before this date, ISO 8601 (e.g. 2024-12-31)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var after, before time.Time
			if s := req.GetString("after", ""); s != "" {
				after, err = time.Parse("2006-01-02", s)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid 'after' date: %v", err)), nil
				}
			}
			if s := req.GetString("before", ""); s != "" {
				before, err = time.Parse("2006-01-02", s)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid 'before' date: %v", err)), nil
				}
			}
			emails, err := c.SearchByDate(folder, after, before)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(emails) == 0 {
				return mcp.NewToolResultText("No emails found in that date range"), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d emails in %s:\n\n", len(emails), folder))
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("  %s  %-40s  %s\n",
					e.Date.Format("2006-01-02 15:04"), e.Sender, e.Subject))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: search_by_sender
	s.AddTool(
		mcp.NewTool("search_by_sender",
			mcp.WithDescription("Find all emails matching a sender address or domain, optionally scoped to a folder"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("sender",
				mcp.Required(),
				mcp.Description("Sender address or domain substring to match"),
			),
			mcp.WithString("folder",
				mcp.Description("IMAP folder to scope the search (omit for all folders)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sender, err := req.RequireString("sender")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder := req.GetString("folder", "")
			emails, err := c.SearchBySender(sender, folder)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(emails) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No emails found from %q", sender)), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d emails from %q:\n\n", len(emails), sender))
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("  [%s] %-20s  %s  %s\n",
					e.Folder, e.Date.Format("2006-01-02"), e.Sender, e.Subject))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: semantic_search_emails
	s.AddTool(
		mcp.NewTool("semantic_search_emails",
			mcp.WithDescription("Find emails by semantic similarity (natural language query). Requires Ollama with nomic-embed-text."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Natural language query, e.g. 'emails about my lease renewal'"),
			),
			mcp.WithString("folder",
				mcp.Required(),
				mcp.Description("IMAP folder to search"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Max results (default 10)"),
			),
			mcp.WithNumber("min_score",
				mcp.Description("Minimum cosine similarity 0.0-1.0 (default 0.5)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if classifier == nil {
				return mcp.NewToolResultText("Ollama not configured — set ollama.host in ~/.herald/conf.yaml"), nil
			}
			query, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			limit := req.GetInt("limit", 10)
			minScore := req.GetFloat("min_score", 0.5)

			vec, err := classifier.Embed(query)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("embedding error: %v", err)), nil
			}
			emails, err := c.SearchSemantic(folder, vec, limit, minScore)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("search error: %v", err)), nil
			}
			if len(emails) == 0 {
				return mcp.NewToolResultText("No semantically similar emails found"), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d emails semantically similar to %q:\n\n", len(emails), query))
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("  %s  %-40s  %s\n",
					e.Date.Format("2006-01-02 15:04"), e.Sender, e.Subject))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: classify_email
	s.AddTool(
		mcp.NewTool("classify_email",
			mcp.WithDescription("Run AI classification on a single email and persist the category. Requires Ollama."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id",
				mcp.Required(),
				mcp.Description("The Message-ID of the email to classify"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if classifier == nil {
				return mcp.NewToolResultText("Ollama not configured — set ollama.host in ~/.herald/conf.yaml"), nil
			}
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			email, err := c.GetEmailByID(msgID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("email not found: %v", err)), nil
			}
			cat, err := classifier.Classify(email.Sender, email.Subject)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("classification error: %v", err)), nil
			}
			if err := c.SetClassification(msgID, string(cat)); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to save classification: %v", err)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Classified as: %s", cat)), nil
		},
	)

	// Tool: summarise_email
	s.AddTool(
		mcp.NewTool("summarise_email",
			mcp.WithDescription("Generate a concise summary of an email body using the local Ollama model. Requires cached body text."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id",
				mcp.Required(),
				mcp.Description("The Message-ID of the email to summarise"),
			),
			mcp.WithNumber("max_words",
				mcp.Description("Maximum words in the summary (default 100)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if classifier == nil {
				return mcp.NewToolResultText("Ollama not configured — set ollama.host in ~/.herald/conf.yaml"), nil
			}
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			maxWords := req.GetInt("max_words", 100)

			body, err := c.GetBodyText(msgID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if body == "" {
				return mcp.NewToolResultText("Body not cached. Open the email in the TUI to load its body first."), nil
			}

			prompt := fmt.Sprintf("Summarise the following email in at most %d words:\n\n%s", maxWords, body)
			summary, err := classifier.Chat([]ai.ChatMessage{
				{Role: "user", Content: prompt},
			})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Ollama error: %v", err)), nil
			}
			return mcp.NewToolResultText(summary), nil
		},
	)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
