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
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/app"
	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/logger"
	appsmtp "github.com/herald-email/herald-mail-app/internal/smtp"
	buildversion "github.com/herald-email/herald-mail-app/internal/version"
)

func Run(commandName string, args []string) error {
	fs := flag.NewFlagSet(commandName, flag.ExitOnError)
	configPath := fs.String("config", "~/.herald/conf.yaml", "Path to configuration file")
	addr := fs.String("addr", ":2222", "SSH server listen address")
	hostKey := fs.String("host-key", ".ssh/host_ed25519", "Path to SSH host private key (created if missing)")
	daemonURL := fs.String("daemon", "", "connect to herald daemon at URL instead of opening IMAP")
	imageProtocol := fs.String("image-protocol", "auto", "Inline image protocol: auto, iterm2, kitty, links, placeholder, off")
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

	if err := logger.Init(false); err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	defer logger.Close()

	resolvedConfig, err := config.ExpandPath(*configPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	cfg, err := config.Load(resolvedConfig)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if _, err := config.EnsureCacheDatabasePath(resolvedConfig, cfg); err != nil {
		return fmt.Errorf("failed to resolve cache path: %w", err)
	}

	// Ensure host key directory exists
	if err := os.MkdirAll(".ssh", 0700); err != nil {
		return fmt.Errorf("failed to create .ssh dir: %w", err)
	}

	mailer := appsmtp.New(cfg)
	classifier, err := ai.NewFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize AI client: %w", err)
	}

	srv, err := wish.NewServer(
		wish.WithAddress(*addr),
		wish.WithHostKeyPath(*hostKey),
		wish.WithMiddleware(
			bubbletea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				var b backend.Backend
				if *daemonURL != "" {
					rb, err := backend.NewRemote(*daemonURL)
					if err != nil {
						log.Printf("ssh: failed to connect to daemon at %s: %v", *daemonURL, err)
						fmt.Fprintf(s, "Failed to connect to daemon: %v\n", err)
						return nil, nil
					}
					b = rb
				} else {
					// Each SSH connection gets its own backend (own IMAP connection + shared cache)
					lb, err := backend.NewLocal(cfg, resolvedConfig, classifier)
					if err != nil {
						fmt.Fprintf(s, "Failed to create backend: %v\n", err)
						return nil, nil
					}
					b = lb
				}
				m := app.New(b, mailer, cfg.Credentials.Username, classifier, false)
				m.SetLocalImageLinksEnabled(false)
				m.SetPreviewImageMode(imageMode)
				m.SetConfigPath(resolvedConfig)
				m.SetConfig(cfg)
				return m, []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()}
			}),
		),
	)
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
