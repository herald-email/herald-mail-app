package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func writeCompletionFixture(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
	return path
}

func mkdirCompletionFixture(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir fixture %s: %v", path, err)
	}
	return path
}

func TestAttachmentPathCompletion_UniqueFileCompletesFullPath(t *testing.T) {
	root := t.TempDir()
	want := writeCompletionFixture(t, root, "invoice.pdf")
	writeCompletionFixture(t, root, "notes.txt")

	got := completeAttachmentPath(filepath.Join(root, "inv"), root)

	if got.Status != "" {
		t.Fatalf("unexpected status: %q", got.Status)
	}
	if got.Completed != want {
		t.Fatalf("Completed=%q, want %q", got.Completed, want)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Value != want {
		t.Fatalf("expected one candidate for %q, got %+v", want, got.Candidates)
	}
}

func TestAttachmentPathCompletion_CommonPrefixAndSortedMatches(t *testing.T) {
	root := t.TempDir()
	mkdirCompletionFixture(t, root, "apricot-dir")
	writeCompletionFixture(t, root, "apple.txt")
	writeCompletionFixture(t, root, "apricot.txt")
	writeCompletionFixture(t, root, "banana.txt")

	got := completeAttachmentPath(filepath.Join(root, "ap"), root)

	wantPrefix := filepath.Join(root, "ap")
	if runtime.GOOS != "windows" {
		wantPrefix = filepath.Join(root, "ap")
	}
	if got.Completed != wantPrefix {
		t.Fatalf("Completed=%q, want common prefix %q", got.Completed, wantPrefix)
	}
	if len(got.Candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %+v", got.Candidates)
	}
	if !got.Candidates[0].IsDir || got.Candidates[0].Display != "apricot-dir/" {
		t.Fatalf("expected directory first with slash, got %+v", got.Candidates[0])
	}
	if got.Candidates[1].Display != "apple.txt" || got.Candidates[2].Display != "apricot.txt" {
		t.Fatalf("expected files sorted case-insensitively after dirs, got %+v", got.Candidates)
	}
}

func TestAttachmentPathCompletion_HidesDotfilesUntilDotPrefix(t *testing.T) {
	root := t.TempDir()
	hidden := writeCompletionFixture(t, root, ".env")
	writeCompletionFixture(t, root, "email.txt")

	withoutDot := completeAttachmentPath(filepath.Join(root, "e"), root)
	for _, c := range withoutDot.Candidates {
		if strings.HasPrefix(c.Display, ".") {
			t.Fatalf("dotfile should be hidden without dot prefix: %+v", withoutDot.Candidates)
		}
	}

	withDot := completeAttachmentPath(root+string(os.PathSeparator)+".", root)
	if len(withDot.Candidates) != 1 || withDot.Candidates[0].Value != hidden {
		t.Fatalf("dot prefix should reveal hidden file %q, got %+v", hidden, withDot.Candidates)
	}
}

func TestAttachmentPathCompletion_NoMatchesAndUnreadableStatus(t *testing.T) {
	root := t.TempDir()
	blocker := writeCompletionFixture(t, root, "not-a-dir")

	noMatch := completeAttachmentPath(filepath.Join(root, "missing"), root)
	if noMatch.Status != "No matches" {
		t.Fatalf("Status=%q, want No matches", noMatch.Status)
	}
	if noMatch.Completed != "" || len(noMatch.Candidates) != 0 {
		t.Fatalf("no matches should not mutate input, got %+v", noMatch)
	}

	unreadable := completeAttachmentPath(filepath.Join(blocker, "child"), root)
	if !strings.HasPrefix(unreadable.Status, "Cannot read directory") {
		t.Fatalf("Status=%q, want Cannot read directory", unreadable.Status)
	}
}

func TestHandleComposeKey_AttachmentTabCompletesPrefixThenShowsAndCycles(t *testing.T) {
	root := t.TempDir()
	writeCompletionFixture(t, root, "report-final.pdf")
	writeCompletionFixture(t, root, "report-notes.pdf")

	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.attachmentInputActive = true
	m.attachmentPathInput.Focus()
	m.attachmentPathInput.SetValue(filepath.Join(root, "rep"))

	model, _ := m.handleComposeKey(tea.KeyMsg{Type: tea.KeyTab})
	m = model.(*Model)
	wantPrefix := filepath.Join(root, "report-")
	if got := m.attachmentPathInput.Value(); got != wantPrefix {
		t.Fatalf("first Tab Value=%q, want common prefix %q", got, wantPrefix)
	}
	if m.attachmentCompletionVisible {
		t.Fatal("first Tab should complete prefix without showing list")
	}

	model, _ = m.handleComposeKey(tea.KeyMsg{Type: tea.KeyTab})
	m = model.(*Model)
	if !m.attachmentCompletionVisible {
		t.Fatal("second Tab should show suggestions when prefix is exhausted")
	}
	first := filepath.Join(root, "report-final.pdf")
	if got := m.attachmentPathInput.Value(); got != first {
		t.Fatalf("visible list should select first match %q, got %q", first, got)
	}

	model, _ = m.handleComposeKey(tea.KeyMsg{Type: tea.KeyTab})
	m = model.(*Model)
	second := filepath.Join(root, "report-notes.pdf")
	if got := m.attachmentPathInput.Value(); got != second {
		t.Fatalf("Tab with visible list should cycle to %q, got %q", second, got)
	}

	model, _ = m.handleComposeKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = model.(*Model)
	if got := m.attachmentPathInput.Value(); got != first {
		t.Fatalf("Shift+Tab should cycle back to %q, got %q", first, got)
	}
}

func TestHandleComposeKey_AttachmentEnterDirectoryKeepsPromptOpen(t *testing.T) {
	root := t.TempDir()
	dir := mkdirCompletionFixture(t, root, "docs")

	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabCompose
	m.attachmentInputActive = true
	m.attachmentPathInput.Focus()
	m.attachmentPathInput.SetValue(filepath.Join(root, "do"))

	model, _ := m.handleComposeKey(tea.KeyMsg{Type: tea.KeyTab})
	m = model.(*Model)
	if got := m.attachmentPathInput.Value(); got != dir+string(os.PathSeparator) {
		t.Fatalf("directory completion Value=%q, want %q", got, dir+string(os.PathSeparator))
	}

	model, cmd := m.handleComposeKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(*Model)
	if cmd != nil {
		t.Fatal("Enter on directory should not start addAttachmentCmd")
	}
	if !m.attachmentInputActive {
		t.Fatal("Enter on directory should keep attachment prompt open")
	}
}

func TestComposeAttachmentSuggestions_DoNotPushChromeOffscreen(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"report-a.pdf", "report-b.pdf", "report-c.pdf", "report-d.pdf", "report-e.pdf", "report-f.pdf"} {
		writeCompletionFixture(t, root, name)
	}

	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabCompose
	m.attachmentInputActive = true
	m.attachmentPathInput.Focus()
	m.attachmentPathInput.SetValue(filepath.Join(root, "report-"))
	model, _ := m.handleComposeKey(tea.KeyMsg{Type: tea.KeyTab})
	m = model.(*Model)

	rendered := m.renderMainView()
	if got := len(strings.Split(strings.TrimRight(stripANSI(rendered), "\n"), "\n")); got > 24 {
		t.Fatalf("compose attachment suggestions rendered %d lines at 80x24, exceeding viewport\n%s", got, stripANSI(rendered))
	}
	stripped := stripANSI(rendered)
	if !strings.Contains(stripped, "Herald") || !strings.Contains(stripped, "Attach file:") {
		t.Fatalf("expected compose chrome and attachment prompt to remain visible, got:\n%s", stripped)
	}
}
