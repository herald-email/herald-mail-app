package app

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mail-processor/internal/config"
	"mail-processor/internal/oauth"
)

// OAuthWaitModel is a tea.Model for the OAuth waiting screen.
type OAuthWaitModel struct {
	email       string
	authURL     string
	redirectURI string
	codeCh      <-chan oauth.Result
	cfg         *config.Config
	configPath  string
	spinner     spinner.Model
	browserOpen bool
	done        bool
	err         error
	width       int
	height      int
}

// OAuthDoneMsg is sent when OAuth completes successfully.
type OAuthDoneMsg struct {
	Config *config.Config
}

// OAuthErrorMsg is sent when OAuth fails.
type OAuthErrorMsg struct {
	Err error
}

// oauthCodeReceivedMsg is an internal message carrying the result from the OAuth callback.
type oauthCodeReceivedMsg struct{ result oauth.Result }

// NewOAuthWaitModel creates an OAuthWaitModel. It calls oauth.StartFlow to begin the
// authorization code flow, then returns the model ready to use.
func NewOAuthWaitModel(email string, cfg *config.Config, configPath string) (*OAuthWaitModel, error) {
	authURL, codeCh, err := oauth.StartFlow(context.Background(), email)
	if err != nil {
		return nil, err
	}

	// Extract redirect_uri from the auth URL.
	redirectURI := ""
	if parsed, err := url.Parse(authURL); err == nil {
		redirectURI = parsed.Query().Get("redirect_uri")
	}

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return &OAuthWaitModel{
		email:       email,
		authURL:     authURL,
		redirectURI: redirectURI,
		codeCh:      codeCh,
		cfg:         cfg,
		configPath:  configPath,
		spinner:     sp,
	}, nil
}

// listenForOAuthCode returns a tea.Cmd that waits for a single result from codeCh.
func listenForOAuthCode(codeCh <-chan oauth.Result) tea.Cmd {
	return func() tea.Msg {
		result := <-codeCh
		return oauthCodeReceivedMsg{result: result}
	}
}

// Init implements tea.Model.
func (m *OAuthWaitModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, listenForOAuthCode(m.codeCh))
}

// Update implements tea.Model.
func (m *OAuthWaitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.done {
		return m, nil
	}

	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && !m.browserOpen {
			if err := openBrowserFn(m.authorizeURL()); err == nil {
				m.browserOpen = true
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case oauthCodeReceivedMsg:
		m.done = true
		if msg.result.Err != nil {
			m.err = msg.result.Err
			return m, func() tea.Msg { return OAuthErrorMsg{Err: msg.result.Err} }
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		token, err := oauth.ExchangeCode(ctx, msg.result.Code, m.redirectURI)
		if err != nil {
			m.err = err
			return m, func() tea.Msg { return OAuthErrorMsg{Err: err} }
		}

		m.cfg.Gmail.Email = m.email
		m.cfg.Gmail.AccessToken = token.AccessToken
		m.cfg.Gmail.RefreshToken = token.RefreshToken
		if !token.Expiry.IsZero() {
			m.cfg.Gmail.TokenExpiry = token.Expiry.UTC().Format(time.RFC3339)
		}

		if err := m.cfg.Save(m.configPath); err != nil {
			m.err = err
			return m, func() tea.Msg { return OAuthErrorMsg{Err: err} }
		}

		cfg := m.cfg
		return m, func() tea.Msg { return OAuthDoneMsg{Config: cfg} }
	}

	return m, nil
}

// View implements tea.Model.
func (m *OAuthWaitModel) View() string {
	if m.width > 0 && m.width < minTermWidth {
		return renderMinSizeMessage(m.width, m.height)
	}
	if m.height > 0 && m.height < minTermHeight {
		return renderMinSizeMessage(m.width, m.height)
	}

	contentWidth := 88
	if m.width > 0 && m.width-8 < contentWidth {
		contentWidth = m.width - 8
	}
	if contentWidth < 30 {
		contentWidth = 30
	}

	copyURL := m.authorizeURL()
	urlLines := wrapString(copyURL, contentWidth)
	linkLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")).Render("[here]")
	authPrompt := "  Click: " + wizardHyperlink(linkLabel, copyURL) + " or copy this link to the browser:"

	browserLine := "  Press Enter to open browser automatically"
	if m.browserOpen {
		browserLine = "  Browser opened ✓"
	}

	content := strings.Join([]string{
		"",
		authPrompt,
		"",
		urlLines,
		"",
		"  " + m.spinner.View() + " Waiting for authorization…",
		"",
		browserLine,
		"",
	}, "\n")

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("Herald Setup — Gmail OAuth")

	rendered := lipgloss.JoinVertical(lipgloss.Left,
		title,
		content,
	)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, rendered)
	}
	return rendered
}

func (m *OAuthWaitModel) authorizeURL() string {
	if localURL := localAuthorizeURLFromRedirectURI(m.redirectURI); localURL != "" {
		return localURL
	}
	if parsed, err := url.Parse(m.authURL); err == nil {
		if localURL := localAuthorizeURLFromRedirectURI(parsed.Query().Get("redirect_uri")); localURL != "" {
			return localURL
		}
	}
	return m.authURL
}

func localAuthorizeURLFromRedirectURI(redirectURI string) string {
	if redirectURI == "" {
		return ""
	}
	parsed, err := url.Parse(redirectURI)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Path = "/authorize"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// wrapString wraps s to fit within maxWidth characters per line.
// It breaks on whitespace when possible, otherwise hard-wraps.
func wrapString(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	var result []string
	for len(s) > maxWidth {
		// Try to break at a space.
		breakAt := strings.LastIndex(s[:maxWidth], " ")
		if breakAt <= 0 {
			breakAt = maxWidth
		}
		result = append(result, "  "+s[:breakAt])
		s = strings.TrimLeft(s[breakAt:], " ")
	}
	if s != "" {
		result = append(result, "  "+s)
	}
	return strings.Join(result, "\n")
}
