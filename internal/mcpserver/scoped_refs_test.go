package mcpserver

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestMCPListingOutputsScopedMessageRef(t *testing.T) {
	c, err := cache.New(filepath.Join(t.TempDir(), "mcp-scoped-cache.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	email := (&models.EmailData{
		SourceID:       "work-mail",
		AccountID:      "work",
		MessageID:      "scoped-message@mcp.test",
		UID:            42,
		Sender:         "Roadmap <roadmap@example.test>",
		Subject:        "Scoped MCP reference",
		Date:           time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		Size:           1234,
		HasAttachments: true,
		Folder:         "INBOX",
	}).MessageRef()
	if err := c.CacheEmail(&models.EmailData{
		SourceID:       email.SourceID,
		AccountID:      email.AccountID,
		LocalID:        email.LocalID,
		MessageID:      email.MessageID,
		UID:            email.UID,
		Sender:         "Roadmap <roadmap@example.test>",
		Subject:        "Scoped MCP reference",
		Date:           time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		Size:           1234,
		HasAttachments: true,
		Folder:         email.Folder,
	}); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}

	s := newMCPServer(c, nil)
	got := callVirtualLabTool(t, s, 1, "list_recent_emails", map[string]any{
		"folder": "INBOX",
		"limit":  1,
	})
	for _, want := range []string{
		"message_id=scoped-message@mcp.test",
		"source_id=work-mail",
		"account_id=work",
		"local_id=mail:work-mail:work:INBOX:scoped-message@mcp.test",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("list_recent_emails missing %q in scoped ref output:\n%s", want, got)
		}
	}
}
