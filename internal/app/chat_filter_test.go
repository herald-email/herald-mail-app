package app

import (
	"strings"
	"testing"
)

func TestParseChatFilter_Valid(t *testing.T) {
	text := `Here are relevant emails: <filter>{"ids":["msg-1","msg-2"],"label":"invoices"}</filter>`
	ids, label, found := parseChatFilter(text)
	if !found {
		t.Fatal("expected found=true")
	}
	if len(ids) != 2 || ids[0] != "msg-1" || ids[1] != "msg-2" {
		t.Errorf("got ids=%v", ids)
	}
	if label != "invoices" {
		t.Errorf("got label=%q", label)
	}
}

func TestParseChatFilter_NoBlock(t *testing.T) {
	_, _, found := parseChatFilter("no filter here")
	if found {
		t.Error("expected found=false for text without filter block")
	}
}

func TestParseChatFilter_MalformedJSON(t *testing.T) {
	_, _, found := parseChatFilter(`<filter>not json</filter>`)
	if found {
		t.Error("expected found=false for malformed JSON")
	}
}

func TestParseChatFilter_EmptyIDs(t *testing.T) {
	_, _, found := parseChatFilter(`<filter>{"ids":[],"label":"empty"}</filter>`)
	if found {
		t.Error("expected found=false when ids is empty")
	}
}

func TestParseChatFilter_FirstBlockWins(t *testing.T) {
	text := `<filter>{"ids":["a"],"label":"first"}</filter> <filter>{"ids":["b"],"label":"second"}</filter>`
	ids, label, found := parseChatFilter(text)
	if !found {
		t.Fatal("expected found=true")
	}
	if ids[0] != "a" || label != "first" {
		t.Errorf("first block should win, got ids=%v label=%q", ids, label)
	}
}

func TestStripChatFilter(t *testing.T) {
	text := `Here are results: <filter>{"ids":["x"],"label":"test"}</filter> End.`
	result := stripChatFilter(text)
	if strings.Contains(result, "<filter>") {
		t.Error("expected filter block to be removed")
	}
	if !strings.Contains(result, "Here are results:") {
		t.Error("expected non-filter text preserved")
	}
}

func TestStripChatFilter_MultipleBlocks(t *testing.T) {
	text := `A <filter>{"ids":["1"],"label":"a"}</filter> B <filter>{"ids":["2"],"label":"b"}</filter> C`
	result := stripChatFilter(text)
	if strings.Contains(result, "<filter>") {
		t.Error("all filter blocks should be removed")
	}
}
