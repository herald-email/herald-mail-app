package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestTimelineEmailBodyErrorShowsRootCauseDetails(t *testing.T) {
	m := makeSizedModel(t, 120, 40)
	email := &models.EmailData{
		SourceID:    "gmail-api",
		AccountID:   "work",
		LocalID:     "mail:gmail-api:work:INBOX:msg-auth",
		UIDValidity: 777,
		MessageID:   "msg-auth",
		Folder:      "INBOX",
		UID:         55,
	}
	m.timeline.selectedEmail = email

	model, _, handled := m.handleTimelineMsg(EmailBodyMsg{
		MessageRef: email.MessageRef(),
		MessageID:  email.MessageID,
		Folder:     email.Folder,
		UID:        email.UID,
		Err:        errors.New("select folder INBOX: NO [AUTHENTICATIONFAILED] invalid credentials"),
	})
	if !handled {
		t.Fatal("expected EmailBodyMsg to be handled")
	}
	updated := model.(*Model)
	if updated.timeline.body == nil {
		t.Fatal("expected failure body")
	}
	text := updated.timeline.body.TextPlain
	for _, want := range []string{
		"(Failed to load body)",
		"Reason:",
		"Re-authenticate",
		"Details: select folder INBOX",
		"source_id=gmail-api",
		"account_id=work",
		"Press Ctrl+G",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("failure body missing %q:\n%s", want, text)
		}
	}
}

type stalePreviewCacheBackend struct {
	stubBackend
	deletedRefs []models.MessageRef
}

func (b *stalePreviewCacheBackend) DeleteCachedEmail(ref models.MessageRef) error {
	b.deletedRefs = append(b.deletedRefs, ref.WithDefaults())
	return nil
}

func TestTimelineStaleGmailNotFoundUpdatesCacheAndMovesNext(t *testing.T) {
	backend := &stalePreviewCacheBackend{}
	m := New(backend, nil, "", nil, false)
	updatedModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updatedModel.(*Model)
	m.activeTab = tabTimeline
	m.loading = false
	m.currentFolder = "INBOX"

	stale := &models.EmailData{
		SourceID:  "gmail-api",
		AccountID: "work",
		LocalID:   "mail:gmail-api:work:INBOX:gmail:missing",
		MessageID: "msg-missing",
		Folder:    "INBOX",
		UID:       55,
	}
	next := &models.EmailData{
		SourceID:  "gmail-api",
		AccountID: "work",
		LocalID:   "mail:gmail-api:work:INBOX:gmail:next",
		MessageID: "msg-next",
		Folder:    "INBOX",
		UID:       56,
	}
	m.timeline.emails = []*models.EmailData{stale, next}
	m.timeline.selectedEmail = stale
	m.timeline.bodyLoading = true
	m.updateTimelineTable()
	m.timelineTable.SetCursor(0)

	model, cmd, handled := m.handleTimelineMsg(EmailBodyMsg{
		MessageRef: stale.MessageRef(),
		MessageID:  stale.MessageID,
		Folder:     stale.Folder,
		UID:        stale.UID,
		Err:        errors.New(`gmail api GET /gmail/v1/users/me/messages/missing: status 404: { "error": { "code": 404, "message": "Requested entity was not found.", "status": "NOT_FOUND" } }`),
	})
	if !handled {
		t.Fatal("expected EmailBodyMsg to be handled")
	}
	updated := model.(*Model)
	requireMessageAbsent(t, updated.timeline.emails, stale.MessageID)
	requireMessagePresent(t, updated.timeline.emails, next.MessageID)
	if len(backend.deletedRefs) != 1 {
		t.Fatalf("DeleteCachedEmail calls = %d, want 1", len(backend.deletedRefs))
	}
	if got := backend.deletedRefs[0].LocalID; got != stale.LocalID {
		t.Fatalf("deleted local_id = %q, want %q", got, stale.LocalID)
	}
	if updated.timeline.selectedEmail == nil || updated.timeline.selectedEmail.MessageID != next.MessageID {
		t.Fatalf("selectedEmail = %#v, want next message", updated.timeline.selectedEmail)
	}
	if !updated.timeline.bodyLoading {
		t.Fatal("expected next message body load to start")
	}
	if updated.timeline.body != nil || updated.timeline.bodyMessageID != "" {
		t.Fatalf("expected stale failure body not to render, body=%#v bodyMessageID=%q", updated.timeline.body, updated.timeline.bodyMessageID)
	}
	if cmd == nil {
		t.Fatal("expected command to load next message body")
	}
}

func TestProblemReportCommandWritesLastEventsAndPreviewFailure(t *testing.T) {
	reportDir := t.TempDir()
	t.Setenv("HERALD_REPORT_DIR", reportDir)

	m := makeSizedModel(t, 120, 40)
	m.configPath = "/tmp/herald-test.yaml"
	m.statusMessage = "Body load failed: connection dropped"
	email := &models.EmailData{
		SourceID:    "gmail-api",
		AccountID:   "work",
		LocalID:     "mail:gmail-api:work:INBOX:msg-report",
		UIDValidity: 888,
		MessageID:   "msg-report",
		Folder:      "INBOX",
		UID:         91,
	}
	m.timeline.selectedEmail = email
	m.timeline.previewLoad = previewLoadTelemetry{
		MessageRef: email.MessageRef(),
		MessageID:  email.MessageID,
		Folder:     email.Folder,
		UID:        email.UID,
		Source:     previewLoadSourceIMAP,
		Duration:   5 * time.Second,
		Err:        "uid fetch: EOF",
	}
	m.logViewer.AddLog("INFO", "Preview load: message_id=msg-report status=error")
	m.logViewer.AddLog("WARN", "Failed to fetch email body: uid fetch: EOF")

	msg := m.writeProblemReportCmd()().(ProblemReportMsg)
	if msg.Err != nil {
		t.Fatalf("writeProblemReportCmd error: %v", msg.Err)
	}
	if !strings.HasPrefix(msg.Path, reportDir) {
		t.Fatalf("report path = %q, want inside %q", msg.Path, reportDir)
	}
	raw, err := os.ReadFile(msg.Path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	report := string(raw)
	for _, want := range []string{
		"# Herald Problem Report",
		"Debug enabled:",
		"Config path:",
		"source_id: gmail-api",
		"account_id: work",
		"uid: 91",
		"uid_validity: 888",
		"uid fetch: EOF",
		"Last 100 events",
		"Failed to fetch email body",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("problem report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "/tmp/herald-test.yaml") {
		t.Fatalf("problem report leaked raw config path:\n%s", report)
	}
}

func TestProblemReportMasksPrivateDiagnostics(t *testing.T) {
	reportDir := t.TempDir()
	t.Setenv("HERALD_REPORT_DIR", reportDir)

	m := makeSizedModel(t, 120, 40)
	m.configPath = filepath.Join("/Users", "alice", ".herald", "conf.yaml")
	m.statusMessage = "Failed loading sender=person@example.com subject=\"Project launch\""
	email := &models.EmailData{
		SourceID:    "gmail-api",
		AccountID:   "work",
		LocalID:     "mail:gmail-api:work:INBOX:<secret-message@example.com>",
		UIDValidity: 888,
		MessageID:   "<secret-message@example.com>",
		Folder:      "Folders/Private Plans",
		UID:         91,
	}
	m.timeline.selectedEmail = email
	m.timeline.previewLoad = previewLoadTelemetry{
		MessageRef: email.MessageRef(),
		MessageID:  email.MessageID,
		Folder:     email.Folder,
		UID:        email.UID,
		Source:     previewLoadSourceIMAP,
		Duration:   5 * time.Second,
		Err:        "uid fetch failed for person@example.com at /Users/alice/.herald/conf.yaml",
	}
	m.logViewer.AddLog("WARN", "sender=person@example.com message_id=<secret-message@example.com> subject=\"Project launch\" path=/Users/alice/.herald/conf.yaml")

	report := formatProblemReport(m.problemReportSnapshot(time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)))
	for _, leaked := range []string{"person@example.com", "secret-message@example.com", "Project launch", "/Users/alice/.herald/conf.yaml", "Folders/Private Plans"} {
		if strings.Contains(report, leaked) {
			t.Fatalf("problem report leaked private value %q:\n%s", leaked, report)
		}
	}
	if strings.Count(report, "?????????") < 5 {
		t.Fatalf("expected masked values in problem report, got:\n%s", report)
	}
}

func TestProblemReportShortcutOpensModalWithoutWriting(t *testing.T) {
	reportDir := t.TempDir()
	t.Setenv("HERALD_REPORT_DIR", reportDir)

	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabTimeline
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-modal", Folder: "INBOX", UID: 42}

	model, cmd := m.handleKeyMsg(keyCtrl('g'))
	if cmd != nil {
		t.Fatal("ctrl+g should open the report modal without writing a report command")
	}
	updated := model.(*Model)
	if !updated.showProblemReport {
		t.Fatal("expected problem report modal to be open")
	}
	entries, err := os.ReadDir(reportDir)
	if err != nil {
		t.Fatalf("read report dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("report shortcut wrote files before user chose save: %#v", entries)
	}

	rendered := updated.View().Content
	stripped := stripANSI(rendered)
	for _, want := range []string{
		"Report Problem",
		"[e] Email Support",
		"[c] Copy report/logs",
		"[s] Save to ~/.herald/reports",
		"[f] feedback form",
		"support will reply to the From address",
	} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("report modal missing %q:\n%s", want, stripped)
		}
	}
	if !strings.Contains(rendered, "\x1b]8;;https://herald-mail.app/feedback/\x1b\\") {
		t.Fatalf("feedback form should be an OSC 8 link, got raw:\n%q", rendered)
	}
}

func TestProblemReportShortcutOpensFromContactPreview(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabContacts
	m.timeline.selectedEmail = &models.EmailData{MessageID: "stale-timeline", Folder: "INBOX", UID: 1}
	email := &models.EmailData{
		SourceID:    "gmail-api",
		AccountID:   "work",
		LocalID:     "mail:gmail-api:work:INBOX:msg-contact",
		UIDValidity: 999,
		MessageID:   "msg-contact",
		Folder:      "INBOX",
		UID:         43,
	}
	m.contactPreviewEmail = email

	model, cmd := m.handleKeyMsg(keyCtrl('g'))
	if cmd != nil {
		t.Fatal("ctrl+g should open the report modal without writing a report command")
	}
	updated := model.(*Model)
	if !updated.showProblemReport {
		t.Fatal("expected problem report modal to be open")
	}
	snapshot := updated.problemReportSnapshot(time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC))
	if !snapshot.HasSelected {
		t.Fatal("expected contact preview email in problem report snapshot")
	}
	if snapshot.Selected.MessageID != email.MessageID {
		t.Fatalf("snapshot selected message = %q, want %q", snapshot.Selected.MessageID, email.MessageID)
	}
}

func TestProblemReportShortcutOpensFromNonComposeScreens(t *testing.T) {
	tests := []struct {
		name string
		tab  int
	}{
		{name: "timeline without selected message", tab: tabTimeline},
		{name: "contacts list", tab: tabContacts},
		{name: "calendar agenda", tab: tabCalendar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := makeSizedModel(t, 100, 30)
			m.activeTab = tt.tab

			model, cmd := m.handleKeyMsg(keyCtrl('g'))
			if cmd != nil {
				t.Fatal("ctrl+g should open the report modal without writing a report command")
			}
			updated := model.(*Model)
			if !updated.showProblemReport {
				t.Fatal("expected problem report modal to be open")
			}
		})
	}
}

func TestProblemReportShortcutDoesNotStealComposeBang(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabCompose

	model, _ := m.handleKeyMsg(keyRune('!'))
	updated := model.(*Model)
	if updated.showProblemReport {
		t.Fatal("plain ! should remain available for compose text instead of opening report modal")
	}
}

func TestProblemReportShortcutDoesNotStealComposeCtrlG(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabCompose

	model, _ := m.handleKeyMsg(keyCtrl('g'))
	updated := model.(*Model)
	if updated.showProblemReport {
		t.Fatal("ctrl+g should keep its Compose behavior instead of opening report modal")
	}
}

func TestProblemReportShortcutDoesNotTreatCtrlShiftOneAsReport(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabCalendar

	model, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: '!', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("ctrl+! should not run report commands")
	}
	updated := model.(*Model)
	if updated.showProblemReport {
		t.Fatal("ctrl+! should not open the report modal")
	}
}

func TestProblemReportShortcutDoesNotTreatCtrlOneAsReport(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabCalendar

	model, _ := m.handleKeyMsg(keyCtrl('1'))
	updated := model.(*Model)
	if updated.showProblemReport {
		t.Fatal("ctrl+1 should not open the report modal")
	}
}

func TestProblemReportShortcutIgnoresPlainBang(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabTimeline
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg", Folder: "INBOX", UID: 1}

	model, _ := m.handleKeyMsg(keyRune('!'))
	updated := model.(*Model)
	if updated.showProblemReport {
		t.Fatal("plain ! should not open the report modal")
	}
}

func TestProblemReportEmailActionOpensSupportComposeDraft(t *testing.T) {
	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabTimeline
	m.showProblemReport = true
	email := &models.EmailData{
		SourceID:  "gmail-api",
		AccountID: "work",
		MessageID: "msg-support",
		Folder:    "INBOX",
		UID:       42,
		Subject:   "Subject is redacted from report",
	}
	m.timeline.selectedEmail = email
	m.timeline.previewLoad = previewLoadTelemetry{
		MessageRef: email.MessageRef(),
		MessageID:  email.MessageID,
		Folder:     email.Folder,
		UID:        email.UID,
		Source:     previewLoadSourceIMAP,
		Err:        "uid fetch: EOF",
	}

	model, cmd, handled := m.handleOverlayKey(keyRune('e'))
	if !handled {
		t.Fatal("expected report email action to be handled")
	}
	if cmd != nil {
		t.Fatalf("email action should open compose synchronously, got %T", cmd)
	}
	updated := model.(*Model)
	if updated.showProblemReport {
		t.Fatal("expected modal to close after opening support draft")
	}
	if updated.activeTab != tabCompose {
		t.Fatalf("active tab = %d, want Compose", updated.activeTab)
	}
	if got := updated.composeTo.Value(); got != "support@herald-mail.app" {
		t.Fatalf("compose to = %q, want support", got)
	}
	if !strings.Contains(updated.composeSubject.Value(), "Herald problem report") {
		t.Fatalf("compose subject = %q", updated.composeSubject.Value())
	}
	body := updated.composeBody.Value()
	for _, want := range []string{
		"Please describe what happened",
		"Support will reply to the From address",
		"# Herald Problem Report",
		"uid fetch: EOF",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("support compose body missing %q:\n%s", want, body)
		}
	}
}

func TestProblemReportDefaultDirectoryIsHeraldReports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERALD_REPORT_DIR", "")

	path := problemReportDir()
	want := home + "/.herald/reports"
	if path != want {
		t.Fatalf("problemReportDir = %q, want %q", path, want)
	}
}
