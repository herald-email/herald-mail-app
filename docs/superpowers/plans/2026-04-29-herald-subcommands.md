# Herald Subcommands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add `herald mcp` and `herald ssh` subcommands while preserving the existing TUI default and legacy wrapper binaries.

**Architecture:** Root command dispatch in `main.go` will route `mcp` and `ssh` to importable internal packages. The existing wrapper commands become tiny `main` packages that call the same package `Run` functions, keeping behavior shared and testable.

**Tech Stack:** Go 1.25, standard `flag` package, existing MCP server (`mark3labs/mcp-go`), existing Wish SSH server, current Markdown docs and shell release scripts.

---

## File Map

This section identifies the intended edit surface before code changes begin.

- [x] Modify `main.go` for root command dispatch, help text, and subcommand tests.
- [x] Create `internal/mcpserver/server.go` by moving the current MCP server implementation out of `cmd/herald-mcp-server/main.go`.
- [x] Create `internal/sshserver/server.go` by moving the current SSH server implementation out of `cmd/herald-ssh-server/main.go`.
- [x] Replace `cmd/herald-mcp-server/main.go` and `cmd/herald-ssh-server/main.go` with wrapper entrypoints.
- [x] Move MCP tests from `cmd/herald-mcp-server` to `internal/mcpserver` so unexported helper coverage stays close to the implementation.
- [x] Update `README.md`, `VISION.md`, `ARCHITECTURE.md`, `MCP_TESTPLAN.md`, `SSH_TESTPLAN.md`, `TUI_TESTPLAN.md`, `.github/workflows/release.yml`, `.github/scripts/render-homebrew-formula.sh`, and `homebrew_formula_test.go`.

## Tasks

This section gives the concrete implementation sequence with the required red-green checkpoints.

- [x] **Task 1: Write failing CLI tests.** Add tests in `main_test.go` for `rootCommandFromArgs` routing `mcp` and `ssh`, and for root help text containing `herald mcp`, `herald ssh`, `herald-mcp-server`, and `herald-ssh-server`. Run `go test . -run 'TestRootCommand|TestRootHelp' -v` and confirm it fails on missing helpers or missing help copy.
- [x] **Task 2: Extract shared server packages.** Mechanically move the current MCP and SSH server implementations into `internal/mcpserver` and `internal/sshserver`, convert each `main` to `Run(commandName string, args []string) error`, and keep command-specific version names through the `commandName` argument.
- [x] **Task 3: Add root subcommand dispatch.** Update `main.go` so `herald mcp` calls `mcpserver.Run("herald mcp", os.Args[2:])`, `herald ssh` calls `sshserver.Run("herald ssh", os.Args[2:])`, and existing daemon subcommands still dispatch as before.
- [x] **Task 4: Thin the legacy wrappers.** Replace both wrapper `main.go` files with small entrypoints that call the shared package `Run` functions using legacy command names.
- [x] **Task 5: Update docs and release checks.** Update the docs and release scripts so primary setup examples use `herald mcp` and `herald ssh`, while compatibility examples still mention the legacy wrappers.
- [x] **Task 6: Verify.** Run focused root and MCP tests, full `go test ./...`, build all three binaries, smoke `herald --help`, `herald mcp --version`, `herald ssh --version`, `herald mcp --demo` tools/list, and wrapper `--version` commands.
