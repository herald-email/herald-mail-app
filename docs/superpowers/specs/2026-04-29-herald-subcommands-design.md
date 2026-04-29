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
- [x] Source installs use `go install github.com/herald-email/herald-mail-app/cmd/herald@latest` and produce a `herald` binary.
- [x] The Go module path is the canonical GitHub import path, so remote `go install` and downstream imports do not use the local `mail-processor` module name.

## Architecture

This section defines the package boundary so the root binary and wrappers share code instead of forking behavior.

- [x] Move MCP server startup into an importable internal package with a `Run` function that accepts subcommand arguments.
- [x] Move SSH server startup into an importable internal package with a `Run` function that accepts subcommand arguments.
- [x] Keep `cmd/herald-mcp-server/main.go` and `cmd/herald-ssh-server/main.go` as small wrapper entrypoints.
- [x] Add root command dispatch in `main.go` for `mcp` and `ssh` before falling back to the TUI.
- [x] Keep daemon control subcommands (`serve`, `status`, `stop`, `sync`) unchanged.
- [x] Move reusable root CLI startup into an importable internal package, then make both the repository-root development entrypoint and `cmd/herald` call it.
- [x] Add `cmd/herald/main.go` as the canonical Go-installable package for the primary CLI.

## Acceptance

This section defines the minimum proof needed before the feature is ready for handoff.

- [x] Unit tests cover root help text and subcommand dispatch.
- [x] MCP package tests continue to cover demo mode, daemon reprobe behavior, and attachment download path helpers.
- [x] Build checks cover `herald`, `herald-mcp-server`, and `herald-ssh-server`.
- [x] MCP smoke uses `herald mcp --demo` and `tools/list`.
- [x] SSH smoke verifies `herald ssh --version` and `herald ssh --help` without starting a long-running server.
- [x] README, VISION, ARCHITECTURE, MCP test plan, SSH test plan, TUI test plan, release workflow, and Homebrew formula tests document the new primary entrypoints while noting wrapper compatibility.
- [x] `GOBIN=$(mktemp -d) go install ./cmd/herald` creates a binary named `herald`.
- [x] `go build -o /tmp/herald ./cmd/herald` succeeds and `/tmp/herald --help` advertises `herald mcp` and `herald ssh`.
- [x] `printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' | /tmp/herald mcp --demo` returns a tools list.
- [x] Source-install docs prefer `go install github.com/herald-email/herald-mail-app/cmd/herald@latest` while keeping wrapper install paths documented as compatibility options.
