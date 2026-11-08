package mcpserver

import (
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/mark3labs/mcp-go/server"
	"strings"
	"testing"
)

func newConsentedTestMCPServer(c *cache.Cache, classifier ai.AIClient) *server.MCPServer {
	cfg := &config.Config{}
	cfg.GrantSharing(false, false, true)
	return newMCPServerWithConfig(c, classifier, cfg)
}
func TestMCPConsentBlocksPrivateDataAndRevokes(t *testing.T) {
	c := newScopedMutationTestMCP(t)
	cfg := &config.Config{}
	s := newMCPServerWithConfig(c, nil, cfg)
	denied := callVirtualLabTool(t, s, 1, "list_recent_emails", map[string]any{"folder": "INBOX", "limit": 5})
	if !strings.Contains(denied, "needs consent") {
		t.Fatal("MCP read allowed before consent")
	}
	cfg.GrantSharing(false, false, true)
	allowed := callVirtualLabTool(t, s, 2, "list_recent_emails", map[string]any{"folder": "INBOX", "limit": 5})
	if strings.Contains(allowed, "needs consent") {
		t.Fatal("MCP grant not effective")
	}
	cfg.GrantSharing(false, false, false)
	denied = callVirtualLabTool(t, s, 3, "list_calendar_events", map[string]any{})
	if !strings.Contains(denied, "needs consent") {
		t.Fatalf("MCP calendar gate missing: %s", denied)
	}
}
