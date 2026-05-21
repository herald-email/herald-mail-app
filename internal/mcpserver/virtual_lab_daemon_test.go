package mcpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
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

func TestVirtualLabDaemonBackedMCPForwardAndAttachments(t *testing.T) {
	h := startMCPVirtualLabDaemon(t)

	forwardSubject := "Slice 6 forward source"
	forwardID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		forwardSubject,
		"<slice6.forward/source@herald.test>",
		"Original body for the forwarded message.",
	))

	forwardJSON := callVirtualLabTool(t, h.mcp, 10, "forward_email", map[string]any{
		"message_id": forwardID,
		"to":         h.bob.Address,
		"body":       "Forward note from MCP.",
	})
	if !strings.Contains(forwardJSON, "Forwarded to "+h.bob.Address) {
		t.Fatalf("forward_email response missing success text:\n%s", forwardJSON)
	}
	h.lab.WaitForSubject(h.bob.Address, "INBOX", "Fwd: "+forwardSubject)
	h.lab.WaitForSubject(h.alice.Address, "Sent", "Fwd: "+forwardSubject)

	captured := h.lab.CapturedSMTP()
	if len(captured) == 0 {
		t.Fatal("expected forwarded message to be captured by virtual SMTP")
	}
	forwarded := string(captured[len(captured)-1].Data)
	for _, want := range []string{
		"Subject: Fwd: " + forwardSubject,
		"Forward note from MCP.",
		"---------- Forwarded message ----------",
		"Original body for the forwarded message.",
	} {
		if !strings.Contains(forwarded, want) {
			t.Fatalf("forwarded SMTP data missing %q:\n%s", want, forwarded)
		}
	}

	attachmentID := h.appendAliceInboxAndSync(t, rawVirtualLabAttachmentMessage(
		h.bob.Address,
		h.alice.Address,
		"Slice 6 attachment source",
		"<slice6.attach/source@herald.test>",
		"safe-report.txt",
		"safe attachment body\n",
	))

	listJSON := callVirtualLabTool(t, h.mcp, 11, "list_attachments", map[string]any{
		"message_id": attachmentID,
	})
	if !strings.Contains(listJSON, "safe-report.txt") || !strings.Contains(listJSON, "text/plain") {
		t.Fatalf("list_attachments did not return expected metadata:\n%s", listJSON)
	}

	destPath := filepath.Join(t.TempDir(), "safe-report.txt")
	getJSON := callVirtualLabTool(t, h.mcp, 12, "get_attachment", map[string]any{
		"message_id": attachmentID,
		"filename":   "safe-report.txt",
		"dest_path":  destPath,
	})
	if !strings.Contains(getJSON, "Saved to "+destPath) {
		t.Fatalf("get_attachment response missing saved path:\n%s", getJSON)
	}
	written, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read saved attachment: %v", err)
	}
	if string(written) != "safe attachment body\n" {
		t.Fatalf("saved attachment = %q", written)
	}

	conflictJSON := callVirtualLabTool(t, h.mcp, 13, "get_attachment", map[string]any{
		"message_id": attachmentID,
		"filename":   "safe-report.txt",
		"dest_path":  destPath,
	})
	if !strings.Contains(conflictJSON, "file already exists") || !strings.Contains(conflictJSON, "suggested:") {
		t.Fatalf("get_attachment conflict response missing suggested path:\n%s", conflictJSON)
	}
}

func TestVirtualLabDaemonBackedMCPMailboxMutations(t *testing.T) {
	h := startMCPVirtualLabDaemon(t)

	readID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		"Slice 6 read state",
		"<slice6.read/state@herald.test>",
		"Read state body.",
	))
	if got := h.requireCachedEmail(t, readID); got.IsRead {
		t.Fatalf("seeded read-state message should start unread: %#v", got)
	}
	markReadJSON := callVirtualLabTool(t, h.mcp, 20, "mark_read", map[string]any{
		"message_id": readID,
		"folder":     "INBOX",
	})
	if !strings.Contains(markReadJSON, "Marked as read") {
		t.Fatalf("mark_read response missing success text:\n%s", markReadJSON)
	}
	if got := h.requireCachedEmail(t, readID); !got.IsRead {
		t.Fatalf("cached message should be read after mark_read: %#v", got)
	}
	markUnreadJSON := callVirtualLabTool(t, h.mcp, 21, "mark_unread", map[string]any{
		"message_id": readID,
		"folder":     "INBOX",
	})
	if !strings.Contains(markUnreadJSON, "Marked as unread") {
		t.Fatalf("mark_unread response missing success text:\n%s", markUnreadJSON)
	}
	if got := h.requireCachedEmail(t, readID); got.IsRead {
		t.Fatalf("cached message should be unread after mark_unread: %#v", got)
	}

	archiveSubject := "Slice 6 archive action"
	archiveID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		archiveSubject,
		"<slice6.archive/action@herald.test>",
		"Archive body.",
	))
	archiveJSON := callVirtualLabTool(t, h.mcp, 22, "archive_email", map[string]any{
		"message_id": archiveID,
		"folder":     "INBOX",
	})
	if !strings.Contains(archiveJSON, "Email archived") {
		t.Fatalf("archive_email response missing success text:\n%s", archiveJSON)
	}
	h.lab.WaitForSubject(h.alice.Address, "Archive", archiveSubject)
	h.requireNotCached(t, archiveID)

	moveSubject := "Slice 6 move action"
	moveID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		moveSubject,
		"<slice6.move/action@herald.test>",
		"Move body.",
	))
	moveJSON := callVirtualLabTool(t, h.mcp, 23, "move_email", map[string]any{
		"message_id":  moveID,
		"from_folder": "INBOX",
		"to_folder":   "Archive",
	})
	if !strings.Contains(moveJSON, "Moved to Archive") {
		t.Fatalf("move_email response missing success text:\n%s", moveJSON)
	}
	h.lab.WaitForSubject(h.alice.Address, "Archive", moveSubject)
	h.requireNotCached(t, moveID)

	deleteSubject := "Slice 6 delete action"
	deleteID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		deleteSubject,
		"<slice6.delete/action@herald.test>",
		"Delete body.",
	))
	deleteJSON := callVirtualLabTool(t, h.mcp, 24, "delete_email", map[string]any{
		"message_id": deleteID,
		"folder":     "INBOX",
	})
	if !strings.Contains(deleteJSON, "Email deleted") {
		t.Fatalf("delete_email response missing success text:\n%s", deleteJSON)
	}
	h.lab.WaitForSubject(h.alice.Address, "Trash", deleteSubject)
	h.requireNotCached(t, deleteID)
}

func TestVirtualLabDaemonBackedMCPBulkMoveAndDelete(t *testing.T) {
	h := startMCPVirtualLabDaemon(t)

	moveOneSubject := "Slice 6 bulk move one"
	moveOneID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		moveOneSubject,
		"<slice6.bulk-move/one@herald.test>",
		"Bulk move one.",
	))
	moveTwoSubject := "Slice 6 bulk move two"
	moveTwoID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		moveTwoSubject,
		"<slice6.bulk-move/two@herald.test>",
		"Bulk move two.",
	))
	deleteOneSubject := "Slice 6 bulk delete one"
	deleteOneID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		deleteOneSubject,
		"<slice6.bulk-delete/one@herald.test>",
		"Bulk delete one.",
	))
	deleteTwoSubject := "Slice 6 bulk delete two"
	deleteTwoID := h.appendAliceInboxAndSync(t, rawVirtualLabTextMessage(
		h.bob.Address,
		h.alice.Address,
		deleteTwoSubject,
		"<slice6.bulk-delete/two@herald.test>",
		"Bulk delete two.",
	))

	moveIDs, err := json.Marshal([]string{moveOneID, moveTwoID})
	if err != nil {
		t.Fatalf("marshal bulk move ids: %v", err)
	}
	bulkMoveJSON := callVirtualLabTool(t, h.mcp, 30, "bulk_move", map[string]any{
		"message_ids": string(moveIDs),
		"to_folder":   "Archive",
	})
	if !strings.Contains(bulkMoveJSON, "Moved 2 emails to Archive") {
		t.Fatalf("bulk_move response missing success text:\n%s", bulkMoveJSON)
	}
	h.lab.WaitForSubject(h.alice.Address, "Archive", moveOneSubject)
	h.lab.WaitForSubject(h.alice.Address, "Archive", moveTwoSubject)
	h.requireNotCached(t, moveOneID)
	h.requireNotCached(t, moveTwoID)

	deleteIDs, err := json.Marshal([]string{deleteOneID, deleteTwoID})
	if err != nil {
		t.Fatalf("marshal bulk delete ids: %v", err)
	}
	bulkDeleteJSON := callVirtualLabTool(t, h.mcp, 31, "bulk_delete", map[string]any{
		"message_ids": string(deleteIDs),
	})
	if !strings.Contains(bulkDeleteJSON, "Deleted 2 emails") {
		t.Fatalf("bulk_delete response missing success text:\n%s", bulkDeleteJSON)
	}
	h.lab.WaitForSubject(h.alice.Address, "Trash", deleteOneSubject)
	h.lab.WaitForSubject(h.alice.Address, "Trash", deleteTwoSubject)
	h.requireNotCached(t, deleteOneID)
	h.requireNotCached(t, deleteTwoID)
}

func TestVirtualLabDaemonBackedMCPUnsubscribeAndSoftUnsubscribe(t *testing.T) {
	h := startMCPVirtualLabDaemon(t)

	var posts atomic.Int32
	postBodies := make(chan string, 2)
	unsubServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		postBodies <- string(body)
		if r.Method != http.MethodPost {
			t.Errorf("unsubscribe method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer unsubServer.Close()

	oldTransport := http.DefaultTransport
	http.DefaultTransport = unsubServer.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	oneClickID := h.appendAliceInboxAndSync(t, rawMCPVirtualLabUnsubscribeMessage(
		"digest@herald.test",
		h.alice.Address,
		"MCP unsubscribe one-click",
		"<mcp.unsubscribe.one-click@herald.test>",
		"<"+unsubServer.URL+"/one-click>",
		"List-Unsubscribe=One-Click",
	))

	unsubJSON := callVirtualLabTool(t, h.mcp, 40, "unsubscribe_sender", map[string]any{
		"message_id": oneClickID,
	})
	if !strings.Contains(unsubJSON, "Unsubscribed") {
		t.Fatalf("unsubscribe_sender response missing success text:\n%s", unsubJSON)
	}
	select {
	case body := <-postBodies:
		if body != "List-Unsubscribe=One-Click" {
			t.Fatalf("one-click POST body = %q", body)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for local one-click unsubscribe POST")
	}
	if posts.Load() != 1 {
		t.Fatalf("unsubscribe POST count = %d, want 1", posts.Load())
	}
	if ok, err := h.cache.IsUnsubscribedSender("digest@herald.test"); err != nil || !ok {
		t.Fatalf("expected digest@herald.test recorded as unsubscribed, ok=%v err=%v", ok, err)
	}

	softJSON := callVirtualLabTool(t, h.mcp, 41, "soft_unsubscribe_sender", map[string]any{
		"sender": "digest@herald.test",
	})
	if !strings.Contains(softJSON, "Auto-move rule created for digest@herald.test") {
		t.Fatalf("soft_unsubscribe_sender response missing success text:\n%s", softJSON)
	}
	assertMCPSoftUnsubscribeRule(t, h.cache, "digest@herald.test", "Disabled Subscriptions")

	customJSON := callVirtualLabTool(t, h.mcp, 42, "soft_unsubscribe_sender", map[string]any{
		"sender":    "alerts@herald.test",
		"to_folder": "Archive",
	})
	if !strings.Contains(customJSON, "Auto-move rule created for alerts@herald.test") {
		t.Fatalf("soft_unsubscribe_sender custom response missing success text:\n%s", customJSON)
	}
	assertMCPSoftUnsubscribeRule(t, h.cache, "alerts@herald.test", "Archive")
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

	for i, tc := range []struct {
		tool string
		args map[string]any
	}{
		{
			tool: "forward_email",
			args: map[string]any{
				"message_id": "<missing.forward/source@herald.test>",
				"to":         testmail.DefaultBobAddress,
			},
		},
		{
			tool: "list_attachments",
			args: map[string]any{
				"message_id": "<missing.attach/source@herald.test>",
			},
		},
		{
			tool: "delete_email",
			args: map[string]any{
				"message_id": "<missing.delete/source@herald.test>",
				"folder":     "INBOX",
			},
		},
		{
			tool: "unsubscribe_sender",
			args: map[string]any{
				"message_id": "<missing.unsubscribe/source@herald.test>",
			},
		},
		{
			tool: "soft_unsubscribe_sender",
			args: map[string]any{
				"sender": "digest@herald.test",
			},
		},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			raw := callVirtualLabTool(t, s, 10+i, tc.tool, tc.args)
			if !strings.Contains(raw, "daemon not running") {
				t.Fatalf("%s without daemon should explain daemon requirement:\n%s", tc.tool, raw)
			}
		})
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
	h.syncAndWaitForMessageID(t, h.originalID)
}

func (h *mcpVirtualLabDaemon) appendAliceInboxAndSync(t *testing.T, raw []byte, flags ...string) string {
	t.Helper()
	ref := h.alice.AppendEML("INBOX", raw, flags...)
	h.syncAndWaitForMessageID(t, ref.MessageID)
	return ref.MessageID
}

func (h *mcpVirtualLabDaemon) syncAndWaitForMessageID(t *testing.T, messageID string) {
	t.Helper()
	daemonPostJSON(t, h.baseURL+"/v1/sync", map[string]string{"folder": "INBOX"}, http.StatusAccepted)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body := daemonGetBody(t, h.baseURL+"/v1/emails?folder=INBOX", http.StatusOK)
		var emails []models.EmailData
		if err := json.Unmarshal(body, &emails); err == nil {
			for _, email := range emails {
				if email.MessageID == messageID {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for daemon sync of %s", messageID)
}

func (h *mcpVirtualLabDaemon) requireCachedEmail(t *testing.T, messageID string) *models.EmailData {
	t.Helper()
	email, err := h.cache.GetEmailByID(messageID)
	if err != nil {
		t.Fatalf("expected %s in cache: %v", messageID, err)
	}
	if email == nil {
		t.Fatalf("expected %s in cache, got nil", messageID)
	}
	return email
}

func (h *mcpVirtualLabDaemon) requireNotCached(t *testing.T, messageID string) {
	t.Helper()
	email, err := h.cache.GetEmailByID(messageID)
	if err == nil {
		t.Fatalf("expected %s to be removed from cache, got %#v", messageID, email)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cache lookup for %s failed unexpectedly: %v", messageID, err)
	}
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

func rawVirtualLabTextMessage(from, to, subject, messageID, body string) []byte {
	return []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: Wed, 20 May 2026 10:00:00 -0700\r\nMessage-ID: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", from, to, subject, messageID, body))
}

func rawMCPVirtualLabUnsubscribeMessage(from, to, subject, messageID, listUnsubscribe, listUnsubscribePost string) []byte {
	postHeader := ""
	if listUnsubscribePost != "" {
		postHeader = "List-Unsubscribe-Post: " + listUnsubscribePost + "\r\n"
	}
	return []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: Wed, 20 May 2026 10:00:00 -0700\r\nMessage-ID: %s\r\nList-Unsubscribe: %s\r\n%sContent-Type: text/plain; charset=utf-8\r\n\r\nUnsubscribe fixture body.\r\n", from, to, subject, messageID, listUnsubscribe, postHeader))
}

func rawVirtualLabAttachmentMessage(from, to, subject, messageID, filename, body string) []byte {
	return []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: Wed, 20 May 2026 10:00:00 -0700\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"slice6-attachment-boundary\"\r\n\r\n--slice6-attachment-boundary\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nPlease review the attachment.\r\n--slice6-attachment-boundary\r\nContent-Type: text/plain; name=\"%s\"\r\nContent-Disposition: attachment; filename=\"%s\"\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n--slice6-attachment-boundary--\r\n", from, to, subject, messageID, filename, filename, base64.StdEncoding.EncodeToString([]byte(body))))
}

func assertMCPSoftUnsubscribeRule(t *testing.T, c *cache.Cache, sender, folder string) {
	t.Helper()
	rules, err := c.GetAllRules()
	if err != nil {
		t.Fatalf("GetAllRules: %v", err)
	}
	for _, rule := range rules {
		if rule.TriggerType == models.TriggerSender && rule.TriggerValue == sender && rule.Enabled && len(rule.Actions) == 1 &&
			rule.Actions[0].Type == models.ActionMove && rule.Actions[0].DestFolder == folder {
			return
		}
	}
	t.Fatalf("missing soft-unsubscribe rule sender=%q folder=%q in %#v", sender, folder, rules)
}
