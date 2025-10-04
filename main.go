package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"mail-processor/internal/app"
	"mail-processor/internal/config"
	"mail-processor/internal/logger"
)

func main() {
	// Parse command line flags
	var debug = flag.Bool("debug", false, "Enable debug logging to console")
	var verbose = flag.Bool("verbose", false, "Enable verbose logging")
	var configPath = flag.String("config", "proton.yaml", "Path to configuration file")
	var showHelp = flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *showHelp {
		fmt.Println("Mail Processor - Email analysis and cleanup tool")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Printf("  %s [flags]\n", os.Args[0])
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Printf("  %s                    # Run with default config\n", os.Args[0])
		fmt.Printf("  %s -debug             # Run with debug logging\n", os.Args[0])
		fmt.Printf("  %s -config custom.yaml # Use custom config file\n", os.Args[0])
		fmt.Println()
		fmt.Println("Log files are created as mail_processor_YYYYMMDD_HHMMSS.log")
		os.Exit(0)
	}

	// Initialize logging
	if err := logger.Init(*debug || *verbose); err != nil {
		log.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logger.Close()

	logger.Info("Starting Mail Processor...")
	logger.Debug("Debug mode enabled")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("Failed to load config: %v", err)
		log.Fatalf("Failed to load config: %v", err)
	}

	logger.Info("Configuration loaded successfully")
	logger.Debug("Server: %s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Debug("Username: %s", cfg.Credentials.Username)

	// Create the TUI application
	app := app.New(cfg)

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