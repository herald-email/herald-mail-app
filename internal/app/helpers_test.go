package app

import (
	"testing"
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
