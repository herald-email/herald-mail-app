package mcpserver

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestDaemonPostReprobesAfterInitialMiss(t *testing.T) {
	oldURL := daemonURL
	oldBind := daemonProbeBind
	oldPort := daemonProbePort
	t.Cleanup(func() {
		daemonURL = oldURL
		daemonProbeBind = oldBind
		daemonProbePort = oldPort
	})
	daemonURL = ""

	probeListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for probe port: %v", err)
	}
	_, portText, err := net.SplitHostPort(probeListener.Addr().String())
	if err != nil {
		t.Fatalf("split probe listener address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse probe port: %v", err)
	}
	if err := probeListener.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}

	probeDaemon(port)
	if daemonURL != "" {
		t.Fatalf("expected initial probe to miss before daemon starts, got %q", daemonURL)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode daemon post payload: %v", err)
		}
		_, _ = w.Write([]byte("pong:" + payload["hello"]))
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.Listener, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", portText))
	if err != nil {
		t.Fatalf("start daemon test listener on initial probe port: %v", err)
	}
	srv.Start()
	defer srv.Close()

	body, status, err := daemonPost("/v1/ping", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("daemonPost should re-probe after daemon starts: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if !strings.Contains(string(body), "pong:world") {
		t.Fatalf("unexpected response body: %q", body)
	}
}
