package deeplink

import (
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestMessageLinkRoundTrip(t *testing.T) {
	target := Target{
		Kind:      KindMessage,
		Folder:    "INBOX",
		MessageID: "<message@herald.test>",
		LocalID:   "mail:work-mail:work:INBOX:message",
		SourceID:  models.SourceID("work-mail"),
		AccountID: models.AccountID("work"),
	}

	raw := Build(target)
	if !strings.HasPrefix(raw, "herald://mail/message?") {
		t.Fatalf("message link prefix = %q", raw)
	}
	got, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q): %v", raw, err)
	}
	if got.Kind != KindMessage || got.Folder != target.Folder || got.MessageID != target.MessageID || got.LocalID != target.LocalID {
		t.Fatalf("parsed target = %#v, want %#v", got, target)
	}
	if got.SourceID != target.SourceID || got.AccountID != target.AccountID {
		t.Fatalf("parsed scope = %q/%q, want %q/%q", got.SourceID, got.AccountID, target.SourceID, target.AccountID)
	}
}

func TestBuildEscapesSenderSearchAndComposeLinks(t *testing.T) {
	cases := []struct {
		name   string
		target Target
		want   string
	}{
		{
			name:   "sender",
			target: Target{Kind: KindSender, Folder: "INBOX", Sender: "alerts+ops@example.com"},
			want:   "sender=alerts%2Bops%40example.com",
		},
		{
			name:   "search",
			target: Target{Kind: KindSearch, Folder: "INBOX", Query: "quarterly invoice"},
			want:   "q=quarterly+invoice",
		},
		{
			name:   "compose",
			target: Target{Kind: KindCompose, To: "friend@example.com", Subject: "Hello there"},
			want:   "subject=Hello+there",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			raw := Build(tt.target)
			if !strings.Contains(raw, tt.want) {
				t.Fatalf("Build(%#v) = %q, want query fragment %q", tt.target, raw, tt.want)
			}
			if _, err := Parse(raw); err != nil {
				t.Fatalf("Parse(Build(%#v)): %v", tt.target, err)
			}
		})
	}
}

func TestParseRejectsInvalidLinks(t *testing.T) {
	for _, raw := range []string{
		"",
		"https://example.com/mail/message?folder=INBOX&message_id=x",
		"herald://other/message?folder=INBOX&message_id=x",
		"herald://mail/message?folder=INBOX",
		"herald://mail/folder",
		"herald://mail/search?folder=INBOX",
		"herald://mail/unknown?folder=INBOX",
	} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("Parse(%q) succeeded, want error", raw)
		}
	}
}

func TestMessageTargetFromEmail(t *testing.T) {
	email := &models.EmailData{
		SourceID:  "work-mail",
		AccountID: "work",
		MessageID: "<msg@herald.test>",
		LocalID:   "mail:work-mail:work:INBOX:msg",
		Folder:    "INBOX",
	}
	target := MessageTarget(email)
	if target.Kind != KindMessage || target.MessageID != email.MessageID || target.Folder != email.Folder {
		t.Fatalf("MessageTarget = %#v, want message folder/id", target)
	}
}
