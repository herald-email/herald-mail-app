package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/cache"
	daemonpkg "github.com/herald-email/herald-mail-app/internal/daemon"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testmail"
	"github.com/mark3labs/mcp-go/server"
)

type mcpVirtualLabDaemon struct {
	lab             *testmail.Lab
	alice           *testmail.Account
	bob             *testmail.Account
	baseURL         string
	daemon          *daemonpkg.Server
	cache           *cache.Cache
	mcp             *server.MCPServer
	originalID      string
	originalSubject string
}

func TestVirtualLabDaemonBackedMCPMutations(t *testing.T) {
	h := startMCPVirtualLabDaemon(t)

	sendJSON := callVirtualLabTool(t, h.mcp, 1, "send_email", map[string]any{
		"to":      h.bob.Address,
		"subject": "MCP daemon send",
		"body":    "Hello through MCP.",
		"from":    h.alice.Address,
	})
	if !strings.Contains(sendJSON, "Email sent to "+h.bob.Address) {
		t.Fatalf("send_email response missing success text:\n%s", sendJSON)
	}
	h.lab.WaitForSubject(h.bob.Address, "INBOX", "MCP daemon send")
	h.lab.WaitForSubject(h.alice.Address, "Sent", "MCP daemon send")

	saveJSON := callVirtualLabTool(t, h.mcp, 2, "save_draft", map[string]any{
		"to":      h.bob.Address,
		"subject": "MCP daemon draft",
		"body":    "Draft body through MCP.",
	})
	uid := extractDraftUID(t, saveJSON)
	h.lab.WaitForSubject(h.alice.Address, "Drafts", "MCP daemon draft")

	listJSON := callVirtualLabTool(t, h.mcp, 3, "list_drafts", map[string]any{})
	if !strings.Contains(listJSON, "MCP daemon draft") {
		t.Fatalf("list_drafts missing saved draft:\n%s", listJSON)
	}

	sendDraftJSON := callVirtualLabTool(t, h.mcp, 4, "send_draft", map[string]any{"uid": uid})
	if !strings.Contains(sendDraftJSON, "Draft sent and deleted") {
		t.Fatalf("send_draft response missing success text:\n%s", sendDraftJSON)
	}
	h.lab.WaitForSubject(h.bob.Address, "INBOX", "MCP daemon draft")
	h.lab.WaitForSubject(h.alice.Address, "Sent", "MCP daemon draft")

	afterSendListJSON := callVirtualLabTool(t, h.mcp, 5, "list_drafts", map[string]any{})
	if strings.Contains(afterSendListJSON, "MCP daemon draft") {
		t.Fatalf("list_drafts still shows sent draft:\n%s", afterSendListJSON)
	}

	replyJSON := callVirtualLabTool(t, h.mcp, 6, "reply_to_email", map[string]any{
		"message_id": h.originalID,
		"body":       "Reply body through MCP daemon.",
	})
	if !strings.Contains(replyJSON, "Reply sent") {
		t.Fatalf("reply_to_email response missing success text:\n%s", replyJSON)
	}
	h.lab.WaitForSubject(h.bob.Address, "INBOX", "Re: "+h.originalSubject)

	captured := h.lab.CapturedSMTP()
	if len(captured) < 3 {
		t.Fatalf("captured SMTP count = %d, want send, draft send, and reply", len(captured))
	}
	reply := string(captured[len(captured)-1].Data)
	for _, want := range []string{
		"In-Reply-To: " + h.originalID,
		"References: " + h.originalID,
		"Reply body through MCP daemon.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestVirtualLabMCPMutationReportsMissingDaemon(t *testing.T) {
	setMCPDaemonURLForTest(t, "")

	c, err := cache.New(filepath.Join(t.TempDir(), "missing-daemon-cache.db"))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	s := newMCPServer(c, nil)
	callJSON := callVirtualLabTool(t, s, 1, "send_email", map[string]any{
		"to":      testmail.DefaultBobAddress,
		"subject": "No daemon",
		"body":    "Should not send.",
	})
	if !strings.Contains(callJSON, "daemon not running") {
		t.Fatalf("send_email without daemon should explain daemon requirement:\n%s", callJSON)
	}
}

func startMCPVirtualLabDaemon(t *testing.T) *mcpVirtualLabDaemon {
	t.Helper()

	lab := testmail.Start(t)
	alice := lab.Account(testmail.DefaultAliceAddress)
	bob := lab.Account(testmail.DefaultBobAddress)
	dir := t.TempDir()

	originalSubject := "MCP daemon original"
	originalID := "<mcp-daemon-original@herald.test>"
	alice.AppendEML("INBOX", []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: Wed, 20 May 2026 10:00:00 -0700\r\nMessage-ID: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nOriginal body for MCP daemon reply.\r\n", bob.Address, alice.Address, originalSubject, originalID)))

	cfg := alice.Config(filepath.Join(dir, "alice-cache.db"))
	cfg.Sync.Interval = 60
	cfg.Sync.Idle = false
	cfg.Sync.Background = false
	cfg.Sync.IDLEEnabled = false
	cfg.Semantic.Enabled = false
	cfg.AI.Provider = "disabled"
	cfg.Daemon.BindAddr = "127.0.0.1"
	cfg.Daemon.Port = freeTCPPort(t)

	configPath := filepath.Join(dir, "config.yaml")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	daemonServer, err := daemonpkg.New(cfg, configPath)
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- daemonServer.Start()
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = daemonServer.Shutdown(ctx)
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Fatalf("daemon server exited with error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("daemon server did not shut down")
		}
	})

	baseURL := "http://" + net.JoinHostPort(cfg.Daemon.BindAddr, strconv.Itoa(cfg.Daemon.Port))
	waitForDaemonStatus(t, baseURL)
	setMCPDaemonURLForTest(t, baseURL)

	c, err := cache.New(cfg.Cache.DatabasePath)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	h := &mcpVirtualLabDaemon{
		lab:             lab,
		alice:           alice,
		bob:             bob,
		baseURL:         baseURL,
		daemon:          daemonServer,
		cache:           c,
		mcp:             newMCPServer(c, nil),
		originalID:      originalID,
		originalSubject: originalSubject,
	}
	h.syncAndWaitForOriginal(t)
	return h
}

func (h *mcpVirtualLabDaemon) syncAndWaitForOriginal(t *testing.T) {
	t.Helper()
	daemonPostJSON(t, h.baseURL+"/v1/sync", map[string]string{"folder": "INBOX"}, http.StatusAccepted)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body := daemonGetBody(t, h.baseURL+"/v1/emails?folder=INBOX", http.StatusOK)
		var emails []models.EmailData
		if err := json.Unmarshal(body, &emails); err == nil {
			for _, email := range emails {
				if email.MessageID == h.originalID {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for daemon sync of %s", h.originalID)
}

func setMCPDaemonURLForTest(t *testing.T, baseURL string) {
	t.Helper()
	daemonMu.Lock()
	oldURL := daemonURL
	oldBind := daemonProbeBind
	oldPort := daemonProbePort
	daemonURL = baseURL
	daemonProbeBind = "127.0.0.1"
	daemonProbePort = 0
	daemonMu.Unlock()
	t.Cleanup(func() {
		daemonMu.Lock()
		daemonURL = oldURL
		daemonProbeBind = oldBind
		daemonProbePort = oldPort
		daemonMu.Unlock()
	})
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	defer ln.Close()
	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split free port: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse free port: %v", err)
	}
	return port
}

func waitForDaemonStatus(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/v1/status")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("daemon status never became available at %s", baseURL)
}

func daemonPostJSON(t *testing.T, url string, payload any, wantStatus int) []byte {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("encode daemon request: %v", err)
		}
	}
	resp, err := http.Post(url, "application/json", &body)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s status = %d, want %d:\n%s", url, resp.StatusCode, wantStatus, respBody)
	}
	return respBody
}

func daemonGetBody(t *testing.T, url string, wantStatus int) []byte {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d:\n%s", url, resp.StatusCode, wantStatus, body)
	}
	return body
}

func extractDraftUID(t *testing.T, rawJSON string) int {
	t.Helper()
	matches := regexp.MustCompile(`Draft saved \(UID: ([0-9]+)\)`).FindStringSubmatch(rawJSON)
	if len(matches) != 2 {
		t.Fatalf("could not find draft UID in response:\n%s", rawJSON)
	}
	uid, err := strconv.Atoi(matches[1])
	if err != nil {
		t.Fatalf("parse draft UID %q: %v", matches[1], err)
	}
	if uid == 0 {
		t.Fatalf("draft UID must be nonzero in response:\n%s", rawJSON)
	}
	return uid
}
