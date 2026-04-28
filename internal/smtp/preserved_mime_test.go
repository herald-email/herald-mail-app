package smtp

import (
	"strings"
	"testing"
	"time"

	"mail-processor/internal/models"
)

func TestBuildPreservedMIMEMessage_SafeModePreservesHTMLAndCIDImages(t *testing.T) {
	msg, err := BuildPreservedMIMEMessage(PreservedMessageRequest{
		Kind:            models.PreservedMessageKindForward,
		Mode:            models.PreservationModeSafe,
		From:            "me@example.com",
		To:              "you@example.com",
		Subject:         "Fwd: Styled update",
		TopNoteMarkdown: "FYI **team**",
		Original: models.PreservedMessageOriginal{
			Sender:    "sender@example.com",
			Subject:   "Styled update",
			Date:      time.Date(2026, 4, 28, 9, 30, 0, 0, time.UTC),
			TextPlain: "Original plain",
			TextHTML:  `<div style="color: red" onclick="steal()"><script>bad()</script><img src="cid:logo1"><p>Original <b>HTML</b></p></div>`,
			InlineImages: []models.InlineImage{
				{ContentID: "logo1", MIMEType: "image/png", Data: []byte("png")},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildPreservedMIMEMessage: %v", err)
	}

	for _, want := range []string{
		"Content-Type: multipart/mixed",
		"Content-Type: multipart/related",
		"Content-Type: multipart/alternative",
		"<strong>team</strong>",
		`style="color: red"`,
		`src="cid:logo1"`,
		"Content-ID: <logo1>",
		"Original <b>HTML</b>",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("preserved message missing %q:\n%s", want, msg)
		}
	}
	for _, forbidden := range []string{"<script", "onclick=", "bad()"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("safe mode should strip %q:\n%s", forbidden, msg)
		}
	}
}

func TestBuildPreservedMIMEMessage_PrivacyModeStripsRemoteImages(t *testing.T) {
	msg, err := BuildPreservedMIMEMessage(PreservedMessageRequest{
		Kind:            models.PreservedMessageKindReply,
		Mode:            models.PreservationModePrivacy,
		From:            "me@example.com",
		To:              "sender@example.com",
		Subject:         "Re: Tracking",
		TopNoteMarkdown: "Thanks",
		Original: models.PreservedMessageOriginal{
			Sender:    "sender@example.com",
			Subject:   "Tracking",
			TextPlain: "Open this",
			TextHTML:  `<p>Open this</p><img src="https://tracker.example/open.gif"><img src="cid:chart">`,
			InlineImages: []models.InlineImage{
				{ContentID: "chart", MIMEType: "image/png", Data: []byte("chart")},
			},
			MessageID: "<orig@example.com>",
		},
	})
	if err != nil {
		t.Fatalf("BuildPreservedMIMEMessage: %v", err)
	}
	if strings.Contains(msg, "tracker.example") {
		t.Fatalf("privacy mode should strip remote image URLs:\n%s", msg)
	}
	if !strings.Contains(msg, `src="cid:chart"`) {
		t.Fatalf("privacy mode should preserve cid images:\n%s", msg)
	}
}

func TestBuildPreservedMIMEMessage_ForwardAttachmentsIncludedAndOmitted(t *testing.T) {
	msg, err := BuildPreservedMIMEMessage(PreservedMessageRequest{
		Kind:            models.PreservedMessageKindForward,
		Mode:            models.PreservationModeSafe,
		From:            "me@example.com",
		To:              "you@example.com",
		Subject:         "Fwd: Files",
		TopNoteMarkdown: "See attached",
		Original: models.PreservedMessageOriginal{
			Sender:    "sender@example.com",
			Subject:   "Files",
			TextPlain: "Files attached",
			Attachments: []models.Attachment{
				{Filename: "report.pdf", MIMEType: "application/pdf", Data: []byte("pdf")},
				{Filename: "secret.pdf", MIMEType: "application/pdf", Data: []byte("secret")},
			},
		},
		OmittedOriginalAttachmentNames: []string{"secret.pdf"},
	})
	if err != nil {
		t.Fatalf("BuildPreservedMIMEMessage: %v", err)
	}
	if !strings.Contains(msg, `filename="report.pdf"`) {
		t.Fatalf("expected included original attachment:\n%s", msg)
	}
	if strings.Contains(msg, `filename="secret.pdf"`) || strings.Contains(msg, "c2VjcmV0") {
		t.Fatalf("expected omitted original attachment to be absent:\n%s", msg)
	}
}

func TestBuildPreservedMIMEMessage_ReplyThreadingHeaders(t *testing.T) {
	msg, err := BuildPreservedMIMEMessage(PreservedMessageRequest{
		Kind:            models.PreservedMessageKindReply,
		Mode:            models.PreservationModeSafe,
		From:            "me@example.com",
		To:              "sender@example.com",
		Subject:         "Re: Hello",
		TopNoteMarkdown: "Replying",
		Original: models.PreservedMessageOriginal{
			Sender:     "sender@example.com",
			Subject:    "Hello",
			TextPlain:  "Original",
			MessageID:  "<orig@example.com>",
			References: "<root@example.com>",
		},
	})
	if err != nil {
		t.Fatalf("BuildPreservedMIMEMessage: %v", err)
	}
	if !strings.Contains(msg, "In-Reply-To: <orig@example.com>\r\n") {
		t.Fatalf("expected In-Reply-To header:\n%s", msg)
	}
	if !strings.Contains(msg, "References: <root@example.com> <orig@example.com>\r\n") {
		t.Fatalf("expected References chain:\n%s", msg)
	}
}
