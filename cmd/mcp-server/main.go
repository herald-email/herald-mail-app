// mcp-server exposes email operations as MCP tools over stdio.
// Usage: ./mcp-server [-config proton.yaml]
// Add to Claude Code's MCP config to let Claude search and manage your email.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"mail-processor/internal/cache"
	"mail-processor/internal/config"
)

func main() {
	configPath := flag.String("config", "proton.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	_ = cfg

	c, err := cache.New("email_cache.db")
	if err != nil {
		log.Fatalf("Failed to open cache: %v", err)
	}
	defer c.Close()

	s := server.NewMCPServer(
		"mail-processor",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// Tool: list_recent_emails
	s.AddTool(
		mcp.NewTool("list_recent_emails",
			mcp.WithDescription("List the most recent emails in a folder, sorted newest first"),
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
			sb.WriteString(fmt.Sprintf("Recent emails in %s (%d results):\n\n", folder, len(emails)))
			for _, e := range emails {
				att := ""
				if e.HasAttachments {
					att = " [attach]"
				}
				sb.WriteString(fmt.Sprintf("  %s  %-40s  %s%s\n",
					e.Date.Format("2006-01-02 15:04"), e.Sender, e.Subject, att))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: search_emails
	s.AddTool(
		mcp.NewTool("search_emails",
			mcp.WithDescription("Search emails by sender or subject keyword (case-insensitive, up to 100 results)"),
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

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
