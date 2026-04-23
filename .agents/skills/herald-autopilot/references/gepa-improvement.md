# GEPA Improvement Mode

Use this reference when the user explicitly asks to improve GEPA itself rather than to execute a normal Herald bug or feature. The goal is to make the autopilot workflow more capable over time without losing trust in the current branch-plus-report contract.

## Concurrency Rule

If another agent may still be running `herald-autopilot`, treat the current execution scripts as live infrastructure:

- prefer adding new optimizer files over rewriting the core run-execution helpers
- avoid changing the meaning of existing fields in `run.json`
- update the main skill instructions only in backward-compatible ways
- if you need a breaking workflow change, stage it as a new additive layer first

## Required Inputs

Read these before making workflow changes:

- `docs/superpowers/gepa-evolution.md`
- `VISION.md`
- `ARCHITECTURE.md`
- `docs/superpowers/specs/`
- recent run folders under `.superpowers/autopilot/runs/`
- optimizer outputs under `.superpowers/autopilot/state/` when present

## Improvement Loop

1. Summarize recent runs.
2. Snapshot the current product source of truth.
3. Build a lightweight frontier from scored runs.
4. Extract repeated failure and risk patterns.
5. Produce one improvement brief that identifies the top bottleneck and ranked experiments.
6. Implement only one workflow improvement at a time.
7. Append an improvement-history entry.
8. Re-run the optimizer helpers and sync the ledger.

## Optimizer Helpers

These scripts are additive and safe to run repeatedly:

- `analyze_recent_runs.py`
- `snapshot_product_truth.py`
- `build_frontier.py`
- `extract_feedback_patterns.py`
- `prepare_gepa_improvement.py`
- `append_improvement_log.py`
- `render_improvement_log.py`
- `sync_evolution_ledger.py`

## v2 Scope

This layer supports:

- auto-summarized recent-run state
- a lightweight Pareto-style frontier over scored runs
- repeated failure and risk pattern extraction
- a generated improvement brief
- auto-synced ledger snapshots

This layer does not yet support:

- autonomous prompt mutation
- automatic challenger worktrees
- self-directed execution without an explicit user request
