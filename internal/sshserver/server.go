// Package sshserver serves the Herald TUI over SSH using charmbracelet/wish.
// It is used by `herald ssh` and the legacy herald-ssh-server wrapper.
package sshserver

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/wish/v2"
	"charm.land/wish/v2/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/app"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	demodata "github.com/herald-email/herald-mail-app/internal/demo"
	"github.com/herald-email/herald-mail-app/internal/logger"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
	buildversion "github.com/herald-email/herald-mail-app/internal/version"
)

func Run(commandName string, args []string) error {
	fs := flag.NewFlagSet(commandName, flag.ExitOnError)
	configPath := fs.String("config", "~/.herald/conf.yaml", "Path to configuration file")
	demoMode := fs.Bool("demo", false, "Serve deterministic synthetic demo data without loading config or opening IMAP")
	addr := fs.String("addr", ":2222", "SSH server listen address")
	hostKey := fs.String("host-key", ".ssh/host_ed25519", "Path to SSH host private key (created if missing)")
	daemonURL := fs.String("daemon", "", "connect to herald daemon at URL instead of opening IMAP")
	imageProtocol := fs.String("image-protocol", "auto", "Inline image protocol: auto, iterm2, kitty, links, placeholder, off")
	unsafeLogs := fs.Bool("unsafe-logs", false, "Write unredacted private data to logs for explicit local diagnostics")
	showVersion := fs.Bool("version", false, "Show version information")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		fmt.Println(buildversion.String(commandName))
		return nil
	}

	imageMode, err := app.ParsePreviewImageMode(*imageProtocol)
	if err != nil {
		return err
	}

	if err := logger.Init(false, logger.WithUnsafeLogs(*unsafeLogs)); err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	defer logger.Close()

	resolvedConfig := "demo-config.yaml"
	cfg := &config.Config{}
	cfg.Credentials.Username = "demo@demo.local"
	cfg.Sync.Interval = 60
	cfg.Sync.Idle = false
	cfg.Sync.Background = false
	cfg.Semantic.Enabled = false
	cfg.AI.Provider = "disabled"
	if !*demoMode {
		resolvedConfig, err = config.ExpandPath(*configPath)
		if err != nil {
			return fmt.Errorf("failed to resolve config path: %w", err)
		}

		cfg, err = config.Load(resolvedConfig)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		if _, err := config.EnsureCacheDatabasePath(resolvedConfig, cfg); err != nil {
			return fmt.Errorf("failed to resolve cache path: %w", err)
		}
	}

	mailer := appsmtp.New(cfg)
	var classifier ai.AIClient
	if *demoMode {
		classifier = demodata.NewAI()
	} else {
		classifier, err = ai.NewFromConfig(cfg)
		if err != nil {
			return fmt.Errorf("failed to initialize AI client: %w", err)
		}
	}

	srv, err := newSSHServer(sshServerOptions{
		Addr:           *addr,
		HostKeyPath:    *hostKey,
		Config:         cfg,
		ResolvedConfig: resolvedConfig,
		DemoMode:       *demoMode,
		DaemonURL:      *daemonURL,
		ImageMode:      imageMode,
		Mailer:         mailer,
		Classifier:     classifier,
	})
	if err != nil {
		return fmt.Errorf("failed to create SSH server: %w", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Starting herald-ssh-server on %s", *addr)
	log.Printf("Connect with: ssh -p 2222 localhost")

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-done:
		log.Println("Shutting down SSH server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("SSH server error: %w", err)
		}
		return nil
	}
}

type sshServerOptions struct {
	Addr           string
	HostKeyPath    string
	Config         *config.Config
	ResolvedConfig string
	DemoMode       bool
	DaemonURL      string
	ImageMode      app.PreviewImageMode
	Mailer         *appsmtp.Client
	Classifier     ai.AIClient
}

func newSSHServer(opts sshServerOptions) (*ssh.Server, error) {
	if opts.Mailer == nil {
		opts.Mailer = appsmtp.New(opts.Config)
	}
	if err := ensureHostKeyParent(opts.HostKeyPath); err != nil {
		return nil, err
	}
	return wish.NewServer(
		wish.WithAddress(opts.Addr),
		wish.WithHostKeyPath(opts.HostKeyPath),
		wish.WithMiddleware(
			bubbletea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				return newSessionModel(s, opts)
			}),
		),
	)
}

func ensureHostKeyParent(hostKeyPath string) error {
	if strings.TrimSpace(hostKeyPath) == "" {
		return nil
	}
	dir := filepath.Dir(hostKeyPath)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create host key directory %s: %w", dir, err)
	}
	return nil
}

func newSessionModel(s ssh.Session, opts sshServerOptions) (tea.Model, []tea.ProgramOption) {
	var b backend.Backend
	if opts.DemoMode {
		b = backend.NewDemoBackend()
	} else if opts.DaemonURL != "" {
		rb, err := backend.NewRemote(opts.DaemonURL)
		if err != nil {
			log.Printf("ssh: failed to connect to daemon at %s: %v", opts.DaemonURL, err)
			fmt.Fprintf(s, "Failed to connect to daemon: %v\n", err)
			return nil, nil
		}
		b = rb
	} else {
		// Each SSH connection gets its own backend (own IMAP connection + shared cache)
		lb, err := backend.NewLocal(opts.Config, opts.ResolvedConfig, opts.Classifier)
		if err != nil {
			fmt.Fprintf(s, "Failed to create backend: %v\n", err)
			return nil, nil
		}
		b = lb
	}
	m := app.New(b, opts.Mailer, configuredEmailAddress(opts.Config), opts.Classifier, false)
	m.SetLocalImageLinksEnabled(false)
	m.SetPreviewImageMode(opts.ImageMode)
	m.SetConfigPath(opts.ResolvedConfig)
	m.SetConfig(opts.Config)
	return m, app.ProgramOptions()
}

func configuredEmailAddress(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.IsGmailOAuth() && strings.TrimSpace(cfg.Gmail.Email) != "" {
		return strings.TrimSpace(cfg.Gmail.Email)
	}
	if strings.TrimSpace(cfg.Credentials.Username) != "" {
		return strings.TrimSpace(cfg.Credentials.Username)
	}
	return strings.TrimSpace(cfg.Gmail.Email)
}
