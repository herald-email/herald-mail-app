package app

import (
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func timelineGroupingEmails() []*models.EmailData {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	return []*models.EmailData{
		{
			MessageID: "alice-news",
			Sender:    "Alice Alerts <alice@news.example.co.uk>",
			Subject:   "Flash sale",
			Date:      now,
			Folder:    "INBOX",
		},
		{
			MessageID: "alice-account",
			Sender:    "Alice Alerts <alice@news.example.co.uk>",
			Subject:   "Account notice",
			Date:      now.Add(-time.Minute),
			Folder:    "INBOX",
		},
		{
			MessageID: "bob-report",
			Sender:    "Bob Digest <bob@mail.example.co.uk>",
			Subject:   "Weekly report",
			Date:      now.Add(-2 * time.Minute),
			Folder:    "INBOX",
		},
		{
			MessageID: "carol-note",
			Sender:    "Carol <carol@other.com>",
			Subject:   "Personal note",
			Date:      now.Add(-3 * time.Minute),
			Folder:    "INBOX",
		},
	}
}

func newTimelineGroupingModel(t *testing.T) *Model {
	t.Helper()
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineGroupingEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)
	return m
}

func pressTimelineGroupKey(t *testing.T, m *Model) *Model {
	t.Helper()
	return timelineKeyPress(t, m, keyRunes("G"))
}

func TestTimelineGroupingDefaultPreservesThreadMode(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = []*models.EmailData{
		{MessageID: "new", Sender: "alice@example.com", Subject: "Roadmap", Date: time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC), Folder: "INBOX"},
		{MessageID: "old", Sender: "bob@example.com", Subject: "Re: Roadmap", Date: time.Date(2026, 5, 15, 9, 0, 0, 0, time.UTC), Folder: "INBOX"},
		{MessageID: "solo", Sender: "carol@example.com", Subject: "Solo", Date: time.Date(2026, 5, 15, 8, 0, 0, 0, time.UTC), Folder: "INBOX"},
	}

	m.updateTimelineTable()

	rows := m.timelineTable.Rows()
	if len(rows) != 2 {
		t.Fatalf("default Timeline grouping should preserve thread rows, got %d rows: %#v", len(rows), rows)
	}
	if got := stripANSI(rows[0][2]); !strings.Contains(got, "[2] Roadmap") {
		t.Fatalf("expected collapsed thread subject in default mode, got %q", got)
	}
	if status := stripANSI(m.renderStatusBar()); !strings.Contains(status, "Group: Thread") {
		t.Fatalf("expected Timeline status to show default thread group, got %q", status)
	}
}

func TestTimelineGroupingUsesProviderThreadWhenSubjectChanges(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	m.timeline.emails = []*models.EmailData{
		{
			MessageID:        "<newest@example.test>",
			Sender:           "Sam Marsh <sam@example.test>",
			Subject:          "Re: Logistics for next week",
			InReplyTo:        "<middle@example.test>",
			References:       "<root@example.test> <middle@example.test>",
			ProviderThreadID: "gmail-thread-1",
			Date:             time.Date(2026, 6, 9, 15, 39, 0, 0, time.UTC),
			Folder:           "INBOX",
		},
		{
			MessageID:        "<middle@example.test>",
			Sender:           "me@example.test",
			Subject:          "Re: Anton Golubtsov and Sam Marsh",
			InReplyTo:        "<root@example.test>",
			References:       "<root@example.test>",
			ProviderThreadID: "gmail-thread-1",
			Date:             time.Date(2026, 6, 2, 9, 32, 0, 0, time.UTC),
			Folder:           "Sent",
		},
		{
			MessageID:        "<root@example.test>",
			Sender:           "John Doe <john@example.test>",
			Subject:          "Anton Golubtsov and Sam Marsh",
			ProviderThreadID: "gmail-thread-1",
			Date:             time.Date(2026, 5, 18, 8, 8, 0, 0, time.UTC),
			Folder:           "INBOX",
		},
	}

	m.updateTimelineTable()

	rows := m.timelineTable.Rows()
	if len(rows) != 1 {
		t.Fatalf("provider-thread messages with changed subjects should collapse into 1 row, got %d rows: %#v", len(rows), rows)
	}
	if got := stripANSI(rows[0][2]); !strings.Contains(got, "[3]") || !strings.Contains(got, "Logistics for next week") {
		t.Fatalf("collapsed provider-thread subject = %q, want newest subject with count", got)
	}
}

func TestTimelineExpandedThreadReplyRowsStayIndented(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	m.activeTab = tabTimeline
	subject := "Fractional AI - Anthropic Basecamp"
	m.timeline.emails = []*models.EmailData{
		{
			MessageID: "ran-reply-newest",
			Sender:    "Ran Ji <ran@example.com>",
			Subject:   "Re: " + subject,
			Date:      time.Date(2026, 6, 24, 11, 4, 0, 0, time.UTC),
			Folder:    "INBOX",
		},
		{
			MessageID: "me-reply-1",
			Sender:    "logrusadm@gmail.com",
			Subject:   "Re: " + subject,
			Date:      time.Date(2026, 6, 24, 9, 58, 0, 0, time.UTC),
			Folder:    "Sent",
		},
		{
			MessageID: "me-reply-2",
			Sender:    "logrusadm@gmail.com",
			Subject:   "Re: " + subject,
			Date:      time.Date(2026, 6, 24, 9, 53, 0, 0, time.UTC),
			Folder:    "Sent",
		},
		{
			MessageID: "ran-original",
			Sender:    "Ran Ji <ran@example.com>",
			Subject:   subject,
			Date:      time.Date(2026, 6, 19, 17, 37, 0, 0, time.UTC),
			Folder:    "INBOX",
		},
	}
	m.timeline.expandedThreads[normalizeSubject(subject)] = true

	m.updateTimelineTable()

	rows := m.timelineTable.Rows()
	if len(rows) != 4 {
		t.Fatalf("expanded thread rows = %d, want 4: %#v", len(rows), rows)
	}
	for _, idx := range []int{1, 2} {
		got := stripANSI(rows[idx][1])
		if !strings.Contains(got, threadNestedPrefix) || !strings.Contains(got, threadReplyPrefix) {
			t.Fatalf("reply child row %d sender cell = %q, want nested reply prefix", idx, got)
		}
	}
}

func TestTimelineGroupingKeyCyclesSenderDomainThread(t *testing.T) {
	m := newTimelineGroupingModel(t)

	senderMode := pressTimelineGroupKey(t, m)
	if status := stripANSI(senderMode.renderStatusBar()); !strings.Contains(status, "Group: Sender") {
		t.Fatalf("expected G to switch to sender grouping, got status %q", status)
	}
	senderRows := senderMode.timelineTable.Rows()
	if len(senderRows) != 3 {
		t.Fatalf("sender grouping should collapse repeated sender into 3 rows, got %d rows: %#v", len(senderRows), senderRows)
	}
	if got := stripANSI(senderRows[0][1]); !strings.Contains(got, "▸") || !strings.Contains(got, "Alice Alerts") {
		t.Fatalf("expected collapsed sender row for Alice, got %q", got)
	}
	if got := stripANSI(senderRows[0][2]); !strings.Contains(got, "[2]") {
		t.Fatalf("expected sender group subject to show count, got %q", got)
	}

	domainMode := pressTimelineGroupKey(t, senderMode)
	if status := stripANSI(domainMode.renderStatusBar()); !strings.Contains(status, "Group: Domain") {
		t.Fatalf("expected second G to switch to domain grouping, got status %q", status)
	}
	domainRows := domainMode.timelineTable.Rows()
	if len(domainRows) != 2 {
		t.Fatalf("domain grouping should collapse example.co.uk into 2 rows, got %d rows: %#v", len(domainRows), domainRows)
	}
	if got := stripANSI(domainRows[0][1]); !strings.Contains(got, "▸") || !strings.Contains(got, "example.co.uk") {
		t.Fatalf("expected collapsed domain row for example.co.uk, got %q", got)
	}
	if got := stripANSI(domainRows[0][2]); !strings.Contains(got, "[3]") {
		t.Fatalf("expected domain group subject to show grouped count, got %q", got)
	}

	threadMode := pressTimelineGroupKey(t, domainMode)
	if status := stripANSI(threadMode.renderStatusBar()); !strings.Contains(status, "Group: Thread") {
		t.Fatalf("expected third G to return to thread grouping, got status %q", status)
	}
}

func TestTimelineGroupedDeleteArchiveCopyUsesSenderAndDomainGroupLabels(t *testing.T) {
	m := newTimelineGroupingModel(t)

	senderMode := pressTimelineGroupKey(t, m)
	if desc := senderMode.buildDeleteDesc(); !strings.Contains(desc, "Delete sender group") || strings.Contains(desc, "thread") {
		t.Fatalf("expected sender grouped delete copy, got %q", desc)
	}
	if desc := senderMode.buildArchiveDesc(); !strings.Contains(desc, "Archive sender group") || strings.Contains(desc, "thread") {
		t.Fatalf("expected sender grouped archive copy, got %q", desc)
	}

	domainMode := pressTimelineGroupKey(t, senderMode)
	if desc := domainMode.buildDeleteDesc(); !strings.Contains(desc, "Delete domain group") || strings.Contains(desc, "thread") {
		t.Fatalf("expected domain grouped delete copy, got %q", desc)
	}
	if desc := domainMode.buildArchiveDesc(); !strings.Contains(desc, "Archive domain group") || strings.Contains(desc, "thread") {
		t.Fatalf("expected domain grouped archive copy, got %q", desc)
	}
}

func TestTimelineGroupingSwitchClosesPreviewAndKeepsSelectionByMessageID(t *testing.T) {
	m := newTimelineGroupingModel(t)
	m.timeline.selectedEmail = m.timeline.emails[0]
	m.timeline.body = &models.EmailBody{TextPlain: "open preview"}
	m.timeline.bodyMessageID = m.timeline.emails[0].MessageID
	m.timeline.selectedMessageIDs[m.timeline.emails[1].MessageID] = true

	updated := pressTimelineGroupKey(t, m)

	if updated.timeline.selectedEmail != nil || updated.timeline.body != nil || updated.timeline.bodyMessageID != "" {
		t.Fatalf("grouping switch should close the open preview, selected=%#v body=%#v bodyID=%q", updated.timeline.selectedEmail, updated.timeline.body, updated.timeline.bodyMessageID)
	}
	if !updated.timeline.selectedMessageIDs["alice-account"] {
		t.Fatalf("grouping switch should preserve message-ID selection, got %#v", updated.timeline.selectedMessageIDs)
	}
}

func TestTimelineGroupingHintsAndHelpAdvertiseGroupSwitch(t *testing.T) {
	m := newTimelineGroupingModel(t)
	hints := stripANSI(m.rawKeyHintsForWidth(120, m.chromeState(m.buildLayoutPlan(120, 40))))
	if strings.Contains(hints, "G: group") {
		t.Fatalf("expected calm Default Timeline hints to omit grouping, got %q", hints)
	}

	help := pressQuestion(m)
	rendered := stripANSI(help.View().Content)
	if !strings.Contains(rendered, "G") || !strings.Contains(rendered, "cycle Timeline grouping") {
		t.Fatalf("expected Timeline shortcut help to include grouping command, got:\n%s", rendered)
	}
}

func TestTimelineGroupingNoticeRendersInTimelineFrame(t *testing.T) {
	m := newTimelineGroupingModel(t)

	rendered := stripANSI(m.renderTimelineView())
	firstLine := strings.Split(rendered, "\n")[0]
	if !strings.Contains(firstLine, "Grouped by: Thread (G to change)") {
		t.Fatalf("expected Timeline frame notice in top border, got first line %q\n%s", firstLine, rendered)
	}

	m = pressTimelineGroupKey(t, m)
	rendered = stripANSI(m.renderTimelineView())
	firstLine = strings.Split(rendered, "\n")[0]
	if !strings.Contains(firstLine, "Grouped by: Sender (G to change)") {
		t.Fatalf("expected sender grouping notice in top border, got first line %q\n%s", firstLine, rendered)
	}

	m = pressTimelineGroupKey(t, m)
	rendered = stripANSI(m.renderTimelineView())
	firstLine = strings.Split(rendered, "\n")[0]
	if !strings.Contains(firstLine, "Grouped by: Domain (G to change)") {
		t.Fatalf("expected domain grouping notice in top border, got first line %q\n%s", firstLine, rendered)
	}
}

func TestTimelineGroupingNoticeFitsAt80Columns(t *testing.T) {
	m := makeSizedModel(t, 80, 24)
	m.activeTab = tabTimeline
	m.timeline.emails = timelineGroupingEmails()
	m.updateTimelineTable()
	m.setFocusedPanel(panelTimeline)

	rendered := m.renderTimelineView()
	assertFitsWidth(t, 80, rendered)

	firstLine := strings.Split(stripANSI(rendered), "\n")[0]
	if !strings.Contains(firstLine, "Grouped by: Thread (G to change)") {
		t.Fatalf("expected compact Timeline frame notice, got first line %q\n%s", firstLine, stripANSI(rendered))
	}
}

func TestTimelineGroupingKeyStaysInsideTextEntrySurfaces(t *testing.T) {
	t.Run("compose", func(t *testing.T) {
		m := newTimelineGroupingModel(t)
		m.activeTab = tabCompose
		focusComposeTextField(m, composeFieldBody)

		model, cmd := m.handleKeyMsg(keyRunes("G"))
		if commandIsQuit(cmd) {
			t.Fatal("plain G from compose returned quit command")
		}
		updated := model.(*Model)

		if got := updated.composeBody.Value(); got != "G" {
			t.Fatalf("compose body value=%q, want literal G", got)
		}
		if updated.timeline.groupingMode != timelineGroupingThread {
			t.Fatalf("compose G changed Timeline grouping to %s", updated.timeline.groupingMode.Label())
		}
	})

	t.Run("timeline search prompt", func(t *testing.T) {
		m := newTimelineGroupingModel(t)
		m.openTimelineSearch()

		model, cmd := m.handleKeyMsg(keyRunes("G"))
		if commandIsQuit(cmd) {
			t.Fatal("plain G from Timeline search returned quit command")
		}
		updated := model.(*Model)

		if got := updated.timeline.searchInput.Value(); got != "G" {
			t.Fatalf("Timeline search value=%q, want literal G", got)
		}
		if updated.timeline.groupingMode != timelineGroupingThread {
			t.Fatalf("Timeline search G changed grouping to %s", updated.timeline.groupingMode.Label())
		}
	})

	t.Run("prompt editor", func(t *testing.T) {
		m := newTimelineGroupingModel(t)
		m.showPromptEditor = true
		m.promptEditor = NewPromptEditor(nil, m.windowWidth, m.windowHeight)
		_ = m.promptEditor.Init()

		model, cmd := m.Update(keyRunes("G"))
		if commandIsQuit(cmd) {
			t.Fatal("plain G from prompt editor returned quit command")
		}
		updated := model.(*Model)

		if got := updated.promptEditor.name; got != "G" {
			t.Fatalf("prompt editor name=%q, want literal G", got)
		}
		if updated.timeline.groupingMode != timelineGroupingThread {
			t.Fatalf("prompt editor G changed Timeline grouping to %s", updated.timeline.groupingMode.Label())
		}
	})
}
