# Herald Subcommands Design

This spec defines the issue #8 CLI consolidation: MCP and SSH become discoverable subcommands of the main Herald binary while the existing companion binary names remain available as compatibility wrappers for at least one release.

## Goals

This section captures the user-visible behavior that makes the feature valuable for install, help, and agent setup flows.

- [x] Running `herald` with no subcommand still launches the current TUI path.
- [x] Running `herald --help` advertises `mcp` and `ssh` alongside the existing daemon control subcommands.
- [x] Running `herald mcp` starts the same stdio MCP server that `herald-mcp-server` starts today.
- [x] Running `herald ssh` starts the same Wish SSH TUI server that `herald-ssh-server` starts today.
- [x] Running `herald mcp --version` and `herald ssh --version` works without loading config or opening network services.
- [x] Legacy `herald-mcp-server` and `herald-ssh-server` binaries remain thin wrappers, so Homebrew formulas, MCP configs, and scripts do not break immediately.

## Architecture

This section defines the package boundary so the root binary and wrappers share code instead of forking behavior.

- [x] Move MCP server startup into an importable internal package with a `Run` function that accepts subcommand arguments.
- [x] Move SSH server startup into an importable internal package with a `Run` function that accepts subcommand arguments.
- [x] Keep `cmd/herald-mcp-server/main.go` and `cmd/herald-ssh-server/main.go` as small wrapper entrypoints.
- [x] Add root command dispatch in `main.go` for `mcp` and `ssh` before falling back to the TUI.
- [x] Keep daemon control subcommands (`serve`, `status`, `stop`, `sync`) unchanged.

## Acceptance

This section defines the minimum proof needed before the feature is ready for handoff.

- [x] Unit tests cover root help text and subcommand dispatch.
- [x] MCP package tests continue to cover demo mode, daemon reprobe behavior, and attachment download path helpers.
- [x] Build checks cover `herald`, `herald-mcp-server`, and `herald-ssh-server`.
- [x] MCP smoke uses `herald mcp --demo` and `tools/list`.
- [x] SSH smoke verifies `herald ssh --version` and `herald ssh --help` without starting a long-running server.
- [x] README, VISION, ARCHITECTURE, MCP test plan, SSH test plan, TUI test plan, release workflow, and Homebrew formula tests document the new primary entrypoints while noting wrapper compatibility.
