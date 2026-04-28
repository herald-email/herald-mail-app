package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestAttachmentDownloadDaemonPathEscapesFilenameAndDestPath(t *testing.T) {
	got := attachmentDownloadDaemonPath("msg/123", "invoice #1.pdf", "/tmp/invoice #1.pdf")

	if !strings.Contains(got, "/v1/emails/msg%2F123/attachments/invoice%20%231.pdf") {
		t.Fatalf("daemon path did not escape path segments: %q", got)
	}
	if !strings.Contains(got, "dest_path=%2Ftmp%2Finvoice+%231.pdf") {
		t.Fatalf("daemon path did not escape dest_path query: %q", got)
	}
}

func TestFormatAttachmentDownloadErrorIncludesSuggestedPathOnConflict(t *testing.T) {
	body := []byte(`{"error":"file already exists","path":"/tmp/report.pdf","suggested_path":"/tmp/report (1).pdf"}`)

	got := formatAttachmentDownloadError(http.StatusConflict, body)

	for _, want := range []string{"file already exists", "/tmp/report.pdf", "/tmp/report (1).pdf"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q to contain %q", got, want)
		}
	}
}
