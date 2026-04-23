# Herald GEPA Evolution Ledger

This document is the human-facing memory for the repo-local Herald autopilot workflow. It complements the raw run folders under `.superpowers/autopilot/runs/` by keeping a curated record of what the workflow does today, what has been learned so far, and what to improve next.

## Current Workflow

This section describes the current behavior that future sessions should treat as the stable baseline. The goal is to make it easy to answer "what does GEPA do right now?" without digging through scripts or old reports.

- [x] `herald-autopilot` is a repo-local skill under `.agents/skills/` for one Herald task per invocation.
- [x] The workflow bootstraps a durable run folder under `.superpowers/autopilot/runs/<run-id>/` before doing significant work.
- [x] The workflow uses a single worktree and a single candidate branch in v1.
- [x] The workflow records evidence, reflections, scores, and a human-readable report.
- [x] The default finish line is branch + worktree + report, not push, PR, or merge.
- [x] Verification is impact-based: code-only tasks stay focused, while TUI, SSH, and MCP checks are added only when the task touches those surfaces.

## What Changed In This Version

This section records the current bootstrap milestone so later sessions can compare the workflow after real runs accumulate. Each item should describe a durable capability, not one-off implementation noise.

- [x] Added the `herald-autopilot` skill with explicit instructions for planning, worktree setup, verification routing, reflection, scoring, and report rendering.
- [x] Added helper scripts for run bootstrap, evidence capture, reflection capture, run scoring, and final report generation.
- [x] Added run schema and workflow reference docs inside the skill so future sessions can load only the relevant details.
- [x] Established this living ledger as the canonical entrypoint for future "improve GEPA" work.
- [x] Seeded three validation runs: a successful bootstrap run, a failed TUI-path run, and a workflow-tuning run.

## Run Patterns Observed

This section should summarize recurring themes across recent runs. At bootstrap time there is almost no empirical history yet, so the current notes are intentionally provisional and should be replaced with real observations after the first few task runs.

- [x] Initial bootstrap indicates the repo already supports the required storage layout because `.worktrees/`, `.superpowers/`, and `reports/` are available and ignored.
- [x] The repo has strong surface-specific verification docs already, especially for TUI, SSH, and MCP checks.
- [x] Validation history now includes one successful code-oriented run, one failed TUI-path reflection run, and one workflow-tuning ledger run.
- [ ] No meaningful production bug or feature history has been recorded yet beyond bootstrap validation.
- [ ] No empirical comparison of retry patterns or verification cost has been recorded yet across real tasks.

## Known Weaknesses And Pain Points

This section should stay honest about what still hurts. Items remain unchecked until the weakness is materially addressed and validated in later runs.

- [ ] v1 only supports reflective single-run optimization; it does not explore challenger worktrees or a Pareto frontier yet.
- [ ] The workflow does not self-edit its own prompts or policies based on accumulated traces.
- [ ] Cross-run learning is still mediated by this ledger and human judgment rather than automated frontier selection.
- [ ] Verification routing is documented, but its real cost and false-positive rate are not yet measured across multiple tasks.
- [ ] The helper scripts produce durable artifacts, but they do not yet enforce every status transition automatically.

## Candidate Next Experiments

This section ranks the most valuable next improvements so a future session can start from a crisp backlog. Keep the list short and ordered by likely payoff, not by novelty.

- [ ] Add challenger worktrees for the highest-risk tasks and compare candidates on verification completeness, retry count, and handoff readiness.
- [ ] Derive a lightweight Pareto frontier from recent runs so later candidate selection is grounded in actual repo experience.
- [ ] Auto-summarize recent run folders into this ledger after each meaningful task to reduce manual curation.
- [ ] Measure verification cost by surface so the skill can choose between focused and broad gates more intelligently.
- [ ] Learn common failure-mode prompts from repeated reflections and use them as reusable feedback templates.

## Ask Me Next

This section is the handoff bridge for future sessions. Each prompt should be phrased so you can point me here later and immediately continue improving the workflow.

- [ ] "Improve GEPA by reducing verification cost for code-only tasks while keeping handoff confidence high."
- [ ] "Improve GEPA by adding challenger worktrees for tasks with repeated reflection failures."
- [ ] "Improve GEPA by summarizing the last three runs and updating the ranked experiment list."
- [ ] "Improve GEPA by tightening the run schema and removing fields we never actually use."
