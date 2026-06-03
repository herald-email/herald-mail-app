package imap

import (
	"strings"
	"testing"
)

func TestParseMIMEBodyCapturesInlineTextCalendarInvitation(t *testing.T) {
	raw := strings.Join([]string{
		"From: Scheduler <scheduler@example.test>",
		"To: Alice <alice@example.test>",
		"Subject: Invite",
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="b"`,
		"",
		"--b",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"See you there.",
		"--b",
		"Content-Type: text/calendar; charset=utf-8; method=REQUEST",
		"",
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"METHOD:REQUEST",
		"BEGIN:VEVENT",
		"UID:inline-invite@example.test",
		"SUMMARY:Inline planning",
		"DTSTART:20260602T160000Z",
		"DTEND:20260602T163000Z",
		"END:VEVENT",
		"END:VCALENDAR",
		"--b--",
		"",
	}, "\r\n")

	body, err := ParseMIMEBody([]byte(raw))
	if err != nil {
		t.Fatalf("ParseMIMEBody: %v", err)
	}
	if len(body.CalendarInvitations) != 1 {
		t.Fatalf("calendar invitations = %#v, want one inline invitation", body.CalendarInvitations)
	}
	invite := body.CalendarInvitations[0]
	if invite.Filename != "" || invite.MIMEType != "text/calendar" || invite.PartPath != "2" {
		t.Fatalf("inline invite metadata = %#v", invite)
	}
	if !strings.Contains(invite.Data, "UID:inline-invite@example.test") || !strings.Contains(invite.Data, "METHOD:REQUEST") {
		t.Fatalf("inline invite data missing ICS content:\n%s", invite.Data)
	}
}

func TestParseMIMEBodyCapturesICSAttachmentInvitation(t *testing.T) {
	raw := strings.Join([]string{
		"From: Scheduler <scheduler@example.test>",
		"To: Alice <alice@example.test>",
		"Subject: Invite",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b"`,
		"",
		"--b",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Attached invite.",
		"--b",
		`Content-Type: text/calendar; charset=utf-8; method=REQUEST; name="invite.ics"`,
		`Content-Disposition: attachment; filename="invite.ics"`,
		"",
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:attachment-invite@example.test",
		"SUMMARY:Attachment planning",
		"DTSTART:20260602T170000Z",
		"DTEND:20260602T173000Z",
		"END:VEVENT",
		"END:VCALENDAR",
		"--b--",
		"",
	}, "\r\n")

	body, err := ParseMIMEBody([]byte(raw))
	if err != nil {
		t.Fatalf("ParseMIMEBody: %v", err)
	}
	if len(body.Attachments) != 1 {
		t.Fatalf("attachments = %#v, want one downloadable .ics attachment", body.Attachments)
	}
	if len(body.CalendarInvitations) != 1 {
		t.Fatalf("calendar invitations = %#v, want one attachment-backed invitation", body.CalendarInvitations)
	}
	invite := body.CalendarInvitations[0]
	if invite.Filename != "invite.ics" || invite.MIMEType != "text/calendar" || invite.PartPath != "2" {
		t.Fatalf("attachment invite metadata = %#v", invite)
	}
	if !strings.Contains(invite.Data, "UID:attachment-invite@example.test") {
		t.Fatalf("attachment invite data missing ICS content:\n%s", invite.Data)
	}
}
