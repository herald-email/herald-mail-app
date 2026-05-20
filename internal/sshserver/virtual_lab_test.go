package sshserver

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/app"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/testmail"
	cryptossh "golang.org/x/crypto/ssh"
)

func TestVirtualLabSSHPreviewRendersCalendlyScenario(t *testing.T) {
	h := startVirtualLabSSH(t, testmail.ScenarioCalendlyInvite, 120, 40)
	h.waitForVisible(t, "Invitation")
	h.sendKey(t, "enter")
	h.waitForVisible(t, "Join meeting")
	h.forceRedraw(t, 120, 40)

	frame := h.currentVisibleFrame()
	requireVisibleContains(t, frame, "Product review", "Join meeting", "Herald", "Timeline", "INBOX", "?: help")
	requireVisibleFitsWidth(t, frame, 120)
}

func TestVirtualLabSSHPreviewHandlesStandardResizeAndLongLinks(t *testing.T) {
	h := startVirtualLabSSH(t, testmail.ScenarioLongLinkTracking, 120, 40)
	h.waitForVisible(t, "Long safe link")
	h.sendKey(t, "enter")
	h.waitForVisible(t, "Open safe fixture link")
	h.forceRedraw(t, 80, 24)

	frame := h.currentVisibleFrame()
	requireVisibleContains(t, frame, "Open safe fixture link", "Herald", "Timeline", "INBOX", "?: help")
	for _, forbidden := range []string{
		"redacted-fixture-token",
		"links.herald.test/path/with/a/very/long",
	} {
		if strings.Contains(normalizeSSHText(frame), normalizeSSHText(forbidden)) {
			t.Fatalf("visible SSH frame leaked %q:\n%s", forbidden, frame)
		}
	}
	raw := h.currentRawFrame()
	for _, forbidden := range []string{"utm_", "fbclid", "gclid", "mc_cid", "mc_eid"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("raw SSH frame leaked tracker param %q:\n%q", forbidden, raw)
		}
	}
	requireVisibleFitsWidth(t, frame, 80)
}

func TestVirtualLabSSHResizeShowsMinimumGuardAndRecovers(t *testing.T) {
	h := startVirtualLabSSH(t, testmail.ScenarioCalendlyInvite, 120, 40)
	h.waitForVisible(t, "Invitation")
	h.sendKey(t, "enter")
	h.waitForVisible(t, "Product review")

	h.forceRedraw(t, 50, 15)
	guard := h.currentVisibleFrame()
	requireVisibleContains(t, guard, "Terminal too narrow")
	if strings.Contains(normalizeSSHText(guard), normalizeSSHText("Product review")) {
		t.Fatalf("minimum-size guard should replace preview body, got:\n%s", guard)
	}
	requireVisibleFitsWidth(t, guard, 50)

	h.forceRedraw(t, 80, 24)
	recovered := h.currentVisibleFrame()
	requireVisibleContains(t, recovered, "Product review", "Herald", "Timeline", "INBOX", "?: help")
	if strings.Contains(recovered, "Terminal too narrow") {
		t.Fatalf("minimum-size guard should clear after resize, got:\n%s", recovered)
	}
	requireVisibleFitsWidth(t, recovered, 80)
}

type sshHarness struct {
	t       testing.TB
	width   int
	height  int
	client  *cryptossh.Client
	session *cryptossh.Session
	stdin   io.WriteCloser
	output  *lockedBuffer
}

func startVirtualLabSSH(t testing.TB, scenario testmail.ScenarioName, width, height int) *sshHarness {
	t.Helper()
	t.Setenv("HERALD_LOG_DIR", t.TempDir())

	seeded := testmail.StartScenario(t, scenario)
	alice := seeded.Lab.Account(testmail.DefaultAliceAddress)
	if alice == nil {
		t.Fatal("virtual lab missing Alice account")
	}
	dir := t.TempDir()
	cfg := alice.Config(filepath.Join(dir, "alice-cache.db"))
	cfg.Sync.Interval = 60
	cfg.Sync.Idle = false
	cfg.Sync.Background = false
	cfg.Sync.IDLEEnabled = false
	cfg.Semantic.Enabled = false
	cfg.AI.Provider = "disabled"

	configPath := filepath.Join(dir, "config.yaml")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save virtual lab config: %v", err)
	}
	resolvedConfig, err := config.ExpandPath(configPath)
	if err != nil {
		t.Fatalf("resolve config path: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	hostKeyPath := filepath.Join(dir, "keys", "host_ed25519")
	srv, err := newSSHServer(sshServerOptions{
		Addr:           listener.Addr().String(),
		HostKeyPath:    hostKeyPath,
		Config:         cfg,
		ResolvedConfig: resolvedConfig,
		ImageMode:      app.PreviewImageModePlaceholder,
	})
	if err != nil {
		_ = listener.Close()
		t.Fatalf("new SSH server: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		select {
		case <-errCh:
		case <-time.After(time.Second):
		}
	})

	client, err := cryptossh.Dial("tcp", listener.Addr().String(), &cryptossh.ClientConfig{
		User:            "alice",
		HostKeyCallback: cryptossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("dial SSH server: %v", err)
	}
	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		t.Fatalf("new SSH session: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		_ = session.Close()
		_ = client.Close()
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := session.RequestPty("xterm-256color", height, width, cryptossh.TerminalModes{
		cryptossh.ECHO: 0,
	}); err != nil {
		_ = session.Close()
		_ = client.Close()
		t.Fatalf("request pty: %v", err)
	}

	output := &lockedBuffer{}
	go func() { _, _ = io.Copy(output, stdout) }()
	go func() { _, _ = io.Copy(output, stderr) }()
	if err := session.Shell(); err != nil {
		_ = session.Close()
		_ = client.Close()
		t.Fatalf("start shell: %v", err)
	}

	h := &sshHarness{
		t:       t,
		width:   width,
		height:  height,
		client:  client,
		session: session,
		stdin:   stdin,
		output:  output,
	}
	t.Cleanup(func() {
		h.sendKeyNoFail("q")
		_ = h.stdin.Close()
		_ = h.session.Close()
		_ = h.client.Close()
	})
	return h
}

func (h *sshHarness) sendKey(t testing.TB, key string) {
	t.Helper()
	if _, err := h.stdin.Write([]byte(sshKeyBytes(key))); err != nil {
		t.Fatalf("send key %q: %v", key, err)
	}
}

func (h *sshHarness) sendKeyNoFail(key string) {
	_, _ = h.stdin.Write([]byte(sshKeyBytes(key)))
}

func sshKeyBytes(key string) string {
	switch key {
	case "enter":
		return "\r"
	case "escape":
		return "\x1b"
	default:
		return key
	}
}

func (h *sshHarness) forceRedraw(t testing.TB, width, height int) {
	t.Helper()
	intermediateWidth := width + 1
	if width > 61 {
		intermediateWidth = width - 1
	}
	h.output.Reset()
	if err := h.session.WindowChange(height, intermediateWidth); err != nil {
		t.Fatalf("resize SSH pty to %dx%d: %v", intermediateWidth, height, err)
	}
	time.Sleep(75 * time.Millisecond)
	h.output.Reset()
	h.width = width
	h.height = height
	if err := h.session.WindowChange(height, width); err != nil {
		t.Fatalf("resize SSH pty to %dx%d: %v", width, height, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if h.output.Len() > 0 {
			time.Sleep(100 * time.Millisecond)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for SSH redraw at %dx%d", width, height)
}

func (h *sshHarness) waitForVisible(t testing.TB, want string) string {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		frame := h.currentVisibleFrame()
		if strings.Contains(normalizeSSHText(frame), normalizeSSHText(want)) {
			return frame
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q; current frame:\n%s\nraw:\n%q", want, h.currentVisibleFrame(), h.currentRawFrame())
	return ""
}

func (h *sshHarness) currentRawFrame() string {
	return h.output.String()
}

func (h *sshHarness) currentVisibleFrame() string {
	return renderSSHScreen(h.currentRawFrame(), h.width, h.height)
}

func requireVisibleContains(t testing.TB, frame string, wants ...string) {
	t.Helper()
	normalized := normalizeSSHText(frame)
	for _, want := range wants {
		if !strings.Contains(normalized, normalizeSSHText(want)) {
			t.Fatalf("visible SSH frame missing %q:\n%s", want, frame)
		}
	}
}

func requireVisibleFitsWidth(t testing.TB, frame string, width int) {
	t.Helper()
	for i, line := range strings.Split(frame, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("visible SSH line %d width=%d exceeds %d:\n%s", i, got, width, frame)
		}
	}
}

func normalizeSSHText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func (b *lockedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b.Reset()
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Len()
}

func renderSSHScreen(raw string, width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	cells := make([][]rune, height)
	for y := range cells {
		cells[y] = make([]rune, width)
		for x := range cells[y] {
			cells[y][x] = ' '
		}
	}
	row, col := 0, 0
	clearScreen := func() {
		for y := range cells {
			for x := range cells[y] {
				cells[y][x] = ' '
			}
		}
	}
	newLine := func() {
		row++
		if row >= height {
			copy(cells[0:], cells[1:])
			cells[height-1] = make([]rune, width)
			for x := range cells[height-1] {
				cells[height-1][x] = ' '
			}
			row = height - 1
		}
	}
	drawRune := func(r rune) {
		if r < ' ' {
			return
		}
		if col >= width {
			col = 0
			newLine()
		}
		w := ansi.StringWidth(string(r))
		if w <= 0 {
			return
		}
		if row >= 0 && row < height && col >= 0 && col < width {
			cells[row][col] = r
			for x := 1; x < w && col+x < width; x++ {
				cells[row][col+x] = ' '
			}
		}
		col += w
	}

	for i := 0; i < len(raw); {
		switch raw[i] {
		case '\x1b':
			next, ok := applySSHControl(raw, i, &row, &col, width, height, cells, clearScreen)
			if !ok {
				i++
				continue
			}
			i = next
		case '\r':
			col = 0
			i++
		case '\n':
			newLine()
			i++
		case '\t':
			for range 4 {
				drawRune(' ')
			}
			i++
		default:
			r, size := utf8.DecodeRuneInString(raw[i:])
			if r == utf8.RuneError && size == 1 {
				i++
				continue
			}
			drawRune(r)
			i += size
		}
	}

	lines := make([]string, height)
	for y, rowCells := range cells {
		lines[y] = strings.TrimRight(string(rowCells), " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func applySSHControl(raw string, i int, row, col *int, width, height int, cells [][]rune, clearScreen func()) (int, bool) {
	if i+1 >= len(raw) {
		return i + 1, true
	}
	switch raw[i+1] {
	case ']':
		return skipSSHOSC(raw, i+2), true
	case '[':
		j := i + 2
		for j < len(raw) && (raw[j] < 0x40 || raw[j] > 0x7e) {
			j++
		}
		if j >= len(raw) {
			return len(raw), true
		}
		applySSHCSI(raw[i+2:j], raw[j], row, col, width, height, cells, clearScreen)
		return j + 1, true
	default:
		return i + 2, true
	}
}

func skipSSHOSC(raw string, i int) int {
	for i < len(raw) {
		if raw[i] == '\a' {
			return i + 1
		}
		if raw[i] == '\x1b' && i+1 < len(raw) && raw[i+1] == '\\' {
			return i + 2
		}
		i++
	}
	return len(raw)
}

func applySSHCSI(params string, final byte, row, col *int, width, height int, cells [][]rune, clearScreen func()) {
	values := parseSSHCSIParams(params)
	first := func(def int) int {
		if len(values) == 0 || values[0] == 0 {
			return def
		}
		return values[0]
	}
	switch final {
	case 'A':
		*row = max(0, *row-first(1))
	case 'B':
		*row = min(height-1, *row+first(1))
	case 'C':
		*col = min(width-1, *col+first(1))
	case 'D':
		*col = max(0, *col-first(1))
	case 'G':
		*col = min(width-1, max(0, first(1)-1))
	case 'H', 'f':
		r, c := 1, 1
		if len(values) > 0 && values[0] > 0 {
			r = values[0]
		}
		if len(values) > 1 && values[1] > 0 {
			c = values[1]
		}
		*row = min(height-1, max(0, r-1))
		*col = min(width-1, max(0, c-1))
	case 'J':
		if first(0) == 2 || first(0) == 3 {
			clearScreen()
		}
	case 'K':
		mode := first(0)
		switch mode {
		case 0:
			for x := *col; x < width; x++ {
				cells[*row][x] = ' '
			}
		case 1:
			for x := 0; x <= *col && x < width; x++ {
				cells[*row][x] = ' '
			}
		case 2:
			for x := 0; x < width; x++ {
				cells[*row][x] = ' '
			}
		}
	}
}

func parseSSHCSIParams(params string) []int {
	params = strings.TrimLeft(params, "?")
	if params == "" {
		return nil
	}
	parts := strings.Split(params, ";")
	values := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			values[i] = 0
			continue
		}
		values[i] = n
	}
	return values
}

func TestHostKeyParentUsesConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	hostKeyPath := filepath.Join(dir, "custom", "ssh", "host_ed25519")
	if err := ensureHostKeyParent(hostKeyPath); err != nil {
		t.Fatalf("ensureHostKeyParent: %v", err)
	}
	if info, err := os.Stat(filepath.Dir(hostKeyPath)); err != nil || !info.IsDir() {
		t.Fatalf("host key parent was not created: info=%v err=%v", info, err)
	}
}
