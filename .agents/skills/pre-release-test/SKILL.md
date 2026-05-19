---
name: pre-release-test
description: Use when preparing a Herald beta release, checking release readiness, or running broad deterministic integration gates across TUI themes, inline images, SSH, and MCP before tagging.
---

# Pre-Release Test

Use this skill before cutting a Herald beta or after risky UI/media/theme changes. It is a deterministic demo-data gate and does not touch live IMAP.

## Default Gate

Run from the repository root:

```bash
.agents/skills/pre-release-test/scripts/run_pre_release_gate.sh
```

The runner writes one report directory under `reports/pre-release-gate_<timestamp>/` and exits nonzero if any required gate fails.

## What It Covers

- Go verification: `make test`, `make vet`.
- Release binaries: `make build build-ssh build-mcp`.
- TUI visuals: themed demo captures at `220x50`, `80x24`, and `50x15`.
- Inline images: custom ttyd browser-raster probes with no app theme and with `HERALD_THEME=jade-signal`.
- SSH: demo-mode server startup and local SSH connection smoke.
- MCP: demo-mode `tools/list` stdio smoke.

## Useful Overrides

```bash
HERALD_PRE_RELEASE_THEME=solar-paper \
HERALD_PRE_RELEASE_IMAGE_PROTOCOL=iterm2 \
HERALD_PRE_RELEASE_PORT_BASE=7780 \
  .agents/skills/pre-release-test/scripts/run_pre_release_gate.sh
```

Use a different `HERALD_PRE_RELEASE_PORT_BASE` if local ttyd or SSH ports are busy.

## Rules

- Do not replace this with stock ttyd; custom ttyd raster proof is the required image gate.
- If the runner fails because a local dependency is missing, install or configure that dependency and rerun instead of marking the release ready.
- If a native iTerm2, Ghostty, or Kitty placement claim matters, add manual screenshots to the report directory; native evidence is additive, not a substitute for the scripted gate.
