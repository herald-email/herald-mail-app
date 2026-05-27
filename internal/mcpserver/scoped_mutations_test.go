package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/cache"
)

func withTestDaemonURL(t *testing.T, handler http.Handler) {
	t.Helper()
	oldURL := daemonURL
	oldBind := daemonProbeBind
	oldPort := daemonProbePort
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
		daemonMu.Lock()
		daemonURL = oldURL
		daemonProbeBind = oldBind
		daemonProbePort = oldPort
		daemonMu.Unlock()
	})
	daemonMu.Lock()
	daemonURL = server.URL
	daemonProbeBind = "127.0.0.1"
	daemonProbePort = 0
	daemonMu.Unlock()
}

func newScopedMutationTestMCP(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.New(filepath.Join(t.TempDir(), "mcp-scoped-mutations.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestMCPMutationToolForwardsScopedRefToDaemon(t *testing.T) {
	var capturedPath string
	withTestDaemonURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			w.WriteHeader(http.StatusOK)
			return
		}
		capturedPath = r.URL.String()
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/emails/same-message/read" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("folder") != "INBOX" || q.Get("source_id") != "work-mail" || q.Get("account_id") != "work" || q.Get("local_id") != "mail:work-mail:work:INBOX:same-message" {
			t.Fatalf("scoped query not forwarded: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	s := newMCPServer(newScopedMutationTestMCP(t), nil)
	got := callVirtualLabTool(t, s, 1, "mark_read", map[string]any{
		"message_id": "same-message",
		"folder":     "INBOX",
		"source_id":  "work-mail",
		"account_id": "work",
		"local_id":   "mail:work-mail:work:INBOX:same-message",
	})

	if !strings.Contains(got, "Marked as read") {
		t.Fatalf("mark_read response = %s", got)
	}
	if capturedPath == "" {
		t.Fatalf("daemon was not called")
	}
}

func TestMCPMoveEmailForwardsScopedRefToDaemon(t *testing.T) {
	var capturedPath string
	withTestDaemonURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			w.WriteHeader(http.StatusOK)
			return
		}
		capturedPath = r.URL.String()
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/emails/same-message/move" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("source_id") != "personal-mail" || q.Get("account_id") != "personal" || q.Get("local_id") != "mail:personal-mail:personal:INBOX:same-message" {
			t.Fatalf("scoped query not forwarded: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	s := newMCPServer(newScopedMutationTestMCP(t), nil)
	got := callVirtualLabTool(t, s, 2, "move_email", map[string]any{
		"message_id":  "same-message",
		"from_folder": "INBOX",
		"to_folder":   "Archive",
		"source_id":   "personal-mail",
		"account_id":  "personal",
		"local_id":    "mail:personal-mail:personal:INBOX:same-message",
	})

	if !strings.Contains(got, "Moved to Archive") {
		t.Fatalf("move_email response = %s", got)
	}
	if capturedPath == "" {
		t.Fatalf("daemon was not called")
	}
}
