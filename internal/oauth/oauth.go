// Package oauth provides Gmail OAuth2 authentication support.
// It implements the authorization code flow with a local HTTP callback server,
// token exchange, and automatic token refresh.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/herald-email/herald-mail-app/internal/config"
)

// Scopes required for Gmail IMAP access.
var Scopes = []string{
	"https://mail.google.com/",
}

// clientID and clientSecret are the OAuth2 application credentials.
// Release builds set defaultClientID and defaultClientSecret with -ldflags -X.
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
		return "", "", fmt.Errorf("Google OAuth credentials are not configured; set HERALD_GOOGLE_CLIENT_ID and HERALD_GOOGLE_CLIENT_SECRET or build with OAuth defaults")
	}
	return id, secret, nil
}

// NewGmailConfig returns an OAuth2 config for Gmail using the provided
// client ID and secret. The redirect URL should be a localhost callback
// for the desktop auth flow.
func NewGmailConfig(clientIDVal, clientSecretVal, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientIDVal,
		ClientSecret: clientSecretVal,
		RedirectURL:  redirectURL,
		Scopes:       Scopes,
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

	cfg := NewGmailConfig(clientID, clientSecret, redirectURI)

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
