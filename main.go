package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/ai"
	"mail-processor/internal/app"
	"mail-processor/internal/backend"
	"mail-processor/internal/config"
	"mail-processor/internal/logger"
	appsmtp "mail-processor/internal/smtp"
)

// expandPath replaces a leading "~" with the current user's home directory.
func expandPath(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, p[1:]), nil
}

func main() {
	// Parse command line flags
	var debug = flag.Bool("debug", false, "Enable debug logging to console")
	var verbose = flag.Bool("verbose", false, "Enable verbose logging")
	var configPath = flag.String("config", "~/.herald/conf.yaml", "Path to configuration file")
	var showHelp = flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *showHelp {
		fmt.Println("Herald - Email analysis and cleanup tool")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s [flags]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Printf("  %s                    # Run with default config (~/.herald/conf.yaml)\n", os.Args[0])
		fmt.Printf("  %s -debug             # Run with debug logging\n", os.Args[0])
		fmt.Printf("  %s -config custom.yaml # Use custom config file\n", os.Args[0])
		fmt.Println()
		fmt.Println("Log files are created as mail_processor_YYYYMMDD_HHMMSS.log")
		os.Exit(0)
	}

	// Expand ~ in config path
	resolvedConfig, err := expandPath(*configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	// Backwards-compat: if proton.yaml exists in CWD and ~/.herald/conf.yaml is absent, guide migration
	if *configPath == "~/.herald/conf.yaml" {
		if _, statErr := os.Stat("proton.yaml"); statErr == nil {
			if _, statErr2 := os.Stat(resolvedConfig); os.IsNotExist(statErr2) {
				fmt.Fprintln(os.Stderr, "Found proton.yaml in current directory. Herald now uses ~/.herald/conf.yaml.")
				fmt.Fprintln(os.Stderr, "Please move your config:")
				fmt.Fprintln(os.Stderr, "  mkdir -p ~/.herald && mv proton.yaml ~/.herald/conf.yaml")
				os.Exit(1)
			}
		}
	}

	// Ensure ~/.herald directory exists (only when using the default config path)
	heraldDir := filepath.Dir(resolvedConfig)
	if err := os.MkdirAll(heraldDir, 0700); err != nil {
		log.Fatalf("Failed to create config directory %s: %v", heraldDir, err)
	}

	// Initialize logging
	if err := logger.Init(*debug || *verbose); err != nil {
		log.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logger.Close()

	logger.Info("Starting Herald...")
	logger.Debug("Debug mode enabled")

	// Load configuration
	cfg, err := config.Load(resolvedConfig)
	if err != nil {
		logger.Error("Failed to load config: %v", err)
		log.Fatalf("Failed to load config: %v", err)
	}

	logger.Info("Configuration loaded successfully")
	logger.Debug("Server: %s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Debug("Username: %s", cfg.Credentials.Username)

	// Create AI classifier (talks to local Ollama; nil-safe if not configured)
	classifier := ai.New(cfg.Ollama.Host, cfg.Ollama.Model)
	classifier.SetEmbeddingModel(cfg.Ollama.EmbeddingModel)

	// Create the backend (pass classifier so semantic search can embed queries)
	b, err := backend.NewLocal(cfg, classifier)
	if err != nil {
		logger.Error("Failed to create backend: %v", err)
		log.Fatalf("Failed to create backend: %v", err)
	}

	// Create SMTP client for compose/reply
	mailer := appsmtp.New(cfg)

	// Create the TUI application
	app := app.New(b, mailer, cfg.Credentials.Username, classifier)

	logger.Info("Starting TUI application...")

	// Run the application
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		logger.Error("Application error: %v", err)
		fmt.Printf("Error running application: %v", err)
		os.Exit(1)
	}

	logger.Info("Application finished successfully")
}