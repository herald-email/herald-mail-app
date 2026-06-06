// Package oauth provides Gmail OAuth2 authentication support.
// It implements the authorization code flow with a local HTTP callback server,
// token exchange, and automatic token refresh.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/herald-email/herald-mail-app/internal/config"
)

const (
	ScopeOpenID               = "openid"
	ScopeEmail                = "email"
	ScopeGmailModify          = "https://www.googleapis.com/auth/gmail.modify"
	ScopeCalendarListReadonly = "https://www.googleapis.com/auth/calendar.calendarlist.readonly"
	ScopeCalendarEvents       = "https://www.googleapis.com/auth/calendar.events"
)

// Scopes is the default browser OAuth scope set for new Gmail API mail setup.
// Provider-aware callers should prefer ScopesForSources.
var Scopes = []string{ScopeOpenID, ScopeEmail, ScopeGmailModify}

var (
	ErrAuthenticatedEmailUnavailable = errors.New("authenticated Google email is unavailable")
	ErrAuthenticatedEmailUnverified  = errors.New("authenticated Google email is not verified")
)

// ScopesForSources returns the minimum Google OAuth scopes needed for the
// configured Google-backed sources. Gmail OAuth uses the Gmail API mail scope;
// credential-based Gmail IMAP/App Password sources do not start this flow.
func ScopesForSources(sources []config.SourceConfig) []string {
	if len(sources) == 0 {
		return append([]string(nil), Scopes...)
	}
	var out []string
	add := func(scope string) {
		for _, existing := range out {
			if existing == scope {
				return
			}
		}
		out = append(out, scope)
	}
	add(ScopeOpenID)
	add(ScopeEmail)
	addedProviderScope := false
	for _, source := range sources {
		kind := strings.TrimSpace(source.Kind)
		provider := strings.ToLower(strings.TrimSpace(source.Provider))
		switch {
		case kind == "calendar" && provider == "google_calendar":
			add(ScopeCalendarListReadonly)
			add(ScopeCalendarEvents)
			addedProviderScope = true
		case (kind == "" || kind == "mail") && (provider == "gmail" || provider == "gmail_api"):
			add(ScopeGmailModify)
			addedProviderScope = true
		}
	}
	if !addedProviderScope {
		return append([]string(nil), Scopes...)
	}
	return out
}

// clientID and clientSecret are the OAuth2 application credentials.
// Builds can set defaultClientID and defaultClientSecret with -ldflags -X.
// Runtime environment variables still take precedence for local testing.
var (
	defaultClientID     = ""
	defaultClientSecret = ""
)

func credentials() (string, string, error) {
	id := os.Getenv("HERALD_GOOGLE_CLIENT_ID")
	if id == "" {
		id = defaultClientID
	}
	secret := os.Getenv("HERALD_GOOGLE_CLIENT_SECRET")
	if secret == "" {
		secret = defaultClientSecret
	}
	if id == "" || secret == "" {
		return "", "", fmt.Errorf("Google OAuth credentials are not configured; set HERALD_GOOGLE_CLIENT_ID and HERALD_GOOGLE_CLIENT_SECRET or build with OAuth defaults from .herald-dev.env")
	}
	return id, secret, nil
}

// Configured reports whether Herald has Google OAuth application credentials
// available for starting a desktop OAuth flow.
func Configured() bool {
	_, _, err := credentials()
	return err == nil
}

// NewGmailConfig returns an OAuth2 config for Gmail using the provided
// client ID and secret. The redirect URL should be a localhost callback
// for the desktop auth flow.
func NewGmailConfig(clientIDVal, clientSecretVal, redirectURL string) *oauth2.Config {
	return NewGoogleConfigWithScopes(clientIDVal, clientSecretVal, redirectURL, Scopes)
}

func NewGoogleConfigWithScopes(clientIDVal, clientSecretVal, redirectURL string, scopes []string) *oauth2.Config {
	if len(scopes) == 0 {
		scopes = Scopes
	}
	return &oauth2.Config{
		ClientID:     clientIDVal,
		ClientSecret: clientSecretVal,
		RedirectURL:  redirectURL,
		Scopes:       append([]string(nil), scopes...),
		Endpoint:     google.Endpoint,
	}
}

// Result is returned on the code channel from StartFlow.
type Result struct {
	Code string // authorization code, empty on error
	Err  error
}

// StartFlow starts the OAuth2 authorization code flow.
// It starts a local HTTP server on a random port, constructs the Google
// authorization URL, and returns both the URL and a channel that will
// receive the authorization code when the user completes the flow.
// The server shuts down automatically after receiving one code or when ctx is cancelled.
func StartFlow(ctx context.Context, email string) (authURL string, codeCh <-chan Result, err error) {
	return StartFlowForSources(ctx, email, nil)
}

func StartFlowForSources(ctx context.Context, email string, sources []config.SourceConfig) (authURL string, codeCh <-chan Result, err error) {
	clientID, clientSecret, err := credentials()
	if err != nil {
		return "", nil, err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("failed to start local HTTP server: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	cfg := NewGoogleConfigWithScopes(clientID, clientSecret, redirectURI, ScopesForSources(sources))

	// Generate a random state parameter for CSRF protection.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		_ = listener.Close()
		return "", nil, fmt.Errorf("failed to generate state parameter: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	authURLOpts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	}
	if email != "" {
		authURLOpts = append(authURLOpts, oauth2.SetAuthURLParam("login_hint", email))
	}
	url := cfg.AuthCodeURL(state, authURLOpts...)

	ch := make(chan Result, 1)

	mux := http.NewServeMux()
	server := &http.Server{Handler: mux}

	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, url, http.StatusFound)
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "invalid state parameter", http.StatusBadRequest)
			ch <- Result{Err: fmt.Errorf("OAuth2 callback: state mismatch (possible CSRF)")}
			go server.Shutdown(context.Background()) //nolint:errcheck
			return
		}
		if errParam := q.Get("error"); errParam != "" {
			http.Error(w, "authorization denied: "+errParam, http.StatusUnauthorized)
			ch <- Result{Err: fmt.Errorf("OAuth2 authorization denied: %s", errParam)}
			go server.Shutdown(context.Background()) //nolint:errcheck
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			ch <- Result{Err: fmt.Errorf("OAuth2 callback: missing code")}
			go server.Shutdown(context.Background()) //nolint:errcheck
			return
		}
		fmt.Fprintln(w, "Authorization successful! You may close this tab and return to Herald.")
		ch <- Result{Code: code}
		go server.Shutdown(context.Background()) //nolint:errcheck
	})

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			select {
			case ch <- Result{Err: fmt.Errorf("OAuth2 callback server error: %w", err)}:
			default:
			}
		}
	}()

	// Shut down when context is cancelled (user abandoned the flow).
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
		select {
		case ch <- Result{Err: ctx.Err()}:
		default:
		}
	}()

	return url, ch, nil
}

// ExchangeCode exchanges an authorization code for OAuth2 tokens.
func ExchangeCode(ctx context.Context, code, redirectURI string) (*oauth2.Token, error) {
	clientID, clientSecret, err := credentials()
	if err != nil {
		return nil, err
	}
	cfg := NewGmailConfig(clientID, clientSecret, redirectURI)
	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}
	return token, nil
}

// AuthenticatedEmailFromToken extracts the Google account email returned by
// the OAuth token endpoint when the flow includes the OpenID Connect email
// scope. The token came from Google's TLS-protected exchange response; this
// helper decodes the claim so app code can compare it to configured source
// identity before persisting tokens.
func AuthenticatedEmailFromToken(token *oauth2.Token) (string, error) {
	if token == nil {
		return "", ErrAuthenticatedEmailUnavailable
	}
	idToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(idToken) == "" {
		return "", ErrAuthenticatedEmailUnavailable
	}
	return authenticatedEmailFromIDToken(idToken)
}

func authenticatedEmailFromIDToken(idToken string) (string, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", ErrAuthenticatedEmailUnavailable
	}
	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAuthenticatedEmailUnavailable, err)
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAuthenticatedEmailUnavailable, err)
	}
	email := strings.TrimSpace(claims.Email)
	if email == "" {
		return "", ErrAuthenticatedEmailUnavailable
	}
	if claims.EmailVerified != nil && !*claims.EmailVerified {
		return "", ErrAuthenticatedEmailUnverified
	}
	return email, nil
}

func decodeJWTPart(part string) ([]byte, error) {
	payload, err := base64.RawURLEncoding.DecodeString(part)
	if err == nil {
		return payload, nil
	}
	return base64.URLEncoding.DecodeString(part)
}

// RefreshIfNeeded checks whether the stored access token is expired (or close
// to expiry within 5 minutes) and uses the refresh token to obtain a new one.
// It updates cfg.Gmail fields in place and returns the current access token.
// Returns an error if the refresh token is missing or the refresh fails.
func RefreshIfNeeded(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg.Gmail.RefreshToken == "" {
		return "", fmt.Errorf("no OAuth2 refresh token stored; please re-authenticate")
	}

	// Determine whether the current access token is still valid.
	needsRefresh := true
	if cfg.Gmail.TokenExpiry != "" {
		expiry, err := time.Parse(time.RFC3339, cfg.Gmail.TokenExpiry)
		if err == nil {
			// Refresh only if the token expires within 5 minutes.
			needsRefresh = time.Until(expiry) < 5*time.Minute
		}
	}

	if !needsRefresh {
		return cfg.Gmail.AccessToken, nil
	}

	clientID, clientSecret, err := credentials()
	if err != nil {
		return "", err
	}
	oauthCfg := NewGmailConfig(clientID, clientSecret, "")
	existing := &oauth2.Token{
		RefreshToken: cfg.Gmail.RefreshToken,
		// Set Expiry to the zero value to force the token source to refresh.
		Expiry: time.Time{},
	}
	tokenSource := oauthCfg.TokenSource(ctx, existing)
	newToken, err := tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to refresh OAuth2 token: %w", err)
	}

	cfg.Gmail.AccessToken = newToken.AccessToken
	if !newToken.Expiry.IsZero() {
		cfg.Gmail.TokenExpiry = newToken.Expiry.UTC().Format(time.RFC3339)
	}
	if newToken.RefreshToken != "" {
		cfg.Gmail.RefreshToken = newToken.RefreshToken
	}

	return cfg.Gmail.AccessToken, nil
}

// RefreshGoogleConfigIfNeeded returns a usable Google access token for a
// source-level Google config. It refreshes expired calendar/source tokens using
// the same Herald Google OAuth credentials as Gmail OAuth, while preserving the
// existing static-token path used by deterministic provider tests.
func RefreshGoogleConfigIfNeeded(ctx context.Context, googleCfg *config.GoogleConfig) (string, error) {
	if googleCfg == nil {
		return "", fmt.Errorf("Google OAuth config is nil")
	}
	if !googleAccessTokenNeedsRefresh(googleCfg.AccessToken, googleCfg.TokenExpiry) {
		return strings.TrimSpace(googleCfg.AccessToken), nil
	}
	if strings.TrimSpace(googleCfg.RefreshToken) == "" {
		if strings.TrimSpace(googleCfg.AccessToken) != "" && strings.TrimSpace(googleCfg.TokenExpiry) == "" {
			return strings.TrimSpace(googleCfg.AccessToken), nil
		}
		return "", fmt.Errorf("no OAuth2 refresh token stored; please re-authenticate")
	}

	clientID, clientSecret, err := credentials()
	if err != nil {
		return "", err
	}
	tokenURL := strings.TrimSpace(googleCfg.TokenURL)
	if tokenURL == "" {
		tokenURL = google.Endpoint.TokenURL
	}

	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {strings.TrimSpace(googleCfg.RefreshToken)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to refresh OAuth2 token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read OAuth2 token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("failed to refresh OAuth2 token: %s", strings.TrimSpace(string(body)))
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("failed to decode OAuth2 token response: %w", err)
	}
	if payload.Error != "" {
		message := strings.TrimSpace(payload.ErrorDesc)
		if message == "" {
			message = payload.Error
		}
		return "", fmt.Errorf("failed to refresh OAuth2 token: %s", message)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("failed to refresh OAuth2 token: response did not include an access token")
	}

	googleCfg.AccessToken = strings.TrimSpace(payload.AccessToken)
	if strings.TrimSpace(payload.RefreshToken) != "" {
		googleCfg.RefreshToken = strings.TrimSpace(payload.RefreshToken)
	}
	if payload.ExpiresIn > 0 {
		googleCfg.TokenExpiry = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	return googleCfg.AccessToken, nil
}

func googleAccessTokenNeedsRefresh(accessToken, tokenExpiry string) bool {
	if strings.TrimSpace(accessToken) == "" {
		return true
	}
	tokenExpiry = strings.TrimSpace(tokenExpiry)
	if tokenExpiry == "" {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, tokenExpiry)
	if err != nil {
		return true
	}
	return time.Until(expiry) < 5*time.Minute
}
