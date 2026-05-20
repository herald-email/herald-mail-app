package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/imap"
	"github.com/herald-email/herald-mail-app/internal/models"
	emailrender "github.com/herald-email/herald-mail-app/internal/render"
	"github.com/herald-email/herald-mail-app/internal/testmail"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestVirtualLabCacheBackedMCPReadsScenarioMail(t *testing.T) {
	c, msgID := populateVirtualLabScenarioCache(t, testmail.ScenarioCalendlyInvite)
	s := newVirtualLabCacheMCPServer(c)

	listJSON := callVirtualLabTool(t, s, 1, "list_recent_emails", map[string]any{
		"folder": "INBOX",
		"limit":  5,
	})
	if !strings.Contains(listJSON, "Invitation: Product review with Bob") || !strings.Contains(listJSON, "message_id=") || !strings.Contains(listJSON, strings.Trim(msgID, "<>")) {
		t.Fatalf("list_recent_emails did not expose calendly-like scenario mail: %s", listJSON)
	}

	searchJSON := callVirtualLabTool(t, s, 2, "search_emails", map[string]any{
		"folder": "INBOX",
		"query":  "Product review",
	})
	if !strings.Contains(searchJSON, "Invitation: Product review with Bob") {
		t.Fatalf("search_emails did not find scenario subject: %s", searchJSON)
	}

	bodyJSON := callVirtualLabTool(t, s, 3, "get_email_body", map[string]any{
		"message_id": msgID,
	})
	if !strings.Contains(bodyJSON, "Join meeting") || strings.Contains(bodyJSON, "utm_") {
		t.Fatalf("get_email_body returned unexpected scenario body: %s", bodyJSON)
	}
}

func populateVirtualLabScenarioCache(t *testing.T, name testmail.ScenarioName) (*cache.Cache, string) {
	t.Helper()
	seeded := testmail.StartScenario(t, name)
	alice := seeded.Lab.Account(testmail.DefaultAliceAddress)
	c, err := cache.New(filepath.Join(t.TempDir(), "mcp-cache.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	client := imap.New(alice.Config(filepath.Join(t.TempDir(), "imap-cache.db")), "", c, make(chan models.ProgressInfo, 8))
	if err := client.Connect(); err != nil {
		t.Fatalf("connect virtual IMAP: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	var firstMessageID string
	for _, msg := range seeded.Messages {
		if msg.Account != testmail.DefaultAliceAddress || msg.Folder != "INBOX" {
			continue
		}
		ref := seeded.Refs[msg.Key]
		body, err := client.FetchEmailBody(ref.UID, ref.Folder)
		if err != nil {
			t.Fatalf("FetchEmailBody(%s/%d): %v", ref.Folder, ref.UID, err)
		}
		if err := c.CacheEmail(&models.EmailData{
			MessageID:      ref.MessageID,
			UID:            ref.UID,
			Sender:         body.From,
			Subject:        body.Subject,
			Date:           time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC),
			Size:           len(msg.Data),
			HasAttachments: len(body.Attachments) > 0,
			Folder:         ref.Folder,
			LastUpdated:    time.Now(),
		}); err != nil {
			t.Fatalf("CacheEmail(%s): %v", ref.MessageID, err)
		}
		if err := c.CacheBodyText(ref.MessageID, emailrender.EmailBodyMarkdown(body)); err != nil {
			t.Fatalf("CacheBodyText(%s): %v", ref.MessageID, err)
		}
		if firstMessageID == "" {
			firstMessageID = ref.MessageID
		}
	}
	if firstMessageID == "" {
		t.Fatalf("scenario %q did not seed Alice INBOX mail", name)
	}
	return c, firstMessageID
}

func newVirtualLabCacheMCPServer(c *cache.Cache) *server.MCPServer {
	s := server.NewMCPServer("herald-test", "test", server.WithToolCapabilities(false))
	s.AddTool(
		mcp.NewTool("list_recent_emails",
			mcp.WithString("folder", mcp.Required()),
			mcp.WithNumber("limit"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			folder, err := req.RequireString("folder")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			limit := req.GetInt("limit", 20)
			emails, err := c.GetEmailsSortedByDate(folder)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("cache error: %v", err)), nil
			}
			if len(emails) > limit {
				emails = emails[:limit]
			}
			var sb strings.Builder
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("%s %s %s\n", e.Sender, e.Subject, mcpMessageIDRef(e)))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)
	s.AddTool(
		mcp.NewTool("search_emails",
			mcp.WithString("folder", mcp.Required()),
			mcp.WithString("query", mcp.Required()),
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
			var sb strings.Builder
			for _, e := range emails {
				sb.WriteString(fmt.Sprintf("%s %s %s\n", e.Sender, e.Subject, mcpMessageIDRef(e)))
			}
			return mcp.NewToolResultText(sb.String()), nil
		},
	)
	s.AddTool(
		mcp.NewTool("get_email_body", mcp.WithString("message_id", mcp.Required())),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msgID, err := req.RequireString("message_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			body, err := c.GetBodyText(msgID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
			}
			return mcp.NewToolResultText(body), nil
		},
	)
	return s
}

func callVirtualLabTool(t *testing.T, s *server.MCPServer, id int, name string, args map[string]any) string {
	t.Helper()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	}
	rawReq, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp := s.HandleMessage(context.Background(), rawReq)
	rawResp, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	return string(rawResp)
}
