package cache

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestPreviewBodyCacheAddsScopedLifecycleColumns(t *testing.T) {
	c := newTestCache(t)

	cols := tableColumns(t, c.db, "email_preview_bodies")
	for _, name := range []string{"source_id", "account_id", "local_id", "uid_validity", "cache_policy", "invalidated_at"} {
		if !cols[name] {
			t.Fatalf("email_preview_bodies missing column %s", name)
		}
	}
}

func TestCachePreviewBodyByRefWritesScopeAndPolicy(t *testing.T) {
	c := newTestCache(t)
	ref := models.MessageRef{
		SourceID:    models.SourceID("work-mail"),
		AccountID:   models.AccountID("work"),
		Folder:      "INBOX",
		UID:         42,
		UIDValidity: 777,
		MessageID:   "msg-scoped",
	}.WithDefaults()

	if err := c.CachePreviewBodyByRef(ref, &models.EmailBody{TextPlain: "scoped body"}, config.CacheStoragePolicyPreserveAll); err != nil {
		t.Fatalf("CachePreviewBodyByRef: %v", err)
	}

	got, err := c.GetPreviewBodyByRef(ref)
	if err != nil {
		t.Fatalf("GetPreviewBodyByRef: %v", err)
	}
	if got == nil || got.TextPlain != "scoped body" {
		t.Fatalf("scoped preview = %#v, want cached body", got)
	}

	var sourceID, accountID, localID, policy string
	var uidValidity uint32
	if err := c.db.QueryRow(`SELECT source_id, account_id, local_id, uid_validity, cache_policy FROM email_preview_bodies WHERE message_id = ?`, ref.MessageID).Scan(&sourceID, &accountID, &localID, &uidValidity, &policy); err != nil {
		t.Fatalf("select scoped preview row: %v", err)
	}
	if sourceID != "work-mail" || accountID != "work" || localID != ref.LocalID || uidValidity != 777 || policy != config.CacheStoragePolicyPreserveAll {
		t.Fatalf("scope row = %q/%q/%q/%d/%q, want work-mail/work/%q/777/%q", sourceID, accountID, localID, uidValidity, policy, ref.LocalID, config.CacheStoragePolicyPreserveAll)
	}
}

func TestGetMessageBodyByRefRequiresPreserveAllAndHonorsInvalidation(t *testing.T) {
	c := newTestCache(t)
	ref := models.MessageRef{MessageID: "msg-body", Folder: "INBOX", UID: 7}.WithDefaults()

	if err := c.CachePreviewBodyByRef(ref, &models.EmailBody{TextPlain: "preview only"}, config.CacheStoragePolicyNoAttachments); err != nil {
		t.Fatalf("CachePreviewBodyByRef preview: %v", err)
	}
	if got, err := c.GetMessageBodyByRef(ref); err != nil || got != nil {
		t.Fatalf("GetMessageBodyByRef with no_attachments = (%#v, %v), want nil nil", got, err)
	}

	if err := c.PutMessageBodyByRef(ref, &models.EmailBody{TextPlain: "full body"}, config.CacheStoragePolicyPreserveAll); err != nil {
		t.Fatalf("PutMessageBodyByRef: %v", err)
	}
	got, err := c.GetMessageBodyByRef(ref)
	if err != nil {
		t.Fatalf("GetMessageBodyByRef preserve_all: %v", err)
	}
	if got == nil || got.TextPlain != "full body" {
		t.Fatalf("full body = %#v, want cached full body", got)
	}

	if err := c.InvalidatePreviewByRef(ref); err != nil {
		t.Fatalf("InvalidatePreviewByRef: %v", err)
	}
	if got, err := c.GetMessageBodyByRef(ref); err != nil || got != nil {
		t.Fatalf("GetMessageBodyByRef after invalidation = (%#v, %v), want nil nil", got, err)
	}
	if got, err := c.GetPreviewBodyByRef(ref); err != nil || got != nil {
		t.Fatalf("GetPreviewBodyByRef after invalidation = (%#v, %v), want nil nil", got, err)
	}
}

func TestInvalidateMessageByRefClearsBodyTextAndPreview(t *testing.T) {
	c := newTestCache(t)
	email := &models.EmailData{
		MessageID: "msg-invalidate",
		UID:       8,
		Folder:    "INBOX",
	}
	if err := c.CacheEmail(email); err != nil {
		t.Fatalf("CacheEmail: %v", err)
	}
	ref := email.MessageRef()
	if err := c.CacheBodyText(ref.MessageID, "indexed body"); err != nil {
		t.Fatalf("CacheBodyText: %v", err)
	}
	if err := c.PutMessageBodyByRef(ref, &models.EmailBody{TextPlain: "full body"}, config.CacheStoragePolicyPreserveAll); err != nil {
		t.Fatalf("PutMessageBodyByRef: %v", err)
	}

	if err := c.InvalidateMessageByRef(ref); err != nil {
		t.Fatalf("InvalidateMessageByRef: %v", err)
	}

	if got, err := c.GetBodyText(ref.MessageID); err != nil || got != "" {
		t.Fatalf("GetBodyText after invalidation = %q, %v; want empty nil", got, err)
	}
	if got, err := c.GetMessageBodyByRef(ref); err != nil || got != nil {
		t.Fatalf("GetMessageBodyByRef after invalidation = (%#v, %v), want nil nil", got, err)
	}
}

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

func TestPrunePreviewBodiesForPolicyNoAttachmentsRemovesAttachmentBytesOnly(t *testing.T) {
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
	if err := c.CachePreviewBody("msg-prune-no-attachments", body, "preserve_all"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}

	result, err := c.PrunePreviewBodiesForPolicy("no_attachments")
	if err != nil {
		t.Fatalf("PrunePreviewBodiesForPolicy: %v", err)
	}
	if result.RowsScanned != 1 || result.RowsChanged != 1 {
		t.Fatalf("result rows = scanned %d changed %d, want 1/1", result.RowsScanned, result.RowsChanged)
	}
	if result.AttachmentBytesRemoved != int64(len("pdf-bytes")) {
		t.Fatalf("AttachmentBytesRemoved = %d, want %d", result.AttachmentBytesRemoved, len("pdf-bytes"))
	}
	if result.InlineImageBytesRemoved != 0 {
		t.Fatalf("InlineImageBytesRemoved = %d, want 0", result.InlineImageBytesRemoved)
	}

	got, err := c.GetPreviewBody("msg-prune-no-attachments")
	if err != nil {
		t.Fatalf("GetPreviewBody: %v", err)
	}
	if len(got.InlineImages) != 1 || string(got.InlineImages[0].Data) != "png-bytes" {
		t.Fatalf("inline images = %#v, want image bytes retained", got.InlineImages)
	}
	if len(got.Attachments) != 1 {
		t.Fatalf("attachments = %#v, want metadata retained", got.Attachments)
	}
	if got.Attachments[0].Filename != "invoice.pdf" || got.Attachments[0].PartPath != "2" {
		t.Fatalf("attachment metadata = %#v", got.Attachments[0])
	}
	if len(got.Attachments[0].Data) != 0 {
		t.Fatalf("attachment bytes lingered after no_attachments prune: %d", len(got.Attachments[0].Data))
	}
}

func TestPrunePreviewBodiesForPolicyLightweightRemovesAttachmentAndInlineImageBytes(t *testing.T) {
	c := newTestCache(t)
	body := &models.EmailBody{
		TextPlain: "body",
		TextHTML:  "<p>body</p>",
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
		Attachments: []models.Attachment{
			{Filename: "archive.zip", MIMEType: "application/zip", Size: 3, PartPath: "2", Data: []byte("zip")},
		},
	}
	if err := c.CachePreviewBody("msg-prune-lightweight", body, "preserve_all"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}

	result, err := c.PrunePreviewBodiesForPolicy("lightweight")
	if err != nil {
		t.Fatalf("PrunePreviewBodiesForPolicy: %v", err)
	}
	if result.RowsScanned != 1 || result.RowsChanged != 1 {
		t.Fatalf("result rows = scanned %d changed %d, want 1/1", result.RowsScanned, result.RowsChanged)
	}
	if result.AttachmentBytesRemoved != int64(len("zip")) {
		t.Fatalf("AttachmentBytesRemoved = %d, want %d", result.AttachmentBytesRemoved, len("zip"))
	}
	if result.InlineImageBytesRemoved != int64(len("png-bytes")) {
		t.Fatalf("InlineImageBytesRemoved = %d, want %d", result.InlineImageBytesRemoved, len("png-bytes"))
	}

	got, err := c.GetPreviewBody("msg-prune-lightweight")
	if err != nil {
		t.Fatalf("GetPreviewBody: %v", err)
	}
	if got.TextPlain != "body" || got.TextHTML != "<p>body</p>" {
		t.Fatalf("cached body lost text: %#v", got)
	}
	if len(got.InlineImages) != 0 {
		t.Fatalf("lightweight prune should remove inline image bytes, got %#v", got.InlineImages)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].PartPath != "2" {
		t.Fatalf("attachments = %#v, want metadata retained", got.Attachments)
	}
	if len(got.Attachments[0].Data) != 0 {
		t.Fatalf("attachment bytes lingered after lightweight prune: %d", len(got.Attachments[0].Data))
	}
}

func TestPrunePreviewBodiesForPolicyLightweightRemovesInlineImagesFromNoAttachmentsRows(t *testing.T) {
	c := newTestCache(t)
	body := &models.EmailBody{
		TextPlain: "body",
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
		Attachments: []models.Attachment{
			{Filename: "invoice.pdf", MIMEType: "application/pdf", Size: 9, PartPath: "2"},
		},
	}
	if err := c.CachePreviewBody("msg-prune-inline", body, "no_attachments"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}

	result, err := c.PrunePreviewBodiesForPolicy("lightweight")
	if err != nil {
		t.Fatalf("PrunePreviewBodiesForPolicy: %v", err)
	}
	if result.RowsScanned != 1 || result.RowsChanged != 1 {
		t.Fatalf("result rows = scanned %d changed %d, want 1/1", result.RowsScanned, result.RowsChanged)
	}
	if result.AttachmentBytesRemoved != 0 {
		t.Fatalf("AttachmentBytesRemoved = %d, want 0", result.AttachmentBytesRemoved)
	}
	if result.InlineImageBytesRemoved != int64(len("png-bytes")) {
		t.Fatalf("InlineImageBytesRemoved = %d, want %d", result.InlineImageBytesRemoved, len("png-bytes"))
	}

	got, err := c.GetPreviewBody("msg-prune-inline")
	if err != nil {
		t.Fatalf("GetPreviewBody: %v", err)
	}
	if len(got.InlineImages) != 0 {
		t.Fatalf("inline image bytes lingered after lightweight prune: %#v", got.InlineImages)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].PartPath != "2" {
		t.Fatalf("attachments = %#v, want metadata retained", got.Attachments)
	}
}

func TestPrunePreviewBodiesForPolicyIsIdempotent(t *testing.T) {
	c := newTestCache(t)
	body := &models.EmailBody{
		TextPlain: "body",
		Attachments: []models.Attachment{
			{Filename: "archive.zip", MIMEType: "application/zip", Size: 3, PartPath: "2", Data: []byte("zip")},
		},
	}
	if err := c.CachePreviewBody("msg-prune-idempotent", body, "preserve_all"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}
	if _, err := c.PrunePreviewBodiesForPolicy("lightweight"); err != nil {
		t.Fatalf("first PrunePreviewBodiesForPolicy: %v", err)
	}

	result, err := c.PrunePreviewBodiesForPolicy("lightweight")
	if err != nil {
		t.Fatalf("second PrunePreviewBodiesForPolicy: %v", err)
	}
	if result.RowsScanned != 1 {
		t.Fatalf("RowsScanned = %d, want 1", result.RowsScanned)
	}
	if result.RowsChanged != 0 {
		t.Fatalf("RowsChanged = %d, want 0", result.RowsChanged)
	}
	if result.AttachmentBytesRemoved != 0 || result.InlineImageBytesRemoved != 0 {
		t.Fatalf("removed bytes on idempotent prune: %#v", result)
	}
}

func TestEstimatePreviewCacheStorageForPolicyCountsReclaimableBytes(t *testing.T) {
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
	if err := c.CachePreviewBody("msg-estimate", body, "preserve_all"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}

	lightweight, err := c.EstimatePreviewCacheStorageForPolicy("lightweight")
	if err != nil {
		t.Fatalf("EstimatePreviewCacheStorageForPolicy lightweight: %v", err)
	}
	total := int64(len("png-bytes") + len("pdf-bytes"))
	if lightweight.RowsScanned != 1 || lightweight.RowsReclaimable != 1 {
		t.Fatalf("lightweight rows = scanned %d reclaimable %d, want 1/1", lightweight.RowsScanned, lightweight.RowsReclaimable)
	}
	if lightweight.CurrentBytes != total || lightweight.ReclaimableBytes != total || lightweight.EstimatedAfterBytes != 0 {
		t.Fatalf("lightweight estimate = %#v, want current/reclaimable %d and after 0", lightweight, total)
	}
	if lightweight.ReclaimableInlineImageBytes != int64(len("png-bytes")) {
		t.Fatalf("ReclaimableInlineImageBytes = %d", lightweight.ReclaimableInlineImageBytes)
	}
	if lightweight.ReclaimableAttachmentBytes != int64(len("pdf-bytes")) {
		t.Fatalf("ReclaimableAttachmentBytes = %d", lightweight.ReclaimableAttachmentBytes)
	}

	noAttachments, err := c.EstimatePreviewCacheStorageForPolicy("no_attachments")
	if err != nil {
		t.Fatalf("EstimatePreviewCacheStorageForPolicy no_attachments: %v", err)
	}
	if noAttachments.CurrentBytes != total {
		t.Fatalf("no_attachments current bytes = %d, want %d", noAttachments.CurrentBytes, total)
	}
	if noAttachments.ReclaimableBytes != int64(len("pdf-bytes")) {
		t.Fatalf("no_attachments reclaimable bytes = %d, want attachment bytes", noAttachments.ReclaimableBytes)
	}
	if noAttachments.EstimatedAfterBytes != int64(len("png-bytes")) {
		t.Fatalf("no_attachments estimated after = %d, want inline image bytes", noAttachments.EstimatedAfterBytes)
	}

	preserveAll, err := c.EstimatePreviewCacheStorageForPolicy("preserve_all")
	if err != nil {
		t.Fatalf("EstimatePreviewCacheStorageForPolicy preserve_all: %v", err)
	}
	if preserveAll.ReclaimableBytes != 0 || preserveAll.EstimatedAfterBytes != total {
		t.Fatalf("preserve_all estimate = %#v, want no policy reclaim", preserveAll)
	}
}

func TestReclaimPreviewCacheStorageForPolicyPrunesAndCompacts(t *testing.T) {
	c := newTestCache(t)
	body := &models.EmailBody{
		TextPlain: "body",
		TextHTML:  "<p>body</p>",
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
		Attachments: []models.Attachment{
			{Filename: "archive.zip", MIMEType: "application/zip", Size: 3, PartPath: "2", Data: []byte("zip")},
		},
	}
	if err := c.CachePreviewBody("msg-reclaim", body, "preserve_all"); err != nil {
		t.Fatalf("CachePreviewBody: %v", err)
	}

	result, err := c.ReclaimPreviewCacheStorageForPolicy("lightweight")
	if err != nil {
		t.Fatalf("ReclaimPreviewCacheStorageForPolicy: %v", err)
	}
	wantRemoved := int64(len("png-bytes") + len("zip"))
	if result.Estimate.ReclaimableBytes != wantRemoved {
		t.Fatalf("Estimate.ReclaimableBytes = %d, want %d", result.Estimate.ReclaimableBytes, wantRemoved)
	}
	if result.PruneResult.RowsChanged != 1 {
		t.Fatalf("RowsChanged = %d, want 1", result.PruneResult.RowsChanged)
	}
	if !result.Compacted || result.CompactionError != "" {
		t.Fatalf("compaction status = compacted %v error %q, want compacted without error", result.Compacted, result.CompactionError)
	}

	got, err := c.GetPreviewBody("msg-reclaim")
	if err != nil {
		t.Fatalf("GetPreviewBody: %v", err)
	}
	if got.TextPlain != "body" || got.TextHTML != "<p>body</p>" {
		t.Fatalf("reclaim lost preview text/html: %#v", got)
	}
	if len(got.InlineImages) != 0 {
		t.Fatalf("inline image bytes remained after reclaim: %#v", got.InlineImages)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].PartPath != "2" || len(got.Attachments[0].Data) != 0 {
		t.Fatalf("attachment metadata/data after reclaim = %#v", got.Attachments)
	}

	after, err := c.EstimatePreviewCacheStorageForPolicy("lightweight")
	if err != nil {
		t.Fatalf("EstimatePreviewCacheStorageForPolicy after reclaim: %v", err)
	}
	if after.CurrentBytes != 0 || after.ReclaimableBytes != 0 {
		t.Fatalf("after estimate = %#v, want no remaining binary bytes", after)
	}
}
