# Herald GEPA Evolution Ledger

This document is the human-facing memory for the repo-local Herald autopilot workflow. It complements the raw run folders under `.superpowers/autopilot/runs/` by keeping a curated record of what the workflow does today, what has been learned so far, and what to improve next.

Related docs:

- Improvement history: [gepa-improvement-log.md](gepa-improvement-log.md)
- Consolidated improvement plan: [gepa-consolidated-improvement-plan.md](gepa-consolidated-improvement-plan.md)
- Pending approvals: [gepa-pending-approvals.md](gepa-pending-approvals.md)
- Phase impact: [gepa-phase-impact.md](gepa-phase-impact.md)
- Product truth snapshot: `.superpowers/autopilot/state/product-truth.md`

## Current Workflow

This section describes the current behavior that future sessions should treat as the stable baseline. The goal is to make it easy to answer "what does GEPA do right now?" without digging through scripts or old reports.

- [x] `herald-autopilot` is a repo-local skill under `.agents/skills/` for one Herald task per invocation.
- [x] The workflow bootstraps a durable run folder under `.superpowers/autopilot/runs/<run-id>/` before doing significant work.
- [x] The workflow uses a single worktree and a single candidate branch in v1.
- [x] The workflow treats a branch in the main checkout as insufficient isolation for implementation work; tracked-file edits belong in `.worktrees/...` unless the user explicitly opts out.
- [x] The workflow records evidence, reflections, scores, and a human-readable report.
- [x] The default finish line is branch + worktree + report, not push, PR, or merge.
- [x] Verification is impact-based: code-only tasks stay focused, while TUI, SSH, and MCP checks are added only when the task touches those surfaces.
- [x] Explicit "improve GEPA" work now has a dedicated optimizer layer that summarizes recent runs, builds a lightweight frontier, extracts feedback patterns, and syncs the ledger snapshot.
- [x] Improvement work can now append a structured history entry and render a publication-friendly change log.
- [x] Product behavior changes are now meant to ground themselves in `VISION.md`, `ARCHITECTURE.md`, and real specs before implementation.
- [x] Run artifacts and optimizer summaries can now record whether product-truth grounding was required, whether docs were updated first, and how often grounded runs occur.
- [x] GitHub issue-backed runs now preserve issue references in commits and PR/merge bodies so GitHub can cross-reference or auto-close completed issues.
- [x] Requested commit, merge, push, and PR steps can now be recorded in run metadata, and final reports now include a visible self-reflection section with approval-ready workflow suggestions.
- [x] Post-publish self-reflection suggestions can now sync into a visible pending-approval queue with stable queue keys and batch-approval commands.
- [x] Repeated failure classes can now match reusable remediation templates, and self-reflection reports surface those checklists directly for `focused-tests`, `app-tests`, `app-package-tests`, and `diff-check`.
- [x] Docs, SSH, and media-heavy runs can now execute a first-class preflight step that records prerequisites and prepared resources before baseline verification starts.
- [x] Run metadata and evidence manifests now use serialized helper writes so nearby workflow steps do not clobber each other.
- [x] TUI-facing runs can now close a first-class visual-evidence gate that requires matched before/after PNG plus ANSI captures at `220x50`, `80x24`, and `50x15`.
- [x] Shortcut-sensitive TUI runs can now close a first-class input-routing safety gate that proves text entry still works on `compose`, `prompt`, and `editor` surfaces.
- [x] A dedicated phase-impact report can now compare baseline runs with post-improvement cohorts, while separating workflow-validation evidence from real bug/feature evidence.

## What Changed In This Version

This section records the current bootstrap milestone so later sessions can compare the workflow after real runs accumulate. Each item should describe a durable capability, not one-off implementation noise.

- [x] Added the `herald-autopilot` skill with explicit instructions for planning, worktree setup, verification routing, reflection, scoring, and report rendering.
- [x] Added helper scripts for run bootstrap, evidence capture, reflection capture, run scoring, and final report generation.
- [x] Added run schema and workflow reference docs inside the skill so future sessions can load only the relevant details.
- [x] Established this living ledger as the canonical entrypoint for future "improve GEPA" work.
- [x] Seeded three validation runs: a successful bootstrap run, a failed TUI-path run, and a workflow-tuning run.
- [x] Added an optimizer state layer under `.superpowers/autopilot/state/` plus helper scripts for recent-run analysis, frontier building, feedback-pattern extraction, improvement-brief generation, and auto-synced ledger snapshots.
- [x] Added a dedicated improvement-history log so GEPA changes can be tracked over time with metrics, deltas, article notes, and follow-ups.
- [x] Added a product-truth grounding layer so GEPA can treat `VISION.md`, `ARCHITECTURE.md`, and spec docs as the product-definition source of truth.
- [x] Added a GitHub issue association rule after issue #7 was completed locally without a closing keyword in the squash commit.
- [x] Added publish-action tracking plus self-reflection artifacts so normal feature runs can surface suggested GEPA changes without silently changing the workflow.
- [x] Added a cross-run pending-approval queue so published runs contribute to one visible approval backlog instead of leaving GEPA suggestions scattered across per-run reports.
- [x] Added an initial remediation-template layer so repeated verification failures can reuse checklists instead of rediscovering the same retry strategy in each run.
- [x] Added workflow-safety infrastructure with explicit preflight checks and locked artifact writes for `run.json` and `evidence/manifest.json`.
- [x] Added a scored visual-evidence gate so TUI runs must record canonical terminal captures and repro paths instead of treating screenshots as optional.
- [x] Added a scored input-routing safety gate plus a reusable template for `red-compose-comma-alias` so shortcut-sensitive TUI work has explicit text-entry proof and reusable recovery guidance.
- [x] Added a phase-impact measurement layer that renders a durable report comparing retries, skipped gates, and clarification load before and after the first four workflow improvements.
- [x] Tightened the worktree policy after the adaptive terminal-theme implementation used a branch in the main checkout and blocked parallel task setup.

## Run Patterns Observed

This section should summarize recurring themes across recent runs. At bootstrap time there is almost no empirical history yet, so the current notes are intentionally provisional and should be replaced with real observations after the first few task runs.

- [x] Initial bootstrap indicates the repo already supports the required storage layout because `.worktrees/`, `.superpowers/`, and `reports/` are available and ignored.
- [x] The repo has strong surface-specific verification docs already, especially for TUI, SSH, and MCP checks.
- [x] Validation history now includes one successful code-oriented run, one failed TUI-path reflection run, and one workflow-tuning ledger run.
- [x] The first phase-impact report shows lower retries, skipped gates, and clarification touches in the post-Phase 1 cohort, but every post-Phase 1 run so far is still a workflow-improvement validation run.
- [ ] We still do not have post-Phase 1 bug or feature runs, so the current measurement pass is directional rather than strong real-task evidence.

## Auto Snapshot

This section is generated from the optimizer state under `.superpowers/autopilot/state/`. It should stay machine-updated so future sessions can see the current run picture and top recommendation without reading every raw artifact.

<!-- AUTOGEN:BEGIN -->
- [x] Auto snapshot generated at 2026-05-01T22:30:48+00:00.
- [x] Recent runs analyzed: 30.
- [x] Frontier members available: 2.
- [x] Most repeated failing evidence: `focused-tests` (3 occurrences).
- [x] Current top recommended experiment: `template-user-repro-after-ed02a1d-feedback` (medium value, low risk).
- [x] Pending-approval queue: 3 pending, 0 approved, 0 implemented.
- [x] Phase-impact report: 0 post-Phase 1 real bug/feature run(s) measured so far.
<!-- AUTOGEN:END -->

## Known Weaknesses And Pain Points

This section should stay honest about what still hurts. Items remain unchecked until the weakness is materially addressed and validated in later runs.

- [ ] v1 only supports reflective single-run optimization; it does not explore challenger worktrees or a Pareto frontier yet.
- [ ] The workflow does not self-edit its own prompts or policies based on accumulated traces.
- [ ] Cross-run learning is still mediated by this ledger and human judgment rather than automated frontier selection.
- [ ] Verification routing is documented, but its real cost and false-positive rate are not yet measured across multiple tasks.
- [ ] The helper scripts produce durable artifacts, but they do not yet enforce every status transition automatically.
- [ ] If another agent is actively using `herald-autopilot`, breaking changes to the core execution helpers are still risky and should be staged additively first.
- [ ] The workflow still needs empirical proof that grounding on product docs reduces feature drift on real tasks.
- [ ] The workflow does not yet enforce issue-reference notation mechanically; future helpers could validate commit messages, PR bodies, and reports against the intake issue.
- [ ] The queue is visible now, but approvals are still user-driven decisions rather than automatically turning into measured workflow changes.
- [ ] The current phase-impact report is honest but sample-starved: it has no post-Phase 1 bug or feature runs yet, so it cannot prove real-task gains from the recent workflow changes.
- [ ] Non-autopilot implementation requests can still bypass this repo-local skill unless future sessions load the worktree policy before editing; a lightweight repo-level reminder may be needed outside GEPA.

## Candidate Next Experiments

This section ranks the most valuable next improvements so a future session can start from a crisp backlog. Keep the list short and ordered by likely payoff, not by novelty.

- [ ] Add challenger worktrees for the highest-risk tasks and compare candidates on verification completeness, retry count, and handoff readiness.
- [ ] Derive a lightweight Pareto frontier from recent runs so later candidate selection is grounded in actual repo experience.
- [ ] Auto-summarize recent run folders into this ledger after each meaningful task to reduce manual curation.
- [ ] Measure verification cost by surface so the skill can choose between focused and broad gates more intelligently.
- [ ] Learn common failure-mode prompts from repeated reflections and use them as reusable feedback templates.
- [x] Learned and codified reusable feedback templates for the most repeated current verification failures.
- [x] Added workflow preflight plus serialized artifact writes to catch environment blockers before feature-level verification begins.
- [x] Added a pending-approval queue that consolidates post-publish self-reflection suggestions across runs so the user can batch-approve GEPA changes.
- [x] Added a first phase-impact report so retry, skip, and clarification trends are at least measured before more autonomy is added.
- [ ] Measure whether updating product-definition docs first reduces rework on feature implementation runs.
- [ ] Add a scored issue-linking gate that checks `Refs #N` for branch handoff and `Closes #N` / `Fixes #N` for PR or default-branch completion.
- [ ] Measure whether the visual and input-routing gates reduce TUI retry count and post-handoff clarification load enough to justify stricter automatic enforcement.
- [ ] Collect and measure the next 3 to 5 real bug or feature autopilot runs so the phase-impact report stops relying on workflow-validation-only evidence.
- [ ] Add a pre-edit worktree gate that checks `pwd` and `git worktree list` before tracked-file edits, and records an explicit opt-out when working in the main checkout.

## Ask Me Next

This section is the handoff bridge for future sessions. Each prompt should be phrased so you can point me here later and immediately continue improving the workflow.

- [ ] "Improve GEPA by reducing verification cost for code-only tasks while keeping handoff confidence high."
- [ ] "Improve GEPA by adding challenger worktrees for tasks with repeated reflection failures."
- [ ] "Improve GEPA by summarizing the last three runs and updating the ranked experiment list."
- [ ] "Improve GEPA by tightening the run schema and removing fields we never actually use."
- [ ] "Improve GEPA by adding an issue-reference validator before commit, PR, or merge handoff."
- [ ] "Improve GEPA by reviewing the pending-approval queue and turning the approved items into tracked workflow changes."
- [ ] "Improve GEPA by measuring whether the pending-approval queue reduced follow-up questions and hidden process drift."
- [ ] "Improve GEPA by collecting enough real bug and feature runs to turn the current phase-impact report into stronger evidence."
