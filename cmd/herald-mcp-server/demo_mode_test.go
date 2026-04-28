package main

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
	if !strings.Contains(string(callJSON), "Northstar Cloud") {
		t.Fatalf("expected fictional demo mailbox data in response: %s", callJSON)
	}
	if !strings.Contains(string(callJSON), "message_id=") {
		t.Fatalf("expected list_recent_emails response to expose message_id values: %s", callJSON)
	}
}
