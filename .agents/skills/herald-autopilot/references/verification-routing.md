# Verification Routing

This skill uses impact-based gates. The goal is to verify enough to trust the handoff without paying the cost of every repo surface on every task.

## Core Rule

Always run the smallest set of commands that can honestly prove the requested change. Add broader surfaces only when the task touches them.

## Surface Map

### Code-only tasks

Run:

- focused unit or package tests
- build command if compilation is a meaningful risk

Usually skip:

- TUI
- SSH
- MCP

### TUI tasks

Run:

- focused tests if the bug is testable in Go
- tmux-based visual checks
- the relevant `TUI_TESTPLAN.md` cases

For visual styling, layout, copy, or chrome changes, capture before/after evidence:

- Before: same screen/state before implementation when safely reproducible
- After: same screen/state after implementation at the matching terminal size
- Always save PNG screenshots plus plain-text/ANSI captures in the run evidence folder
- Use evidence summaries beginning with `Before:` and `After:` so the report renderer can include the images directly

If the task is layout-only and not realistically expressible in Go tests, document the terminal-only repro and rely on tmux evidence.

### SSH tasks

Run:

- build the SSH server
- exercise the affected flow over SSH
- include the relevant SSH evidence in the report

### MCP tasks

Run:

- build or run `cmd/mcp-server`
- invoke the affected tool path
- capture request and result evidence

### Mixed tasks

Run the union of the required surfaces, then summarize which gates were skipped and why.

## Required Negative Paths

The skill must also handle:

- dirty or failing baseline
- missing tool prerequisite for a requested surface
- ambiguous intake that should force a question instead of guessing

Record these cases in the run folder even when the task cannot continue.
