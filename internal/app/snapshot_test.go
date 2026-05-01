package app

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

var updateSnapshots = flag.Bool("update", false, "update golden snapshots")

// requireGolden compares got against the golden file at path.
// Pass -update to the test binary to write got to the file instead.
func requireGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *updateSnapshots {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	if !bytes.Equal(want, got) {
		// teatest captures the whole terminal stream. Bubble Tea can legitimately
		// repaint the same final screen more than once before Quit completes, so
		// compare a stable final-frame view before treating raw byte drift as a
		// visual snapshot failure.
		if bytes.Equal(normalizeSnapshotForCompare(want), normalizeSnapshotForCompare(got)) {
			return
		}
		t.Fatalf("snapshot mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

func normalizeSnapshotForCompare(b []byte) []byte {
	s := string(b)
	if idx := strings.LastIndex(s, "\x1b[2J\x1b[H"); idx >= 0 {
		s = s[idx+len("\x1b[2J\x1b[H"):]
	}
	s = stripANSI(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	s = strings.Join(lines, "\n")
	if idx := strings.LastIndex(s, " Herald"); idx >= 0 {
		s = s[idx:]
	}
	return []byte(strings.TrimRight(s, "\n"))
}

func TestNormalizeSnapshotForCompareUsesLastRenderedFrame(t *testing.T) {
	got := []byte("\x1b[?25l\r Herald\nold frame\x1b[2J\x1b[H\x1b[8A\r Herald\nfinal frame\x1b[?25h")
	want := []byte("\x1b[?25l\x1b[2J\x1b[H\r Herald\nfinal frame\x1b[?25h")

	if !bytes.Equal(normalizeSnapshotForCompare(got), normalizeSnapshotForCompare(want)) {
		t.Fatalf("expected duplicate terminal frames to normalize to the final visible frame\ngot:\n%s\nwant:\n%s", normalizeSnapshotForCompare(got), normalizeSnapshotForCompare(want))
	}
}

// testModelWithEmails creates a Model with the given emails loaded into the timeline.
func testModelWithEmails(emails []*models.EmailData) *Model {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.windowWidth = 120
	m.windowHeight = 40
	m.loading = false
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	if len(emails) > 0 {
		m.timeline.emails = emails
		m.updateTimelineTable()
	}
	return m
}

func mockEmails() []*models.EmailData {
	return []*models.EmailData{
		{
			MessageID: "msg-001",
			Sender:    "alice@example.com",
			Subject:   "Meeting tomorrow",
			Date:      time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
			Size:      1200,
			Folder:    "INBOX",
		},
		{
			MessageID: "msg-002",
			Sender:    "bob@example.com",
			Subject:   "Invoice #4521",
			Date:      time.Date(2026, 4, 1, 8, 30, 0, 0, time.UTC),
			Size:      3400,
			Folder:    "INBOX",
		},
		{
			MessageID: "msg-003",
			Sender:    "carol@example.com",
			Subject:   "Quarterly report",
			Date:      time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC),
			Size:      8900,
			Folder:    "INBOX",
		},
	}
}

func freezeComposeCursors(m *Model) {
	toStyles := m.composeTo.Styles()
	toStyles.Cursor.Blink = false
	m.composeTo.SetStyles(toStyles)

	ccStyles := m.composeCC.Styles()
	ccStyles.Cursor.Blink = false
	m.composeCC.SetStyles(ccStyles)

	bccStyles := m.composeBCC.Styles()
	bccStyles.Cursor.Blink = false
	m.composeBCC.SetStyles(bccStyles)

	subjectStyles := m.composeSubject.Styles()
	subjectStyles.Cursor.Blink = false
	m.composeSubject.SetStyles(subjectStyles)

	bodyStyles := m.composeBody.Styles()
	bodyStyles.Cursor.Blink = false
	m.composeBody.SetStyles(bodyStyles)

	aiInputStyles := m.composeAIInput.Styles()
	aiInputStyles.Cursor.Blink = false
	m.composeAIInput.SetStyles(aiInputStyles)

	aiResponseStyles := m.composeAIResponse.Styles()
	aiResponseStyles.Cursor.Blink = false
	m.composeAIResponse.SetStyles(aiResponseStyles)
}

func snapshotView(m *Model) []byte {
	return []byte(normalizeSnapshot(m.View().Content))
}

func normalizeSnapshot(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func TestSnapshot_TimelineEmpty(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabTimeline
	requireGolden(t, "testdata/snapshots/timeline_empty.txt", snapshotView(m))
}

func TestSnapshot_TimelinePopulated(t *testing.T) {
	m := testModelWithEmails(mockEmails())
	m.activeTab = tabTimeline
	requireGolden(t, "testdata/snapshots/timeline_populated.txt", snapshotView(m))
}

func TestSnapshot_ComposeBlank(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabCompose
	m.composeField = 0
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	freezeComposeCursors(m)
	requireGolden(t, "testdata/snapshots/compose_blank.txt", snapshotView(m))
}

func TestSnapshot_ComposeWithCCBCC(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabCompose
	m.composeField = 0
	m.composeTo.SetValue("alice@example.com")
	m.composeCC.SetValue("bob@example.com")
	m.composeBCC.SetValue("carol@example.com")
	m.composeSubject.SetValue("Hello world")
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	freezeComposeCursors(m)
	requireGolden(t, "testdata/snapshots/compose_with_cc_bcc.txt", snapshotView(m))
}

func TestSnapshot_ComposeAIPanel(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabCompose
	m.composeField = 4
	m.composeBody.SetValue("Hey Alice,\n\nCan we meet tomorrow for a quick sync?\n\nThanks")
	m.composeAIPanel = true
	freezeComposeCursors(m)
	// Pre-populate with a fake AI result so the diff renders
	original := m.composeBody.Value()
	revised := "Hi Alice,\n\nAre you available tomorrow for a quick catch-up?\n\nBest regards"
	m.composeAIDiff = wordDiff(original, revised)
	m.composeAIResponse.SetValue(revised)
	m.updateTableDimensions(m.windowWidth, m.windowHeight)
	requireGolden(t, "testdata/snapshots/compose_ai_panel.txt", snapshotView(m))
}
