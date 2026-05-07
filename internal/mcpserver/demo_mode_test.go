package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDemoMCPServerListsAndReadsDemoEmails(t *testing.T) {
	s := newDemoMCPServer()

	listResp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	listJSON, err := json.Marshal(listResp)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}
	if !strings.Contains(string(listJSON), "list_recent_emails") {
		t.Fatalf("expected list_recent_emails in tools/list response: %s", listJSON)
	}

	callResp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_recent_emails","arguments":{"folder":"INBOX","limit":5}}}`))
	callJSON, err := json.Marshal(callResp)
	if err != nil {
		t.Fatalf("marshal list_recent_emails response: %v", err)
	}
	if !strings.Contains(string(callJSON), "Herald Welcome") {
		t.Fatalf("expected Herald onboarding demo mailbox data in response: %s", callJSON)
	}
	if !strings.Contains(string(callJSON), "message_id=") {
		t.Fatalf("expected list_recent_emails response to expose message_id values: %s", callJSON)
	}
}

func TestDemoMCPSearchFindsCreativeCommonsImageSampler(t *testing.T) {
	s := newDemoMCPServer()

	callResp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"creative commons"}}}`))
	callJSON, err := json.Marshal(callResp)
	if err != nil {
		t.Fatalf("marshal search_emails response: %v", err)
	}
	body := strings.ToLower(string(callJSON))
	if !strings.Contains(body, "step 4: view inline images in full screen") {
		t.Fatalf("expected Step 4 image sampler in search response: %s", callJSON)
	}
	if !strings.Contains(body, "message_id=") {
		t.Fatalf("expected search_emails response to expose message_id values: %s", callJSON)
	}

	callResp = s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"images"}}}`))
	callJSON, err = json.Marshal(callResp)
	if err != nil {
		t.Fatalf("marshal image search_emails response: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(callJSON)), "step 4: view inline images in full screen") {
		t.Fatalf("expected image query to find Step 4 sampler: %s", callJSON)
	}
}
