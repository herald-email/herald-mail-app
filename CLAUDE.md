# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This repository contains email analysis tools that connect to IMAP servers to help users identify and delete frequent email senders. Two implementations are available:

1. **Python Version** (`email_grouper.py`) - Original implementation using Textual TUI
2. **Go Version** (`main.go` + `internal/`) - High-performance rewrite using Bubble Tea

Both provide interactive TUI experiences for managing email cleanup.

## Architecture

### Core Components

1. **ProtonMailAnalyzer** (`email_grouper.py:177-517`) - Main class that handles IMAP connections, email processing, and deletion operations
2. **EmailCache** (`email_grouper.py:54-150`) - SQLite-based caching system to avoid re-processing emails on subsequent runs
3. **ProtonMailTUI** (`email_grouper.py:519-967`) - Textual-based terminal user interface with dual-pane layout

### Key Features

- **Email Grouping**: Groups emails by sender or domain for bulk analysis
- **Caching**: Uses SQLite to cache email metadata for faster subsequent launches
- **Interactive Deletion**: Supports single email or bulk sender deletion
- **TUI Interface**: Split-pane view showing sender summary and individual email details

## Common Commands

### Python Version
```bash
# Run Python version
python email_grouper.py

# Install dependencies
pip install -r requirements.txt
```

### Go Version  
```bash
# Build and run Go version
make build && ./bin/mail-processor

# Or run directly
make run

# Development
make deps     # Install dependencies
make fmt      # Format code
make test     # Run tests
```

### Configuration
Both versions use the same `proton.yaml` configuration file:
```yaml
credentials:
  username: "your_email@mail.com"
  password: "your_password"
server:
  host: "imap.mail.com"
  port: 993
```

### Dependencies

**Python Version:**
- pyyaml>=6.0
- textual>=0.38.1

**Go Version:**
- Go 1.21+
- Dependencies managed via go.mod

## Development Notes

### Database Schema
The SQLite cache stores emails with these fields:
- message_id (PRIMARY KEY)
- sender, subject, date, size, has_attachments, folder, last_updated

### Key TUI Bindings
- `q` - Quit application
- `d` - Toggle domain/sender grouping mode
- `r` - Refresh email data
- `D` - Delete selected emails/senders
- `space` - Toggle selection for multi-delete

### Error Handling
- Logs are written to timestamped files: `protonmail_analyzer_YYYYMMDD_HHMMSS.log`
- Cache cleanup runs automatically to sync with IMAP server state
- IMAP connections use SSL with disabled certificate verification for local bridges

### Code Patterns

**Python Version:**
- Async/await pattern for IMAP operations to avoid blocking the TUI
- Worker threads in Textual for background email processing
- SQLite transactions for cache consistency

**Go Version:**
- Goroutines for concurrent email processing
- Channels for progress updates between goroutines
- Bubble Tea's Elm architecture for UI state management
- Structured logging and proper error handling

## Go Implementation Details

### Project Structure
```
internal/
├── app/           # Bubble Tea TUI application
├── cache/         # SQLite caching layer  
├── config/        # YAML configuration handling
├── imap/          # IMAP client and operations
└── models/        # Data structures and types
```

### Key Libraries
- **Bubble Tea**: TUI framework with Elm architecture
- **Lipgloss**: Terminal styling and layout
- **go-imap**: IMAP client library
- **go-sqlite3**: SQLite database driver

### Performance Benefits
- ~5x faster email processing than Python version
- Lower memory usage (~20-30MB vs 50-100MB)
- Single binary distribution
- Better error handling and recovery