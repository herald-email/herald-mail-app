package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/oauth"
	"golang.org/x/oauth2"
)

// OAuthWaitModel is a tea.Model for the OAuth waiting screen.
type OAuthWaitModel struct {
	email                      string
	serviceLabel               string
	authURL                    string
	redirectURI                string
	codeCh                     <-chan oauth.Result
	cfg                        *config.Config
	configPath                 string
	spinner                    spinner.Model
	browserOpen                bool
	cancel                     context.CancelFunc
	timeout                    time.Duration
	done                       bool
	err                        error
	width                      int
	height                     int
	returnToMenu               bool
	reclaimOfflineCacheStorage bool
	validateAccount            bool
	validateCalendar           bool
	calendarSourceIDs          []models.SourceID
	sourceIDs                  []models.SourceID
}

// OAuthDoneMsg is sent when OAuth completes successfully.
type OAuthDoneMsg struct {
	Config                     *config.Config
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	ValidateAccount            bool
	ValidateCalendar           bool
	CalendarSourceIDs          []models.SourceID
}

// OAuthErrorMsg is sent when OAuth fails.
type OAuthErrorMsg struct {
	Err         error
	UserMessage string
}

// oauthCodeReceivedMsg is an internal message carrying the result from the OAuth callback.
type oauthCodeReceivedMsg struct{ result oauth.Result }

type oauthWaitTimeoutMsg struct{}

type OAuthWaitOptions struct {
	ReturnToMenu               bool
	ReclaimOfflineCacheStorage bool
	ValidateAccount            bool
	ValidateCalendar           bool
	CalendarSourceIDs          []models.SourceID
	SourceIDs                  []models.SourceID
	ServiceLabel               string
}

var (
	ErrOAuthCancelled = errors.New("OAuth authorization cancelled")
	ErrOAuthTimeout   = errors.New("OAuth authorization timed out")

	oauthWaitTimeout           = 5 * time.Minute
	exchangeOAuthCodeFn        = oauth.ExchangeCode
	authenticatedGoogleEmailFn = oauth.AuthenticatedEmailFromToken
)

// NewOAuthWaitModel creates an OAuthWaitModel. It calls oauth.StartFlow to begin the
// authorization code flow, then returns the model ready to use.
func NewOAuthWaitModel(email string, cfg *config.Config, configPath string) (*OAuthWaitModel, error) {
	return NewOAuthWaitModelWithOptions(email, cfg, configPath, OAuthWaitOptions{})
}

func NewOAuthWaitModelWithOptions(email string, cfg *config.Config, configPath string, opts OAuthWaitOptions) (*OAuthWaitModel, error) {
	ctx, cancel := context.WithCancel(context.Background())
	var sources []config.SourceConfig
	if cfg != nil {
		sources = cfg.NormalizedSources()
	}
	authURL, codeCh, err := oauth.StartFlowForSources(ctx, email, sources)
	if err != nil {
		cancel()
		return nil, err
	}

	// Extract redirect_uri from the auth URL.
	redirectURI := ""
	if parsed, err := url.Parse(authURL); err == nil {
		redirectURI = parsed.Query().Get("redirect_uri")
	}

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = defaultTheme.Setup.Spinner.Style()

	return &OAuthWaitModel{
		email:                      email,
		serviceLabel:               strings.TrimSpace(opts.ServiceLabel),
		authURL:                    authURL,
		redirectURI:                redirectURI,
		codeCh:                     codeCh,
		cfg:                        cfg,
		configPath:                 configPath,
		spinner:                    sp,
		cancel:                     cancel,
		timeout:                    oauthWaitTimeout,
		returnToMenu:               opts.ReturnToMenu,
		reclaimOfflineCacheStorage: opts.ReclaimOfflineCacheStorage,
		validateAccount:            opts.ValidateAccount,
		validateCalendar:           opts.ValidateCalendar,
		calendarSourceIDs:          append([]models.SourceID(nil), opts.CalendarSourceIDs...),
		sourceIDs:                  append([]models.SourceID(nil), opts.SourceIDs...),
	}, nil
}

// listenForOAuthCode returns a tea.Cmd that waits for a single result from codeCh.
func listenForOAuthCode(codeCh <-chan oauth.Result) tea.Cmd {
	return func() tea.Msg {
		result := <-codeCh
		return oauthCodeReceivedMsg{result: result}
	}
}

func waitForOAuthTimeout(timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		if timeout <= 0 {
			return nil
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		<-timer.C
		return oauthWaitTimeoutMsg{}
	}
}

// Init implements tea.Model.
func (m *OAuthWaitModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, listenForOAuthCode(m.codeCh), waitForOAuthTimeout(m.timeout))
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

	case tea.KeyPressMsg:
		if msg.Code == tea.KeyEscape || strings.EqualFold(msg.String(), "q") {
			m.done = true
			m.err = ErrOAuthCancelled
			if m.cancel != nil {
				m.cancel()
			}
			return m, func() tea.Msg {
				return OAuthErrorMsg{Err: ErrOAuthCancelled, UserMessage: oauthFailureMessage(ErrOAuthCancelled)}
			}
		}
		if msg.Code == tea.KeyEnter && !m.browserOpen {
			if err := openBrowserFn(m.authorizeURL()); err == nil {
				m.browserOpen = true
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case oauthWaitTimeoutMsg:
		m.done = true
		m.err = ErrOAuthTimeout
		if m.cancel != nil {
			m.cancel()
		}
		return m, func() tea.Msg {
			return OAuthErrorMsg{Err: ErrOAuthTimeout, UserMessage: oauthFailureMessage(ErrOAuthTimeout)}
		}

	case oauthCodeReceivedMsg:
		m.done = true
		if msg.result.Err != nil {
			m.err = msg.result.Err
			return m, func() tea.Msg {
				return OAuthErrorMsg{Err: msg.result.Err, UserMessage: oauthFailureMessage(msg.result.Err)}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		token, err := exchangeOAuthCodeFn(ctx, msg.result.Code, m.redirectURI)
		if err != nil {
			m.err = err
			return m, func() tea.Msg { return OAuthErrorMsg{Err: err, UserMessage: oauthFailureMessage(err)} }
		}

		authenticatedEmail, err := authenticatedGoogleEmailFn(token)
		if err != nil {
			m.err = err
			return m, func() tea.Msg { return OAuthErrorMsg{Err: err, UserMessage: oauthFailureMessage(err)} }
		}
		if err := validateGoogleOAuthAccount(m.cfg, m.email, authenticatedEmail, m.sourceIDs); err != nil {
			m.err = err
			return m, func() tea.Msg { return OAuthErrorMsg{Err: err, UserMessage: oauthFailureMessage(err)} }
		}
		if err := applyGoogleOAuthToken(m.cfg, authenticatedEmail, token, m.sourceIDs); err != nil {
			m.err = err
			return m, func() tea.Msg { return OAuthErrorMsg{Err: err, UserMessage: oauthFailureMessage(err)} }
		}

		cfg := m.cfg
		return m, func() tea.Msg {
			return OAuthDoneMsg{
				Config:                     cfg,
				ReturnToMenu:               m.returnToMenu,
				ReclaimOfflineCacheStorage: m.reclaimOfflineCacheStorage,
				ValidateAccount:            m.validateAccount,
				ValidateCalendar:           m.validateCalendar,
				CalendarSourceIDs:          append([]models.SourceID(nil), m.calendarSourceIDs...),
			}
		}
	}

	return m, nil
}

func validateGoogleOAuthAccount(cfg *config.Config, configuredEmail, authenticatedEmail string, sourceIDs []models.SourceID) error {
	authenticatedEmail = strings.TrimSpace(authenticatedEmail)
	if authenticatedEmail == "" {
		return fmt.Errorf("authenticated Google account email was empty")
	}
	for _, expected := range expectedGoogleOAuthEmails(cfg, configuredEmail, sourceIDs) {
		if !sameEmailAddress(expected, authenticatedEmail) {
			return fmt.Errorf("authenticated Google account %q does not match configured source email %q", authenticatedEmail, expected)
		}
	}
	return nil
}

func expectedGoogleOAuthEmails(cfg *config.Config, configuredEmail string, sourceIDs []models.SourceID) []string {
	seen := make(map[string]bool)
	var emails []string
	add := func(email string) {
		email = strings.TrimSpace(email)
		if email == "" {
			return
		}
		key := strings.ToLower(email)
		if seen[key] {
			return
		}
		seen[key] = true
		emails = append(emails, email)
	}
	add(configuredEmail)
	if cfg == nil {
		return emails
	}
	targets := googleOAuthTargetSourceIDs(sourceIDs)
	for _, source := range cfg.Sources {
		if !settingsSourceUsesGoogleOAuth(source) {
			continue
		}
		if len(targets) > 0 && !targets[settingsSourceIDForSource(source)] {
			continue
		}
		if len(targets) == 0 && strings.TrimSpace(configuredEmail) != "" && !sameEmailAddress(source.Google.Email, configuredEmail) {
			continue
		}
		add(source.Google.Email)
	}
	return emails
}

func applyGoogleOAuthToken(cfg *config.Config, email string, token *oauth2.Token, sourceIDs []models.SourceID) error {
	if cfg == nil || token == nil {
		return nil
	}
	email = strings.TrimSpace(email)
	if len(cfg.Sources) == 0 {
		applyTokenToLegacyGmail(cfg, email, token)
		return nil
	}

	targetIndexes := googleOAuthTokenTargetIndexes(cfg, email, sourceIDs)
	if len(targetIndexes) == 0 {
		return fmt.Errorf("Google OAuth target source was not found")
	}
	targetedFirstMail := firstMailSourceWasTargeted(cfg, targetIndexes)
	for _, i := range targetIndexes {
		if strings.TrimSpace(cfg.Sources[i].Google.Email) == "" {
			cfg.Sources[i].Google.Email = email
		}
		cfg.Sources[i].Google.AccessToken = token.AccessToken
		cfg.Sources[i].Google.RefreshToken = token.RefreshToken
		if !token.Expiry.IsZero() {
			cfg.Sources[i].Google.TokenExpiry = token.Expiry.UTC().Format(time.RFC3339)
		}
	}
	if targetedFirstMail {
		syncLegacyGmailFromFirstMailSource(cfg)
	}
	return nil
}

func applyTokenToLegacyGmail(cfg *config.Config, email string, token *oauth2.Token) {
	cfg.Gmail.Email = email
	cfg.Gmail.AccessToken = token.AccessToken
	cfg.Gmail.RefreshToken = token.RefreshToken
	if !token.Expiry.IsZero() {
		cfg.Gmail.TokenExpiry = token.Expiry.UTC().Format(time.RFC3339)
	}
}

func googleOAuthTokenTargetIndexes(cfg *config.Config, email string, sourceIDs []models.SourceID) []int {
	if cfg == nil {
		return nil
	}
	targets := make(map[models.SourceID]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		normalized := models.NormalizeSourceID(id, "")
		if normalized != "" {
			targets[normalized] = true
		}
	}
	var googleIndexes []int
	var indexes []int
	for i := range cfg.Sources {
		source := cfg.Sources[i]
		if !settingsSourceUsesGoogleOAuth(source) {
			continue
		}
		googleIndexes = append(googleIndexes, i)
		if len(targets) > 0 {
			if targets[settingsSourceIDForSource(source)] {
				indexes = append(indexes, i)
			}
			continue
		}
		if strings.TrimSpace(email) != "" && sameEmailAddress(source.Google.Email, email) {
			indexes = append(indexes, i)
		}
	}
	if len(indexes) > 0 {
		return indexes
	}
	if len(targets) == 0 && len(googleIndexes) == 1 {
		return googleIndexes
	}
	return nil
}

func googleOAuthTargetSourceIDs(sourceIDs []models.SourceID) map[models.SourceID]bool {
	targets := make(map[models.SourceID]bool, len(sourceIDs))
	for _, id := range sourceIDs {
		normalized := models.NormalizeSourceID(id, "")
		if normalized != "" {
			targets[normalized] = true
		}
	}
	return targets
}

func firstMailSourceWasTargeted(cfg *config.Config, targetIndexes []int) bool {
	if cfg == nil {
		return false
	}
	targets := make(map[int]bool, len(targetIndexes))
	for _, i := range targetIndexes {
		targets[i] = true
	}
	for i, source := range cfg.Sources {
		if strings.TrimSpace(source.Kind) != "" && strings.TrimSpace(source.Kind) != string(models.SourceKindMail) {
			continue
		}
		return targets[i]
	}
	return false
}

func syncLegacyGmailFromFirstMailSource(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for _, source := range cfg.Sources {
		if strings.TrimSpace(source.Kind) != "" && strings.TrimSpace(source.Kind) != string(models.SourceKindMail) {
			continue
		}
		cfg.Gmail.Email = source.Google.Email
		cfg.Gmail.AccessToken = source.Google.AccessToken
		cfg.Gmail.RefreshToken = source.Google.RefreshToken
		cfg.Gmail.TokenExpiry = source.Google.TokenExpiry
		return
	}
}

func sameEmailAddress(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// View implements tea.Model.
func (m *OAuthWaitModel) View() tea.View {
	if m.width > 0 && m.width < minTermWidth {
		return newHeraldView(renderMinSizeMessage(m.width, m.height))
	}
	if m.height > 0 && m.height < minTermHeight {
		return newHeraldView(renderMinSizeMessage(m.width, m.height))
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
	linkLabel := defaultTheme.Setup.Link.Style().Render("[here]")
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
		"  " + m.spinner.View() + " Waiting for Google authorization...",
		"",
		browserLine,
		"  Esc/q cancels without saving settings",
		"",
	}, "\n")

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(defaultTheme.Setup.Title.ForegroundColor()).
		Render("Herald Setup — " + m.oauthServiceLabel())

	rendered := lipgloss.JoinVertical(lipgloss.Left,
		title,
		content,
	)

	if m.width > 0 && m.height > 0 {
		return newHeraldView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, rendered))
	}
	return newHeraldView(rendered)
}

func (m *OAuthWaitModel) oauthServiceLabel() string {
	if m != nil && strings.TrimSpace(m.serviceLabel) != "" {
		return strings.TrimSpace(m.serviceLabel)
	}
	return "Gmail OAuth"
}

func oauthFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrOAuthCancelled) {
		return "Authorization cancelled; settings were not saved."
	}
	if errors.Is(err, ErrOAuthTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return "No authorization received; settings were not saved. Complete the Google consent screen in your browser, or cancel and start OAuth again."
	}
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "authorization denied") || strings.Contains(strings.ToLower(msg), "access_denied") {
		return "Authorization cancelled; settings were not saved."
	}
	return "OAuth failed: " + msg + ". Settings were not saved."
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
