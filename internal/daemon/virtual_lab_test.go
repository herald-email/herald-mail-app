package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/testmail"
)

type virtualLabDaemon struct {
	lab             *testmail.Lab
	alice           *testmail.Account
	bob             *testmail.Account
	server          *Server
	httpServer      *httptest.Server
	originalID      string
	originalSubject string
}

func TestVirtualLabDaemonUnsubscribeAndSoftUnsubscribe(t *testing.T) {
	h := startVirtualLabDaemon(t)

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

	oneClickID := h.appendAliceInboxAndSync(t, rawVirtualLabUnsubscribeMessage(
		"digest@herald.test",
		h.alice.Address,
		"Daemon unsubscribe one-click",
		"<daemon.unsubscribe.one-click@herald.test>",
		"<"+unsubServer.URL+"/one-click>",
		"List-Unsubscribe=One-Click",
	))

	h.postJSON(t, "/v1/emails/"+url.PathEscape(oneClickID)+"/unsubscribe", nil, http.StatusOK)
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
	if ok, err := h.server.cache.IsUnsubscribedSender("digest@herald.test"); err != nil || !ok {
		t.Fatalf("expected digest@herald.test recorded as unsubscribed, ok=%v err=%v", ok, err)
	}

	noHeaderID := h.appendAliceInboxAndSync(t, []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Daemon unsubscribe no header\r\nDate: Wed, 20 May 2026 10:00:00 -0700\r\nMessage-ID: <daemon.unsubscribe.no-header@herald.test>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nNo unsubscribe header here.\r\n", h.bob.Address, h.alice.Address)))
	noHeaderBody := h.postJSON(t, "/v1/emails/"+url.PathEscape(noHeaderID)+"/unsubscribe", nil, http.StatusInternalServerError)
	if !bytes.Contains(noHeaderBody, []byte("no List-Unsubscribe header")) {
		t.Fatalf("missing-header unsubscribe error should be clear:\n%s", noHeaderBody)
	}

	h.postJSON(t, "/v1/senders/"+url.PathEscape("digest@herald.test")+"/soft-unsubscribe", nil, http.StatusOK)
	assertSoftUnsubscribeRule(t, h.server.cache, "digest@herald.test", "Disabled Subscriptions")

	h.postJSON(t, "/v1/senders/"+url.PathEscape("alerts@herald.test")+"/soft-unsubscribe", map[string]string{"to_folder": "Archive"}, http.StatusOK)
	assertSoftUnsubscribeRule(t, h.server.cache, "alerts@herald.test", "Archive")
}

func TestVirtualLabDaemonRoutesSendDraftAndReply(t *testing.T) {
	h := startVirtualLabDaemon(t)

	h.postJSON(t, "/v1/emails/send", map[string]string{
		"to":      h.bob.Address,
		"subject": "Daemon virtual send",
		"body":    "Hello from the daemon route.",
		"from":    h.alice.Address,
	}, http.StatusOK)
	h.lab.WaitForSubject(h.bob.Address, "INBOX", "Daemon virtual send")
	h.lab.WaitForSubject(h.alice.Address, "Sent", "Daemon virtual send")

	saveBody := h.postJSON(t, "/v1/drafts", map[string]string{
		"to":      h.bob.Address,
		"subject": "Daemon virtual draft",
		"body":    "Draft body through daemon.",
	}, http.StatusOK)
	var saved struct {
		UID    uint32 `json:"uid"`
		Folder string `json:"folder"`
	}
	if err := json.Unmarshal(saveBody, &saved); err != nil {
		t.Fatalf("decode save draft response: %v\n%s", err, saveBody)
	}
	if saved.UID == 0 || saved.Folder != "Drafts" {
		t.Fatalf("saved draft = uid %d folder %q, want nonzero Drafts", saved.UID, saved.Folder)
	}
	h.lab.WaitForSubject(h.alice.Address, "Drafts", "Daemon virtual draft")

	draftsBody := h.get(t, "/v1/drafts", http.StatusOK)
	if !bytes.Contains(draftsBody, []byte("Daemon virtual draft")) {
		t.Fatalf("list drafts missing saved subject:\n%s", draftsBody)
	}

	h.postJSON(t, fmt.Sprintf("/v1/drafts/%d/send?folder=Drafts", saved.UID), nil, http.StatusOK)
	h.lab.WaitForSubject(h.bob.Address, "INBOX", "Daemon virtual draft")
	h.lab.WaitForSubject(h.alice.Address, "Sent", "Daemon virtual draft")
	afterSendDrafts := h.get(t, "/v1/drafts", http.StatusOK)
	if bytes.Contains(afterSendDrafts, []byte("Daemon virtual draft")) {
		t.Fatalf("sent draft should be deleted, got drafts:\n%s", afterSendDrafts)
	}

	h.postJSON(t, "/v1/emails/"+url.PathEscape(h.originalID)+"/reply", map[string]string{
		"body": "Reply body through daemon.",
	}, http.StatusOK)
	h.lab.WaitForSubject(h.bob.Address, "INBOX", "Re: "+h.originalSubject)

	captured := h.lab.CapturedSMTP()
	if len(captured) < 3 {
		t.Fatalf("captured SMTP count = %d, want send, draft send, and reply", len(captured))
	}
	reply := string(captured[len(captured)-1].Data)
	for _, want := range []string{
		"In-Reply-To: " + h.originalID,
		"References: " + h.originalID,
		"Reply body through daemon.",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func startVirtualLabDaemon(t *testing.T) *virtualLabDaemon {
	t.Helper()

	lab := testmail.Start(t)
	alice := lab.Account(testmail.DefaultAliceAddress)
	bob := lab.Account(testmail.DefaultBobAddress)
	dir := t.TempDir()

	originalSubject := "Daemon virtual original"
	originalID := "<daemon-virtual-original@herald.test>"
	alice.AppendEML("INBOX", []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: Wed, 20 May 2026 10:00:00 -0700\r\nMessage-ID: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nOriginal body for daemon reply.\r\n", bob.Address, alice.Address, originalSubject, originalID)))

	cfg := alice.Config(filepath.Join(dir, "alice-cache.db"))
	cfg.Sync.Interval = 60
	cfg.Sync.Idle = false
	cfg.Sync.Background = false
	cfg.Sync.IDLEEnabled = false
	cfg.Semantic.Enabled = false
	cfg.AI.Provider = "disabled"
	configPath := filepath.Join(dir, "config.yaml")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	server, err := New(cfg, configPath)
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	mux := http.NewServeMux()
	server.registerRoutes(mux)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(func() {
		httpServer.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	h := &virtualLabDaemon{
		lab:             lab,
		alice:           alice,
		bob:             bob,
		server:          server,
		httpServer:      httpServer,
		originalID:      originalID,
		originalSubject: originalSubject,
	}
	h.syncAndWaitForOriginal(t)
	return h
}

func (h *virtualLabDaemon) syncAndWaitForOriginal(t *testing.T) {
	t.Helper()
	h.syncAndWaitForMessageID(t, h.originalID)
}

func (h *virtualLabDaemon) appendAliceInboxAndSync(t *testing.T, raw []byte, flags ...string) string {
	t.Helper()
	ref := h.alice.AppendEML("INBOX", raw, flags...)
	h.syncAndWaitForMessageID(t, ref.MessageID)
	return ref.MessageID
}

func (h *virtualLabDaemon) syncAndWaitForMessageID(t *testing.T, messageID string) {
	t.Helper()
	h.postJSON(t, "/v1/sync", map[string]string{"folder": "INBOX"}, http.StatusAccepted)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		body := h.get(t, "/v1/emails?folder=INBOX", http.StatusOK)
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

func (h *virtualLabDaemon) postJSON(t *testing.T, path string, payload any, wantStatus int) []byte {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	resp, err := http.Post(h.httpServer.URL+path, "application/json", &body)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	respBody := new(bytes.Buffer)
	_, _ = respBody.ReadFrom(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST %s status = %d, want %d:\n%s", path, resp.StatusCode, wantStatus, respBody.String())
	}
	return respBody.Bytes()
}

func (h *virtualLabDaemon) get(t *testing.T, path string, wantStatus int) []byte {
	t.Helper()
	resp, err := http.Get(h.httpServer.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d:\n%s", path, resp.StatusCode, wantStatus, body.String())
	}
	return body.Bytes()
}

func rawVirtualLabUnsubscribeMessage(from, to, subject, messageID, listUnsubscribe, listUnsubscribePost string) []byte {
	postHeader := ""
	if listUnsubscribePost != "" {
		postHeader = "List-Unsubscribe-Post: " + listUnsubscribePost + "\r\n"
	}
	return []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: Wed, 20 May 2026 10:00:00 -0700\r\nMessage-ID: %s\r\nList-Unsubscribe: %s\r\n%sContent-Type: text/plain; charset=utf-8\r\n\r\nUnsubscribe fixture body.\r\n", from, to, subject, messageID, listUnsubscribe, postHeader))
}

func assertSoftUnsubscribeRule(t *testing.T, store interface {
	GetAllRules() ([]*models.Rule, error)
}, sender, folder string) {
	t.Helper()
	rules, err := store.GetAllRules()
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
