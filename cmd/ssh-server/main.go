// ssh-server serves the herald TUI over SSH using charmbracelet/wish.
// Any SSH client can connect and use the full email client remotely.
// Usage: ./ssh-server [-config ~/.herald/conf.yaml] [-addr :2222] [-host-key .ssh/host_ed25519]
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

	"github.com/charmbracelet/ssh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"mail-processor/internal/ai"
	"mail-processor/internal/app"
	"mail-processor/internal/backend"
	"mail-processor/internal/config"
	"mail-processor/internal/logger"
	appsmtp "mail-processor/internal/smtp"
)

func main() {
	configPath := flag.String("config", "~/.herald/conf.yaml", "Path to configuration file")
	addr := flag.String("addr", ":2222", "SSH server listen address")
	hostKey := flag.String("host-key", ".ssh/host_ed25519", "Path to SSH host private key (created if missing)")
	daemonURL := flag.String("daemon", "", "connect to herald daemon at URL instead of opening IMAP")
	flag.Parse()

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

	// Ensure host key directory exists
	if err := os.MkdirAll(".ssh", 0700); err != nil {
		log.Fatalf("Failed to create .ssh dir: %v", err)
	}

	mailer := appsmtp.New(cfg)
	classifier := ai.New(cfg.Ollama.Host, cfg.Ollama.Model)
	classifier.SetEmbeddingModel(cfg.Ollama.EmbeddingModel)

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
				m := app.New(b, mailer, cfg.Credentials.Username, classifier)
				return m, []tea.ProgramOption{tea.WithAltScreen()}
			}),
		),
	)
	if err != nil {
		log.Fatalf("Failed to create SSH server: %v", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Starting SSH mail server on %s", *addr)
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
