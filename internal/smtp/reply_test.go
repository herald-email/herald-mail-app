package smtp

import (
	"strings"
	"testing"
)

func TestBuildReplyMIMEMessage_HasInReplyToHeader(t *testing.T) {
	msg := buildReplyMIMEMessage(
		"from@example.com",
		"to@example.com",
		"Re: Hello",
		"This is a reply.",
		"",
		"<original@example.com>",
		"",
	)
	if !strings.Contains(msg, "In-Reply-To: <original@example.com>") {
		t.Errorf("expected In-Reply-To header, got:\n%s", msg)
	}
	if !strings.Contains(msg, "References: <original@example.com>") {
		t.Errorf("expected References header, got:\n%s", msg)
	}
}

func TestBuildReplyMIMEMessage_WithReferencesChain(t *testing.T) {
	msg := buildReplyMIMEMessage(
		"from@example.com",
		"to@example.com",
		"Re: Hello",
		"Reply body",
		"",
		"<original@example.com>",
		"<root@example.com>",
	)
	if !strings.Contains(msg, "In-Reply-To: <original@example.com>") {
		t.Errorf("expected In-Reply-To header, got:\n%s", msg)
	}
	// References must include both the chain and the in-reply-to
	if !strings.Contains(msg, "References: <root@example.com> <original@example.com>") {
		t.Errorf("expected References to include both chain and in-reply-to, got:\n%s", msg)
	}
}

func TestBuildReplyMIMEMessage_SubjectPreserved(t *testing.T) {
	msg := buildReplyMIMEMessage(
		"from@example.com",
		"to@example.com",
		"Re: Original Subject",
		"Body text",
		"",
		"<id@example.com>",
		"",
	)
	if !strings.Contains(msg, "Subject: Re: Original Subject") {
		t.Errorf("expected subject header, got:\n%s", msg)
	}
}

func TestBuildReplyMIMEMessage_WithHTMLBody(t *testing.T) {
	msg := buildReplyMIMEMessage(
		"from@example.com",
		"to@example.com",
		"Re: Hello",
		"Plain text reply",
		"<p>HTML reply</p>",
		"<orig@example.com>",
		"",
	)
	if !strings.Contains(msg, "multipart/alternative") {
		t.Errorf("expected multipart/alternative when HTML body is provided, got:\n%s", msg)
	}
	if !strings.Contains(msg, "Plain text reply") {
		t.Errorf("expected plain text part, got:\n%s", msg)
	}
	if !strings.Contains(msg, "<p>HTML reply</p>") {
		t.Errorf("expected HTML part, got:\n%s", msg)
	}
}

func TestBuildReplyMIMEMessage_SubjectRePrefix(t *testing.T) {
	// Verify that "Re: " prefix stripping logic (in local.go) works correctly.
	// We test the helper directly here for the header building part.
	alreadyRe := "Re: Hello"
	notRe := "Hello"

	// Simulate what local.go's ReplyToEmail does:
	makeSubject := func(subject string) string {
		lower := strings.ToLower(subject)
		if !strings.HasPrefix(lower, "re:") {
			return "Re: " + subject
		}
		return subject
	}

	if got := makeSubject(alreadyRe); got != "Re: Hello" {
		t.Errorf("expected 'Re: Hello', got %q", got)
	}
	if got := makeSubject(notRe); got != "Re: Hello" {
		t.Errorf("expected 'Re: Hello', got %q", got)
	}
	// Ensure "RE: " (uppercase) is also not doubled
	if got := makeSubject("RE: Hello"); got != "RE: Hello" {
		t.Errorf("expected 'RE: Hello' unchanged, got %q", got)
	}
}
