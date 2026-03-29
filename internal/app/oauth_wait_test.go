package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/config"
	"mail-processor/internal/oauth"
)

// TestNewOAuthWaitModel_ReturnsValidModel verifies that NewOAuthWaitModel returns a
// non-nil model and that the authURL is a valid Google authorization URL.
func TestNewOAuthWaitModel_ReturnsValidModel(t *testing.T) {
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

	if m.browserOpen {
		t.Fatal("browserOpen should be false before Enter")
	}

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updatedModel, _ := m.Update(enterMsg)
	updated := updatedModel.(*OAuthWaitModel)

	if !updated.browserOpen {
		t.Error("browserOpen should be true after pressing Enter")
	}
}

// TestOAuthWaitModel_ViewContainsURL verifies that View() renders the auth URL.
func TestOAuthWaitModel_ViewContainsURL(t *testing.T) {
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

	view := m.View()
	if !strings.Contains(view, "accounts.google.com") {
		t.Errorf("View() should contain the auth URL, got:\n%s", view)
	}
	if !strings.Contains(view, "Herald Setup") {
		t.Errorf("View() should contain the title, got:\n%s", view)
	}
}
