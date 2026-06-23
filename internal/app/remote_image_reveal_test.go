package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type remoteImageRoundTripFunc func(*http.Request) (*http.Response, error)

func (f remoteImageRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func publicRemoteImageResolver(context.Context, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("93.184.216.34")}, nil
}

func TestRemoteImagePlaceholderHidesRawURLButKeepsSanitizedOSC8Target(t *testing.T) {
	doc := buildPreviewDocument(&models.EmailBody{
		TextHTML: `<p>Before</p><img alt="Launch chart" src="https://cdn.example.test/chart.png?utm_source=email&token=secret"><p>After</p>`,
	}, nil)
	layout := layoutPreviewDocument(doc, previewLayoutOptions{
		InnerWidth:    120,
		AvailableRows: 20,
		ImageMode:     previewImageModeLinks,
	})

	var raw strings.Builder
	for _, row := range layout.Rows {
		raw.WriteString(row.Content)
		raw.WriteByte('\n')
	}
	rendered := raw.String()
	visible := ansi.Strip(rendered)

	if !strings.Contains(visible, "image: Launch chart (press o to reveal)") {
		t.Fatalf("placeholder missing readable reveal hint:\n%s", visible)
	}
	for _, leaked := range []string{"https://cdn.example.test", "chart.png", "token=secret"} {
		if strings.Contains(visible, leaked) {
			t.Fatalf("visible placeholder leaked %q:\n%s", leaked, visible)
		}
	}
	if strings.Contains(rendered, "utm_source=email") {
		t.Fatalf("placeholder should strip tracker params from raw OSC8 output, got:\n%q", rendered)
	}
	if !strings.Contains(rendered, "\x1b]8;;https://cdn.example.test/chart.png?token=secret") {
		t.Fatalf("placeholder should keep sanitized OSC8 target in raw output, got:\n%q", rendered)
	}
}

func TestRemoteImageFetcherUsesSanitizedURL(t *testing.T) {
	ctx := context.Background()
	var requested string
	fetcher := remoteImageHTTPFetcher{
		resolve: publicRemoteImageResolver,
		client: &http.Client{Transport: remoteImageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requested = req.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/png"}},
				Body:       io.NopCloser(strings.NewReader("png-bytes")),
				Request:    req,
			}, nil
		})},
	}

	image, err := fetchRemoteImage(ctx, previewRemoteImage{
		URL: "https://cdn.example.test/chart.png?utm_source=email&token=secret",
	}, false, fetcher)
	if err != nil {
		t.Fatalf("fetchRemoteImage returned error: %v", err)
	}
	if image.ContentID == "" {
		t.Fatal("expected sanitized remote image to keep a content ID")
	}
	if requested != "https://cdn.example.test/chart.png?token=secret" {
		t.Fatalf("requested URL = %q, want sanitized URL with token preserved", requested)
	}
}

func TestTimelinePreviewORevealsDemoRemoteImage(t *testing.T) {
	m := makeTimelineRemoteImageRevealModel(t, panelPreview, true)
	defer m.cleanup()

	before := m.renderFullScreenEmail()
	if !strings.Contains(ansi.Strip(before), "image: Demo chart (press o to reveal)") {
		t.Fatalf("expected initial placeholder, got:\n%s", ansi.Strip(before))
	}
	if strings.Contains(ansi.Strip(before), "open image") {
		t.Fatalf("remote image should not be revealed before o:\n%s", ansi.Strip(before))
	}

	model, cmd, handled := m.handleTimelineKey(keyRunes("o"))
	if !handled || cmd == nil {
		t.Fatalf("o should start remote image reveal, handled=%v cmd=%v", handled, cmd)
	}
	updated := model.(*Model)
	if !strings.Contains(ansi.Strip(updated.renderFullScreenEmail()), "image: Demo chart (loading") {
		t.Fatalf("expected loading placeholder after o, got:\n%s", ansi.Strip(updated.renderFullScreenEmail()))
	}

	rawMsg := cmd()
	msg, ok := rawMsg.(RemoteImageRevealMsg)
	if !ok {
		t.Fatalf("reveal command returned %T, want RemoteImageRevealMsg", rawMsg)
	}
	model, _, handled = updated.handleTimelineMsg(msg)
	if !handled {
		t.Fatal("expected reveal message to be handled")
	}
	updated = model.(*Model)
	after := updated.renderFullScreenEmail()
	plain := ansi.Strip(after)
	if !strings.Contains(plain, "open image 1") {
		t.Fatalf("expected revealed remote image to reuse local image link renderer, got:\n%s", plain)
	}
	if strings.Contains(plain, "press o to reveal") {
		t.Fatalf("placeholder should be replaced after reveal, got:\n%s", plain)
	}
}

func TestTimelineSplitPreviewRemoteImageHintUsesAccentStyle(t *testing.T) {
	m := makeTimelineRemoteImageRevealModel(t, panelPreview, false)
	defer m.cleanup()
	m.timeline.previewWidth = 90

	hint := splitRemoteImageHint(1, previewImageModeLinks, true)
	rendered := m.renderEmailPreview()
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, hint) {
		t.Fatalf("expected split preview to keep remote image hint text %q, got:\n%s", hint, plain)
	}
	styledHint := m.theme.Severity.Warning.Style().Bold(true).Render(hint)
	if !strings.Contains(rendered, styledHint) {
		t.Fatalf("expected split preview remote image hint to use warning accent style, got raw:\n%q", rendered)
	}

	rows := m.timelinePreviewBodyChromeRows(86)
	foundRow := false
	for _, row := range rows {
		if row.Plain != hint {
			continue
		}
		foundRow = true
		if row.Rendered != styledHint {
			t.Fatalf("selectable remote image hint row rendered = %q, want %q", row.Rendered, styledHint)
		}
	}
	if !foundRow {
		t.Fatalf("selectable preview chrome rows missing remote image hint %q: %#v", hint, rows)
	}
}

func TestTimelinePreviewORevealsDemoRemoteImageFromTimelineFocus(t *testing.T) {
	m := makeTimelineRemoteImageRevealModel(t, panelTimeline, false)
	defer m.cleanup()

	model, cmd, handled := m.handleTimelineKey(keyRunes("o"))
	if !handled || cmd == nil {
		t.Fatalf("o should reveal the loaded split preview from timeline focus, handled=%v cmd=%v", handled, cmd)
	}
	updated := applyRemoteRevealCmd(t, model.(*Model), cmd)

	layout := updated.timelinePreviewDocumentLayout(90, 20)
	var raw strings.Builder
	for _, row := range layout.Rows {
		raw.WriteString(row.Content)
		raw.WriteByte('\n')
	}
	plain := ansi.Strip(raw.String())
	if !strings.Contains(plain, "open image") {
		t.Fatalf("expected revealed remote image to render as an open-image link, got:\n%s", plain)
	}
	if strings.Contains(plain, "press o to reveal") {
		t.Fatalf("timeline-focus reveal should replace placeholder, got:\n%s", plain)
	}
}

func TestTimelinePreviewORevealsDemoRemoteImageFromSearchResultsFocus(t *testing.T) {
	m := makeTimelineRemoteImageRevealModel(t, panelTimeline, false)
	defer m.cleanup()
	email := m.timeline.selectedEmail
	body := m.timeline.body
	m.openTimelineSearch()
	m.timeline.searchResults = []*models.EmailData{email}
	m.timeline.searchResultsQuery = "remote"
	m.timeline.searchFocus = timelineSearchFocusResults
	m.timeline.searchInput.Blur()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = body

	model, cmd, handled := m.handleTimelineKey(keyRunes("o"))
	if !handled || cmd == nil {
		t.Fatalf("o should reveal the loaded split preview from search-results focus, handled=%v cmd=%v", handled, cmd)
	}
	updated := applyRemoteRevealCmd(t, model.(*Model), cmd)

	if !updated.timelineHasLoadedRemoteImages() {
		t.Fatal("expected search-results reveal to load the remote image")
	}
}

func TestTimelineSearchInputOIsTextNotRemoteReveal(t *testing.T) {
	m := makeTimelineRemoteImageRevealModel(t, panelTimeline, false)
	defer m.cleanup()
	m.openTimelineSearch()
	m.timeline.searchInput.SetValue("rem")
	m.timeline.searchFocus = timelineSearchFocusInput
	m.timeline.searchInput.Focus()

	model, _, handled := m.handleOverlayKey(keyRunes("o"))
	if !handled {
		t.Fatal("expected active search input to handle typed o")
	}
	updated := model.(*Model)
	if got := updated.timeline.searchInput.Value(); got != "remo" {
		t.Fatalf("search input value = %q, want %q", got, "remo")
	}
	if updated.timeline.remoteImageLoads != nil {
		t.Fatalf("typed o in search input should not start remote-image reveal, got %#v", updated.timeline.remoteImageLoads)
	}
}

func makeTimelineRemoteImageRevealModel(t *testing.T, focus int, fullScreen bool) *Model {
	t.Helper()
	t.Setenv("TERM_PROGRAM", "")
	m := makeSizedModel(t, 100, 32)
	m.demoMode = true
	m.SetPreviewImageMode(PreviewImageModeLinks)
	m.activeTab = tabTimeline
	m.focusedPanel = focus
	email := &models.EmailData{
		MessageID: "remote-demo",
		Sender:    "image-lab@example.test",
		Subject:   "Remote demo",
		Date:      time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
		Folder:    "INBOX",
	}
	m.timeline.emails = []*models.EmailData{email}
	m.updateTimelineTable()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>Before</p><img alt="Demo chart" src="https://assets.herald.demo/color-chart-330px.png"><p>After</p>`,
	}
	m.timeline.fullScreen = fullScreen
	return m
}

func applyRemoteRevealCmd(t *testing.T, m *Model, cmd tea.Cmd) *Model {
	t.Helper()
	rawMsg := cmd()
	msg, ok := rawMsg.(RemoteImageRevealMsg)
	if !ok {
		t.Fatalf("reveal command returned %T, want RemoteImageRevealMsg", rawMsg)
	}
	model, _, handled := m.handleTimelineMsg(msg)
	if !handled {
		t.Fatal("expected reveal message to be handled")
	}
	return model.(*Model)
}

func TestTimelineReplyKeysRemainAlignedAfterRemoteRevealKeyAdded(t *testing.T) {
	resolver := NewKeyboardResolver(nil)
	if command, ok := resolver.Resolve("timeline", keyboardModeNormal, "r"); !ok || command != CommandMailReplySender {
		t.Fatalf("timeline r resolves to %q ok=%v, want reply sender", command, ok)
	}
	if command, ok := resolver.Resolve("timeline", keyboardModeNormal, "R"); !ok || command != CommandMailReplyAll {
		t.Fatalf("timeline R resolves to %q ok=%v, want reply all", command, ok)
	}
	if command, ok := resolver.Resolve("timeline", keyboardModeNormal, "o"); !ok || command != CommandPreviewRevealRemoteImages {
		t.Fatalf("timeline o resolves to %q ok=%v, want remote image reveal", command, ok)
	}
}

func TestRemoteImageFetcherRejectsUnsafeDestinationsAndResponses(t *testing.T) {
	ctx := context.Background()
	if err := validateRemoteImageURL(ctx, "https://localhost/image.png", publicRemoteImageResolver); err == nil {
		t.Fatal("localhost should be rejected")
	}
	privateResolver := func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}
	if err := validateRemoteImageURL(ctx, "https://cdn.example.test/image.png", privateResolver); err == nil {
		t.Fatal("private DNS resolution should be rejected")
	}

	t.Run("non-image response", func(t *testing.T) {
		fetcher := remoteImageHTTPFetcher{
			resolve: publicRemoteImageResolver,
			client: &http.Client{Transport: remoteImageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader("<html>not an image</html>")),
					Request:    req,
				}, nil
			})},
		}
		if _, err := fetcher.Fetch(ctx, "https://cdn.example.test/not-image"); err == nil || !strings.Contains(err.Error(), "not an image") {
			t.Fatalf("non-image response error = %v, want not image", err)
		}
	})

	t.Run("oversized response", func(t *testing.T) {
		fetcher := remoteImageHTTPFetcher{
			resolve: publicRemoteImageResolver,
			client: &http.Client{Transport: remoteImageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					ContentLength: maxPreviewImageBytes + 1,
					Header:        http.Header{"Content-Type": []string{"image/png"}},
					Body:          io.NopCloser(strings.NewReader("")),
					Request:       req,
				}, nil
			})},
		}
		if _, err := fetcher.Fetch(ctx, "https://cdn.example.test/too-large.png"); err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("oversized response error = %v, want too large", err)
		}
	})

	t.Run("unsafe redirect", func(t *testing.T) {
		fetcher := remoteImageHTTPFetcher{
			resolve: publicRemoteImageResolver,
			client: &http.Client{Transport: remoteImageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Hostname() == "cdn.example.test" {
					return &http.Response{
						StatusCode: http.StatusFound,
						Header:     http.Header{"Location": []string{"http://127.0.0.1/private.png"}},
						Body:       io.NopCloser(strings.NewReader("")),
						Request:    req,
					}, nil
				}
				return nil, errors.New("unsafe redirect target should not be fetched")
			})},
		}
		if _, err := fetcher.Fetch(ctx, "https://cdn.example.test/redirect.png"); err == nil || !strings.Contains(err.Error(), "private") {
			t.Fatalf("unsafe redirect error = %v, want private host rejection", err)
		}
	})
}

func TestRemoteImageFetcherFetchesImagesWithoutAmbientHeaders(t *testing.T) {
	ctx := context.Background()
	fetcher := remoteImageHTTPFetcher{
		resolve: publicRemoteImageResolver,
		client: &http.Client{Transport: remoteImageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Accept") == "" {
				t.Fatal("expected image Accept header")
			}
			for _, header := range []string{"Authorization", "Cookie", "Referer"} {
				if got := req.Header.Get(header); got != "" {
					t.Fatalf("%s header should not be sent, got %q", header, got)
				}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/png"}},
				Body:       io.NopCloser(strings.NewReader("png-bytes")),
				Request:    req,
			}, nil
		})},
	}
	image, err := fetcher.Fetch(ctx, "https://cdn.example.test/image.png")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if image.MIMEType != "image/png" || string(image.Data) != "png-bytes" {
		t.Fatalf("image = %#v, want png bytes", image)
	}
}
