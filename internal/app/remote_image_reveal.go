package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/herald-email/herald-mail-app/internal/demo"
	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	remoteImageFetchTimeout     = 8 * time.Second
	remoteImageMaxRedirects     = 3
	remoteImageAcceptHeader     = "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
	remoteImageRevealCommandKey = "o"
)

type previewRemoteImageState struct {
	Image   models.InlineImage
	Err     string
	Loading bool
}

type remoteImageRevealResult struct {
	Key   string
	URL   string
	Image models.InlineImage
	Err   error
}

type RemoteImageRevealMsg struct {
	MessageID string
	Results   []remoteImageRevealResult
}

type remoteImageHTTPFetcher struct {
	client  *http.Client
	resolve func(context.Context, string) ([]net.IP, error)
}

func remoteImageDocumentKey(rawURL string) string {
	sum := sha1.Sum([]byte(normalizeRemoteImageURL(rawURL)))
	return "remote-" + hex.EncodeToString(sum[:8])
}

func normalizeRemoteImageURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(rawURL))
	for _, r := range rawURL {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isRemoteImageURL(rawURL string) bool {
	u, err := url.Parse(normalizeRemoteImageURL(rawURL))
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func remoteImageDisplayLabel(remote previewRemoteImage) string {
	for _, candidate := range []string{remote.Alt, remote.Title} {
		if label := strings.TrimSpace(candidate); label != "" {
			return label
		}
	}
	u, err := url.Parse(remote.URL)
	if err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return "linked image"
}

func (m *Model) timelineRemoteImages() []previewRemoteImage {
	doc := buildPreviewDocument(m.timeline.body, m.timeline.inlineImageDescs)
	return previewDocumentRemoteImages(doc)
}

func (m *Model) timelineRemoteImageCount() int {
	if m.timeline.body == nil || strings.TrimSpace(m.timeline.body.TextHTML) == "" {
		return 0
	}
	return len(m.timelineRemoteImages())
}

func (m *Model) timelineRemoteRevealAvailable() bool {
	if m.activeTab != tabTimeline || m.loading || m.timeline.body == nil || m.timeline.selectedEmail == nil {
		return false
	}
	if m.timeline.bodyMessageID != "" && m.timeline.bodyMessageID != m.timeline.selectedEmail.MessageID {
		return false
	}
	for _, remote := range m.timelineRemoteImages() {
		state := m.timeline.remoteImageLoads[remote.Key]
		if !state.Loading && len(state.Image.Data) == 0 {
			return true
		}
	}
	return false
}

func (m *Model) revealTimelineRemoteImages() tea.Cmd {
	if m.loading || m.timeline.body == nil || m.timeline.selectedEmail == nil {
		return nil
	}
	targets := m.timelineRemoteImages()
	if len(targets) == 0 {
		m.statusMessage = "No linked images in this message"
		return nil
	}
	if m.timeline.remoteImageLoads == nil {
		m.timeline.remoteImageLoads = make(map[string]previewRemoteImageState, len(targets))
	}
	var pending []previewRemoteImage
	for _, remote := range targets {
		if remote.Key == "" {
			remote.Key = remoteImageDocumentKey(remote.URL)
		}
		state := m.timeline.remoteImageLoads[remote.Key]
		if state.Loading || len(state.Image.Data) > 0 {
			continue
		}
		state.Loading = true
		state.Err = ""
		m.timeline.remoteImageLoads[remote.Key] = state
		pending = append(pending, remote)
	}
	if len(pending) == 0 {
		m.statusMessage = "Linked images already revealed"
		return nil
	}
	m.timeline.remoteImageRevision++
	m.clearTimelinePreviewDocumentCache()
	m.statusMessage = fmt.Sprintf("Revealing %d linked image(s)...", len(pending))
	messageID := m.timeline.selectedEmail.MessageID
	return revealRemoteImagesCmd(messageID, pending, m.demoMode)
}

func revealRemoteImagesCmd(messageID string, targets []previewRemoteImage, demoMode bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), remoteImageFetchTimeout*time.Duration(maxInt(1, len(targets))))
		defer cancel()
		results := make([]remoteImageRevealResult, 0, len(targets))
		fetcher := defaultRemoteImageHTTPFetcher()
		for _, target := range targets {
			image, err := fetchRemoteImage(ctx, target, demoMode, fetcher)
			results = append(results, remoteImageRevealResult{
				Key:   target.Key,
				URL:   target.URL,
				Image: image,
				Err:   err,
			})
		}
		return RemoteImageRevealMsg{MessageID: messageID, Results: results}
	}
}

func fetchRemoteImage(ctx context.Context, target previewRemoteImage, demoMode bool, fetcher remoteImageHTTPFetcher) (models.InlineImage, error) {
	target.URL = normalizeRemoteImageURL(target.URL)
	if target.Key == "" {
		target.Key = remoteImageDocumentKey(target.URL)
	}
	if demoMode {
		if data, mimeType, ok := demo.RemoteImageAsset(target.URL); ok {
			return models.InlineImage{ContentID: target.Key, MIMEType: mimeType, Data: append([]byte(nil), data...)}, nil
		}
	}
	image, err := fetcher.Fetch(ctx, target.URL)
	if err != nil {
		return models.InlineImage{}, err
	}
	image.ContentID = target.Key
	return image, nil
}

func defaultRemoteImageHTTPFetcher() remoteImageHTTPFetcher {
	resolver := defaultRemoteImageResolver
	return remoteImageHTTPFetcher{
		client: &http.Client{
			Timeout:   remoteImageFetchTimeout,
			Transport: safeRemoteImageTransport(resolver),
		},
		resolve: resolver,
	}
}

func defaultRemoteImageResolver(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		out = append(out, addr.IP)
	}
	return out, nil
}

func (f remoteImageHTTPFetcher) Fetch(ctx context.Context, rawURL string) (models.InlineImage, error) {
	rawURL = normalizeRemoteImageURL(rawURL)
	if f.resolve == nil {
		f.resolve = defaultRemoteImageResolver
	}
	if err := validateRemoteImageURL(ctx, rawURL, f.resolve); err != nil {
		return models.InlineImage{}, err
	}
	client := f.client
	if client == nil {
		client = &http.Client{
			Timeout:   remoteImageFetchTimeout,
			Transport: safeRemoteImageTransport(f.resolve),
		}
	}
	previousCheckRedirect := client.CheckRedirect
	client = cloneHTTPClient(client)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("Referer")
		if len(via) >= remoteImageMaxRedirects {
			return errors.New("too many redirects")
		}
		if err := validateRemoteImageURL(req.Context(), req.URL.String(), f.resolve); err != nil {
			return err
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(req, via)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return models.InlineImage{}, err
	}
	req.Header.Set("Accept", remoteImageAcceptHeader)
	resp, err := client.Do(req)
	if err != nil {
		return models.InlineImage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return models.InlineImage{}, fmt.Errorf("image request returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxPreviewImageBytes {
		return models.InlineImage{}, fmt.Errorf("image is too large")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPreviewImageBytes+1))
	if err != nil {
		return models.InlineImage{}, err
	}
	if len(data) > maxPreviewImageBytes {
		return models.InlineImage{}, fmt.Errorf("image is too large")
	}
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
		detected := strings.ToLower(http.DetectContentType(data))
		if !strings.HasPrefix(detected, "image/") {
			return models.InlineImage{}, fmt.Errorf("response is not an image")
		}
		mimeType = detected
	}
	return models.InlineImage{MIMEType: mimeType, Data: data}, nil
}

func cloneHTTPClient(src *http.Client) *http.Client {
	if src == nil {
		return &http.Client{
			Timeout:   remoteImageFetchTimeout,
			Transport: safeRemoteImageTransport(defaultRemoteImageResolver),
		}
	}
	cp := *src
	return &cp
}

func safeRemoteImageTransport(resolve func(context.Context, string) ([]net.IP, error)) http.RoundTripper {
	if resolve == nil {
		resolve = defaultRemoteImageResolver
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: remoteImageFetchTimeout, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if err := validateRemoteImageHost(ctx, host, resolve); err != nil {
			return nil, err
		}
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok && unsafeRemoteImageIP(tcpAddr.IP) {
			_ = conn.Close()
			return nil, fmt.Errorf("remote image destination is private or local")
		}
		return conn, nil
	}
	return transport
}

func validateRemoteImageURL(ctx context.Context, rawURL string, resolve func(context.Context, string) ([]net.IP, error)) error {
	u, err := url.Parse(normalizeRemoteImageURL(rawURL))
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("remote image URL must use HTTP(S)")
	}
	if u.User != nil {
		return fmt.Errorf("remote image URL must not contain userinfo")
	}
	return validateRemoteImageHost(ctx, u.Hostname(), resolve)
}

func validateRemoteImageHost(ctx context.Context, host string, resolve func(context.Context, string) ([]net.IP, error)) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("remote image URL missing host")
	}
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("remote image host is local")
	}
	if ip := net.ParseIP(host); ip != nil {
		if unsafeRemoteImageIP(ip) {
			return fmt.Errorf("remote image host is private or local")
		}
		return nil
	}
	if resolve == nil {
		resolve = defaultRemoteImageResolver
	}
	addrs, err := resolve(ctx, host)
	if err != nil {
		return err
	}
	if len(addrs) == 0 {
		return fmt.Errorf("remote image host did not resolve")
	}
	for _, ip := range addrs {
		if unsafeRemoteImageIP(ip) {
			return fmt.Errorf("remote image host resolves to private or local address")
		}
	}
	return nil
}

func unsafeRemoteImageIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	return addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified()
}
