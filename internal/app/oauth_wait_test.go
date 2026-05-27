package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/oauth"
	"golang.org/x/oauth2"
)

// TestNewOAuthWaitModel_ReturnsValidModel verifies that NewOAuthWaitModel returns a
// non-nil model and that the authURL is a valid Google authorization URL.
func TestNewOAuthWaitModel_ReturnsValidModel(t *testing.T) {
	t.Setenv("HERALD_GOOGLE_CLIENT_ID", "test-client-id.apps.googleusercontent.com")
	t.Setenv("HERALD_GOOGLE_CLIENT_SECRET", "test-client-secret")

	cfg := &config.Config{}
	cfg.Gmail.Email = "test@gmail.com"

	m, err := NewOAuthWaitModel("test@gmail.com", cfg, "/tmp/test-herald-conf.yaml")
	if err != nil {
		t.Fatalf("NewOAuthWaitModel returned unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("NewOAuthWaitModel returned nil model")
	}
	if m.authURL == "" {
		t.Fatal("authURL is empty")
	}
	if !strings.Contains(m.authURL, "accounts.google.com") {
		t.Errorf("authURL does not look like a Google auth URL: %s", m.authURL)
	}
	if m.redirectURI == "" {
		t.Error("redirectURI was not extracted from authURL")
	}
	if !strings.Contains(m.redirectURI, "localhost") {
		t.Errorf("redirectURI does not point to localhost: %s", m.redirectURI)
	}
}

// TestOAuthWaitModel_CodeErrorSetsErrorField verifies that when oauthCodeReceivedMsg
// carries an error, the model's err field is set and an OAuthErrorMsg is returned.
func TestOAuthWaitModel_CodeErrorSetsErrorField(t *testing.T) {
	cfg := &config.Config{}
	// Build a minimal model without calling StartFlow to avoid network activity.
	codeCh := make(chan oauth.Result, 1)
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     "https://accounts.google.com/o/oauth2/auth?test=1",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      codeCh,
		cfg:         cfg,
		configPath:  "/tmp/test-herald-conf.yaml",
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	expectedErr := errors.New("authorization denied")
	errMsg := oauthCodeReceivedMsg{result: oauth.Result{Err: expectedErr}}

	updatedModel, cmd := m.Update(errMsg)
	if updatedModel == nil {
		t.Fatal("Update returned nil model")
	}
	updated := updatedModel.(*OAuthWaitModel)
	if updated.err == nil {
		t.Fatal("expected err to be set on model after error result, got nil")
	}
	if !errors.Is(updated.err, expectedErr) {
		t.Errorf("expected err %v, got %v", expectedErr, updated.err)
	}
	if cmd == nil {
		t.Fatal("expected a Cmd to be returned (OAuthErrorMsg), got nil")
	}

	// Execute the cmd and verify it produces OAuthErrorMsg.
	resultMsg := cmd()
	errResult, ok := resultMsg.(OAuthErrorMsg)
	if !ok {
		t.Fatalf("expected OAuthErrorMsg, got %T", resultMsg)
	}
	if !errors.Is(errResult.Err, expectedErr) {
		t.Errorf("OAuthErrorMsg.Err = %v, want %v", errResult.Err, expectedErr)
	}
}

// TestOAuthWaitModel_EnterOpensBrowser verifies that pressing Enter sets browserOpen.
func TestOAuthWaitModel_EnterOpensBrowser(t *testing.T) {
	var openedURL string
	originalOpenBrowserFn := openBrowserFn
	openBrowserFn = func(rawURL string) error {
		openedURL = rawURL
		return nil
	}
	t.Cleanup(func() {
		openBrowserFn = originalOpenBrowserFn
	})

	cfg := &config.Config{}
	codeCh := make(chan oauth.Result, 1)
	authURL := "https://accounts.google.com/o/oauth2/auth?redirect_uri=http%3A%2F%2Flocalhost%3A12345%2Fcallback&test=1"
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     authURL,
		redirectURI: "http://localhost:12345/callback",
		codeCh:      codeCh,
		cfg:         cfg,
		configPath:  "/tmp/test-herald-conf.yaml",
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	if m.browserOpen {
		t.Fatal("browserOpen should be false before Enter")
	}

	enterMsg := tea.KeyPressMsg{Code: tea.KeyEnter}
	updatedModel, _ := m.Update(enterMsg)
	updated := updatedModel.(*OAuthWaitModel)

	if !updated.browserOpen {
		t.Error("browserOpen should be true after pressing Enter")
	}
	if openedURL != "http://localhost:12345/authorize" {
		t.Errorf("openBrowserFn called with %q, want short local authorize URL", openedURL)
	}
}

func TestOAuthWaitModel_EnterKeepsBrowserOpenFalseWhenOpenFails(t *testing.T) {
	originalOpenBrowserFn := openBrowserFn
	openBrowserFn = func(rawURL string) error {
		return fmt.Errorf("refusing to open %s", rawURL)
	}
	t.Cleanup(func() {
		openBrowserFn = originalOpenBrowserFn
	})

	cfg := &config.Config{}
	codeCh := make(chan oauth.Result, 1)
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     "https://accounts.google.com/o/oauth2/auth?test=1",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      codeCh,
		cfg:         cfg,
		configPath:  "/tmp/test-herald-conf.yaml",
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	enterMsg := tea.KeyPressMsg{Code: tea.KeyEnter}
	updatedModel, _ := m.Update(enterMsg)
	updated := updatedModel.(*OAuthWaitModel)

	if updated.browserOpen {
		t.Error("browserOpen should remain false when opening the browser fails")
	}
}

func TestOAuthWaitModel_EscapeCancelsWithoutSaving(t *testing.T) {
	cfg := &config.Config{}
	path := filepath.Join(t.TempDir(), "conf.yaml")
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     "https://accounts.google.com/o/oauth2/auth?test=1",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      make(chan oauth.Result, 1),
		cfg:         cfg,
		configPath:  path,
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		cancel:      func() {},
	}

	updatedModel, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("expected cancel command")
	}
	updated := updatedModel.(*OAuthWaitModel)
	if updated.err == nil || !strings.Contains(updated.err.Error(), "cancelled") {
		t.Fatalf("expected cancellation error, got %v", updated.err)
	}
	msg, ok := cmd().(OAuthErrorMsg)
	if !ok {
		t.Fatalf("expected OAuthErrorMsg, got %T", cmd())
	}
	if !strings.Contains(msg.UserMessage, "not saved") {
		t.Fatalf("expected not-saved user message, got %q", msg.UserMessage)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("OAuth cancel must not save config, stat err=%v", err)
	}
}

func TestOAuthWaitModel_TimeoutExplainsGoogleContinuePath(t *testing.T) {
	cfg := &config.Config{}
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     "https://accounts.google.com/o/oauth2/auth?test=1",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      make(chan oauth.Result, 1),
		cfg:         cfg,
		configPath:  filepath.Join(t.TempDir(), "conf.yaml"),
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		cancel:      func() {},
	}

	_, cmd := m.Update(oauthWaitTimeoutMsg{})
	if cmd == nil {
		t.Fatal("expected timeout command")
	}
	msg, ok := cmd().(OAuthErrorMsg)
	if !ok {
		t.Fatalf("expected OAuthErrorMsg, got %T", cmd())
	}
	if !strings.Contains(msg.UserMessage, "Continue") || !strings.Contains(msg.UserMessage, "Back to safety") {
		t.Fatalf("timeout message should explain Google test-app buttons, got %q", msg.UserMessage)
	}
}

func TestOAuthWaitModel_CodeSuccessDoesNotSaveBeforeValidation(t *testing.T) {
	originalExchange := exchangeOAuthCodeFn
	exchangeOAuthCodeFn = func(_ context.Context, _, _ string) (*oauth2.Token, error) {
		return &oauth2.Token{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			Expiry:       time.Now().Add(time.Hour),
		}, nil
	}
	t.Cleanup(func() { exchangeOAuthCodeFn = originalExchange })

	cfg := &config.Config{}
	path := filepath.Join(t.TempDir(), "conf.yaml")
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     "https://accounts.google.com/o/oauth2/auth?test=1",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      make(chan oauth.Result, 1),
		cfg:         cfg,
		configPath:  path,
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	_, cmd := m.Update(oauthCodeReceivedMsg{result: oauth.Result{Code: "code"}})
	if cmd == nil {
		t.Fatal("expected OAuthDoneMsg command")
	}
	if _, ok := cmd().(OAuthDoneMsg); !ok {
		t.Fatalf("expected OAuthDoneMsg, got %T", cmd())
	}
	if cfg.Gmail.RefreshToken != "refresh-token" {
		t.Fatalf("expected token on candidate config, got %q", cfg.Gmail.RefreshToken)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("OAuth success must wait for parent validation before saving config, stat err=%v", err)
	}
}

func TestOAuthWaitModel_CodeSuccessAppliesTokensToTargetGoogleSources(t *testing.T) {
	originalExchange := exchangeOAuthCodeFn
	exchangeOAuthCodeFn = func(_ context.Context, _, _ string) (*oauth2.Token, error) {
		return &oauth2.Token{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			Expiry:       time.Now().Add(time.Hour),
		}, nil
	}
	t.Cleanup(func() { exchangeOAuthCodeFn = originalExchange })

	cfg := &config.Config{Sources: []config.SourceConfig{
		{
			ID:        "work-calendar",
			Kind:      "calendar",
			Provider:  "google_calendar",
			AccountID: "work",
			Google:    config.GoogleConfig{Email: "work@example.test"},
		},
		{
			ID:        "personal-calendar",
			Kind:      "calendar",
			Provider:  "google_calendar",
			AccountID: "personal",
			Google:    config.GoogleConfig{Email: "me@example.test"},
		},
	}}
	m := &OAuthWaitModel{
		email:       "work@example.test",
		authURL:     "https://accounts.google.com/o/oauth2/auth?test=1",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      make(chan oauth.Result, 1),
		cfg:         cfg,
		sourceIDs:   []models.SourceID{"work-calendar"},
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	_, cmd := m.Update(oauthCodeReceivedMsg{result: oauth.Result{Code: "code"}})
	if cmd == nil {
		t.Fatal("expected OAuthDoneMsg command")
	}
	if _, ok := cmd().(OAuthDoneMsg); !ok {
		t.Fatalf("expected OAuthDoneMsg, got %T", cmd())
	}

	if cfg.Sources[0].Google.RefreshToken != "refresh-token" || cfg.Sources[0].Google.AccessToken != "access-token" {
		t.Fatalf("target calendar source tokens = %#v, want OAuth tokens", cfg.Sources[0].Google)
	}
	if cfg.Sources[1].Google.RefreshToken != "" || cfg.Sources[1].Google.AccessToken != "" {
		t.Fatalf("untargeted calendar source was overwritten: %#v", cfg.Sources[1].Google)
	}
}

// TestOAuthWaitModel_ViewContainsCopyURL verifies that View() renders a short copy URL.
func TestOAuthWaitModel_ViewContainsCopyURL(t *testing.T) {
	cfg := &config.Config{}
	codeCh := make(chan oauth.Result, 1)
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     "https://accounts.google.com/o/oauth2/auth?redirect_uri=http%3A%2F%2Flocalhost%3A12345%2Fcallback&client_id=test",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      codeCh,
		cfg:         cfg,
		configPath:  "/tmp/test-herald-conf.yaml",
		width:       80,
		height:      24,
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	view := m.View().Content
	if !strings.Contains(ansi.Strip(view), "http://localhost:12345/authorize") {
		t.Errorf("View() should contain the short copy URL, got:\n%s", view)
	}
	if !strings.Contains(view, "Herald Setup") {
		t.Errorf("View() should contain the title, got:\n%s", view)
	}
}

func TestOAuthWaitModel_ViewOffersClickableHereAndShortCopyURLWithoutBox(t *testing.T) {
	cfg := &config.Config{}
	codeCh := make(chan oauth.Result, 1)
	authURL := "https://accounts.google.com/o/oauth2/auth?redirect_uri=http%3A%2F%2Flocalhost%3A12345%2Fcallback&client_id=test&scope=mail"
	copyURL := "http://localhost:12345/authorize"
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     authURL,
		redirectURI: "http://localhost:12345/callback",
		codeCh:      codeCh,
		cfg:         cfg,
		configPath:  "/tmp/test-herald-conf.yaml",
		width:       120,
		height:      32,
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	view := m.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "Click: [here] or copy this link to the browser:") {
		t.Fatalf("expected clickable/copyable auth prompt, got:\n%s", plain)
	}
	if !strings.Contains(plain, copyURL) {
		t.Fatalf("expected short local authorize URL to remain visible for copying, got:\n%s", plain)
	}
	if strings.Contains(plain, "accounts.google.com") {
		t.Fatalf("expected visible copy fallback to hide the long Google auth URL, got:\n%s", plain)
	}
	if !strings.Contains(view, "\x1b]8;;"+copyURL+"\x1b\\") {
		t.Fatalf("expected [here] to be an OSC 8 hyperlink for the short local authorize URL, got raw view:\n%q", view)
	}
	for _, border := range []string{"╭", "╮", "╰", "╯"} {
		if strings.Contains(plain, border) {
			t.Fatalf("expected OAuth wait prompt to avoid boxed link chrome, found %q in:\n%s", border, plain)
		}
	}
}

func TestOAuthWaitModel_ViewUsesMinimumSizeGuardWhenTooNarrow(t *testing.T) {
	cfg := &config.Config{}
	codeCh := make(chan oauth.Result, 1)
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     "https://accounts.google.com/o/oauth2/auth?client_id=test",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      codeCh,
		cfg:         cfg,
		configPath:  "/tmp/test-herald-conf.yaml",
		width:       50,
		height:      15,
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "Terminal too narrow") {
		t.Fatalf("expected OAuth wait view to use minimum-size guard, got:\n%s", plain)
	}
	if strings.Contains(plain, "accounts.google.com") {
		t.Fatalf("expected minimum-size guard to replace the clipped auth prompt, got:\n%s", plain)
	}
}

func TestOAuthWaitModel_ViewShowsGmailOAuthTitle(t *testing.T) {
	cfg := &config.Config{}
	codeCh := make(chan oauth.Result, 1)
	m := &OAuthWaitModel{
		email:       "test@gmail.com",
		authURL:     "https://accounts.google.com/o/oauth2/auth?client_id=test",
		redirectURI: "http://localhost:12345/callback",
		codeCh:      codeCh,
		cfg:         cfg,
		configPath:  "/tmp/test-herald-conf.yaml",
		width:       80,
		height:      24,
		spinner:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Gmail OAuth") {
		t.Fatalf("expected Gmail OAuth title in OAuth wait view, got:\n%s", view)
	}
	if strings.Contains(view, "Experimental") {
		t.Fatalf("expected OAuth wait view to avoid experimental marker, got:\n%s", view)
	}
}
