package app

import (
	"fmt"
	"testing"
	"time"

	"mail-processor/internal/models"
)

// --- sanitizeText ---

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "Hello World"},
		{"Hello  World", "Hello World"},   // multiple spaces collapsed
		{"café", "café"},                  // accented letters kept
		{"日本語テスト", "日本語テスト"},        // CJK kept
		{"test@example.com", "test@example.com"}, // @ is punctuation
		{"emoji 🎉 here", "emoji here"},   // emoji stripped
		{"🚀launch", "launch"},
		{"", ""},
		{"   ", ""},
		{"abc\t def", "abc def"}, // tab → whitespace
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeText(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- buildFolderTree ---

func TestBuildFolderTree_TopLevelFolders(t *testing.T) {
	folders := []string{"Trash", "INBOX", "Sent"}
	roots := buildFolderTree(folders)

	if len(roots) != 3 {
		t.Fatalf("expected 3 roots, got %d", len(roots))
	}
	// INBOX should come first (priority 0)
	if roots[0].name != "INBOX" {
		t.Errorf("expected INBOX first, got %q", roots[0].name)
	}
	// Sent second (priority 1), Trash third (priority 5)
	if roots[1].name != "Sent" {
		t.Errorf("expected Sent second, got %q", roots[1].name)
	}
}

func TestBuildFolderTree_NestedFolders(t *testing.T) {
	folders := []string{"INBOX", "INBOX/Work", "INBOX/Personal"}
	roots := buildFolderTree(folders)

	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	inbox := roots[0]
	if inbox.name != "INBOX" {
		t.Errorf("expected INBOX root, got %q", inbox.name)
	}
	if len(inbox.children) != 2 {
		t.Errorf("expected 2 INBOX children, got %d", len(inbox.children))
	}
}

func TestBuildFolderTree_SyntheticParent(t *testing.T) {
	// Only the leaf is given; the parent should be created as a synthetic node
	folders := []string{"INBOX/Sub"}
	roots := buildFolderTree(folders)

	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	parent := roots[0]
	if parent.name != "INBOX" {
		t.Errorf("expected INBOX as synthetic parent, got %q", parent.name)
	}
	// Synthetic parent has no fullPath
	if parent.fullPath != "" {
		t.Errorf("synthetic parent should have empty fullPath, got %q", parent.fullPath)
	}
	if len(parent.children) != 1 || parent.children[0].name != "Sub" {
		t.Errorf("expected child Sub, got %v", parent.children)
	}
}

func TestBuildFolderTree_UnknownFoldersSortedAlphabetically(t *testing.T) {
	folders := []string{"Zebra", "Apple", "Mango"}
	roots := buildFolderTree(folders)

	if len(roots) != 3 {
		t.Fatalf("expected 3 roots, got %d", len(roots))
	}
	names := []string{roots[0].name, roots[1].name, roots[2].name}
	expected := []string{"Apple", "Mango", "Zebra"}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("position %d: got %q, want %q", i, n, expected[i])
		}
	}
}

// --- flattenTree ---

func TestFlattenTree_ExpandedNodes(t *testing.T) {
	folders := []string{"INBOX", "INBOX/Work", "Sent"}
	roots := buildFolderTree(folders)
	// All nodes start expanded
	items := flattenTree(roots)

	// Should see: INBOX, INBOX/Work, Sent  (3 items)
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d: %v", len(items), items)
	}
	if items[0].node.name != "INBOX" || items[0].depth != 0 {
		t.Errorf("item 0: got name=%q depth=%d", items[0].node.name, items[0].depth)
	}
	if items[1].node.name != "Work" || items[1].depth != 1 {
		t.Errorf("item 1: got name=%q depth=%d", items[1].node.name, items[1].depth)
	}
}

func TestFlattenTree_CollapsedNodeHidesChildren(t *testing.T) {
	folders := []string{"INBOX", "INBOX/Work", "INBOX/Personal"}
	roots := buildFolderTree(folders)

	// Collapse INBOX
	roots[0].expanded = false

	items := flattenTree(roots)
	// Only INBOX itself should appear
	if len(items) != 1 {
		t.Errorf("expected 1 item when collapsed, got %d", len(items))
	}
	if items[0].node.name != "INBOX" {
		t.Errorf("expected INBOX, got %q", items[0].node.name)
	}
}

func TestFlattenTree_EmptyTree(t *testing.T) {
	items := flattenTree(nil)
	if len(items) != 0 {
		t.Errorf("expected empty slice for nil roots, got %d items", len(items))
	}
}

// --- normalizeSubject ---

func TestNormalizeSubject(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Re: Hello", "hello"},
		{"RE: RE: Hello", "hello"},
		{"Fwd: Hello", "hello"},
		{"FWD: Fw: Hello", "hello"},
		{"Aw: Tr: Hello", "hello"},
		{"Hello World", "hello world"},
		{"  Re:  spaced  ", "spaced"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeSubject(tt.input)
			if got != tt.want {
				t.Errorf("normalizeSubject(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- buildThreadGroups ---

func makeEmail(subject string, date time.Time) *models.EmailData {
	return &models.EmailData{
		MessageID: subject + date.String(),
		Subject:   subject,
		Date:      date,
		Sender:    "sender@example.com",
	}
}

func TestBuildThreadGroups_Grouping(t *testing.T) {
	now := time.Now()
	emails := []*models.EmailData{
		makeEmail("Re: Hello", now),
		makeEmail("Other", now.Add(-time.Hour)),
		makeEmail("Hello", now.Add(-2*time.Hour)),
	}
	groups := buildThreadGroups(emails)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// First group: "hello" (most recent email first)
	if groups[0].normalizedSubject != "hello" {
		t.Errorf("group[0] subject = %q, want %q", groups[0].normalizedSubject, "hello")
	}
	if len(groups[0].emails) != 2 {
		t.Errorf("group[0] emails count = %d, want 2", len(groups[0].emails))
	}
	// Second group: "other"
	if groups[1].normalizedSubject != "other" {
		t.Errorf("group[1] subject = %q, want %q", groups[1].normalizedSubject, "other")
	}
}

func TestBuildThreadGroups_Order(t *testing.T) {
	now := time.Now()
	// Older thread first in input, newer thread second
	emails := []*models.EmailData{
		makeEmail("Newer", now),
		makeEmail("Older", now.Add(-24*time.Hour)),
	}
	groups := buildThreadGroups(emails)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].normalizedSubject != "newer" {
		t.Errorf("expected newer group first, got %q", groups[0].normalizedSubject)
	}
}

func TestBuildThreadGroups_SingleEmail(t *testing.T) {
	emails := []*models.EmailData{makeEmail("Solo", time.Now())}
	groups := buildThreadGroups(emails)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].emails) != 1 {
		t.Errorf("expected 1 email in group, got %d", len(groups[0].emails))
	}
}

// --- unsubscribeCmd browser fallback ---

// TestUnsubscribeBrowserFallback verifies that unsubscribeCmd returns a
// "browser-opened" result when the email has an HTTPS List-Unsubscribe URL
// and one-click POST conditions are not met. The openBrowserFn variable is
// replaced with a no-op so no real browser process is spawned.
func TestUnsubscribeBrowserFallback(t *testing.T) {
	// Swap out the real browser opener with a no-op that always succeeds.
	orig := openBrowserFn
	defer func() { openBrowserFn = orig }()
	openBrowserFn = func(url string) error { return nil }

	body := &models.EmailBody{
		// One-click POST requires ListUnsubscribePost == "List-Unsubscribe=One-Click";
		// leaving it empty ensures we fall through to the browser branch.
		ListUnsubscribe:     "<https://example.com/unsub>",
		ListUnsubscribePost: "",
	}

	cmd := unsubscribeCmd(body)
	msg := cmd()

	result, ok := msg.(UnsubscribeResultMsg)
	if !ok {
		t.Fatalf("expected UnsubscribeResultMsg, got %T", msg)
	}
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Method != "browser-opened" {
		t.Errorf("Method = %q, want %q", result.Method, "browser-opened")
	}
	if result.URL != "https://example.com/unsub" {
		t.Errorf("URL = %q, want %q", result.URL, "https://example.com/unsub")
	}
}

// TestUnsubscribeBrowserFallback_HTTP verifies the browser branch also fires
// for plain http:// URLs.
func TestUnsubscribeBrowserFallback_HTTP(t *testing.T) {
	orig := openBrowserFn
	defer func() { openBrowserFn = orig }()
	openBrowserFn = func(url string) error { return nil }

	body := &models.EmailBody{
		ListUnsubscribe:     "<http://example.com/unsub>",
		ListUnsubscribePost: "",
	}

	cmd := unsubscribeCmd(body)
	msg := cmd()

	result, ok := msg.(UnsubscribeResultMsg)
	if !ok {
		t.Fatalf("expected UnsubscribeResultMsg, got %T", msg)
	}
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Method != "browser-opened" {
		t.Errorf("Method = %q, want %q", result.Method, "browser-opened")
	}
}

// TestUnsubscribeBrowserFallback_ExecError verifies that when the browser
// open fails, the function falls through to the clipboard fallback
// ("url-copied") rather than returning an error.
func TestUnsubscribeBrowserFallback_ExecError(t *testing.T) {
	orig := openBrowserFn
	defer func() { openBrowserFn = orig }()
	openBrowserFn = func(url string) error {
		return fmt.Errorf("no browser available")
	}

	body := &models.EmailBody{
		ListUnsubscribe:     "<https://example.com/unsub>",
		ListUnsubscribePost: "",
	}

	cmd := unsubscribeCmd(body)
	msg := cmd()

	result, ok := msg.(UnsubscribeResultMsg)
	if !ok {
		t.Fatalf("expected UnsubscribeResultMsg, got %T", msg)
	}
	// Should fall through to clipboard — either url-copied or an error from
	// pbcopy/xclip not being present in the test environment is acceptable,
	// but "browser-opened" must NOT be returned.
	if result.Method == "browser-opened" {
		t.Error("expected fall-through to clipboard, but got browser-opened")
	}
}
