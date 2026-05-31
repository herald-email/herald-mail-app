package backend

import (
	"context"
	"strings"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestDefaultSourceRegistryOpensConfiguredSourcesAndReportsCapabilities(t *testing.T) {
	cfg := &config.Config{Sources: []config.SourceConfig{
		{
			ID:          "work-mail",
			Kind:        string(models.SourceKindMail),
			Provider:    "gmail",
			DisplayName: "Work Mail",
			AccountID:   "work",
			Credentials: config.CredentialsConfig{Username: "work@example.test", Password: "secret"},
			IMAP:        config.ServerConfig{Host: "imap.example.test", Port: 993},
			SMTP:        config.ServerConfig{Host: "smtp.example.test", Port: 587},
		},
		{
			ID:          "work-calendar",
			Kind:        string(models.SourceKindCalendar),
			Provider:    "google_calendar",
			DisplayName: "Work Calendar",
			AccountID:   "work",
			Google:      config.GoogleConfig{APIBaseURL: "https://calendar.example.test", Email: "work@example.test"},
		},
	}}

	opened, err := DefaultSourceRegistry().OpenConfiguredSources(context.Background(), cfg, SourceDeps{
		ProfileConfig: cfg,
		Cache:         newMemoryCache(t),
	})
	if err != nil {
		t.Fatalf("OpenConfiguredSources: %v", err)
	}
	if len(opened) != 2 {
		t.Fatalf("opened sources = %d, want 2", len(opened))
	}

	mail := opened[0]
	if mail.ID() != "work-mail" || mail.AccountID() != "work" || mail.Kind() != models.SourceKindMail {
		t.Fatalf("mail identity = %#v", mail)
	}
	if mail.Provider != "gmail" || mail.DisplayName() != "Work Mail" {
		t.Fatalf("mail provider/display = %q/%q", mail.Provider, mail.DisplayName())
	}
	if mail.Mail == nil {
		t.Fatal("mail source was not opened")
	}
	mailCaps := mail.Capabilities()
	if !mailCaps.Mail || !mailCaps.MailSync || !mailCaps.MailMutations || !mailCaps.Drafts || !mailCaps.CacheBypassReads {
		t.Fatalf("mail capabilities = %#v", mailCaps)
	}
	if !mailCaps.Freshness.UIDValidity {
		t.Fatalf("mail freshness metadata = %#v, want UIDVALIDITY support", mailCaps.Freshness)
	}
	if mailCaps.Calendar || mailCaps.CalendarMutations || mailCaps.SyncTokens {
		t.Fatalf("mail reported calendar capabilities: %#v", mailCaps)
	}

	cal := opened[1]
	if cal.ID() != "work-calendar" || cal.AccountID() != "work" || cal.Kind() != models.SourceKindCalendar {
		t.Fatalf("calendar identity = %#v", cal)
	}
	if cal.Provider != "google_calendar" || cal.Calendar == nil || cal.CalendarMutation == nil {
		t.Fatalf("calendar source = provider %q calendar=%T mutation=%T", cal.Provider, cal.Calendar, cal.CalendarMutation)
	}
	calCaps := cal.Capabilities()
	if !calCaps.Calendar || !calCaps.CalendarEvents || !calCaps.CalendarMutations || !calCaps.SyncTokens {
		t.Fatalf("calendar capabilities = %#v", calCaps)
	}
	if !calCaps.Freshness.ETag || !calCaps.Freshness.SyncToken || !calCaps.Freshness.Revision {
		t.Fatalf("calendar freshness metadata = %#v, want ETag, sync token, and revision support", calCaps.Freshness)
	}
	if calCaps.Mail || calCaps.MailMutations || calCaps.Drafts {
		t.Fatalf("calendar reported mail capabilities: %#v", calCaps)
	}
}

func TestSourceRegistryNormalizesLegacyConfigToDefaultMailSource(t *testing.T) {
	cfg := &config.Config{
		Credentials: config.CredentialsConfig{Username: "legacy@example.test", Password: "secret"},
		Server:      config.ServerConfig{Host: "127.0.0.1", Port: 1143},
		SMTP:        config.ServerConfig{Host: "127.0.0.1", Port: 1025},
	}

	opened, err := DefaultSourceRegistry().OpenConfiguredSources(context.Background(), cfg, SourceDeps{
		ProfileConfig: cfg,
		Cache:         newMemoryCache(t),
	})
	if err != nil {
		t.Fatalf("OpenConfiguredSources: %v", err)
	}
	if len(opened) != 1 {
		t.Fatalf("opened sources = %d, want 1", len(opened))
	}
	got := opened[0]
	if got.ID() != models.DefaultMailSourceID || got.AccountID() != models.DefaultAccountID || got.Kind() != models.SourceKindMail {
		t.Fatalf("legacy identity = %#v", got)
	}
	if got.Mail == nil || !got.Capabilities().Mail {
		t.Fatalf("legacy mail source/capabilities = source %T caps %#v", got.Mail, got.Capabilities())
	}
}

func TestSourceRegistryRoutesGmailOAuthProviderToGmailAPI(t *testing.T) {
	cfg := &config.Config{Sources: []config.SourceConfig{{
		ID:        "oauth-gmail",
		Kind:      string(models.SourceKindMail),
		Provider:  "gmail",
		AccountID: "personal",
		Google:    config.GoogleConfig{Email: "me@gmail.example", AccessToken: "access-token"},
		IMAP:      config.ServerConfig{Host: "imap.gmail.com", Port: 993},
		SMTP:      config.ServerConfig{Host: "smtp.gmail.com", Port: 587},
	}}}

	opened, err := DefaultSourceRegistry().OpenConfiguredSources(context.Background(), cfg, SourceDeps{
		ProfileConfig: cfg,
		Cache:         newMemoryCache(t),
	})
	if err != nil {
		t.Fatalf("OpenConfiguredSources: %v", err)
	}
	got := opened[0]
	if got.Provider != "gmail" || got.Mail == nil {
		t.Fatalf("opened source = %#v, want Gmail API-backed gmail provider", got)
	}
	caps := got.Capabilities()
	if !caps.Mail || !caps.MailSync || !caps.MailMutations || !caps.CacheBypassReads || caps.Drafts {
		t.Fatalf("gmail oauth capabilities = %#v, want Gmail API capabilities", caps)
	}
}

func TestSourceRegistryKeepsGmailAppPasswordOnIMAP(t *testing.T) {
	cfg := &config.Config{Sources: []config.SourceConfig{{
		ID:          "app-password-gmail",
		Kind:        string(models.SourceKindMail),
		Provider:    "gmail",
		Credentials: config.CredentialsConfig{Username: "me@gmail.example", Password: "app-password"},
		IMAP:        config.ServerConfig{Host: "imap.gmail.com", Port: 993},
		SMTP:        config.ServerConfig{Host: "smtp.gmail.com", Port: 587},
	}}}

	opened, err := DefaultSourceRegistry().OpenConfiguredSources(context.Background(), cfg, SourceDeps{
		ProfileConfig: cfg,
		Cache:         newMemoryCache(t),
	})
	if err != nil {
		t.Fatalf("OpenConfiguredSources: %v", err)
	}
	caps := opened[0].Capabilities()
	if !caps.Drafts || !caps.Freshness.UIDValidity {
		t.Fatalf("gmail app password capabilities = %#v, want IMAP capabilities", caps)
	}
}

func TestSourceRegistryRejectsUnsupportedProvider(t *testing.T) {
	cfg := &config.Config{Sources: []config.SourceConfig{
		{ID: "icloud-calendar", Kind: string(models.SourceKindCalendar), Provider: "apple_calendar"},
	}}

	_, err := DefaultSourceRegistry().OpenConfiguredSources(context.Background(), cfg, SourceDeps{
		ProfileConfig: cfg,
		Cache:         newMemoryCache(t),
	})
	if err == nil {
		t.Fatal("OpenConfiguredSources succeeded for unsupported provider")
	}
	if !strings.Contains(err.Error(), "unsupported source provider") {
		t.Fatalf("error = %v", err)
	}
}
