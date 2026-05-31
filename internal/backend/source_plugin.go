package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/calendar"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

// SourceCapabilities describes what a configured provider adapter can do after
// Herald opens it. Queueing, cache policy, and UI intent rules remain above
// this boundary.
type SourceCapabilities struct {
	Mail             bool
	MailCollections  bool
	MailSync         bool
	MailSearch       bool
	MailMutations    bool
	Drafts           bool
	CacheBypassReads bool

	Calendar          bool
	CalendarEvents    bool
	CalendarMutations bool
	SyncTokens        bool

	Freshness ProviderFreshnessMetadata
}

// ProviderFreshnessMetadata names the provider fields a source returns so
// Herald-owned cache services can decide whether cached rows are still fresh.
type ProviderFreshnessMetadata struct {
	UIDValidity bool
	ModSeq      bool
	ETag        bool
	SyncToken   bool
	Revision    bool
}

// SourceDeps are Herald-owned dependencies a plugin may need when adapting a
// configured source. Provider plugins should not create shared app services.
type SourceDeps struct {
	ProfileConfig *config.Config
	ConfigPath    string
	Cache         *cache.Cache
	ProgressCh    chan models.ProgressInfo
}

// OpenedSource is the normalized Herald-facing source handle returned by a
// SourcePlugin. Exactly one of Mail or Calendar is expected for current sources.
type OpenedSource struct {
	SourceID   models.SourceID
	Account    models.AccountID
	SourceKind models.SourceKind
	Provider   string
	Name       string
	Caps       SourceCapabilities

	Mail             MailSource
	Calendar         calendar.Source
	CalendarMutation calendar.MutationSource
}

func (s *OpenedSource) ID() models.SourceID {
	if s == nil {
		return ""
	}
	return s.SourceID
}

func (s *OpenedSource) AccountID() models.AccountID {
	if s == nil {
		return ""
	}
	return s.Account
}

func (s *OpenedSource) Kind() models.SourceKind {
	if s == nil {
		return ""
	}
	return s.SourceKind
}

func (s *OpenedSource) DisplayName() string {
	if s == nil {
		return ""
	}
	return s.Name
}

func (s *OpenedSource) Capabilities() SourceCapabilities {
	if s == nil {
		return SourceCapabilities{}
	}
	return s.Caps
}

func (s *OpenedSource) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.Mail != nil {
		if err := s.Mail.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.Calendar != nil {
		if err := s.Calendar.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// SourcePlugin opens one normalized source config into a provider adapter.
type SourcePlugin interface {
	Kind() models.SourceKind
	Provider() string
	Open(context.Context, config.SourceConfig, SourceDeps) (*OpenedSource, error)
}

// SourceRegistry maps normalized source configs to provider plugins.
type SourceRegistry struct {
	plugins map[string]SourcePlugin
}

func NewSourceRegistry(plugins ...SourcePlugin) *SourceRegistry {
	r := &SourceRegistry{plugins: make(map[string]SourcePlugin)}
	for _, plugin := range plugins {
		_ = r.Register(plugin)
	}
	return r
}

func DefaultSourceRegistry() *SourceRegistry {
	registry := NewSourceRegistry(
		IMAPSourcePlugin{},
		GmailAPISourcePlugin{},
		GoogleCalendarSourcePlugin{},
		CalDAVSourcePlugin{},
	)
	// Compatibility alias for configs created by the first Gmail API core slice.
	registry.plugins[sourcePluginKey(models.SourceKindMail, "gmail_api")] = GmailAPISourcePlugin{}
	return registry
}

func (r *SourceRegistry) Register(plugin SourcePlugin) error {
	if plugin == nil {
		return fmt.Errorf("source plugin is nil")
	}
	kind := plugin.Kind()
	provider := strings.TrimSpace(plugin.Provider())
	if kind == "" || provider == "" {
		return fmt.Errorf("source plugin must report kind and provider")
	}
	if r.plugins == nil {
		r.plugins = make(map[string]SourcePlugin)
	}
	r.plugins[sourcePluginKey(kind, provider)] = plugin
	return nil
}

func (r *SourceRegistry) OpenConfiguredSources(ctx context.Context, cfg *config.Config, deps SourceDeps) ([]*OpenedSource, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	deps.ProfileConfig = firstProfileConfig(deps.ProfileConfig, cfg)
	sources := cfg.NormalizedSources()
	opened := make([]*OpenedSource, 0, len(sources))
	for _, source := range sources {
		handle, err := r.Open(ctx, source, deps)
		if err != nil {
			for _, existing := range opened {
				_ = existing.Close()
			}
			return nil, err
		}
		opened = append(opened, handle)
	}
	return opened, nil
}

func (r *SourceRegistry) Open(ctx context.Context, source config.SourceConfig, deps SourceDeps) (*OpenedSource, error) {
	source = normalizedSourceConfig(source)
	key := sourceRegistryKey(source)
	plugin := r.plugins[key]
	if plugin == nil {
		return nil, fmt.Errorf("unsupported source provider %q for kind %q", source.Provider, source.Kind)
	}
	return plugin.Open(ctx, source, deps)
}

func normalizedSourceConfig(source config.SourceConfig) config.SourceConfig {
	cfg := config.Config{Sources: []config.SourceConfig{source}}
	return cfg.NormalizedSources()[0]
}

func sourceRegistryKey(source config.SourceConfig) string {
	kind := models.SourceKind(strings.TrimSpace(source.Kind))
	provider := strings.ToLower(strings.TrimSpace(source.Provider))
	if kind == models.SourceKindMail {
		switch provider {
		case "gmail":
			if strings.TrimSpace(source.Google.Email) == "" &&
				strings.TrimSpace(source.Google.AccessToken) == "" &&
				strings.TrimSpace(source.Google.RefreshToken) == "" {
				provider = "imap"
			}
		case "", "imap", "protonmail", "fastmail", "outlook", "icloud":
			provider = "imap"
		}
	}
	return sourcePluginKey(kind, provider)
}

func sourcePluginKey(kind models.SourceKind, provider string) string {
	return string(kind) + ":" + strings.ToLower(strings.TrimSpace(provider))
}

func firstProfileConfig(candidate *config.Config, fallback *config.Config) *config.Config {
	if candidate != nil {
		return candidate
	}
	return fallback
}

func displayNameForSource(source config.SourceConfig) string {
	name := strings.TrimSpace(source.DisplayName)
	if name != "" {
		return name
	}
	if strings.TrimSpace(source.ID) != "" {
		return source.ID
	}
	if strings.TrimSpace(source.Kind) == string(models.SourceKindCalendar) {
		return string(models.DefaultCalendarSourceID)
	}
	return string(models.NormalizeSourceID("", models.DefaultMailSourceID))
}

func firstConfiguredMailSource(cfg *config.Config) (config.SourceConfig, bool) {
	if cfg == nil {
		return config.SourceConfig{}, false
	}
	for _, source := range cfg.NormalizedSources() {
		if strings.TrimSpace(source.Kind) == "" || source.Kind == string(models.SourceKindMail) {
			return source, true
		}
	}
	return config.SourceConfig{}, false
}

func calendarSourceHasProviderConfig(source config.SourceConfig) bool {
	switch strings.ToLower(strings.TrimSpace(source.Provider)) {
	case "google_calendar":
		return strings.TrimSpace(source.Google.AccessToken) != "" ||
			strings.TrimSpace(source.Google.RefreshToken) != "" ||
			strings.TrimSpace(source.Google.APIBaseURL) != ""
	case "caldav":
		return strings.TrimSpace(source.CalDAV.URL) != ""
	default:
		return false
	}
}

type IMAPSourcePlugin struct{}

func (IMAPSourcePlugin) Kind() models.SourceKind { return models.SourceKindMail }
func (IMAPSourcePlugin) Provider() string        { return "imap" }

func (IMAPSourcePlugin) Open(ctx context.Context, source config.SourceConfig, deps SourceDeps) (*OpenedSource, error) {
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	if deps.Cache == nil {
		return nil, fmt.Errorf("open %s: cache is required for mail source", source.ID)
	}
	profile := deps.ProfileConfig
	if profile == nil {
		profile = &config.Config{}
	}
	childCfg := profile
	imapConfigPath := deps.ConfigPath
	if len(profile.Sources) > 0 {
		childCfg = configForMailSource(profile, deps.ConfigPath, source)
		// A derived child config must not be persisted over the profile YAML.
		imapConfigPath = ""
	}
	progressCh := deps.ProgressCh
	if progressCh == nil {
		progressCh = make(chan models.ProgressInfo, 100)
	}
	mailSource := NewIMAPMailSource(childCfg, imapConfigPath, deps.Cache, progressCh)
	return &OpenedSource{
		SourceID:   models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultMailSourceID),
		Account:    models.NormalizeAccountID(models.AccountID(source.AccountID)),
		SourceKind: models.SourceKindMail,
		Provider:   source.Provider,
		Name:       displayNameForSource(source),
		Caps: SourceCapabilities{
			Mail:             true,
			MailCollections:  true,
			MailSync:         true,
			MailSearch:       true,
			MailMutations:    true,
			Drafts:           true,
			CacheBypassReads: true,
			Freshness: ProviderFreshnessMetadata{
				UIDValidity: true,
			},
		},
		Mail: mailSource,
	}, nil
}

type GoogleCalendarSourcePlugin struct{}

func (GoogleCalendarSourcePlugin) Kind() models.SourceKind { return models.SourceKindCalendar }
func (GoogleCalendarSourcePlugin) Provider() string        { return "google_calendar" }

func (GoogleCalendarSourcePlugin) Open(ctx context.Context, source config.SourceConfig, _ SourceDeps) (*OpenedSource, error) {
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	src, err := calendar.NewGoogleCalendarSource(source)
	if err != nil {
		return nil, err
	}
	return openedCalendarSource(source, src, true), nil
}

type CalDAVSourcePlugin struct{}

func (CalDAVSourcePlugin) Kind() models.SourceKind { return models.SourceKindCalendar }
func (CalDAVSourcePlugin) Provider() string        { return "caldav" }

func (CalDAVSourcePlugin) Open(ctx context.Context, source config.SourceConfig, _ SourceDeps) (*OpenedSource, error) {
	if err := mailSourceContextErr(ctx); err != nil {
		return nil, err
	}
	src, err := calendar.NewCalDAVSource(source)
	if err != nil {
		return nil, err
	}
	return openedCalendarSource(source, src, true), nil
}

func openedCalendarSource(source config.SourceConfig, src calendar.Source, syncTokens bool) *OpenedSource {
	var mutation calendar.MutationSource
	if mut, ok := src.(calendar.MutationSource); ok {
		mutation = mut
	}
	return &OpenedSource{
		SourceID:         models.NormalizeSourceID(models.SourceID(source.ID), models.DefaultCalendarSourceID),
		Account:          models.NormalizeAccountID(models.AccountID(source.AccountID)),
		SourceKind:       models.SourceKindCalendar,
		Provider:         source.Provider,
		Name:             displayNameForSource(source),
		Calendar:         src,
		CalendarMutation: mutation,
		Caps: SourceCapabilities{
			Calendar:          true,
			CalendarEvents:    true,
			CalendarMutations: mutation != nil,
			SyncTokens:        syncTokens,
			Freshness: ProviderFreshnessMetadata{
				ETag:      true,
				SyncToken: syncTokens,
				Revision:  true,
			},
		},
	}
}
