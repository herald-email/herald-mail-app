# Workflow Contract

This skill is the repo-local conductor for one Herald task at a time. It is intentionally opinionated so future workflow evolution has stable artifacts to compare.

## Lifecycle

Each run should move through these states in order unless it is blocked:

1. `initialized`
2. `baseline_checked`
3. `worktree_ready`
4. `in_progress`
5. `verifying`
6. `passed` or `failed` or `blocked`
7. `reported`

## Worktree Convention

- Branch prefix: `codex/`
- Branch pattern: `codex/autopilot-<slug>-<timestamp>`
- Worktree root: `.worktrees/`
- Worktree path: `.worktrees/<run-id>-<slug>`

## User Interaction Policy

- Ask questions only when the missing answer materially changes implementation or safety.
- A concise plan summary is required before implementation.
- Proceed after the summary unless a non-obvious decision still needs the user.
- Never silently push, merge, or open a PR in v1.

## Artifact Split

Keep machine-readable artifacts in `.superpowers/autopilot/runs/<run-id>/`.

Use that run folder for:

- intake
- plan summary
- evidence manifest
- reflections
- score output
- run-local summary

Keep the human report in `reports/`.

Use `docs/superpowers/gepa-evolution.md` as the curated cross-run ledger for improving the workflow itself.

## GEPA-Compatible v1 Rules

- One active candidate per run
- Same-worktree retries only
- Required failures generate explicit natural-language feedback
- Score the run on comparable axes so future versions can support multiple candidates without changing the interface

## Terminal State

Success means:

- the task has a branch
- the task has a worktree
- required verification is recorded
- the report is rendered
- the evolution ledger is updated if the workflow changed

Success does not mean:

- merged
- pushed
- PR opened
