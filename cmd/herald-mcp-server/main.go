// herald-mcp-server exposes herald email operations as MCP tools over stdio.
// Usage: ./herald-mcp-server [-config ~/.herald/conf.yaml]
// Add to Claude Code's MCP config to let Claude search and manage your email.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"mail-processor/internal/ai"
	"mail-processor/internal/cache"
	"mail-processor/internal/config"
	"mail-processor/internal/models"
	rulesengine "mail-processor/internal/rules"
	buildversion "mail-processor/internal/version"
)

// daemonURL is the base URL of the running herald daemon.
// Empty string means daemon is not available; write operations will fail gracefully.
var daemonURL string

// probeDaemon checks if the daemon is running and sets daemonURL.
func probeDaemon(port int) {
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/v1/status")
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		daemonURL = url
		log.Printf("daemon available at %s", daemonURL)
	}
}

// daemonPost makes a POST to the daemon. Returns error if daemon unavailable.
func daemonPost(path string, body any) ([]byte, int, error) {
	if daemonURL == "" {
		return nil, 0, fmt.Errorf("daemon not running — start herald daemon first")
	}
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, 0, err
		}
	}
	resp, err := http.Post(daemonURL+path, "application/json", &buf)
	if err != nil {
		return nil, 0, fmt.Errorf("daemon unreachable: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// daemonGet makes a GET to the daemon. Returns error if daemon unavailable.
func daemonGet(path string) ([]byte, int, error) {
	if daemonURL == "" {
		return nil, 0, fmt.Errorf("daemon not running — start herald daemon first")
	}
	resp, err := http.Get(daemonURL + path)
	if err != nil {
		return nil, 0, fmt.Errorf("daemon unreachable: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// daemonDelete makes a DELETE to the daemon.
func daemonDelete(path string) (int, error) {
	if daemonURL == "" {
		return 0, fmt.Errorf("daemon not running — start herald daemon first")
	}
	req, err := http.NewRequest(http.MethodDelete, daemonURL+path, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func main() {
	configPath := flag.String("config", "~/.herald/conf.yaml", "Path to configuration file")
	demoMode := flag.Bool("demo", false, "Serve deterministic synthetic demo data without loading config or cache")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildversion.String("herald-mcp-server"))
		return
	}

	if *demoMode {
		if err := server.ServeStdio(newDemoMCPServer()); err != nil {
			log.Fatalf("MCP demo server error: %v", err)
		}
		return
	}

	expanded, err := config.ExpandPath(*configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	cfg, err := config.Load(expanded)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	cachePath, err := config.EnsureCacheDatabasePath(expanded, cfg)
	if err != nil {
		log.Fatalf("Failed to resolve cache path: %v", err)
	}
	c, err := cache.New(cachePath)
	if err != nil {
		log.Fatalf("Failed to open cache: %v", err)
	}
	defer c.Close()
	if _, err := c.EnsureEmbeddingModel(cfg.EffectiveEmbeddingModel()); err != nil {
		log.Fatalf("Failed to initialize embedding model state: %v", err)
	}

	// Classifier is optional — nil if no AI backend is configured
	classifier, err := ai.NewFromConfig(cfg)
	if err != nil {
		log.Fatalf("Failed to init AI client: %v", err)
	}

	probeDaemon(cfg.Daemon.Port)

	s := server.NewMCPServer(
		"herald",
		buildversion.Version,
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
			mcp.WithDescription("Find emails by semantic similarity (natural language query). Requires Ollama with a configured embedding model (default: nomic-embed-text-v2-moe)."),
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

			vec, err := classifier.Embed(ai.BuildQueryText(query))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("embedding error: %v", err)), nil
			}
			results, err := c.SearchSemanticChunked(folder, vec, limit, minScore)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("search error: %v", err)), nil
			}
			if len(results) == 0 {
				return mcp.NewToolResultText("No semantically similar emails found"), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d emails semantically similar to %q:\n\n", len(results), query))
			for _, result := range results {
				pct := int(result.Score * 100)
				sb.WriteString(fmt.Sprintf("  [%d%%] %s  %-40s  %s\n",
					pct, result.Email.Date.Format("2006-01-02"), result.Email.Sender, result.Email.Subject))
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

	// Tool: classify_folder
	s.AddTool(
		mcp.NewTool("classify_folder",
			mcp.WithDescription("Batch classify all unclassified emails in a folder using AI. Requires Ollama and a running herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder",
				mcp.Required(),
				mcp.Description("IMAP folder name (e.g. INBOX)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := daemonPost("/v1/classify/folder", map[string]string{"folder": folder})
			if err != nil {
				return mcp.NewToolResultText("Error: " + err.Error()), nil
			}
			if status != 200 {
				return mcp.NewToolResultText(fmt.Sprintf("Daemon error (HTTP %d): %s", status, string(body))), nil
			}
			var result map[string]int
			if err := json.Unmarshal(body, &result); err != nil {
				return mcp.NewToolResultText(string(body)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf(
				"Classified %d emails in %s (%d skipped, %d total)",
				result["classified"], folder, result["skipped"], result["total"],
			)), nil
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

	// Tool: list_rules
	s.AddTool(mcp.NewTool("list_rules",
		mcp.WithDescription("List all enabled email automation rules"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rules, err := c.GetEnabledRules()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get rules: %v", err)), nil
		}
		data, _ := json.Marshal(rules)
		return mcp.NewToolResultText(string(data)), nil
	})

	// Tool: add_rule
	s.AddTool(mcp.NewTool("add_rule",
		mcp.WithDescription("Create a new email automation rule"),
		mcp.WithString("trigger_type", mcp.Required(), mcp.Description("sender | domain | category")),
		mcp.WithString("trigger_value", mcp.Required(), mcp.Description("The value to match (email address, domain, or category name)")),
		mcp.WithString("name", mcp.Description("Rule name (optional, auto-generated if empty)")),
		mcp.WithString("actions", mcp.Required(), mcp.Description("JSON array of RuleAction objects")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		triggerType, err := req.RequireString("trigger_type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		triggerValue, err := req.RequireString("trigger_value")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		actionsJSON, err := req.RequireString("actions")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name := req.GetString("name", "")
		if name == "" {
			name = triggerType + ": " + triggerValue
		}

		var actions []models.RuleAction
		if err := json.Unmarshal([]byte(actionsJSON), &actions); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("parse actions JSON: %v", err)), nil
		}

		rule := &models.Rule{
			Name:         name,
			Enabled:      true,
			TriggerType:  models.RuleTriggerType(triggerType),
			TriggerValue: triggerValue,
			Actions:      actions,
		}
		if err := c.SaveRule(rule); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("save rule: %v", err)), nil
		}
		data, _ := json.Marshal(rule)
		return mcp.NewToolResultText(string(data)), nil
	})

	// Tool: run_rules
	s.AddTool(mcp.NewTool("run_rules",
		mcp.WithDescription("Evaluate all automation rules against cached emails in a folder (dry run — no IMAP actions)"),
		mcp.WithString("folder", mcp.Description("Folder to process (default: INBOX)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		folder := req.GetString("folder", "INBOX")
		if folder == "" {
			folder = "INBOX"
		}

		rules, err := c.GetEnabledRules()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get rules: %v", err)), nil
		}

		emails, err := c.GetEmailsSortedByDate(folder)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get emails: %v", err)), nil
		}

		classifications, err := c.GetClassifications(folder)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get classifications: %v", err)), nil
		}

		type result struct {
			MessageID  string `json:"message_id"`
			RulesFired int    `json:"rules_fired"`
		}
		var results []result
		for _, email := range emails {
			category := classifications[email.MessageID]
			fired := 0
			for _, rule := range rules {
				if rulesengine.MatchRule(rule, email, category) {
					fired++
				}
			}
			if fired > 0 {
				results = append(results, result{MessageID: email.MessageID, RulesFired: fired})
			}
		}

		data, _ := json.Marshal(results)
		return mcp.NewToolResultText(string(data)), nil
	})

	// Tool: list_contacts
	s.AddTool(
		mcp.NewTool("list_contacts",
			mcp.WithDescription("List contacts sorted by recency, with email count and enrichment data"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of contacts to return (default 50)"),
			),
			mcp.WithString("sort_by",
				mcp.Description("Sort order: last_seen, name, or email_count (default last_seen)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			limit := req.GetInt("limit", 50)
			sortBy := req.GetString("sort_by", "last_seen")
			switch sortBy {
			case "last_seen", "name", "email_count":
				// valid
			default:
				return mcp.NewToolResultError(fmt.Sprintf("invalid sort_by %q: valid values are last_seen, name, email_count", sortBy)), nil
			}
			contacts, err := c.ListContacts(limit, sortBy)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list contacts error: %v", err)), nil
			}
			if len(contacts) == 0 {
				return mcp.NewToolResultText("No contacts found"), nil
			}
			return mcp.NewToolResultText(formatContacts(contacts)), nil
		},
	)

	// Tool: search_contacts
	s.AddTool(
		mcp.NewTool("search_contacts",
			mcp.WithDescription("Search contacts by name, email, company, or topics (keyword search)"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Search terms"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := req.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			contacts, err := c.SearchContacts(query)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("search contacts error: %v", err)), nil
			}
			if len(contacts) == 0 {
				return mcp.NewToolResultText(fmt.Sprintf("No contacts found matching %q", query)), nil
			}
			return mcp.NewToolResultText(formatContacts(contacts)), nil
		},
	)

	// Tool: semantic_search_contacts
	s.AddTool(
		mcp.NewTool("semantic_search_contacts",
			mcp.WithDescription("Semantic search over contacts using natural language (finds contacts by topics, company, or communication context). Requires Ollama with a configured embedding model (default: nomic-embed-text-v2-moe)."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Natural language query"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Max results (default 10)"),
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
			limit := req.GetInt("limit", 10)
			vec, err := classifier.Embed(ai.BuildQueryText(query))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("embedding error: %v", err)), nil
			}
			results, err := c.SearchContactsSemantic(vec, limit, 0.3)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("semantic search error: %v", err)), nil
			}
			if len(results) == 0 {
				return mcp.NewToolResultText("No semantically similar contacts found"), nil
			}
			return mcp.NewToolResultText(formatContactsWithScores(results)), nil
		},
	)

	// Tool: get_contact
	s.AddTool(
		mcp.NewTool("get_contact",
			mcp.WithDescription("Get full contact profile including recent emails"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("email",
				mcp.Required(),
				mcp.Description("Contact's email address"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			email, err := req.RequireString("email")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			contact, err := c.GetContactByEmail(email)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("lookup error: %v", err)), nil
			}
			if contact == nil {
				return mcp.NewToolResultText(fmt.Sprintf("No contact found for %q", email)), nil
			}
			recentEmails, err := c.GetContactEmails(email, 10)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("email lookup error: %v", err)), nil
			}

			var sb strings.Builder
			sb.WriteString("=== Contact Profile ===\n")
			if contact.DisplayName != "" {
				sb.WriteString(fmt.Sprintf("Name: %s\n", contact.DisplayName))
			}
			sb.WriteString(fmt.Sprintf("Email: %s\n", contact.Email))
			if contact.Company != "" {
				sb.WriteString(fmt.Sprintf("Company: %s\n", contact.Company))
			}
			if len(contact.Topics) > 0 {
				sb.WriteString(fmt.Sprintf("Topics: %s\n", strings.Join(contact.Topics, ", ")))
			}
			if contact.Notes != "" {
				sb.WriteString(fmt.Sprintf("Notes: %s\n", contact.Notes))
			}
			sb.WriteString("\nStats:\n")
			sb.WriteString(fmt.Sprintf("First seen: %s\n", contact.FirstSeen.Format("2006-01-02")))
			sb.WriteString(fmt.Sprintf("Last seen: %s\n", contact.LastSeen.Format("2006-01-02")))
			sb.WriteString(fmt.Sprintf("Emails received: %d\n", contact.EmailCount))
			sb.WriteString(fmt.Sprintf("Emails sent: %d\n", contact.SentCount))
			if contact.EnrichedAt != nil {
				sb.WriteString("Enriched: yes\n")
			} else {
				sb.WriteString("Enriched: no\n")
			}
			if len(recentEmails) > 0 {
				sb.WriteString(fmt.Sprintf("\nRecent Emails (%d):\n", len(recentEmails)))
				for _, e := range recentEmails {
					sb.WriteString(fmt.Sprintf("- %s %s\n", e.Date.Format("2006-01-02"), e.Subject))
				}
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: list_folders
	s.AddTool(
		mcp.NewTool("list_folders",
			mcp.WithDescription("List all email folders available in the local cache"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folders, err := c.GetCachedFolders()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(folders) == 0 {
				return mcp.NewToolResultText("No folders found in cache. Open the TUI to sync first."), nil
			}
			return mcp.NewToolResultText(strings.Join(folders, "\n")), nil
		},
	)

	// Tool: get_server_info
	s.AddTool(
		mcp.NewTool("get_server_info",
			mcp.WithDescription("Get herald configuration and status information"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var sb strings.Builder
			sb.WriteString("Herald Server Info:\n")
			sb.WriteString(fmt.Sprintf("IMAP: %s:%d\n", cfg.Server.Host, cfg.Server.Port))
			sb.WriteString(fmt.Sprintf("AI provider: %s\n", cfg.AI.Provider))
			if cfg.Ollama.Host != "" {
				sb.WriteString(fmt.Sprintf("Ollama: %s (%s)\n", cfg.Ollama.Host, cfg.Ollama.Model))
			}
			if daemonURL != "" {
				sb.WriteString(fmt.Sprintf("Daemon: %s (running)\n", daemonURL))
			} else {
				sb.WriteString(fmt.Sprintf("Daemon: port %d (not running)\n", cfg.Daemon.Port))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: mark_read
	s.AddTool(
		mcp.NewTool("mark_read",
			mcp.WithDescription("Mark an email as read. Requires the herald daemon to be running."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID")),
			mcp.WithString("folder", mcp.Description("Folder name (default: INBOX)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder := req.GetString("folder", "INBOX")
			_, status, err := daemonPost(fmt.Sprintf("/v1/emails/%s/read?folder=%s", msgID, folder), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusNoContent {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d", status)), nil
			}
			return mcp.NewToolResultText("Marked as read"), nil
		},
	)

	// Tool: mark_unread
	s.AddTool(
		mcp.NewTool("mark_unread",
			mcp.WithDescription("Mark an email as unread. Requires the herald daemon to be running."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID")),
			mcp.WithString("folder", mcp.Description("Folder name (default: INBOX)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder := req.GetString("folder", "INBOX")
			_, status, err := daemonPost(fmt.Sprintf("/v1/emails/%s/unread?folder=%s", msgID, folder), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusNoContent {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d", status)), nil
			}
			return mcp.NewToolResultText("Marked as unread"), nil
		},
	)

	// Tool: delete_email
	s.AddTool(
		mcp.NewTool("delete_email",
			mcp.WithDescription("Delete an email. Requires the herald daemon to be running."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID")),
			mcp.WithString("folder", mcp.Description("Folder name (default: INBOX)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder := req.GetString("folder", "INBOX")
			status, err := daemonDelete(fmt.Sprintf("/v1/emails/%s?folder=%s", msgID, folder))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusNoContent {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d", status)), nil
			}
			return mcp.NewToolResultText("Email deleted"), nil
		},
	)

	// Tool: archive_email
	s.AddTool(
		mcp.NewTool("archive_email",
			mcp.WithDescription("Archive an email (move to Archive folder). Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID")),
			mcp.WithString("folder", mcp.Description("Source folder (default: INBOX)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder := req.GetString("folder", "INBOX")
			_, status, err := daemonPost(fmt.Sprintf("/v1/emails/%s/archive?folder=%s", msgID, folder), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusNoContent {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d", status)), nil
			}
			return mcp.NewToolResultText("Email archived"), nil
		},
	)

	// Tool: move_email
	s.AddTool(
		mcp.NewTool("move_email",
			mcp.WithDescription("Move an email to a different folder. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID")),
			mcp.WithString("from_folder", mcp.Required(), mcp.Description("Source folder")),
			mcp.WithString("to_folder", mcp.Required(), mcp.Description("Destination folder")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			fromFolder, err := req.RequireString("from_folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			toFolder, err := req.RequireString("to_folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, status, err := daemonPost(fmt.Sprintf("/v1/emails/%s/move", msgID), map[string]string{
				"fromFolder": fromFolder,
				"toFolder":   toFolder,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusNoContent {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d", status)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Moved to %s", toFolder)), nil
		},
	)

	// Tool: sync_folder
	s.AddTool(
		mcp.NewTool("sync_folder",
			mcp.WithDescription("Trigger an IMAP sync for a folder. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder", mcp.Description("Folder to sync (default: INBOX)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder := req.GetString("folder", "INBOX")
			_, status, err := daemonPost("/v1/sync", map[string]string{"folder": folder})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusAccepted {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d", status)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Sync started for %s", folder)), nil
		},
	)

	// Tool: get_thread
	s.AddTool(
		mcp.NewTool("get_thread",
			mcp.WithDescription("Get all emails in the same thread (by subject) in a folder"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("subject", mcp.Required(), mcp.Description("Email subject (Re:/Fwd: prefixes are stripped automatically)")),
			mcp.WithString("folder", mcp.Description("Folder to search (default: INBOX)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			subject, err := req.RequireString("subject")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder := req.GetString("folder", "INBOX")
			emails, err := c.GetEmailsByThread(folder, subject)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(emails) == 0 {
				return mcp.NewToolResultText("No thread found for that subject"), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Thread: %q — %d emails in %s\n\n", subject, len(emails), folder))
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("  %s  %-40s  %s\n",
					e.Date.Format("2006-01-02 15:04"), e.Sender, e.Subject))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: send_email
	s.AddTool(
		mcp.NewTool("send_email",
			mcp.WithDescription("Send an email via SMTP. Requires the herald daemon to be running."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient email address")),
			mcp.WithString("subject", mcp.Required(), mcp.Description("Email subject")),
			mcp.WithString("body", mcp.Required(), mcp.Description("Email body (plain text)")),
			mcp.WithString("from", mcp.Description("Sender address (optional, uses configured account)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			to, err := req.RequireString("to")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			subject, err := req.RequireString("subject")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, err := req.RequireString("body")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			from := req.GetString("from", "")
			_, status, err := daemonPost("/v1/emails/send", map[string]string{
				"to": to, "subject": subject, "body": body, "from": from,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d", status)), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Email sent to %s", to)), nil
		},
	)

	// Tool: summarise_thread
	s.AddTool(
		mcp.NewTool("summarise_thread",
			mcp.WithDescription("Generate a summary of an email thread. Requires AI configured."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("subject", mcp.Required(), mcp.Description("Thread subject")),
			mcp.WithString("folder", mcp.Description("Folder (default: INBOX)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if classifier == nil {
				return mcp.NewToolResultText("AI not configured — set ollama.host or claude.api_key in config"), nil
			}
			subject, err := req.RequireString("subject")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder := req.GetString("folder", "INBOX")
			emails, err := c.GetEmailsByThread(folder, subject)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(emails) == 0 {
				return mcp.NewToolResultText("No thread found"), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Thread with %d emails. Summarise concisely:\n\n", len(emails)))
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("From: %s | Date: %s | Subject: %s\n",
					e.Sender, e.Date.Format("2006-01-02"), e.Subject))
				bodyText, _ := c.GetBodyText(e.MessageID)
				if bodyText != "" {
					if len(bodyText) > 300 {
						bodyText = bodyText[:300] + "..."
					}
					sb.WriteString(bodyText + "\n\n")
				}
			}
			summary, err := classifier.Chat([]ai.ChatMessage{{Role: "user", Content: sb.String()}})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("AI error: %v", err)), nil
			}
			return mcp.NewToolResultText(summary), nil
		},
	)

	// Tool: extract_action_items
	s.AddTool(
		mcp.NewTool("extract_action_items",
			mcp.WithDescription("Extract action items from an email as a JSON array. Requires AI and cached body."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if classifier == nil {
				return mcp.NewToolResultText("AI not configured"), nil
			}
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			bodyText, err := c.GetBodyText(msgID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if bodyText == "" {
				return mcp.NewToolResultText("Body not cached. Open the email in the TUI first."), nil
			}
			prompt := fmt.Sprintf("Extract all action items from this email as a JSON array of strings. Respond with JSON only, no explanation:\n\n%s", bodyText)
			reply, err := classifier.Chat([]ai.ChatMessage{{Role: "user", Content: prompt}})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("AI error: %v", err)), nil
			}
			return mcp.NewToolResultText(reply), nil
		},
	)

	// Tool: draft_reply
	s.AddTool(
		mcp.NewTool("draft_reply",
			mcp.WithDescription("Draft a reply to an email. Requires AI and cached body."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID to reply to")),
			mcp.WithString("tone", mcp.Description("Tone of reply: professional or casual (default: professional)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if classifier == nil {
				return mcp.NewToolResultText("AI not configured"), nil
			}
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			tone := req.GetString("tone", "professional")
			email, err := c.GetEmailByID(msgID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("email not found: %v", err)), nil
			}
			bodyText, _ := c.GetBodyText(msgID)
			if bodyText == "" {
				bodyText = "(body not cached)"
			}
			prompt := fmt.Sprintf(
				"Draft a %s reply to this email. Write only the reply body, no subject line or headers.\n\nFrom: %s\nSubject: %s\n\n%s",
				tone, email.Sender, email.Subject, bodyText,
			)
			reply, err := classifier.Chat([]ai.ChatMessage{{Role: "user", Content: prompt}})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("AI error: %v", err)), nil
			}
			return mcp.NewToolResultText(reply), nil
		},
	)

	// Tool: list_classification_prompts
	s.AddTool(
		mcp.NewTool("list_classification_prompts",
			mcp.WithDescription("List all custom classification prompt templates stored in the database"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			prompts, err := c.GetAllCustomPrompts()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(prompts) == 0 {
				return mcp.NewToolResultText("No custom classification prompts found"), nil
			}
			data, _ := json.Marshal(prompts)
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool: classify_email_custom
	s.AddTool(
		mcp.NewTool("classify_email_custom",
			mcp.WithDescription("Run a custom classification prompt against an email and persist the result"),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id",
				mcp.Required(),
				mcp.Description("The Message-ID of the email to classify"),
			),
			mcp.WithNumber("prompt_id",
				mcp.Required(),
				mcp.Description("The ID of the custom prompt to use (from list_classification_prompts)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if classifier == nil {
				return mcp.NewToolResultText("AI client not configured"), nil
			}
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			promptID := int64(req.GetInt("prompt_id", 0))

			email, err := c.GetEmailByID(msgID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("email not found: %v", err)), nil
			}

			prompt, err := c.GetCustomPrompt(promptID)
			if err != nil || prompt == nil {
				return mcp.NewToolResultError(fmt.Sprintf("prompt not found: %v", err)), nil
			}

			result, err := rulesengine.RunCustomPromptForEmail(classifier, prompt, email.Sender, email.Subject)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("classify error: %v", err)), nil
			}
			if err := c.SaveCustomCategory(msgID, promptID, result); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to save result: %v", err)), nil
			}
			return mcp.NewToolResultText(result), nil
		},
	)

	// Tool: get_custom_category
	s.AddTool(
		mcp.NewTool("get_custom_category",
			mcp.WithDescription("Retrieve a stored custom classification result for an email"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id",
				mcp.Required(),
				mcp.Description("The Message-ID of the email"),
			),
			mcp.WithNumber("prompt_id",
				mcp.Required(),
				mcp.Description("The ID of the custom prompt"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			promptID := int64(req.GetInt("prompt_id", 0))

			result, err := c.GetCustomCategory(msgID, promptID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return mcp.NewToolResultText(fmt.Sprintf("No custom category result found for message_id=%s, prompt_id=%d. Run classify_email_custom first.", msgID, promptID)), nil
				}
				return nil, fmt.Errorf("get custom category: %w", err)
			}
			return mcp.NewToolResultText(result), nil
		},
	)

	// Tool: list_cleanup_rules
	s.AddTool(
		mcp.NewTool("list_cleanup_rules",
			mcp.WithDescription("List all auto-cleanup rules stored in the local cache"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			rules, err := c.GetAllCleanupRules()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			if len(rules) == 0 {
				return mcp.NewToolResultText("No cleanup rules defined. Use create_cleanup_rule to add one."), nil
			}
			data, _ := json.Marshal(rules)
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool: create_cleanup_rule
	s.AddTool(
		mcp.NewTool("create_cleanup_rule",
			mcp.WithDescription("Create a new auto-cleanup rule. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Human-readable rule name"),
			),
			mcp.WithString("match_type",
				mcp.Required(),
				mcp.Description("Match type: 'sender' or 'domain'"),
			),
			mcp.WithString("match_value",
				mcp.Required(),
				mcp.Description("Value to match, e.g. newsletter@example.com or example.com"),
			),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("Action to perform: 'delete' or 'archive'"),
			),
			mcp.WithNumber("older_than_days",
				mcp.Description("Only affect emails older than this many days (default 30)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			matchType, err := req.RequireString("match_type")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if matchType != "sender" && matchType != "domain" {
				return mcp.NewToolResultError("match_type must be 'sender' or 'domain'"), nil
			}
			matchValue, err := req.RequireString("match_value")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			action, err := req.RequireString("action")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if action != "delete" && action != "archive" {
				return mcp.NewToolResultError("action must be 'delete' or 'archive'"), nil
			}
			olderThanDays := req.GetInt("older_than_days", 30)
			if olderThanDays <= 0 {
				olderThanDays = 30
			}

			body := map[string]interface{}{
				"name":            name,
				"match_type":      matchType,
				"match_value":     matchValue,
				"action":          action,
				"older_than_days": olderThanDays,
				"enabled":         true,
			}
			respBody, status, err := daemonPost("/v1/cleanup-rules", body)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Created cleanup rule: %s", string(respBody))), nil
		},
	)

	// Tool: run_cleanup_rules
	s.AddTool(
		mcp.NewTool("run_cleanup_rules",
			mcp.WithDescription("Trigger immediate execution of all enabled cleanup rules. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			respBody, status, err := daemonPost("/v1/cleanup-rules/run", nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK && status != http.StatusAccepted {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			return mcp.NewToolResultText(string(respBody)), nil
		},
	)

	// Tool: save_draft
	s.AddTool(
		mcp.NewTool("save_draft",
			mcp.WithDescription("Save an email draft to the IMAP Drafts folder. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("to",
				mcp.Required(),
				mcp.Description("Recipient email address"),
			),
			mcp.WithString("subject",
				mcp.Required(),
				mcp.Description("Email subject"),
			),
			mcp.WithString("body",
				mcp.Required(),
				mcp.Description("Email body (Markdown supported)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			to, err := req.RequireString("to")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			subject, err := req.RequireString("subject")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, err := req.RequireString("body")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			respBody, status, err := daemonPost("/v1/drafts", map[string]string{
				"to": to, "subject": subject, "body": body,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			var result map[string]any
			if err := json.Unmarshal(respBody, &result); err != nil {
				return mcp.NewToolResultText(string(respBody)), nil
			}
			uid := result["uid"]
			return mcp.NewToolResultText(fmt.Sprintf("Draft saved (UID: %v)", uid)), nil
		},
	)

	// Tool: list_drafts
	s.AddTool(
		mcp.NewTool("list_drafts",
			mcp.WithDescription("List all draft emails from the IMAP Drafts folder. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			respBody, status, err := daemonGet("/v1/drafts")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			var drafts []struct {
				UID     uint32 `json:"UID"`
				Folder  string `json:"Folder"`
				To      string `json:"To"`
				Subject string `json:"Subject"`
				Date    string `json:"Date"`
			}
			if err := json.Unmarshal(respBody, &drafts); err != nil {
				return mcp.NewToolResultText(string(respBody)), nil
			}
			if len(drafts) == 0 {
				return mcp.NewToolResultText("No drafts found"), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("%d drafts:\n", len(drafts)))
			for i, d := range drafts {
				sb.WriteString(fmt.Sprintf("%d. [%s] → %s (%s)\n", i+1, d.Subject, d.To, d.Date))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: send_draft
	s.AddTool(
		mcp.NewTool("send_draft",
			mcp.WithDescription("Send a saved draft and delete it from the Drafts folder. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("uid",
				mcp.Required(),
				mcp.Description("The UID of the draft to send (from list_drafts)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			uid := req.GetInt("uid", 0)
			if uid == 0 {
				return mcp.NewToolResultError("uid is required and must be non-zero"), nil
			}
			respBody, status, err := daemonPost(fmt.Sprintf("/v1/drafts/%d/send?folder=Drafts", uid), nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			return mcp.NewToolResultText("Draft sent and deleted"), nil
		},
	)

	// Tool: reply_to_email
	s.AddTool(
		mcp.NewTool("reply_to_email",
			mcp.WithDescription("Send a reply to an existing email. Requires the herald daemon to be running."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID of the email to reply to")),
			mcp.WithString("body", mcp.Required(), mcp.Description("Reply body (Markdown supported)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			messageID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, err := req.RequireString("body")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			respBody, status, err := daemonPost("/v1/emails/"+messageID+"/reply", map[string]string{"body": body})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			return mcp.NewToolResultText("Reply sent"), nil
		},
	)

	// Tool: forward_email
	s.AddTool(
		mcp.NewTool("forward_email",
			mcp.WithDescription("Forward an email to a new recipient. Requires the herald daemon to be running."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID of the email to forward")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Recipient email address")),
			mcp.WithString("body", mcp.Description("Optional covering note (Markdown supported)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			messageID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			to, err := req.RequireString("to")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body := req.GetString("body", "")
			respBody, status, err := daemonPost("/v1/emails/"+messageID+"/forward", map[string]string{"to": to, "body": body})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			return mcp.NewToolResultText("Forwarded to " + to), nil
		},
	)

	// Tool: list_attachments
	s.AddTool(
		mcp.NewTool("list_attachments",
			mcp.WithDescription("List attachment metadata for an email (no binary data). Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID of the email")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			messageID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			respBody, status, err := daemonGet("/v1/emails/" + messageID + "/attachments")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			var attachments []struct {
				Filename string `json:"filename"`
				MIMEType string `json:"mimeType"`
				Size     int    `json:"size"`
			}
			if err := json.Unmarshal(respBody, &attachments); err != nil {
				return mcp.NewToolResultText(string(respBody)), nil
			}
			if len(attachments) == 0 {
				return mcp.NewToolResultText("No attachments found"), nil
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("%d attachment(s):\n", len(attachments)))
			for i, a := range attachments {
				kb := float64(a.Size) / 1024.0
				sb.WriteString(fmt.Sprintf("%d. %s (%s, %.1f KB)\n", i+1, a.Filename, a.MIMEType, kb))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	// Tool: get_attachment
	s.AddTool(
		mcp.NewTool("get_attachment",
			mcp.WithDescription("Retrieve a specific attachment from an email. Optionally save to disk. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID of the email")),
			mcp.WithString("filename", mcp.Required(), mcp.Description("Attachment filename (from list_attachments)")),
			mcp.WithString("dest_path", mcp.Description("If provided, save attachment to this local file path and return the path")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			messageID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			filename, err := req.RequireString("filename")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			destPath := req.GetString("dest_path", "")

			urlPath := "/v1/emails/" + messageID + "/attachments/" + filename
			if destPath != "" {
				urlPath += "?dest_path=" + destPath
			}
			respBody, status, err := daemonGet(urlPath)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(respBody))), nil
			}
			var result map[string]any
			if err := json.Unmarshal(respBody, &result); err != nil {
				return mcp.NewToolResultText(string(respBody)), nil
			}
			if destPath != "" {
				if path, ok := result["path"].(string); ok {
					return mcp.NewToolResultText("Saved to " + path), nil
				}
			}
			mimeType, _ := result["mimeType"].(string)
			data, _ := result["data"].(string)
			return mcp.NewToolResultText(fmt.Sprintf("%s (%s):\n%s", filename, mimeType, data)), nil
		},
	)

	// Tool: delete_thread
	s.AddTool(
		mcp.NewTool("delete_thread",
			mcp.WithDescription("Delete all emails in a thread (grouped by subject). Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("folder", mcp.Required(), mcp.Description("IMAP folder name")),
			mcp.WithString("subject", mcp.Required(), mcp.Description("Thread subject to match")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			subject, err := req.RequireString("subject")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := daemonPost("/v1/threads/delete", map[string]string{"folder": folder, "subject": subject})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(body))), nil
			}
			return mcp.NewToolResultText("Thread deleted"), nil
		},
	)

	// Tool: archive_thread
	s.AddTool(
		mcp.NewTool("archive_thread",
			mcp.WithDescription("Archive all emails in a thread (grouped by subject). Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("folder", mcp.Required(), mcp.Description("IMAP folder name")),
			mcp.WithString("subject", mcp.Required(), mcp.Description("Thread subject to match")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			subject, err := req.RequireString("subject")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := daemonPost("/v1/threads/archive", map[string]string{"folder": folder, "subject": subject})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(body))), nil
			}
			return mcp.NewToolResultText("Thread archived"), nil
		},
	)

	// Tool: bulk_delete
	s.AddTool(
		mcp.NewTool("bulk_delete",
			mcp.WithDescription("Delete a list of emails by message ID. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("message_ids", mcp.Required(), mcp.Description("JSON array of message IDs, e.g. [\"id1\",\"id2\"]")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			idsJSON, err := req.RequireString("message_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var ids []string
			if err := json.Unmarshal([]byte(idsJSON), &ids); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid message_ids JSON: %v", err)), nil
			}
			if len(ids) == 0 {
				return mcp.NewToolResultError("message_ids must not be empty"), nil
			}
			body, status, err := daemonPost("/v1/emails/bulk-delete", map[string]any{"message_ids": ids})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(body))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Deleted %d emails", len(ids))), nil
		},
	)

	// Tool: archive_sender
	s.AddTool(
		mcp.NewTool("archive_sender",
			mcp.WithDescription("Archive all emails from a sender. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("sender", mcp.Required(), mcp.Description("Sender email address")),
			mcp.WithString("folder", mcp.Description("Source folder (default: INBOX)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sender, err := req.RequireString("sender")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			folder := req.GetString("folder", "INBOX")
			body, status, err := daemonPost("/v1/senders/"+url.PathEscape(sender)+"/archive", map[string]string{"folder": folder})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(body))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Archived all emails from %s", sender)), nil
		},
	)

	// Tool: bulk_move
	s.AddTool(
		mcp.NewTool("bulk_move",
			mcp.WithDescription("Move a list of emails to a target folder. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_ids", mcp.Required(), mcp.Description("JSON array of message IDs")),
			mcp.WithString("to_folder", mcp.Required(), mcp.Description("Destination folder name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			idsJSON, err := req.RequireString("message_ids")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var ids []string
			if err := json.Unmarshal([]byte(idsJSON), &ids); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid message_ids JSON: %v", err)), nil
			}
			if len(ids) == 0 {
				return mcp.NewToolResultError("message_ids must not be empty"), nil
			}
			toFolder, err := req.RequireString("to_folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := daemonPost("/v1/emails/bulk-move", map[string]any{"message_ids": ids, "to_folder": toFolder})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(body))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Moved %d emails to %s", len(ids), toFolder)), nil
		},
	)

	// Tool: unsubscribe_sender
	s.AddTool(
		mcp.NewTool("unsubscribe_sender",
			mcp.WithDescription("Execute unsubscribe via the List-Unsubscribe header of an email (RFC 8058 POST or browser open). Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("Message ID of the email to unsubscribe from")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			messageID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := daemonPost("/v1/emails/"+url.PathEscape(messageID)+"/unsubscribe", nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(body))), nil
			}
			return mcp.NewToolResultText("Unsubscribed"), nil
		},
	)

	// Tool: soft_unsubscribe_sender
	s.AddTool(
		mcp.NewTool("soft_unsubscribe_sender",
			mcp.WithDescription("Create an auto-move rule that moves all future emails from a sender to a folder. Requires the herald daemon."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("sender", mcp.Required(), mcp.Description("Sender email address to create rule for")),
			mcp.WithString("to_folder", mcp.Description("Destination folder (default: Disabled Subscriptions)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			sender, err := req.RequireString("sender")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			toFolder := req.GetString("to_folder", "")
			body, status, err := daemonPost("/v1/senders/"+url.PathEscape(sender)+"/soft-unsubscribe", map[string]string{"to_folder": toFolder})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != http.StatusOK {
				return mcp.NewToolResultError(fmt.Sprintf("daemon returned %d: %s", status, string(body))), nil
			}
			return mcp.NewToolResultText(fmt.Sprintf("Auto-move rule created for %s", sender)), nil
		},
	)

	// Tool: create_folder
	s.AddTool(
		mcp.NewTool("create_folder",
			mcp.WithDescription("Create a new IMAP mailbox folder"),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name for the new folder, e.g. 'Work/Projects'"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := daemonPost("/v1/folders", map[string]string{"name": name})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != 201 {
				return mcp.NewToolResultError(fmt.Sprintf("daemon error (status %d): %s", status, string(body))), nil
			}
			return mcp.NewToolResultText("Folder '" + name + "' created"), nil
		},
	)

	// Tool: rename_folder
	s.AddTool(
		mcp.NewTool("rename_folder",
			mcp.WithDescription("Rename an existing IMAP mailbox folder"),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Current name of the folder to rename"),
			),
			mcp.WithString("new_name",
				mcp.Required(),
				mcp.Description("New name for the folder"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			newName, err := req.RequireString("new_name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, status, err := daemonPost("/v1/folders/"+url.PathEscape(name)+"/rename", map[string]string{"new_name": newName})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != 200 {
				return mcp.NewToolResultError(fmt.Sprintf("daemon error (status %d): %s", status, string(body))), nil
			}
			return mcp.NewToolResultText("Folder '" + name + "' renamed to '" + newName + "'"), nil
		},
	)

	// Tool: delete_folder
	s.AddTool(
		mcp.NewTool("delete_folder",
			mcp.WithDescription("Permanently delete an IMAP mailbox folder"),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Name of the folder to delete"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, err := req.RequireString("name")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			status, err := daemonDelete("/v1/folders/" + url.PathEscape(name))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != 200 {
				return mcp.NewToolResultError(fmt.Sprintf("daemon error (status %d)", status)), nil
			}
			return mcp.NewToolResultText("Folder '" + name + "' deleted"), nil
		},
	)

	// Tool: sync_all_folders
	s.AddTool(
		mcp.NewTool("sync_all_folders",
			mcp.WithDescription("Trigger background sync for all known IMAP folders"),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			body, status, err := daemonPost("/v1/sync/all", nil)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != 200 {
				return mcp.NewToolResultError(fmt.Sprintf("daemon error (status %d): %s", status, string(body))), nil
			}
			var resp struct {
				NewEmails int `json:"new_emails"`
			}
			if json.Unmarshal(body, &resp) == nil && resp.NewEmails > 0 {
				return mcp.NewToolResultText(fmt.Sprintf("Sync started (%d new emails found)", resp.NewEmails)), nil
			}
			return mcp.NewToolResultText("Sync started"), nil
		},
	)

	// Tool: get_sync_status
	s.AddTool(
		mcp.NewTool("get_sync_status",
			mcp.WithDescription("Get per-folder email counts from the IMAP server"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			body, status, err := daemonGet("/v1/sync/status")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if status != 200 {
				return mcp.NewToolResultError(fmt.Sprintf("daemon error (status %d): %s", status, string(body))), nil
			}
			var folderStatus map[string]struct {
				Total  int `json:"Total"`
				Unseen int `json:"Unseen"`
			}
			if err := json.Unmarshal(body, &folderStatus); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("parse error: %v", err)), nil
			}
			if len(folderStatus) == 0 {
				return mcp.NewToolResultText("No folders found"), nil
			}
			var sb strings.Builder
			sb.WriteString("Sync status:\n")
			for folder, st := range folderStatus {
				sb.WriteString(fmt.Sprintf("- %s: %d messages, %d unseen\n", folder, st.Total, st.Unseen))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}

// formatContactsWithScores formats semantic search results including similarity scores.
func formatContactsWithScores(results []*models.ContactSearchResult) string {
	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("---\n")
		}
		pct := int(r.Score * 100)
		cd := r.Contact
		if cd.DisplayName != "" {
			sb.WriteString(fmt.Sprintf("[%d%%] %s <%s>\n", pct, cd.DisplayName, cd.Email))
		} else {
			sb.WriteString(fmt.Sprintf("[%d%%] <%s>\n", pct, cd.Email))
		}
		if cd.Company != "" || len(cd.Topics) > 0 {
			line := ""
			if cd.Company != "" {
				line += fmt.Sprintf("Company: %s", cd.Company)
			}
			if len(cd.Topics) > 0 {
				if line != "" {
					line += "  "
				}
				line += fmt.Sprintf("Topics: %s", strings.Join(cd.Topics, ", "))
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString(fmt.Sprintf("Last seen: %s  Emails: %d\n",
			cd.LastSeen.Format("2006-01-02"), cd.EmailCount))
	}
	return sb.String()
}

// formatContacts formats a slice of ContactData into a human-readable string.
func formatContacts(contacts []models.ContactData) string {
	var sb strings.Builder
	for i, cd := range contacts {
		if i > 0 {
			sb.WriteString("---\n")
		}
		if cd.DisplayName != "" {
			sb.WriteString(fmt.Sprintf("%s <%s>\n", cd.DisplayName, cd.Email))
		} else {
			sb.WriteString(fmt.Sprintf("<%s>\n", cd.Email))
		}
		if cd.Company != "" || len(cd.Topics) > 0 {
			line := ""
			if cd.Company != "" {
				line += fmt.Sprintf("Company: %s", cd.Company)
			}
			if len(cd.Topics) > 0 {
				if line != "" {
					line += "  "
				}
				line += fmt.Sprintf("Topics: %s", strings.Join(cd.Topics, ", "))
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString(fmt.Sprintf("Last seen: %s  Emails: %d\n",
			cd.LastSeen.Format("2006-01-02"), cd.EmailCount))
	}
	return sb.String()
}
