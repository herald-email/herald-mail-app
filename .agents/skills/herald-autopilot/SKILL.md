---
name: herald-autopilot
description: Use when you want one repo-local Herald workflow to take a single bug, feature, or workflow improvement from intake through planning, isolated worktree setup, implementation, impact-based verification, branch handoff, and GEPA-style run logging.
---

# Herald Autopilot

Use this skill when the user wants to hand off one Herald task and come back later to a branch, verification evidence, and a readable report. This skill is intentionally single-task and single-worktree in v1 so it stays predictable while still capturing enough structure to evolve later.

## When To Use

- One Herald bug or feature should be driven end-to-end with minimal supervision.
- The work should leave behind a branch, a worktree, a run folder, and a human-readable report.
- The task benefits from repo-specific verification routing across code, TUI, SSH, and MCP.
- The user later wants to say "improve GEPA" and have you continue from a durable workflow history.

## Do Not Use

- The user is asking for a broad multi-task sprint. Split it into one invocation per task first.
- The task is purely exploratory and should not create worktrees or branch handoff artifacts.
- The user explicitly wants manual step-by-step collaboration instead of autopilot.

## Required Reads

Read these before you start:

- The living workflow ledger: [`docs/superpowers/gepa-evolution.md`](../../../docs/superpowers/gepa-evolution.md)
- Workflow contract: [`references/workflow-contract.md`](references/workflow-contract.md)
- Run schema: [`references/run-schema.md`](references/run-schema.md)
- Verification routing: [`references/verification-routing.md`](references/verification-routing.md)

If the task touches the TUI, also read and follow [`../tui-test/SKILL.md`](../tui-test/SKILL.md) for the tmux-driven visual checks.

## Default Contract

1. Treat one invocation as one task.
2. Ask only critical questions that change implementation or safety.
3. Show a concise plan summary, then proceed unless a risky or non-obvious tradeoff needs the user's decision.
4. Verify baseline, then create a dedicated worktree under `.worktrees/`.
5. Keep all raw machine-readable artifacts under `.superpowers/autopilot/runs/<run-id>/`.
6. Stop at local branch + worktree + report. Do not push, create a PR, or merge unless the user asks.

## Bootstrap A Run

Create the run folder first so the workflow has durable state from the beginning:

```bash
python3 .agents/skills/herald-autopilot/scripts/bootstrap_run.py \
  --repo-root "$(pwd)" \
  --task "Fix the cleanup preview overflow at 80x24" \
  --task-type bug \
  --surfaces code,tui \
  --plan-summary "Reproduce in tmux, add failing test if possible, fix layout, run focused TUI checks." \
  --status initialized
```

This creates:

- `.superpowers/autopilot/runs/<run-id>/run.json`
- `.superpowers/autopilot/runs/<run-id>/intake.md`
- `.superpowers/autopilot/runs/<run-id>/plan.md`
- `.superpowers/autopilot/runs/<run-id>/evidence/manifest.json`
- `.superpowers/autopilot/runs/<run-id>/reflections/`

## Worktree And Branch Policy

Use the run metadata to create:

- Branch: `codex/autopilot-<slug>-<timestamp>`
- Worktree: `.worktrees/<run-id>-<slug>`

Baseline verification happens before implementation. If the baseline is already failing, record that in the run, summarize it clearly, and ask whether to proceed on top of the dirty baseline only if it materially obscures the requested task.

## Impact-Based Verification

Route verification by affected surface instead of running every surface every time:

- `code`: focused tests, builds, linters, or targeted commands that prove the requested behavior
- `tui`: tmux-driven checks and visual inspection using `tui-test`
- `ssh`: build `cmd/herald-ssh-server`, exercise the affected flow over SSH if the change touches the SSH surface
- `mcp`: build or run `cmd/mcp-server`, invoke the relevant tool path if the change touches MCP behavior

Record every verification result with:

```bash
python3 .agents/skills/herald-autopilot/scripts/capture_evidence.py \
  --run-dir ".superpowers/autopilot/runs/<run-id>" \
  --kind command \
  --summary "go test ./internal/app -run TestBuildLayoutPlan_CleanupPreviewCollapsesSummaryAt80Cols -v" \
  --status pass \
  --gate focused-tests \
  --artifact "/tmp/autopilot-focused-test.log"
```

## Reflection Loop

When a required gate fails, do not guess. Record the failure, the hypothesis, and the next bounded step:

```bash
python3 .agents/skills/herald-autopilot/scripts/record_reflection.py \
  --run-dir ".superpowers/autopilot/runs/<run-id>" \
  --attempt 1 \
  --failing-evidence "focused-tests" \
  --hypothesis "Cleanup preview width still depends on stale summary width at 80x24." \
  --next-step "Trace layout plan inputs, update failing test, then patch cleanup width calculation." \
  --decision continue \
  --feedback "Required gate focused-tests failed: expected usable preview width at 80x24."
```

Stay in the same worktree for v1. Keep retries bounded by the run's retry limit.

## Update Run State

Use the helper instead of hand-editing `run.json` when the run state changes:

```bash
python3 .agents/skills/herald-autopilot/scripts/update_run.py \
  --run-dir ".superpowers/autopilot/runs/<run-id>" \
  --status passed \
  --outcome-summary "Implemented the fix, verified the required gates, and left the branch ready for review." \
  --files-changed 4
```

## Final Scoring And Report

Score the run before claiming success:

```bash
python3 .agents/skills/herald-autopilot/scripts/score_run.py \
  --run-dir ".superpowers/autopilot/runs/<run-id>"
```

Then render both the run summary and the human report:

```bash
python3 .agents/skills/herald-autopilot/scripts/render_report.py \
  --run-dir ".superpowers/autopilot/runs/<run-id>"
```

The report should make it easy for the user to answer:

- What was requested?
- What changed?
- Which gates passed, failed, or were skipped?
- What remains risky?
- Where is the worktree and branch?

## Evolving GEPA

When the user later asks to improve GEPA itself:

1. Read `docs/superpowers/gepa-evolution.md`.
2. Inspect the most recent relevant runs under `.superpowers/autopilot/runs/`.
3. Identify the single highest-value workflow bottleneck.
4. Propose and implement one focused workflow change.
5. Update the evolution doc with what changed, what improved, what still hurts, and what to try next.

v1 is intentionally a reflective single-run system. Do not introduce challenger worktrees or Pareto frontier selection unless the user asks for the next phase.
