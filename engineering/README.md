# Engineering

This directory holds maintainer-facing and agent-facing project material that should not be mixed into the public documentation site under `docs/`.

- `testplans/` contains manual QA plans, surface smoke checks, and TUI automation guidance used during development and release verification.
- `testplans/REPORT_TEMPLATE.md` is the default shape for reports saved under `reports/`, including the required verification surface checklist.
- `testplans/VIRTUAL_MAIL_LAB_COVERAGE.md` maps each `internal/testmail` scenario to the TUI, SSH, MCP, daemon, backend, and image-evidence lanes that cover it.
- Realistic mail regressions should prefer `internal/testmail` virtual lab scenarios before private live-mail repros.
