# Herald GEPA Improvement Log

This document is the durable history of changes to the Herald autopilot workflow. It is designed to answer two questions quickly:

- Are we getting better?
- Do we have enough structured evidence to write about the approach later?

## Snapshot Table

| Logged At | Title | Status | Runs | Avg Score | Grounding | Failed Runs | Frontier |
|---|---|---:|---:|---:|---:|---:|---:|
| 2026-05-01T18:35:30 | Canonical visual-evidence gate for TUI runs | validated | 30 | 87.42857142857143 | 100% | 0 | 2 |
| 2026-05-01T17:21:43 | Workflow safety preflight and serialized artifact writes | validated | 30 | 86.46153846153847 | 100% | 0 | 2 |
| 2026-05-01T17:04:56 | Reusable remediation templates for repeated test failures | validated | 30 | 87.28571428571429 | 100% | 0 | 2 |
| 2026-05-01T16:47:32 | Visible post-publish self-reflection | validated | 30 | 87.28571428571429 | 100% | 0 | 2 |
| 2026-05-01T16:32:02 | Preserve GitHub issue references in autopilot handoffs | applied | 25 | 87.33333333333333 | 100% | 0 | 2 |
| 2026-04-24T02:01:12 | Show before/after screenshots for visual TUI runs | applied | 20 | 83.0 | 93% | 1 | 8 |
| 2026-04-23T19:00:19 | Product-definition grounding for GEPA | validated | 5 | 83.4 | 100% | 1 | 3 |
| 2026-04-23T18:42:12 | Improvement-history logging for GEPA | applied | 4 | 85.66666666666667 | n/a | 1 | 2 |
| 2026-04-23T18:42:12 | Herald Autopilot foundation | reconstructed | 4 | 85.66666666666667 | n/a | 1 | 2 |

## Entries

### Canonical visual-evidence gate for TUI runs

- Logged at: 2026-05-01T18:35:30+00:00
- Status: validated
- Kind: workflow-improvement
- Bottleneck: Recovered reflections kept showing that screenshot guidance was useful but optional, which made small-terminal regressions and repro-path drift easy to rediscover.
- Summary: Added a first-class visual-evidence gate so TUI-facing Herald autopilot runs must record matched before/after PNG plus ANSI captures at 220x50, 80x24, and 50x15, and surfaced that gate in scoring, reports, and optimizer summaries.

Metrics at log time:
- Recent runs: 30
- Average score: 87.42857142857143
- Average retries: 0.6666666666666666
- Failed runs: 0
- Frontier members: 2
- Product-truth required runs: 25
- Product-truth grounding rate: 1.0
- Product-truth updated-first runs: 17
- Preflight required runs: 1
- Preflight ready runs: 1
- Preflight readiness rate: 1.0
- Visual-required runs: 1
- Visual-ready runs: 1
- Visual readiness rate: 1.0
Delta from previous entry:
- recent_run_count: +0
- average_score: +0.9670329670329636
- average_retry_count: +0.0
- failed_run_count: +0
- frontier_count: +0
- product_truth_required_runs: -1
- product_truth_grounding_rate: +0.0
- product_truth_updated_first_runs: -1
- preflight_required_runs: +0
- preflight_ready_runs: +0
- preflight_readiness_rate: +0.0
Changes:
- Added a shared visual-evidence model plus record_visual_evidence.py for canonical TUI gate capture.
- Updated bootstrap, scoring, reports, and optimizer summaries to treat visual evidence as an explicit readiness axis.
- Validated a synthetic TUI run that closed the gate across all required sizes and rendered the new report section.
Recommended experiment at log time:
- `template-red-compose-comma-alias-feedback` (medium value, low risk)
Article notes:
- This phase turns screenshots from optional decoration into structured verification evidence, which is a cleaner story for a future article about agentic UI development.
- The gate also preserves repro-path knowledge, making visual verification more repeatable than ad hoc screenshots.
Follow-ups:
- Add the missing input-routing safety gate for shortcut and alias changes.
- Measure whether explicit visual gating reduces TUI retry count or post-handoff questions.

### Workflow safety preflight and serialized artifact writes

- Logged at: 2026-05-01T17:21:43+00:00
- Status: validated
- Kind: workflow-improvement
- Bottleneck: Recoverable reflections kept showing late docs dependency failures, fragile SSH host-key setup, media-batch restart pain, and direct evidence that nearby helper writes could clobber each other.
- Summary: Added a first-class preflight step for docs, SSH, and media-heavy runs, and hardened herald-autopilot artifact writes so run.json and evidence manifests survive concurrent helper activity.

Metrics at log time:
- Recent runs: 30
- Average score: 86.46153846153847
- Average retries: 0.6666666666666666
- Failed runs: 0
- Frontier members: 2
- Product-truth required runs: 26
- Product-truth grounding rate: 1.0
- Product-truth updated-first runs: 18
- Preflight required runs: 1
- Preflight ready runs: 1
- Preflight readiness rate: 1.0
- Visual-required runs: n/a
- Visual-ready runs: n/a
- Visual readiness rate: n/a
Delta from previous entry:
- recent_run_count: +0
- average_score: -0.8241758241758248
- average_retry_count: -0.10000000000000009
- failed_run_count: +0
- frontier_count: +0
- product_truth_required_runs: -2
- product_truth_grounding_rate: +0.0
- product_truth_updated_first_runs: -1
Changes:
- Added preflight_run.py to record docs dependency readiness, a run-local SSH host-key path, and a resumable media-batch state file before baseline verification.
- Added a shared artifact_io.py helper and moved run.json plus evidence-manifest writers to locked atomic writes.
- Extended reports, scores, optimizer summaries, and workflow docs with preflight readiness visibility.
Recommended experiment at log time:
- `template-red-compose-comma-alias-feedback` (medium value, low risk)
Article notes:
- This phase turns ambient setup assumptions into explicit measured state, which is a stronger story for workflow reliability than treating environment issues as random operator error.
- The concurrent-write validation gives us a concrete before/after systems lesson for a future article: reflection quality depends on artifact integrity.
Follow-ups:
- Convert visual-evidence and input-routing lessons into explicit scored gates for TUI-facing work.
- Decide whether the next GEPA pass should prioritize the phase backlog or the next uncovered failure template.

### Reusable remediation templates for repeated test failures

- Logged at: 2026-05-01T17:04:56+00:00
- Status: validated
- Kind: workflow-improvement
- Bottleneck: Repeated verification failures were still being rediscovered run by run even after we recovered the reflection history and wrote the consolidated improvement plan.
- Summary: Implemented the first remediation-template layer for herald-autopilot so repeated failures like focused-tests and app-tests map to reusable checklists in self-reflection reports, and the optimizer now looks past already-covered failure classes.

Metrics at log time:
- Recent runs: 30
- Average score: 87.28571428571429
- Average retries: 0.7666666666666667
- Failed runs: 0
- Frontier members: 2
- Product-truth required runs: 28
- Product-truth grounding rate: 1.0
- Product-truth updated-first runs: 19
- Preflight required runs: n/a
- Preflight ready runs: n/a
- Preflight readiness rate: n/a
- Visual-required runs: n/a
- Visual-ready runs: n/a
- Visual readiness rate: n/a
Delta from previous entry:
- recent_run_count: +0
- average_score: +0.0
- average_retry_count: +0.0
- failed_run_count: +0
- frontier_count: +0
- product_truth_required_runs: +0
- product_truth_grounding_rate: +0.0
- product_truth_updated_first_runs: +0
Changes:
- Added a remediation-template catalog for focused-tests, app-tests, app-package-tests, and diff-check.
- Updated report rendering so matched templates appear directly in self-reflection artifacts and human-readable reports.
- Updated the improvement brief logic so GEPA skips already-covered failure classes and recommends the next uncovered repeated failure.
Recommended experiment at log time:
- `template-red-compose-comma-alias-feedback` (medium value, low risk)
Article notes:
- This is the first real bridge from reflection mining to reusable workflow policy: the agent can now surface specific retry checklists instead of only saying that a pattern exists.
- The workflow is still user-controlled because template adoption is visible and approval-ready rather than silently self-modifying.
Follow-ups:
- Add a pending-approval queue for template adoption and expansion suggestions.
- Expand template coverage to the next uncovered repeated failure class, currently red-compose-comma-alias.

### Visible post-publish self-reflection

- Logged at: 2026-05-01T16:47:32+00:00
- Status: validated
- Kind: workflow-improvement
- Bottleneck: Normal runs produced useful reflections, but workflow-improvement suggestions stayed too hidden unless a separate explicit improve-GEPA pass happened.
- Summary: Extended herald-autopilot so requested commit, merge, push, and PR steps are recorded in the run, and the final report now includes a visible self-reflection section with approval-ready workflow suggestions instead of burying process learning in raw artifacts.

Metrics at log time:
- Recent runs: 30
- Average score: 87.28571428571429
- Average retries: 0.7666666666666667
- Failed runs: 0
- Frontier members: 2
- Product-truth required runs: 28
- Product-truth grounding rate: 1.0
- Product-truth updated-first runs: 19
- Preflight required runs: n/a
- Preflight ready runs: n/a
- Preflight readiness rate: n/a
- Visual-required runs: n/a
- Visual-ready runs: n/a
- Visual readiness rate: n/a
Delta from previous entry:
- recent_run_count: +5
- average_score: -0.04761904761903679
- average_retry_count: +0.00666666666666671
- failed_run_count: +0
- frontier_count: +0
- product_truth_required_runs: +5
- product_truth_grounding_rate: +0.0
- product_truth_updated_first_runs: +3
Changes:
- Added publication tracking to run metadata for commit, merge, push, and PR actions.
- Updated the report renderer to emit self_reflection.json and self_reflection.md alongside the main report.
- Embedded approval-ready GEPA suggestions directly into the final human-readable report after publish actions.
Recommended experiment at log time:
- `template-focused-tests-feedback` (medium value, low risk)
Article notes:
- Visibility matters as much as learning: a workflow that reflects privately but never surfaces suggested changes is hard to trust or improve collaboratively.
- This separates run-local self-reflection from approved cross-run GEPA changes, which is a cleaner story for a future article.
Follow-ups:
- Add a pending-approval queue that collects suggested GEPA changes across recent published runs.
- Consider a scored gate for missing publication summaries or missing post-publish reflections.

### Preserve GitHub issue references in autopilot handoffs

- Logged at: 2026-05-01T16:32:02+00:00
- Status: applied
- Kind: workflow-improvement
- Bottleneck: Issue #7 was completed and squash-merged locally, but the default-branch commit did not include a closing keyword, so GitHub could not automatically associate or close it from the commit history.
- Summary: Added a workflow rule so issue-backed Herald autopilot runs carry the GitHub issue reference into reports, branch commits, PR bodies, and default-branch squash commits; this prevents completed issue work from being detached from GitHub automation.

Metrics at log time:
- Recent runs: 25
- Average score: 87.33333333333333
- Average retries: 0.76
- Failed runs: 0
- Frontier members: 2
- Product-truth required runs: 23
- Product-truth grounding rate: 1.0
- Product-truth updated-first runs: 16
- Preflight required runs: n/a
- Preflight ready runs: n/a
- Preflight readiness rate: n/a
- Visual-required runs: n/a
- Visual-ready runs: n/a
- Visual readiness rate: n/a
Delta from previous entry:
- recent_run_count: +5
- average_score: +4.333333333333329
- average_retry_count: +0.56
- failed_run_count: -1
- frontier_count: -6
- product_truth_required_runs: +9
- product_truth_grounding_rate: +0.0714285714285714
- product_truth_updated_first_runs: +13
Changes:
- Added GitHub issue association guidance to the herald-autopilot skill.
- Added the same convention to the workflow contract reference.
- Updated the GEPA evolution ledger with the lesson and a future issue-linking validator experiment.
Recommended experiment at log time:
- `template-app-tests-feedback` (medium value, low risk)
Article notes:
- The best workflow memory is often a missing automation edge: one manual issue close exposed a policy that should live in the agent contract.
Follow-ups:
- Add a scored gate or helper that validates Refs/Closes/Fixes notation against the intake issue before commit, PR, or merge handoff.

### Show before/after screenshots for visual TUI runs

- Logged at: 2026-04-24T02:01:12+00:00
- Status: applied
- Kind: workflow-improvement
- Bottleneck: Visual TUI runs produced useful screenshots, but the workflow did not require matched before/after evidence or place those images directly in reports.
- Summary: Added explicit before/after screenshot capture guidance for visual Herald TUI changes and taught the autopilot report renderer to surface screenshot evidence as embedded Markdown images.

Metrics at log time:
- Recent runs: 20
- Average score: 83.0
- Average retries: 0.2
- Failed runs: 1
- Frontier members: 8
- Product-truth required runs: 14
- Product-truth grounding rate: 0.9285714285714286
- Product-truth updated-first runs: 3
- Preflight required runs: n/a
- Preflight ready runs: n/a
- Preflight readiness rate: n/a
- Visual-required runs: n/a
- Visual-ready runs: n/a
- Visual readiness rate: n/a
Delta from previous entry:
- recent_run_count: +15
- average_score: -0.4000000000000057
- average_retry_count: +0.0
- failed_run_count: +0
- frontier_count: +5
- product_truth_required_runs: +13
- product_truth_grounding_rate: -0.0714285714285714
- product_truth_updated_first_runs: +2
Changes:
- Documented matched before/after screenshot capture for visual TUI changes in the Herald autopilot skill and verification-routing reference.
- Updated render_report.py to read the evidence manifest and render Before/After PNG screenshots in a Visual Evidence section.
Recommended experiment at log time:
- `template-focused-tests-feedback` (medium value, low risk)
Article notes:
- Human preference exposed a valuable GEPA signal: visual diffs are part of the handoff contract, not merely optional evidence.
Follow-ups:
- Consider validating screenshot pair completeness as a scored visual-evidence gate for TUI tasks.

### Product-definition grounding for GEPA

- Logged at: 2026-04-23T19:00:19+00:00
- Status: validated
- Kind: workflow-improvement
- Bottleneck: Feature work could still drift because runs did not persist which product-definition docs were consulted before implementation.
- Summary: Grounded feature and behavior work in VISION.md, ARCHITECTURE.md, and repo specs, then extended run artifacts and optimizer summaries so GEPA can record whether a run was doc-first or guess-driven.

Metrics at log time:
- Recent runs: 5
- Average score: 83.4
- Average retries: 0.2
- Failed runs: 1
- Frontier members: 3
- Product-truth required runs: 1
- Product-truth grounding rate: 1.0
- Product-truth updated-first runs: 1
- Preflight required runs: n/a
- Preflight ready runs: n/a
- Preflight readiness rate: n/a
- Visual-required runs: n/a
- Visual-ready runs: n/a
- Visual readiness rate: n/a
Delta from previous entry:
- recent_run_count: +1
- average_score: -2.2666666666666657
- average_retry_count: -0.04999999999999999
- failed_run_count: +0
- frontier_count: +1
Changes:
- Added a product-truth reference and grounding rules to herald-autopilot.
- Extended run artifacts, reports, and scores with product-truth requirement and status fields.
- Added a product-truth snapshot and grounding metrics to optimizer state and the improvement log.
Recommended experiment at log time:
- `template-tui-checks-feedback` (medium value, low risk)
Article notes:
- This creates a measurable bridge between product-definition docs and agent execution instead of relying on code archaeology or screenshots.
- A future article can compare grounded vs ungrounded runs using retries, follow-up rate, and correction churn.
Follow-ups:
- Measure whether grounded runs show lower retry counts and fewer post-handoff clarifications.
- Add a spec template and doc-first gating for new feature requests.

### Improvement-history logging for GEPA

- Logged at: 2026-04-23T18:42:12+00:00
- Status: applied
- Kind: workflow-improvement
- Bottleneck: The workflow had a ledger and optimizer state, but no durable narrative of how the process itself improved over time.
- Summary: Added a dedicated improvement-history log that records workflow changes, metrics snapshots, deltas, article notes, and follow-ups.

Metrics at log time:
- Recent runs: 4
- Average score: 85.66666666666667
- Average retries: 0.25
- Failed runs: 1
- Frontier members: 2
- Product-truth required runs: n/a
- Product-truth grounding rate: n/a
- Product-truth updated-first runs: n/a
- Preflight required runs: n/a
- Preflight ready runs: n/a
- Preflight readiness rate: n/a
- Visual-required runs: n/a
- Visual-ready runs: n/a
- Visual readiness rate: n/a
Delta from previous entry:
- recent_run_count: +0
- average_score: +0.0
- average_retry_count: +0.0
- failed_run_count: +0
- frontier_count: +0
Changes:
- Added machine-readable improvement-log state under .superpowers/autopilot/state/.
- Added a rendered markdown timeline at docs/superpowers/gepa-improvement-log.md.
- Updated the GEPA-improvement workflow so every future improvement can log itself.
Recommended experiment at log time:
- `auto-ledger-and-state-sync` (high value, low risk)
Article notes:
- A good research/dev workflow needs both raw traces and a story of how the method changed.
- This log is the bridge from internal optimization to publishable methodology.
Follow-ups:
- Log each future improve-GEPA change as a separate entry with evidence and deltas.

### Herald Autopilot foundation

- Logged at: 2026-04-23T18:42:12+00:00
- Status: reconstructed
- Kind: workflow-improvement
- Bottleneck: The workflow needed durable structure before it could learn from its own runs.
- Summary: Established the initial repo-local autopilot workflow with run artifacts, scoring, reflection, reports, and the first GEPA optimizer state.

Metrics at log time:
- Recent runs: 4
- Average score: 85.66666666666667
- Average retries: 0.25
- Failed runs: 1
- Frontier members: 2
- Product-truth required runs: n/a
- Product-truth grounding rate: n/a
- Product-truth updated-first runs: n/a
- Preflight required runs: n/a
- Preflight ready runs: n/a
- Preflight readiness rate: n/a
- Visual-required runs: n/a
- Visual-ready runs: n/a
- Visual readiness rate: n/a
Changes:
- Created the Herald autopilot skill and run-artifact schema.
- Added scoring, reflection, report rendering, and the first optimizer state layer.
Recommended experiment at log time:
- `auto-ledger-and-state-sync` (high value, low risk)
Article notes:
- The first milestone was not autonomy for its own sake; it was making every run legible enough to learn from.
Follow-ups:
- Measure real-task behavior now that the workflow has durable state.

