package oauth

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/herald-email/herald-mail-app/internal/config"
	"golang.org/x/oauth2"
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
	t.Setenv("HERALD_GOOGLE_CLIENT_ID", "test-client-id.apps.googleusercontent.com")
	t.Setenv("HERALD_GOOGLE_CLIENT_SECRET", "test-client-secret")

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
	assertAuthURLScopes(t, authURL, []string{ScopeOpenID, ScopeEmail, ScopeGmailModify})
}

func TestDefaultGoogleOAuthScopesIncludeIdentityAndGmailModify(t *testing.T) {
	want := []string{ScopeOpenID, ScopeEmail, ScopeGmailModify}
	if !equalStringSlices(Scopes, want) {
		t.Fatalf("Scopes = %#v, want %#v", Scopes, want)
	}
}

func TestGoogleOAuthScopesAreProviderAware(t *testing.T) {
	tests := []struct {
		name    string
		sources []config.SourceConfig
		want    []string
	}{
		{
			name: "gmail mail defaults to api scope",
			sources: []config.SourceConfig{{
				Kind:     "mail",
				Provider: "gmail",
			}},
			want: []string{ScopeOpenID, ScopeEmail, ScopeGmailModify},
		},
		{
			name: "gmail api alias keeps api scope",
			sources: []config.SourceConfig{{
				Kind:     "mail",
				Provider: "gmail_api",
			}},
			want: []string{ScopeOpenID, ScopeEmail, ScopeGmailModify},
		},
		{
			name: "calendar source only",
			sources: []config.SourceConfig{{
				Kind:     "calendar",
				Provider: "google_calendar",
			}},
			want: []string{ScopeOpenID, ScopeEmail, ScopeCalendarListReadonly, ScopeCalendarEvents},
		},
		{
			name: "gmail api mail plus calendar",
			sources: []config.SourceConfig{
				{Kind: "mail", Provider: "gmail"},
				{Kind: "calendar", Provider: "google_calendar"},
			},
			want: []string{ScopeOpenID, ScopeEmail, ScopeGmailModify, ScopeCalendarListReadonly, ScopeCalendarEvents},
		},
		{
			name: "non-google source falls back to default gmail oauth set",
			sources: []config.SourceConfig{{
				Kind:     "mail",
				Provider: "imap",
			}},
			want: []string{ScopeOpenID, ScopeEmail, ScopeGmailModify},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScopesForSources(tt.sources)
			if !equalStringSlices(got, tt.want) {
				t.Fatalf("ScopesForSources() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func assertAuthURLScopes(t *testing.T, authURL string, want []string) {
	t.Helper()
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("could not parse auth URL: %v", err)
	}
	if got := strings.Fields(parsed.Query().Get("scope")); !equalStringSlices(got, want) {
		t.Fatalf("auth URL scopes = %#v, want %#v", got, want)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAuthenticatedEmailFromTokenDecodesIDToken(t *testing.T) {
	token := (&oauth2.Token{AccessToken: "access"}).WithExtra(map[string]interface{}{
		"id_token": idTokenForTest("work@example.test", true),
	})
	got, err := AuthenticatedEmailFromToken(token)
	if err != nil {
		t.Fatalf("AuthenticatedEmailFromToken returned error: %v", err)
	}
	if got != "work@example.test" {
		t.Fatalf("AuthenticatedEmailFromToken = %q, want work@example.test", got)
	}
}

func TestAuthenticatedEmailFromTokenRejectsUnverifiedEmail(t *testing.T) {
	token := (&oauth2.Token{AccessToken: "access"}).WithExtra(map[string]interface{}{
		"id_token": idTokenForTest("work@example.test", false),
	})
	if _, err := AuthenticatedEmailFromToken(token); err != ErrAuthenticatedEmailUnverified {
		t.Fatalf("AuthenticatedEmailFromToken err = %v, want %v", err, ErrAuthenticatedEmailUnverified)
	}
}

func idTokenForTest(email string, verified bool) string {
	segment := func(v string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(v))
	}
	return strings.Join([]string{
		segment(`{"alg":"none"}`),
		segment(fmt.Sprintf(`{"email":%q,"email_verified":%t}`, email, verified)),
		"signature",
	}, ".")
}

func TestStartFlow_AuthorizeRedirectsToGoogleURL(t *testing.T) {
	t.Setenv("HERALD_GOOGLE_CLIENT_ID", "test-client-id.apps.googleusercontent.com")
	t.Setenv("HERALD_GOOGLE_CLIENT_SECRET", "test-client-secret")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	authURL, codeCh, err := StartFlow(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("StartFlow returned error: %v", err)
	}
	if codeCh == nil {
		t.Fatal("StartFlow returned nil channel")
	}

	localAuthorizeURL := localAuthorizeURLFromAuthURLForTest(t, authURL)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(localAuthorizeURL) //nolint:noctx
	if err != nil {
		t.Fatalf("HTTP GET %s failed: %v", localAuthorizeURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected /authorize to redirect with 302 Found, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != authURL {
		t.Fatalf("expected /authorize Location to be Google auth URL\n got: %s\nwant: %s", got, authURL)
	}
}

func localAuthorizeURLFromAuthURLForTest(t *testing.T, authURL string) string {
	t.Helper()

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("could not parse auth URL: %v", err)
	}
	redirectURI := parsed.Query().Get("redirect_uri")
	if redirectURI == "" {
		t.Fatalf("auth URL missing redirect_uri: %s", authURL)
	}
	redirect, err := url.Parse(redirectURI)
	if err != nil {
		t.Fatalf("could not parse redirect URI: %v", err)
	}
	redirect.Path = "/authorize"
	redirect.RawQuery = ""
	redirect.Fragment = ""
	return redirect.String()
}

// TestStartFlow_CallbackStateValidation verifies the local callback server
// rejects requests with an invalid state parameter and sends an error on the channel.
func TestStartFlow_CallbackStateValidation(t *testing.T) {
	t.Setenv("HERALD_GOOGLE_CLIENT_ID", "test-client-id.apps.googleusercontent.com")
	t.Setenv("HERALD_GOOGLE_CLIENT_SECRET", "test-client-secret")

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
