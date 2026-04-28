package app

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"mail-processor/internal/models"
)

func testImageEmail() *models.EmailData {
	return &models.EmailData{
		MessageID: "msg-inline-image",
		Sender:    "images@example.test",
		Subject:   "Inline image",
		Date:      time.Date(2026, 4, 28, 14, 45, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
}

func testInlineImageBody() *models.EmailBody {
	return &models.EmailBody{
		TextPlain: "Funny text goes here",
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
}

func TestImagePreviewServerServesRegisteredImagesOnly(t *testing.T) {
	server := newImagePreviewServer()
	defer server.Close()

	links, err := server.RegisterSet("msg-1", []models.InlineImage{
		{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
	})
	if err != nil {
		t.Fatalf("RegisterSet returned error: %v", err)
	}
	if len(links) != 1 || links[0].URL == "" {
		t.Fatalf("expected one preview link, got %#v", links)
	}

	resp, err := http.Get(links[0].URL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET registered image: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registered image status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll registered image: %v", err)
	}
	if string(data) != "png-bytes" {
		t.Fatalf("served data = %q, want png-bytes", data)
	}

	oldURL := links[0].URL
	links, err = server.RegisterSet("msg-2", []models.InlineImage{
		{ContentID: "chart", MIMEType: "image/jpeg", Data: []byte("jpeg-bytes")},
	})
	if err != nil {
		t.Fatalf("RegisterSet replacement returned error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected one replacement link, got %#v", links)
	}

	resp, err = http.Get(oldURL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET revoked image: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoked image status = %d, want 404", resp.StatusCode)
	}

	resp, err = http.Get(server.baseURL() + "/image/not-a-real-token") //nolint:noctx
	if err != nil {
		t.Fatalf("GET unknown token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown token status = %d, want 404", resp.StatusCode)
	}
}

func TestTimelineFullScreen_NonItermShowsLocalImageLinks(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 80, 24)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.body = testInlineImageBody()
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.fullScreen = true

	rendered := m.renderFullScreenEmail()
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("expected visible local image link label, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;http://127.0.0.1:") {
		t.Fatalf("expected OSC8 local image link, got raw:\n%q", rendered)
	}
	if strings.Contains(rendered, "\x1b]1337;File=") {
		t.Fatalf("non-iTerm fallback should not emit iTerm2 image escape, got raw:\n%q", rendered)
	}
	assertFitsWidth(t, 80, rendered)
	assertFitsHeight(t, 24, rendered)
}

func TestLocalImageLinksUseMetadataFromLinkedImage(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 80, 24)
	defer m.cleanup()
	images := []models.InlineImage{
		{ContentID: "empty", MIMEType: "image/png"},
		{ContentID: "chart", MIMEType: "image/jpeg", Data: bytesOfSize(2048)},
	}

	rendered, rows := m.renderLocalImageLinks("msg-with-empty-first-image", images, nil, 80, 4)
	plain := ansi.Strip(rendered)

	if rows != 1 {
		t.Fatalf("rows = %d, want 1 for the single linked image", rows)
	}
	if !strings.Contains(plain, "open image 2") {
		t.Fatalf("expected link label to preserve original image number, got:\n%s", plain)
	}
	if !strings.Contains(plain, "image/jpeg  2 KB") {
		t.Fatalf("expected metadata from linked image, got:\n%s", plain)
	}
	if strings.Contains(plain, "image/png") {
		t.Fatalf("empty image metadata should not be shown for linked image, got:\n%s", plain)
	}
}

func TestTimelineFullScreen_ItermRendersBoundedInlineImage(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	m := makeSizedModel(t, 80, 24)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.body = testInlineImageBody()
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.fullScreen = true

	rendered := m.renderFullScreenEmail()
	if !strings.Contains(rendered, "\x1b]1337;File=") {
		t.Fatalf("expected iTerm2 inline image escape, got raw:\n%q", rendered)
	}
	if !strings.Contains(rendered, "width=76") || !strings.Contains(rendered, "height=8") {
		t.Fatalf("expected bounded image dimensions in escape, got raw:\n%q", rendered)
	}
	if strings.Contains(rendered, "http://127.0.0.1:") {
		t.Fatalf("iTerm2 rendering should not also expose local fallback, got raw:\n%q", rendered)
	}
}

func TestItermPreviewImagesDoNotExceedAvailableRows(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	images := []models.InlineImage{
		{ContentID: "one", MIMEType: "image/png", Data: []byte("one")},
		{ContentID: "two", MIMEType: "image/png", Data: []byte("two")},
		{ContentID: "three", MIMEType: "image/png", Data: []byte("three")},
	}

	rendered, rows := renderIterm2PreviewImages(images, nil, 80, 2)

	if rows != 2 {
		t.Fatalf("reserved rows = %d, want 2", rows)
	}
	if count := strings.Count(rendered, "\x1b]1337;File="); count != 2 {
		t.Fatalf("rendered %d image escapes, want 2 within row budget; raw:\n%q", count, rendered)
	}
}

func TestCleanupFullScreen_NonItermShowsLocalImageLinks(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 80, 24)
	defer m.cleanup()
	m.activeTab = tabCleanup
	m.focusedPanel = panelDetails
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewWidth = 80
	m.cleanupPreviewEmail = testImageEmail()
	m.cleanupEmailBody = testInlineImageBody()

	rendered := m.renderCleanupPreview()
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("expected visible cleanup image link label, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;http://127.0.0.1:") {
		t.Fatalf("expected cleanup OSC8 local image link, got raw:\n%q", rendered)
	}
	assertFitsWidth(t, 80, rendered)
	assertFitsHeight(t, 24, rendered)
}

func TestSplitPreviewDoesNotPromiseFullscreenImageRenderingWhenNoViewerAvailable(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 120, 40)
	m.localImageLinks = false
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.body = testInlineImageBody()
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.previewWidth = 60

	rendered := stripANSI(m.renderEmailPreview())
	if strings.Contains(rendered, "press z for full-screen to view") {
		t.Fatalf("split preview should not promise unavailable full-screen image viewing, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "1 image") {
		t.Fatalf("split preview should still mention image count, got:\n%s", rendered)
	}
}

func bytesOfSize(size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = 'x'
	}
	return b
}
