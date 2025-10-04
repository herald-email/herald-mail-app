# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Python-based email analysis tool that connects to IMAP servers to help users identify and delete frequent email senders. The application uses a TUI (Text User Interface) built with Textual to provide an interactive experience for managing email cleanup.

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

### Running the Application
```bash
python email_grouper.py
```

### Configuration
The application requires a `proton.yaml` configuration file with IMAP credentials:
```yaml
credentials:
  username: "your_email@mail.com"
  password: "your_password"
server:
  host: "imap.mail.com"
  port: 993
```

### Dependencies
Install required dependencies:
```bash
pip install -r requirements.txt
```

Required packages:
- pyyaml>=6.0
- textual>=0.38.1

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
- Async/await pattern for IMAP operations to avoid blocking the TUI
- Worker threads in Textual for background email processing
- SQLite transactions for cache consistency