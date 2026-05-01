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
- If the user explicitly asks for a commit, merge, push, or PR, perform that publish step and then produce a visible self-reflection report with approval-ready workflow suggestions.

## GitHub Issue Linking Convention

- If intake references a GitHub issue, carry that issue number through the run artifacts and report.
- Use `Refs #<issue>` in local feature-branch commits when the work is not being pushed or merged yet.
- Use `Closes #<issue>` or `Fixes #<issue>` in PR bodies and default-branch commits when the user asks to publish, merge, or squash completed issue work.
- Do not manually close an issue unless explicitly asked or unless the pushed/merged closing reference has already landed and GitHub confirms the completed state.
- If a merge or commit misses the expected issue notation, surface that miss before push so it can be amended.

## Product Truth Convention

- For feature work and any visible behavior change, decide whether product-truth grounding is required.
- Record the consulted sources, grounding status, and any product docs updated before code in the run artifacts.
- Treat `VISION.md`, `ARCHITECTURE.md`, and specs as canonical product truth; code and screenshots are supporting evidence only.

## Artifact Split

Keep machine-readable artifacts in `.superpowers/autopilot/runs/<run-id>/`.

Use that run folder for:

- intake
- plan summary
- product-truth grounding notes
- publication metadata when commit, merge, push, or PR actions happen
- evidence manifest
- before/after PNG screenshots for visual TUI changes
- reflections
- self-reflection report and suggested workflow changes
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
- requested publish actions are recorded when the user asked for them
- a self-reflection report is rendered after requested publish actions
- the evolution ledger is updated if the workflow changed

Success does not mean:

- merged
- pushed
- PR opened
