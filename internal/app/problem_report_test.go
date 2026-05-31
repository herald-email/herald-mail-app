package app

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

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
		"Press !",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("failure body missing %q:\n%s", want, text)
		}
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
		"Config path: /tmp/herald-test.yaml",
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
}

func TestProblemReportShortcutOpensModalWithoutWriting(t *testing.T) {
	reportDir := t.TempDir()
	t.Setenv("HERALD_REPORT_DIR", reportDir)

	m := makeSizedModel(t, 100, 30)
	m.activeTab = tabTimeline
	m.timeline.selectedEmail = &models.EmailData{MessageID: "msg-modal", Folder: "INBOX", UID: 42}

	model, cmd, handled := m.handleTimelineKey(keyRune('!'))
	if !handled {
		t.Fatal("expected ! to be handled")
	}
	if cmd != nil {
		t.Fatal("! should open the report modal without writing a report command")
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
		t.Fatalf("! wrote files before user chose save: %#v", entries)
	}

	rendered := updated.View().Content
	stripped := stripANSI(rendered)
	for _, want := range []string{
		"Report Problem",
		"Email Support",
		"Copy report/logs",
		"Save to ~/.herald/reports",
		"feedback form",
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

	updated, cmd := m.handleContactsKey(keyRune('!'))
	if cmd != nil {
		t.Fatal("! should open the report modal without writing a report command")
	}
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
