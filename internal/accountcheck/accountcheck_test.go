package accountcheck

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
)

func TestResultErrNamesFailedSurfaces(t *testing.T) {
	result := Result{
		IMAP: Check{Surface: "IMAP", Err: errors.New("login rejected")},
		SMTP: Check{Surface: "SMTP", Err: errors.New("auth rejected")},
	}

	err := result.Err()
	if err == nil {
		t.Fatal("expected combined error")
	}
	if got := err.Error(); !strings.Contains(got, "IMAP") || !strings.Contains(got, "SMTP") {
		t.Fatalf("combined error should name both failed surfaces, got %q", got)
	}
}

func TestResultUserMessageExplainsConfigWasNotSaved(t *testing.T) {
	result := Result{
		IMAP: Check{Surface: "IMAP", Err: errors.New("login rejected")},
		SMTP: Check{Surface: "SMTP"},
	}

	msg := result.UserMessage("/tmp/herald.log", "/tmp/herald.yaml")
	for _, want := range []string{"IMAP", "not saved", "/tmp/herald.yaml", "/tmp/herald.log"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("UserMessage missing %q in %q", want, msg)
		}
	}
}

func TestValidateNilConfigFailsBothSurfaces(t *testing.T) {
	result := Validate(context.Background(), nil, "")
	if result.IMAP.OK() || result.SMTP.OK() {
		t.Fatalf("expected nil config to fail both surfaces, got %#v", result)
	}
	if err := result.Err(); err == nil || !strings.Contains(err.Error(), "IMAP") || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("expected combined nil-config error, got %v", err)
	}
}

func TestValidateGmailOAuthSourceUsesProviderCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/labels" {
			t.Fatalf("unexpected Gmail API validation path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"labels": []map[string]string{{"id": "INBOX", "name": "INBOX", "type": "system"}},
		})
	}))
	defer server.Close()

	cfg := &config.Config{Sources: []config.SourceConfig{{
		ID:        "test-gmail",
		Kind:      "mail",
		Provider:  "gmail",
		AccountID: "test",
		Google: config.GoogleConfig{
			Email:       "test@example.com",
			AccessToken: "access-token",
			APIBaseURL:  server.URL + "/gmail/v1",
		},
	}}}

	result := Validate(context.Background(), cfg, "")
	if !result.OK() {
		t.Fatalf("Validate gmail OAuth = %#v, want provider-backed success", result)
	}
	if result.IMAP.Surface != "Gmail API" || result.SMTP.Surface != "Gmail API send" {
		t.Fatalf("surfaces = %q/%q, want Gmail API labels", result.IMAP.Surface, result.SMTP.Surface)
	}
}
