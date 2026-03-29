package oauth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"mail-processor/internal/config"
)

// TestRefreshIfNeeded_NonExpired verifies that when the stored access token
// is not close to expiry, RefreshIfNeeded returns the existing token without
// performing a network refresh.
func TestRefreshIfNeeded_NonExpired(t *testing.T) {
	expiry := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	cfg := &config.Config{}
	cfg.Gmail.AccessToken = "existing-access-token"
	cfg.Gmail.RefreshToken = "some-refresh-token"
	cfg.Gmail.TokenExpiry = expiry

	token, err := RefreshIfNeeded(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected no error for non-expired token, got: %v", err)
	}
	if token != "existing-access-token" {
		t.Errorf("expected existing access token, got: %q", token)
	}
}

// TestRefreshIfNeeded_EmptyRefreshToken verifies that RefreshIfNeeded returns
// an error when no refresh token is stored.
func TestRefreshIfNeeded_EmptyRefreshToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gmail.AccessToken = "some-token"
	cfg.Gmail.RefreshToken = ""

	_, err := RefreshIfNeeded(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for missing refresh token, got nil")
	}
}

// TestStartFlow_ReturnsGoogleURL verifies that StartFlow starts an HTTP server
// and returns a valid Google OAuth2 authorization URL.
func TestStartFlow_ReturnsGoogleURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	authURL, codeCh, err := StartFlow(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("StartFlow returned error: %v", err)
	}
	if codeCh == nil {
		t.Fatal("StartFlow returned nil channel")
	}
	if !strings.HasPrefix(authURL, "https://accounts.google.com/o/oauth2/auth") {
		t.Errorf("expected Google OAuth URL, got: %q", authURL)
	}
	if !strings.Contains(authURL, "redirect_uri=http%3A%2F%2Flocalhost%3A") {
		t.Errorf("expected redirect_uri with localhost in URL, got: %q", authURL)
	}
	if !strings.Contains(authURL, "access_type=offline") {
		t.Errorf("expected access_type=offline in URL, got: %q", authURL)
	}
	if !strings.Contains(authURL, "prompt=consent") {
		t.Errorf("expected prompt=consent in URL, got: %q", authURL)
	}
	if !strings.Contains(authURL, "login_hint=test%40example.com") {
		t.Errorf("expected login_hint in URL, got: %q", authURL)
	}
}

// TestStartFlow_CallbackStateValidation verifies the local callback server
// rejects requests with an invalid state parameter and sends an error on the channel.
func TestStartFlow_CallbackStateValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	authURL, codeCh, err := StartFlow(ctx, "")
	if err != nil {
		t.Fatalf("StartFlow returned error: %v", err)
	}

	// Extract the port from the redirect_uri embedded in the auth URL.
	const marker = "redirect_uri=http%3A%2F%2Flocalhost%3A"
	idx := strings.Index(authURL, marker)
	if idx == -1 {
		t.Skip("could not extract redirect_uri from auth URL; skipping callback test")
	}
	rest := authURL[idx+len(marker):]
	portEnd := strings.Index(rest, "%2F")
	if portEnd == -1 {
		t.Skip("could not parse port from redirect_uri; skipping callback test")
	}
	port := rest[:portEnd]

	// Issue a request with a wrong state — the server should respond with an error
	// and send an error Result on the channel.
	callbackURL := "http://localhost:" + port + "/callback?state=WRONGSTATE&code=abc123"
	resp, err := http.Get(callbackURL) //nolint:noctx
	if err != nil {
		t.Fatalf("HTTP GET %s failed: %v", callbackURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for bad state, got %d", resp.StatusCode)
	}

	select {
	case result := <-codeCh:
		if result.Err == nil {
			t.Error("expected error result for bad state, got nil error")
		}
		if result.Code != "" {
			t.Errorf("expected empty code for bad state, got: %q", result.Code)
		}
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for result from callback server")
	}
}
