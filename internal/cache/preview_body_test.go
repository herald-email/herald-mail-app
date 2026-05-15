package cache

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestPreviewBodyCacheLightweightStripsBinaryData(t *testing.T) {
	c := newTestCache(t)
	body := &models.EmailBody{
		From:      "sender@example.com",
		To:        "reader@example.com",
		Subject:   "Preview cache",
		TextPlain: "plain body",
		TextHTML:  "<p>plain body</p>",
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
		Attachments: []models.Attachment{
			{Filename: "invoice.pdf", MIMEType: "application/pdf", Size: 9, PartPath: "2", Data: []byte("pdf-bytes")},
		},
		ListUnsubscribe: "<mailto:leave@example.com>",
	}

	if err := c.CachePreviewBody("msg-light", body, "lightweight"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}
	got, err := c.GetPreviewBody("msg-light")
	if err != nil {
		t.Fatalf("GetPreviewBody: %v", err)
	}
	if got == nil {
		t.Fatal("expected cached preview body")
	}
	if got.TextPlain != "plain body" || got.TextHTML != "<p>plain body</p>" {
		t.Fatalf("cached body lost text: %#v", got)
	}
	if len(got.InlineImages) != 0 {
		t.Fatalf("lightweight cache should omit inline image bytes, got %#v", got.InlineImages)
	}
	if len(got.Attachments) != 1 {
		t.Fatalf("attachments = %#v, want one metadata row", got.Attachments)
	}
	if got.Attachments[0].Filename != "invoice.pdf" || got.Attachments[0].PartPath != "2" {
		t.Fatalf("attachment metadata = %#v", got.Attachments[0])
	}
	if len(got.Attachments[0].Data) != 0 {
		t.Fatalf("lightweight cache stored attachment bytes: %d", len(got.Attachments[0].Data))
	}
}

func TestPreviewBodyCacheNoAttachmentsKeepsInlineImagesOnly(t *testing.T) {
	c := newTestCache(t)
	body := &models.EmailBody{
		TextPlain: "body",
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
		Attachments: []models.Attachment{
			{Filename: "invoice.pdf", MIMEType: "application/pdf", Size: 9, PartPath: "2", Data: []byte("pdf-bytes")},
		},
	}

	if err := c.CachePreviewBody("msg-no-attachments", body, "no_attachments"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}
	got, err := c.GetPreviewBody("msg-no-attachments")
	if err != nil {
		t.Fatalf("GetPreviewBody: %v", err)
	}
	if len(got.InlineImages) != 1 || string(got.InlineImages[0].Data) != "png-bytes" {
		t.Fatalf("inline images = %#v, want image bytes retained", got.InlineImages)
	}
	if len(got.Attachments) != 1 || len(got.Attachments[0].Data) != 0 {
		t.Fatalf("attachments = %#v, want metadata without bytes", got.Attachments)
	}
}

func TestPreviewBodyCachePreserveAllKeepsAttachmentBytes(t *testing.T) {
	c := newTestCache(t)
	body := &models.EmailBody{
		TextPlain: "body",
		Attachments: []models.Attachment{
			{Filename: "archive.zip", MIMEType: "application/zip", Size: 3, PartPath: "2", Data: []byte("zip")},
		},
	}

	if err := c.CachePreviewBody("msg-all", body, "preserve_all"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}
	got, err := c.GetPreviewBody("msg-all")
	if err != nil {
		t.Fatalf("GetPreviewBody: %v", err)
	}
	if len(got.Attachments) != 1 || string(got.Attachments[0].Data) != "zip" {
		t.Fatalf("attachments = %#v, want preserved bytes", got.Attachments)
	}
}
