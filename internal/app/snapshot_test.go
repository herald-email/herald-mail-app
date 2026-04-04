package app

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"

	"mail-processor/internal/models"
)

// requireGolden compares got against the golden file at path.
// Pass -update to the test binary to write got to the file instead.
func requireGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	// Check for -update via flag lookup to avoid re-declaring a flag that
	// teatest's golden package already registers.
	updateFlag := flag.Lookup("update")
	doUpdate := updateFlag != nil && updateFlag.Value.String() == "true"
	if doUpdate {
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
		t.Fatalf("snapshot mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

// testModelWithEmails creates a Model with the given emails loaded into the timeline.
func testModelWithEmails(emails []*models.EmailData) *Model {
	b := &stubBackend{}
	m := New(b, nil, "", nil, false)
	m.windowWidth = 120
	m.windowHeight = 40
	m.loading = false
	if len(emails) > 0 {
		m.timelineEmails = emails
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

// readAll reads all bytes from an io.Reader, fataling on error.
func readAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return b
}

func TestSnapshot_TimelineEmpty(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabTimeline
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	// Give the program time to render, then quit. FinalOutput waits for the
	// program to finish and returns the entire accumulated output buffer.
	// Do NOT call WaitFor before FinalOutput — WaitFor drains tm.Output() via
	// io.ReadAll, leaving nothing for FinalOutput to read.
	time.Sleep(200 * time.Millisecond)
	tm.Quit()
	requireGolden(t, "testdata/snapshots/timeline_empty.txt", readAll(t, tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second))))
}

func TestSnapshot_TimelinePopulated(t *testing.T) {
	m := testModelWithEmails(mockEmails())
	m.activeTab = tabTimeline
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	time.Sleep(200 * time.Millisecond)
	tm.Quit()
	requireGolden(t, "testdata/snapshots/timeline_populated.txt", readAll(t, tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second))))
}

func TestSnapshot_ComposeBlank(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabCompose
	m.composeField = 0
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	time.Sleep(200 * time.Millisecond)
	tm.Quit()
	requireGolden(t, "testdata/snapshots/compose_blank.txt", readAll(t, tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second))))
}

func TestSnapshot_ComposeWithCCBCC(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabCompose
	m.composeField = 0
	m.composeTo.SetValue("alice@example.com")
	m.composeCC.SetValue("bob@example.com")
	m.composeBCC.SetValue("carol@example.com")
	m.composeSubject.SetValue("Hello world")
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	time.Sleep(200 * time.Millisecond)
	tm.Quit()
	requireGolden(t, "testdata/snapshots/compose_with_cc_bcc.txt", readAll(t, tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second))))
}

func TestSnapshot_ComposeAIPanel(t *testing.T) {
	m := testModelWithEmails(nil)
	m.activeTab = tabCompose
	m.composeField = 4
	m.composeBody.SetValue("Hey Alice,\n\nCan we meet tomorrow for a quick sync?\n\nThanks")
	m.composeAIPanel = true
	// Pre-populate with a fake AI result so the diff renders
	original := m.composeBody.Value()
	revised := "Hi Alice,\n\nAre you available tomorrow for a quick catch-up?\n\nBest regards"
	m.composeAIDiff = wordDiff(original, revised)
	m.composeAIResponse.SetValue(revised)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	time.Sleep(200 * time.Millisecond)
	tm.Quit()
	requireGolden(t, "testdata/snapshots/compose_ai_panel.txt",
		readAll(t, tm.FinalOutput(t, teatest.WithFinalTimeout(5*time.Second))))
}
