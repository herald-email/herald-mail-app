package app

import (
	"io"
	"net/http"
	"regexp"
	"strconv"
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
	width := itermImageDimension(t, rendered, "width")
	height := itermImageDimension(t, rendered, "height")
	if width <= 0 || width > 78 {
		t.Fatalf("bounded image width = %d, want 1..78; raw:\n%q", width, rendered)
	}
	if height <= 0 || height > 24 {
		t.Fatalf("bounded image height = %d, want within available rows; raw:\n%q", height, rendered)
	}
	if strings.Contains(rendered, "http://127.0.0.1:") {
		t.Fatalf("iTerm2 rendering should not also expose local fallback, got raw:\n%q", rendered)
	}
}

func TestTimelineFullScreen_ItermUsesSafeLandscapeRasterBox(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	m := makeSizedModel(t, 120, 32)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>Before image.</p><img alt="Landscape" src="cid:landscape"><p>After image.</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "landscape", MIMEType: "image/png", Data: tinyPNG(t, 960, 540)},
		},
	}
	m.timeline.fullScreen = true

	rendered := m.renderFullScreenEmail()
	if !strings.Contains(rendered, "\x1b]1337;File=") {
		t.Fatalf("expected iTerm2 raster escape, got raw:\n%q", rendered)
	}
	if strings.Contains(rendered, "open image") || strings.Contains(rendered, "127.0.0.1") {
		t.Fatalf("iTerm2 target path must not use fallback links, got:\n%s", stripANSI(rendered))
	}
	width := itermImageDimension(t, rendered, "width")
	height := itermImageDimension(t, rendered, "height")
	if width < 48 || width > 72 {
		t.Fatalf("safe landscape width = %d cells, want 48..72; raw:\n%q", width, rendered)
	}
	if height != 18 {
		t.Fatalf("safe landscape height = %d cells, want 18; raw:\n%q", height, rendered)
	}
	assertFitsHeight(t, 32, rendered)
}

func TestTimelineFullScreen_RendersCIDImageInDocumentOrder(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextPlain: "Plain fallback",
		TextHTML:  `<p>Before local image.</p><img alt="Local Logo" src="cid:logo"><p>After local image.</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
	m.timeline.fullScreen = true

	rendered := stripANSI(m.renderFullScreenEmail())
	before := strings.Index(rendered, "Before local image")
	image := strings.Index(rendered, "Local Logo")
	after := strings.Index(rendered, "After local image")
	if before == -1 || image == -1 || after == -1 || !(before < image && image < after) {
		t.Fatalf("CID image should be in document order, got:\n%s", rendered)
	}
}

func TestTimelineFullScreen_DocumentRowsDriveScrollIndicator(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 80, 18)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>` + strings.Repeat("body line<br>", 30) + `</p><img alt="Chart" src="cid:chart"><p>tail</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "chart", MIMEType: "image/png", Data: []byte("chart-bytes")},
		},
	}
	m.timeline.fullScreen = true
	m.timeline.bodyScrollOffset = 999

	rendered := m.renderFullScreenEmail()
	assertFitsHeight(t, 18, rendered)
	if !strings.Contains(stripANSI(rendered), "z/esc: exit full-screen") {
		t.Fatalf("expected pinned full-screen hint, got:\n%s", stripANSI(rendered))
	}
}

func TestTimelineFullScreen_DocumentImageUsesLocalOpenLinkWhenNotIterm(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(true)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>Before</p><img alt="Logo" src="cid:logo"><p>After</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
	m.timeline.fullScreen = true

	rendered := m.renderFullScreenEmail()
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("expected local open image link, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;http://127.0.0.1:") {
		t.Fatalf("expected OSC8 localhost target, got raw:\n%q", rendered)
	}
}

func TestTimelineFullScreen_RendersNoCIDInlineImage(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(true)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextPlain: "Body before no-CID image.",
		InlineImages: []models.InlineImage{
			{MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
	m.timeline.fullScreen = true

	rendered := m.renderFullScreenEmail()
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "Inline images:") {
		t.Fatalf("expected orphan image section, got:\n%s", plain)
	}
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("expected no-CID image to get a local open link, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;http://127.0.0.1:") {
		t.Fatalf("expected OSC8 localhost target, got raw:\n%q", rendered)
	}
}

func TestTimelineFullScreen_RebuildsCachedLocalLinksAfterPreviewScopeChanges(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(true)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>Before</p><img alt="Logo" src="cid:logo"><p>After</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
	m.timeline.fullScreen = true

	first := m.renderFullScreenEmail()
	firstURL := firstOSC8URL(t, first)
	assertImageURLStatus(t, firstURL, http.StatusOK)

	otherImages := []models.InlineImage{
		{ContentID: "other", MIMEType: "image/png", Data: []byte("other-png")},
	}
	if rendered, rows := m.renderLocalImageLinks("cleanup:other", otherImages, nil, 80, 4); rows != 1 || !strings.Contains(stripANSI(rendered), "open image 1") {
		t.Fatalf("expected other scope to register a local link, rows=%d rendered=%q", rows, rendered)
	}
	assertImageURLStatus(t, firstURL, http.StatusNotFound)

	second := m.renderFullScreenEmail()
	secondURL := firstOSC8URL(t, second)
	if secondURL == firstURL {
		t.Fatalf("expected rebuilt local image URL after scope change, still got %s", secondURL)
	}
	assertImageURLStatus(t, secondURL, http.StatusOK)
}

func TestTimelinePreviewDocumentLayout_CacheIncludesAvailableRowsForItermImages(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	m := makeSizedModel(t, 100, 40)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>Before</p><img alt="Logo" src="cid:logo"><p>After</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
	m.timeline.fullScreen = true

	tall := m.timelinePreviewDocumentLayout(98, 20)
	tallRendered := renderPreviewDocumentViewport(tall, 0, tall.TotalRows).Content
	tallHeight := itermImageDimension(t, tallRendered, "height")
	if tallHeight <= 2 {
		t.Fatalf("expected tall layout image height > 2, got %d raw=%q", tallHeight, tallRendered)
	}

	short := m.timelinePreviewDocumentLayout(98, 2)
	shortRendered := renderPreviewDocumentViewport(short, 0, short.TotalRows).Content
	shortHeight := itermImageDimension(t, shortRendered, "height")
	if shortHeight > 2 {
		t.Fatalf("expected short layout image height <= 2, got %d raw=%q", shortHeight, shortRendered)
	}
}

func TestTimelineFullScreenVisualSelectionUsesDocumentRows(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(true)
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextPlain: "stale plain fallback",
		TextHTML:  `<p>Before doc</p><img alt="Logo" src="cid:logo"><p>After doc</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}
	m.timeline.fullScreen = true
	m.timeline.bodyWrappedLines = []string{"stale plain fallback"}

	rows := m.timelineFullScreenDocumentPlainRows()
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "Before doc") || !strings.Contains(joined, "Logo") || !strings.Contains(joined, "After doc") {
		t.Fatalf("expected copyable document rows, got:\n%s", joined)
	}
	if strings.Contains(joined, "stale plain fallback") {
		t.Fatalf("document rows should not use stale bodyWrappedLines, got:\n%s", joined)
	}

	model, _, handled := m.handleTimelineKey(keyRunes("v"))
	if !handled {
		t.Fatal("expected v to be handled")
	}
	updated := model.(*Model)
	model, _, handled = updated.handleTimelineKey(keyRunes("j"))
	if !handled {
		t.Fatal("expected j to be handled")
	}
	updated = model.(*Model)
	model, _, handled = updated.handleTimelineKey(keyRunes("j"))
	if !handled {
		t.Fatal("expected second j to be handled")
	}
	updated = model.(*Model)
	if !updated.timeline.visualMode {
		t.Fatal("expected visual mode to stay enabled")
	}
	if updated.timeline.visualEnd != 2 {
		t.Fatalf("expected visual selection to advance over document rows, got visualEnd=%d", updated.timeline.visualEnd)
	}
	selected := updated.timelineFullScreenSelectedPlainText()
	if !strings.Contains(selected, "Before doc") || !strings.Contains(selected, "Logo") {
		t.Fatalf("expected selected text from document rows, got:\n%s", selected)
	}
}

func TestPreviewImageModeLinksUsesProvidedLocalOpenLink(t *testing.T) {
	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeLinks,
		Image:         models.InlineImage{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		InnerWidth:    80,
		AvailableRows: 1,
		LinkLabel:     "open image 1",
		LinkURL:       "http://127.0.0.1:12345/image/token",
	})

	plain := stripANSI(rendered.Content)
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("expected local open image link label, got:\n%s", plain)
	}
	if !strings.Contains(rendered.Content, "\x1b]8;;http://127.0.0.1:12345/image/token") {
		t.Fatalf("expected OSC8 target, got raw:\n%q", rendered.Content)
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

func TestCleanupFullScreen_RendersNoCIDInlineImage(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(true)
	m.activeTab = tabCleanup
	m.focusedPanel = panelDetails
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewWidth = 100
	m.cleanupPreviewEmail = testImageEmail()
	m.cleanupEmailBody = &models.EmailBody{
		TextPlain: "Cleanup body before no-CID image.",
		InlineImages: []models.InlineImage{
			{MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}

	rendered := m.renderCleanupPreview()
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "Inline images:") {
		t.Fatalf("expected cleanup orphan image section, got:\n%s", plain)
	}
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("expected cleanup no-CID image to get a local open link, got:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b]8;;http://127.0.0.1:") {
		t.Fatalf("expected cleanup OSC8 localhost target, got raw:\n%q", rendered)
	}
}

func TestCleanupFullScreen_RendersCIDImageInDocumentOrder(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 30)
	defer m.cleanup()
	m.SetLocalImageLinksEnabled(true)
	m.activeTab = tabCleanup
	m.focusedPanel = panelDetails
	m.showCleanupPreview = true
	m.cleanupFullScreen = true
	m.cleanupPreviewWidth = 100
	m.cleanupPreviewEmail = testImageEmail()
	m.cleanupEmailBody = &models.EmailBody{
		TextHTML: `<p>Before cleanup image.</p><img alt="Cleanup Logo" src="cid:logo"><p>After cleanup image.</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "logo", MIMEType: "image/png", Data: []byte("png-bytes")},
		},
	}

	rendered := stripANSI(m.renderCleanupPreview())
	before := strings.Index(rendered, "Before cleanup image")
	image := strings.Index(rendered, "open image")
	after := strings.Index(rendered, "After cleanup image")
	if before == -1 || image == -1 || after == -1 || !(before < image && image < after) {
		t.Fatalf("cleanup CID image should be in document order, got:\n%s", rendered)
	}
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

func itermImageDimension(t *testing.T, rendered, key string) int {
	t.Helper()
	re := regexp.MustCompile(key + `=(\d+)`)
	match := re.FindStringSubmatch(rendered)
	if len(match) != 2 {
		t.Fatalf("expected %s dimension in iTerm2 escape, got raw:\n%q", key, rendered)
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("parse %s dimension %q: %v", key, match[1], err)
	}
	return value
}

func firstOSC8URL(t *testing.T, rendered string) string {
	t.Helper()
	const prefix = "\x1b]8;;"
	start := strings.Index(rendered, prefix)
	if start == -1 {
		t.Fatalf("expected OSC8 URL in raw render:\n%q", rendered)
	}
	start += len(prefix)
	end := strings.Index(rendered[start:], "\a")
	if end == -1 {
		end = strings.Index(rendered[start:], "\x1b\\")
	}
	if end == -1 {
		t.Fatalf("expected OSC8 terminator in raw render:\n%q", rendered)
	}
	return rendered[start : start+end]
}

func assertImageURLStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, want)
	}
}
