package accountcheck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/cache"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/imap"
	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
)

const defaultSurfaceTimeout = 20 * time.Second

// Check reports the validation result for one mail surface.
type Check struct {
	Surface string
	Err     error
}

// OK returns true when the surface validated successfully.
func (c Check) OK() bool {
	return c.Err == nil
}

// Result reports setup-time account validation for both required mail surfaces.
type Result struct {
	IMAP Check
	SMTP Check
}

// OK returns true when all required surfaces validated successfully.
func (r Result) OK() bool {
	return r.IMAP.OK() && r.SMTP.OK()
}

// Err returns a combined error naming every failed surface.
func (r Result) Err() error {
	if r.OK() {
		return nil
	}
	var failed []string
	if !r.IMAP.OK() {
		failed = append(failed, "IMAP")
	}
	if !r.SMTP.OK() {
		failed = append(failed, "SMTP")
	}
	return fmt.Errorf("account validation failed: %s", strings.Join(failed, " and "))
}

// UserMessage returns bounded copy suitable for the setup wizard and settings UI.
func (r Result) UserMessage(logPath, configPath string) string {
	if r.OK() {
		return "Account validated. IMAP and SMTP are ready."
	}
	var parts []string
	if !r.IMAP.OK() {
		parts = append(parts, "IMAP: "+sanitizeError(r.IMAP.Err))
	}
	if !r.SMTP.OK() {
		parts = append(parts, "SMTP: "+sanitizeError(r.SMTP.Err))
	}
	if configPath != "" {
		parts = append(parts, "Settings were not saved to "+configPath+".")
	} else {
		parts = append(parts, "Settings were not saved.")
	}
	if logPath != "" {
		parts = append(parts, "Debug log: "+logPath)
	}
	return strings.Join(parts, " ")
}

// Validate checks that the candidate account config can authenticate to IMAP
// and SMTP. It does not sync mail and does not send a message.
func Validate(ctx context.Context, cfg *config.Config, configPath string) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	result := Result{
		IMAP: Check{Surface: "IMAP"},
		SMTP: Check{Surface: "SMTP"},
	}
	if cfg == nil {
		err := fmt.Errorf("account config not configured")
		result.IMAP.Err = err
		result.SMTP.Err = err
		return result
	}
	if gmailAPISource, ok := firstGmailAPISource(cfg); ok {
		result.IMAP.Surface = "Gmail API"
		result.SMTP.Surface = "Gmail API send"
		err := runWithTimeout(ctx, func(ctx context.Context) error {
			return checkGmailAPI(ctx, cfg, gmailAPISource)
		})
		result.IMAP.Err = err
		result.SMTP.Err = err
		if err != nil {
			logger.Error("Account validation Gmail API failed: %v", err)
		}
		return result
	}
	result.IMAP.Err = runWithTimeout(ctx, func(ctx context.Context) error {
		return checkIMAP(ctx, cfg)
	})
	if result.IMAP.Err != nil {
		logger.Error("Account validation IMAP failed: %v", result.IMAP.Err)
	}
	result.SMTP.Err = runWithTimeout(ctx, func(ctx context.Context) error {
		return appsmtp.New(cfg).Check(ctx)
	})
	if result.SMTP.Err != nil {
		logger.Error("Account validation SMTP failed: %v", result.SMTP.Err)
	}
	return result
}

func firstGmailAPISource(cfg *config.Config) (config.SourceConfig, bool) {
	if cfg == nil {
		return config.SourceConfig{}, false
	}
	for _, source := range cfg.NormalizedSources() {
		if strings.TrimSpace(source.Kind) == string(models.SourceKindCalendar) {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(source.Provider))
		if provider == "gmail_api" || (provider == "gmail" && gmailSourceUsesOAuth(source)) {
			return source, true
		}
	}
	return config.SourceConfig{}, false
}

func gmailSourceUsesOAuth(source config.SourceConfig) bool {
	return strings.TrimSpace(source.Google.Email) != "" ||
		strings.TrimSpace(source.Google.AccessToken) != "" ||
		strings.TrimSpace(source.Google.RefreshToken) != ""
}

func checkGmailAPI(ctx context.Context, cfg *config.Config, source config.SourceConfig) error {
	c, err := cache.New(":memory:")
	if err != nil {
		return fmt.Errorf("gmail api validation cache: %w", err)
	}
	defer c.Close()
	opened, err := backend.DefaultSourceRegistry().Open(ctx, source, backend.SourceDeps{
		ProfileConfig: cfg,
		Cache:         c,
	})
	if err != nil {
		return err
	}
	defer opened.Close()
	if opened.Mail == nil {
		return fmt.Errorf("gmail api mail source unavailable")
	}
	if err := opened.Mail.Connect(ctx); err != nil {
		return err
	}
	_, err = opened.Mail.ListFolders(ctx)
	return err
}

func checkIMAP(ctx context.Context, cfg *config.Config) error {
	c, err := cache.New(":memory:")
	if err != nil {
		return fmt.Errorf("imap validation cache: %w", err)
	}
	defer c.Close()
	progressCh := make(chan models.ProgressInfo, 4)
	client := imap.New(cfg, "", c, progressCh)
	defer client.Close()
	return runWithTimeout(ctx, func(context.Context) error {
		return client.Connect()
	})
}

func runWithTimeout(parent context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, defaultSurfaceTimeout)
	defer cancel()
	ch := make(chan error, 1)
	go func() {
		ch <- fn(ctx)
	}()
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sanitizeError(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "connection timed out"
	}
	msg := strings.TrimSpace(err.Error())
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	if len(msg) > 220 {
		msg = msg[:217] + "..."
	}
	return msg
}
