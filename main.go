package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/ai"
	"mail-processor/internal/app"
	"mail-processor/internal/backend"
	"mail-processor/internal/cache"
	"mail-processor/internal/cleanup"
	"mail-processor/internal/config"
	"mail-processor/internal/daemon"
	demodata "mail-processor/internal/demo"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
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

// runDemo starts the app with synthetic data and no real IMAP connection.
func runDemo() {
	if err := logger.Init(false); err != nil {
		log.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logger.Close()

	logger.Info("Starting Herald in demo mode...")

	// Fake config — no real credentials needed
	cfg := &config.Config{}
	cfg.Credentials.Username = "demo@demo.local"

	// Build demo backend
	demoBackend := backend.NewDemoBackend()

	// Demo mode uses a deterministic offline AI client.
	var classifier ai.AIClient = demodata.NewAI()

	// Create SMTP client (no-op without real config, but New() is safe)
	mailer := appsmtp.New(cfg)

	// Build the TUI model
	model := app.New(demoBackend, mailer, cfg.Credentials.Username, classifier, false)
	model.SetConfigPath("demo-config.yaml")
	model.SetConfig(cfg)

	logger.Info("Starting demo TUI application...")
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		logger.Error("Demo application error: %v", err)
		fmt.Printf("Error running demo: %v\n", err)
		os.Exit(1)
	}
}

// loadConfigResult holds the resolved config and its path.
type loadConfigResult struct {
	cfg        *config.Config
	configPath string
}

// loadConfig loads and returns the app config using the default config path logic.
// It expands ~ and handles the backwards-compat proton.yaml check.
// Returns nil cfg on error (prints to stderr and exits).
func loadConfig() (*config.Config, string) {
	const defaultConfig = "~/.herald/conf.yaml"
	configPath := defaultConfig

	resolvedConfig, err := config.ExpandPath(configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	cfg, err := config.Load(resolvedConfig)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	return cfg, resolvedConfig
}

func configNeedsOnboarding(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("config path is a directory: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(data)) == "", nil
}

func ensurePrivateConfigDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("config directory path is not a directory: %s", dir)
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// tryConnectDaemon attempts to connect to a running daemon and returns a
// RemoteBackend if the daemon is reachable, or nil if not.
func tryConnectDaemon(cfg *config.Config) backend.Backend {
	url := fmt.Sprintf("http://%s:%d", cfg.Daemon.BindAddr, cfg.Daemon.Port)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	resp, err := client.Get(url + "/v1/status")
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	resp.Body.Close()
	b, err := backend.NewRemote(url)
	if err != nil {
		return nil
	}
	return b
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			runServe(os.Args[2:])
			return
		case "status":
			runStatus(os.Args[2:])
			return
		case "stop":
			runStop(os.Args[2:])
			return
		case "sync":
			runSync(os.Args[2:])
			return
		}
	}
	runTUI()
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "~/.herald/conf.yaml", "Path to configuration file")
	_ = fs.Parse(args)

	resolvedConfig, err := config.ExpandPath(*configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	cfg, err := config.Load(resolvedConfig)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := daemon.WritePID(cfg.Daemon.PidFile); err != nil {
		log.Fatalf("pidfile: %v", err)
	}
	defer daemon.RemovePID(cfg.Daemon.PidFile)

	srv, err := daemon.New(cfg, resolvedConfig)
	if err != nil {
		daemon.RemovePID(cfg.Daemon.PidFile)
		log.Fatalf("daemon: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("daemon shutdown error: %v", err)
		}
	}()

	addr := fmt.Sprintf("%s:%d", cfg.Daemon.BindAddr, cfg.Daemon.Port)
	log.Printf("herald daemon listening on %s", addr)
	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		daemon.RemovePID(cfg.Daemon.PidFile)
		log.Fatalf("server: %v", err)
	}
}

func runStatus(args []string) {
	cfg, _ := loadConfig()
	pid, err := daemon.ReadPID(cfg.Daemon.PidFile)
	if err != nil {
		fmt.Println("daemon: not running (no pidfile)")
		return
	}
	if !daemon.IsRunning(cfg.Daemon.PidFile) {
		fmt.Printf("daemon: stale pidfile (pid %d not running)\n", pid)
		return
	}
	// Call /v1/status
	url := fmt.Sprintf("http://%s:%d/v1/status", cfg.Daemon.BindAddr, cfg.Daemon.Port)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("daemon: running (pid %d) but HTTP unreachable: %v\n", pid, err)
		return
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	fmt.Println()
}

func runStop(args []string) {
	cfg, _ := loadConfig()
	pid, err := daemon.ReadPID(cfg.Daemon.PidFile)
	if err != nil {
		fmt.Println("daemon: not running")
		return
	}
	if !daemon.IsRunning(cfg.Daemon.PidFile) {
		fmt.Printf("daemon: stale pidfile (pid %d not running), removing\n", pid)
		daemon.RemovePID(cfg.Daemon.PidFile)
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("process not found: %v\n", err)
		return
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("signal failed: %v\n", err)
		return
	}
	// Wait for pidfile removal (up to 5s)
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(cfg.Daemon.PidFile); os.IsNotExist(err) {
			fmt.Println("daemon stopped")
			return
		}
	}
	fmt.Println("daemon stop timed out")
}

func runSync(args []string) {
	cfg, _ := loadConfig()
	folder := "INBOX"
	if len(args) > 0 {
		folder = args[0]
	}
	url := fmt.Sprintf("http://%s:%d/v1/sync?folder=%s", cfg.Daemon.BindAddr, cfg.Daemon.Port, folder)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("sync failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("sync started for %s\n", folder)
}

// buildRuleAction converts a flat action_type + action_value pair (from YAML config)
// into a models.RuleAction, populating the appropriate sub-field by type.
func buildRuleAction(actionType, actionValue string) models.RuleAction {
	a := models.RuleAction{Type: models.RuleActionType(actionType)}
	switch models.RuleActionType(actionType) {
	case models.ActionMove:
		a.DestFolder = actionValue
	case models.ActionWebhook:
		a.WebhookURL = actionValue
	case models.ActionCommand:
		a.Command = actionValue
	case models.ActionNotify:
		a.NotifyBody = actionValue
	}
	return a
}

func runTUI() {
	// Parse command line flags
	var debug = flag.Bool("debug", false, "Enable debug logging in the Herald user log directory")
	var verbose = flag.Bool("verbose", false, "Alias for -debug (same behavior today)")
	var demo = flag.Bool("demo", false, "Start with synthetic demo data (no real IMAP required)")
	var dryRun = flag.Bool("dry-run", false, "Log rule and cleanup actions without executing them (dry run)")
	const defaultConfig = "~/.herald/conf.yaml"
	var configPath = flag.String("config", defaultConfig, "Path to configuration file")
	var showHelp = flag.Bool("help", false, "Show help message")
	flag.Parse()

	// Demo mode: skip all real IMAP setup
	if *demo {
		runDemo()
		return
	}

	usingDefault := (*configPath == defaultConfig)

	if *showHelp {
		fmt.Println("Herald - Email analysis and cleanup tool")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s [flags]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Printf("  %s serve   # Start the Herald daemon\n", os.Args[0])
		fmt.Printf("  %s status  # Show daemon status\n", os.Args[0])
		fmt.Printf("  %s stop    # Stop the running daemon\n", os.Args[0])
		fmt.Printf("  %s sync    # Trigger a sync (optionally pass folder name)\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Printf("  %s                    # Run with default config (~/.herald/conf.yaml)\n", os.Args[0])
		fmt.Printf("  %s -debug             # Run with debug logging in the Herald user log directory\n", os.Args[0])
		fmt.Printf("  %s -verbose           # Alias for -debug\n", os.Args[0])
		fmt.Printf("  %s -config custom.yaml # Use custom config file\n", os.Args[0])
		fmt.Println()
		fmt.Println("Log files are created as herald_YYYYMMDD_HHMMSS.log in the user log directory")
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
				fmt.Fprintln(os.Stderr, "  mkdir -p ~/.herald && chmod 700 ~/.herald && mv proton.yaml ~/.herald/conf.yaml")
				os.Exit(1)
			}
		}

		// Ensure ~/.herald directory exists (only when using the default config path)
		heraldDir := filepath.Dir(resolvedConfig)
		if err := ensurePrivateConfigDir(heraldDir); err != nil {
			log.Fatalf("Failed to create config directory %s: %v", heraldDir, err)
		}
	}

	needsOnboarding, err := configNeedsOnboarding(resolvedConfig)
	if err != nil {
		log.Fatalf("Failed to inspect config %s: %v", resolvedConfig, err)
	}

	// First-run: if config doesn't exist or is empty, launch the setup wizard.
	if needsOnboarding {
		if err := runWizard(resolvedConfig); err != nil {
			log.Fatalf("setup wizard failed: %v", err)
		}
		// Wizard exited — verify config was actually written (user may have cancelled).
		stillNeedsOnboarding, checkErr := configNeedsOnboarding(resolvedConfig)
		if checkErr != nil {
			log.Fatalf("Failed to inspect config %s after setup: %v", resolvedConfig, checkErr)
		}
		if stillNeedsOnboarding {
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
	cachePath, err := config.EnsureCacheDatabasePath(resolvedConfig, cfg)
	if err != nil {
		logger.Error("Failed to resolve cache database path: %v", err)
		log.Fatalf("Failed to resolve cache database path: %v", err)
	}

	logger.Info("Configuration loaded successfully")
	logger.Debug("Cache database: %s", cachePath)
	logger.Debug("Server: %s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Debug("Username: %s", cfg.Credentials.Username)

	// Create AI classifier based on configured provider
	classifier, err := ai.NewFromConfig(cfg)
	if err != nil {
		logger.Error("Failed to create AI client: %v", err)
		log.Fatalf("Failed to create AI client: %v", err)
	}

	// Import classification prompts from config into DB (idempotent by name).
	if len(cfg.Classification.Prompts) > 0 {
		if emailCache, cacheErr := cache.New(cachePath); cacheErr == nil {
			existing, _ := emailCache.GetAllCustomPrompts()
			existingNames := make(map[string]bool, len(existing))
			for _, p := range existing {
				existingNames[p.Name] = true
			}
			for _, cp := range cfg.Classification.Prompts {
				if !existingNames[cp.Name] {
					_ = emailCache.SaveCustomPrompt(&models.CustomPrompt{
						Name:         cp.Name,
						SystemText:   cp.SystemText,
						UserTemplate: cp.UserTemplate,
						OutputVar:    cp.OutputVar,
					})
				}
			}
			emailCache.Close()
		}
	}

	// Import classification_actions from config into DB as rules (idempotent by name).
	if len(cfg.ClassificationActions) > 0 {
		if emailCache, cacheErr := cache.New(cachePath); cacheErr == nil {
			existing, existErr := emailCache.GetAllRules()
			if existErr != nil {
				logger.Warn("Failed to load existing rules during config import: %v", existErr)
			}
			existingNames := make(map[string]bool, len(existing))
			for _, r := range existing {
				existingNames[r.Name] = true
			}
			for _, ca := range cfg.ClassificationActions {
				if !existingNames[ca.Name] {
					if saveErr := emailCache.SaveRule(&models.Rule{
						Name:         ca.Name,
						TriggerType:  models.RuleTriggerType(ca.TriggerType),
						TriggerValue: ca.TriggerValue,
						Enabled:      ca.Enabled,
						Actions: []models.RuleAction{
							buildRuleAction(ca.ActionType, ca.ActionValue),
						},
					}); saveErr != nil {
						logger.Warn("Failed to seed classification action %q: %v", ca.Name, saveErr)
					}
				}
			}
			emailCache.Close()
		}
	}

	// Try to connect to the daemon first; fall back to direct LocalBackend.
	var b backend.Backend
	if remoteB := tryConnectDaemon(cfg); remoteB != nil {
		logger.Info("Connected to Herald daemon")
		b = remoteB
	} else {
		// Create the backend (pass classifier so semantic search can embed queries)
		lb, err := backend.NewLocal(cfg, resolvedConfig, classifier)
		if err != nil {
			logger.Error("Failed to create backend: %v", err)
			log.Fatalf("Failed to create backend: %v", err)
		}
		b = lb
	}

	// Create SMTP client for compose/reply
	mailer := appsmtp.New(cfg)

	// Create the TUI application
	app := app.New(b, mailer, cfg.Credentials.Username, classifier, *dryRun)
	app.SetConfigPath(resolvedConfig)
	app.SetConfig(cfg)

	// Wire cleanup scheduler if using a local backend and schedule is configured.
	if lb, ok := b.(*backend.LocalBackend); ok && cfg.Cleanup.ScheduleHours > 0 {
		engine := cleanup.NewEngineWithDryRun(lb.Cache(), b, logger.New(), *dryRun)
		sched := cleanup.NewScheduler(engine, cfg.Cleanup.ScheduleHours)
		sched.Start(context.Background())
		app.SetCleanupScheduler(sched)
	}

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
