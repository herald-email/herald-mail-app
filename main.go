package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/ai"
	"mail-processor/internal/app"
	"mail-processor/internal/backend"
	"mail-processor/internal/config"
	"mail-processor/internal/logger"
	appsmtp "mail-processor/internal/smtp"
)

// wizardState tracks which sub-model is active during the first-run wizard.
type wizardState int

const (
	wizardStateSettings wizardState = iota
	wizardStateOAuth
)

// wizardModel is a thin tea.Model wrapper that drives the first-run setup wizard.
type wizardModel struct {
	settings   *app.Settings
	oauthWait  *app.OAuthWaitModel
	configPath string
	state      wizardState
	width      int
	height     int
	err        error
}

func (m wizardModel) Init() tea.Cmd {
	return m.settings.Init()
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Fall through to delegate WindowSizeMsg to the active sub-model.

	case app.SettingsSavedMsg:
		if err := msg.Config.Save(m.configPath); err != nil {
			m.err = err
		}
		return m, tea.Quit

	case app.SettingsCancelledMsg:
		// Wizard cannot be cancelled — treat as quit (no config written).
		return m, tea.Quit

	case app.OAuthRequiredMsg:
		cfg := msg.Config
		if cfg == nil {
			cfg = &config.Config{}
		}
		oauthModel, err := app.NewOAuthWaitModel(msg.Email, cfg, m.configPath)
		if err != nil {
			m.err = err
			return m, tea.Quit
		}
		m.oauthWait = oauthModel
		m.state = wizardStateOAuth
		return m, m.oauthWait.Init()

	case app.OAuthDoneMsg:
		return m, tea.Quit

	case app.OAuthErrorMsg:
		m.err = msg.Err
		return m, tea.Quit
	}

	// Delegate to the active sub-model.
	switch m.state {
	case wizardStateSettings:
		newModel, cmd := m.settings.Update(msg)
		m.settings = newModel.(*app.Settings)
		return m, cmd
	case wizardStateOAuth:
		newModel, cmd := m.oauthWait.Update(msg)
		m.oauthWait = newModel.(*app.OAuthWaitModel)
		return m, cmd
	}
	return m, nil
}

func (m wizardModel) View() string {
	switch m.state {
	case wizardStateOAuth:
		return m.oauthWait.View()
	default:
		return m.settings.View()
	}
}

// runWizard runs the first-run setup wizard as a standalone Bubble Tea program.
func runWizard(configPath string) error {
	s := app.NewSettings(app.SettingsModeWizard, nil)
	wm := wizardModel{settings: s, configPath: configPath}
	p := tea.NewProgram(wm, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if final, ok := finalModel.(wizardModel); ok && final.err != nil {
		return final.err
	}
	return nil
}

func main() {
	// Parse command line flags
	var debug = flag.Bool("debug", false, "Enable debug logging to console")
	var verbose = flag.Bool("verbose", false, "Enable verbose logging")
	const defaultConfig = "~/.herald/conf.yaml"
	var configPath = flag.String("config", defaultConfig, "Path to configuration file")
	var showHelp = flag.Bool("help", false, "Show help message")
	flag.Parse()

	usingDefault := (*configPath == defaultConfig)

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
		fmt.Println("Log files are created as herald_YYYYMMDD_HHMMSS.log")
		os.Exit(0)
	}

	// Expand ~ in config path
	resolvedConfig, err := config.ExpandPath(*configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	if usingDefault {
		// Backwards-compat: if proton.yaml exists in CWD and ~/.herald/conf.yaml is absent, guide migration
		if _, statErr := os.Stat("proton.yaml"); statErr == nil {
			if _, statErr2 := os.Stat(resolvedConfig); os.IsNotExist(statErr2) {
				fmt.Fprintln(os.Stderr, "Found proton.yaml in current directory. Herald now uses ~/.herald/conf.yaml.")
				fmt.Fprintln(os.Stderr, "Please move your config:")
				fmt.Fprintln(os.Stderr, "  mkdir -p ~/.herald && mv proton.yaml ~/.herald/conf.yaml")
				os.Exit(1)
			}
		}

		// Ensure ~/.herald directory exists (only when using the default config path)
		heraldDir := filepath.Dir(resolvedConfig)
		if err := os.MkdirAll(heraldDir, 0700); err != nil {
			log.Fatalf("Failed to create config directory %s: %v", heraldDir, err)
		}
	}

	// First-run: if config doesn't exist, launch the setup wizard.
	if _, err := os.Stat(resolvedConfig); os.IsNotExist(err) {
		if err := runWizard(resolvedConfig); err != nil {
			log.Fatalf("setup wizard failed: %v", err)
		}
		// Wizard exited — verify config was actually written (user may have cancelled).
		if _, err := os.Stat(resolvedConfig); os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "Setup cancelled. Run herald again to configure.")
			os.Exit(0)
		}
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
	app.SetConfigPath(resolvedConfig)
	app.SetConfig(cfg)

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