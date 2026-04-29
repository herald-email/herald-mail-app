package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mail-processor/internal/models"
)

func stripVirtualRoots(roots []*folderNode) []*folderNode {
	filtered := make([]*folderNode, 0, len(roots))
	for _, root := range roots {
		if root == nil || root.fullPath == virtualFolderAllMailOnly {
			continue
		}
		filtered = append(filtered, root)
	}
	return filtered
}

func stripVirtualItems(items []sidebarItem) []sidebarItem {
	filtered := make([]sidebarItem, 0, len(items))
	for _, item := range items {
		if item.node == nil || item.node.fullPath == virtualFolderAllMailOnly {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// --- sanitizeText ---

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "Hello World"},
		{"Hello  World", "Hello World"},          // multiple spaces collapsed
		{"café", "café"},                         // accented letters kept
		{"日本語テスト", "日本語テスト"},                     // CJK kept
		{"test@example.com", "test@example.com"}, // @ is punctuation
		{"emoji 🎉 here", "emoji here"},           // emoji stripped
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
	roots := stripVirtualRoots(buildFolderTree(folders))

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
	roots := stripVirtualRoots(buildFolderTree(folders))

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
	roots := stripVirtualRoots(buildFolderTree(folders))

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
	roots := stripVirtualRoots(buildFolderTree(folders))

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
	items := stripVirtualItems(flattenTree(roots))

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

	items := stripVirtualItems(flattenTree(roots))
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

func TestBuildThreadGroups_GroupAcrossSendersBySubject(t *testing.T) {
	now := time.Now()
	reply := makeEmail("Re: Next Steps with Cobalt Works!", now)
	reply.Sender = "Rowan Finch <demo@demo.local>"
	original := makeEmail("Next Steps with Cobalt Works!", now.Add(-3*time.Minute))
	original.Sender = "Mina Park <mina@cobalt-works.example>"
	other := makeEmail("Different topic", now.Add(-time.Hour))
	other.Sender = "Mina Park <mina@cobalt-works.example>"

	groups := buildThreadGroups([]*models.EmailData{reply, original, other})
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].normalizedSubject != "next steps with cobalt works!" {
		t.Fatalf("expected cross-participant thread first, got %q", groups[0].normalizedSubject)
	}
	if len(groups[0].emails) != 2 {
		t.Fatalf("expected reply and original in same thread, got %d emails", len(groups[0].emails))
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

func TestUpdateTimelineTable_CollapsedThreadShowsParticipants(t *testing.T) {
	now := time.Now()
	m := New(&stubBackend{}, nil, "demo@demo.local", nil, false)
	m.timeline.senderWidth = 28
	m.timeline.subjectWidth = 42
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "reply",
			Sender:    "Rowan Finch <demo@demo.local>",
			Subject:   "Re: Next Steps with Cobalt Works!",
			Date:      now,
			Folder:    "INBOX",
		},
		{
			MessageID: "original",
			Sender:    "Mina Park <mina@cobalt-works.example>",
			Subject:   "Next Steps with Cobalt Works!",
			Date:      now.Add(-3 * time.Minute),
			Folder:    "INBOX",
		},
	}

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected one collapsed thread row, got %d", len(rows))
	}
	sender := stripANSI(rows[0][1])
	if !strings.Contains(sender, "▸") {
		t.Fatalf("expected collapsed sender to include disclosure marker, got %q", sender)
	}
	if !strings.Contains(sender, "me") {
		t.Fatalf("expected collapsed participants to include me, got %q", sender)
	}
	if !strings.Contains(sender, "Mina Park") {
		t.Fatalf("expected collapsed participants to include other sender display name, got %q", sender)
	}
	if subject := rows[0][2]; !strings.Contains(subject, "[2]") {
		t.Fatalf("expected collapsed thread count prefix, got %q", subject)
	}
}

func TestUpdateTimelineTable_ExpandedThreadReplyRowsShowReplyMarker(t *testing.T) {
	now := time.Now()
	m := New(&stubBackend{}, nil, "demo@demo.local", nil, false)
	m.timeline.senderWidth = 30
	m.timeline.subjectWidth = 42
	m.timeline.expandedThreads["next steps with cobalt works!"] = true
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "reply",
			Sender:    "Rowan Finch <demo@demo.local>",
			Subject:   "Re: Next Steps with Cobalt Works!",
			Date:      now,
			Folder:    "INBOX",
		},
		{
			MessageID: "original",
			Sender:    "Mina Park <mina@cobalt-works.example>",
			Subject:   "Next Steps with Cobalt Works!",
			Date:      now.Add(-3 * time.Minute),
			Folder:    "INBOX",
		},
	}

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected expanded thread rows, got %d", len(rows))
	}
	replySender := stripANSI(rows[0][1])
	if !strings.Contains(replySender, "▾") {
		t.Fatalf("expected expanded root row sender to include disclosure marker, got %q", replySender)
	}
	if !strings.Contains(replySender, "↩") {
		t.Fatalf("expected reply row sender to include reply marker, got %q", replySender)
	}
	originalSender := stripANSI(rows[1][1])
	if !strings.Contains(originalSender, "↳") {
		t.Fatalf("expected non-reply child row to keep nested marker, got %q", originalSender)
	}
}

func TestUpdateTimelineTable_SingleEmailThreadRowsDoNotShowDisclosureMarker(t *testing.T) {
	now := time.Now()
	m := New(&stubBackend{}, nil, "demo@demo.local", nil, false)
	m.timeline.senderWidth = 30
	m.timeline.subjectWidth = 42
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "solo",
			Sender:    "Mina Park <mina@cobalt-works.example>",
			Subject:   "Next Steps with Cobalt Works!",
			Date:      now,
			Folder:    "INBOX",
		},
	}

	m.updateTimelineTable()
	rows := m.timelineTable.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected one single-email row, got %d", len(rows))
	}
	sender := stripANSI(rows[0][1])
	if strings.Contains(sender, "▸") || strings.Contains(sender, "▾") {
		t.Fatalf("expected single-email row sender not to include thread disclosure marker, got %q", sender)
	}
}

// --- linkifyURLs and wrapText ---

func TestShortenURL(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"https://example.com", "example.com"},
		{"https://example.com/", "example.com"},
		{"https://example.com/path/to/page", "example.com/path/to/page"},
		// Long URL gets truncated at 50 chars (47 + "...")
		{"https://link.tesla.com/ls/click?upn=u001.H5C2HFm3je6EmjZ4beiadaSm-2B4nZaA6qcP02EtsQ52UPTrOhQONlsTHMt2JXHIqR", "link.tesla.com/ls/click?upn=u001.H5C2HFm3je6Emj..."},
	}
	for _, tt := range tests {
		got := shortenURL(tt.input)
		if got != tt.want {
			t.Errorf("shortenURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLinkifyURLs(t *testing.T) {
	input := "Visit https://example.com/page for details."
	result := linkifyURLs(input)
	// Should contain OSC 8 escape sequences
	if !strings.Contains(result, "\033]8;;") {
		t.Errorf("expected OSC 8 sequence in output, got: %q", result)
	}
	// Should contain the shortened label
	if !strings.Contains(result, "example.com/page") {
		t.Errorf("expected shortened label in output, got: %q", result)
	}
	// The raw URL should only appear inside the OSC 8 escape, not as standalone visible text.
	// Strip all OSC 8 sequences and check the URL is gone from visible content.
	visible := strings.ReplaceAll(result, "\033]8;;https://example.com/page\033\\", "")
	visible = strings.ReplaceAll(visible, "\033]8;;\033\\", "")
	if strings.Contains(visible, "https://example.com/page") {
		t.Errorf("raw URL should not appear as visible text, got: %q", visible)
	}
}

func TestWrapTextWithOSC8(t *testing.T) {
	// A line with an OSC 8 link should wrap based on visible width only
	link := "\033]8;;https://example.com\033\\example.com\033]8;;\033\\"
	text := "Click " + link + " for info"
	// Visible text: "Click example.com for info" = 26 chars
	lines := wrapText(text, 30)
	if len(lines) != 1 {
		t.Errorf("expected 1 line at width 30, got %d: %v", len(lines), lines)
	}
	// At width 20, should wrap
	lines = wrapText(text, 20)
	if len(lines) < 2 {
		t.Errorf("expected 2+ lines at width 20, got %d", len(lines))
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
