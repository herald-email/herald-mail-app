// herald-ssh-server serves the Herald TUI over SSH using charmbracelet/wish.
// Any SSH client can connect and use the full email client remotely.
// Usage: ./herald-ssh-server [-config ~/.herald/conf.yaml] [-addr :2222] [-host-key .ssh/host_ed25519]
package main

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
	"mail-processor/internal/ai"
	"mail-processor/internal/app"
	"mail-processor/internal/backend"
	"mail-processor/internal/config"
	"mail-processor/internal/logger"
	appsmtp "mail-processor/internal/smtp"
	buildversion "mail-processor/internal/version"
)

func main() {
	configPath := flag.String("config", "~/.herald/conf.yaml", "Path to configuration file")
	addr := flag.String("addr", ":2222", "SSH server listen address")
	hostKey := flag.String("host-key", ".ssh/host_ed25519", "Path to SSH host private key (created if missing)")
	daemonURL := flag.String("daemon", "", "connect to herald daemon at URL instead of opening IMAP")
	imageProtocol := flag.String("image-protocol", "auto", "Inline image protocol: auto, iterm2, kitty, links, placeholder, off")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildversion.String("herald-ssh-server"))
		return
	}

	imageMode, err := app.ParsePreviewImageMode(*imageProtocol)
	if err != nil {
		log.Fatalf("%v", err)
	}

	if err := logger.Init(false); err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer logger.Close()

	resolvedConfig, err := config.ExpandPath(*configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	cfg, err := config.Load(resolvedConfig)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if _, err := config.EnsureCacheDatabasePath(resolvedConfig, cfg); err != nil {
		log.Fatalf("Failed to resolve cache path: %v", err)
	}

	// Ensure host key directory exists
	if err := os.MkdirAll(".ssh", 0700); err != nil {
		log.Fatalf("Failed to create .ssh dir: %v", err)
	}

	mailer := appsmtp.New(cfg)
	classifier, err := ai.NewFromConfig(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize AI client: %v", err)
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
		log.Fatalf("Failed to create SSH server: %v", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Starting herald-ssh-server on %s", *addr)
	log.Printf("Connect with: ssh -p 2222 localhost")

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Fatalf("SSH server error: %v", err)
		}
	}()

	<-done
	log.Println("Shutting down SSH server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}
}
