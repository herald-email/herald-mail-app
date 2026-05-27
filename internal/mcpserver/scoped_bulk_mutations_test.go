package mcpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMCPBulkDeleteForwardsScopedLocalIDs(t *testing.T) {
	var payload struct {
		MessageIDs []string `json:"message_ids"`
		LocalIDs   []string `json:"local_ids"`
	}
	withTestDaemonURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/v1/emails/bulk-delete" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"Deleted 1 emails"}`))
	}))

	s := newMCPServer(newScopedMutationTestMCP(t), nil)
	got := callVirtualLabTool(t, s, 1, "bulk_delete", map[string]any{
		"message_ids": `["same-message"]`,
		"local_ids":   `["mail:work-mail:work:INBOX:same-message"]`,
	})

	if !strings.Contains(got, "Deleted 1 emails") {
		t.Fatalf("bulk_delete response = %s", got)
	}
	if len(payload.LocalIDs) != 1 || payload.LocalIDs[0] != "mail:work-mail:work:INBOX:same-message" {
		t.Fatalf("local_ids not forwarded: %#v", payload)
	}
}

func TestMCPThreadAndSenderMutationsForwardSourceScope(t *testing.T) {
	var seen []map[string]any
	withTestDaemonURL(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/status" {
			w.WriteHeader(http.StatusOK)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload for %s: %v", r.URL.Path, err)
		}
		payload["path"] = r.URL.Path
		seen = append(seen, payload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))

	s := newMCPServer(newScopedMutationTestMCP(t), nil)
	archiveThread := callVirtualLabTool(t, s, 2, "archive_thread", map[string]any{
		"folder":     "INBOX",
		"subject":    "Roadmap",
		"source_id":  "personal-mail",
		"account_id": "personal",
	})
	archiveSender := callVirtualLabTool(t, s, 3, "archive_sender", map[string]any{
		"sender":     "newsletter@example.com",
		"folder":     "INBOX",
		"source_id":  "work-mail",
		"account_id": "work",
	})

	if !strings.Contains(archiveThread, "Thread archived") || !strings.Contains(archiveSender, "Archived all emails") {
		t.Fatalf("unexpected responses:\nthread=%s\nsender=%s", archiveThread, archiveSender)
	}
	if len(seen) != 2 {
		t.Fatalf("daemon calls = %d, want 2", len(seen))
	}
	if seen[0]["source_id"] != "personal-mail" || seen[0]["account_id"] != "personal" {
		t.Fatalf("thread scope not forwarded: %#v", seen[0])
	}
	if seen[1]["source_id"] != "work-mail" || seen[1]["account_id"] != "work" {
		t.Fatalf("sender scope not forwarded: %#v", seen[1])
	}
}
