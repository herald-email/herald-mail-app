# Herald GEPA Consolidated Improvement Plan

This document consolidates recoverable workflow lessons from Herald autopilot run artifacts. It turns scattered `reflections/*.json`, `latest_feedback`, and the first formal `self_reflection.json` artifact into one ranked approval backlog for future GEPA work.

## Recovery Summary

This section records what recoverable self-reflection data exists right now and why it is trustworthy enough to drive workflow improvements. The goal is to separate missing instrumentation from real repeated friction in the current process.

- [x] Recovered feedback from 25 runs with reflection files, 31 reflection records, 23 runs with `latest_feedback`, and 1 formal post-publish `self_reflection.json`.
- [x] Confirmed that older runs still preserve useful self-reflection signal even when they predate the dedicated post-publish self-reflection artifact.
- [x] Verified that the current top repeated failing evidence is `focused-tests`, with `app-tests` and `app-package-tests` as the next most common verification failures.
- [x] Grouped the recoverable lessons into five repeatable themes: verification templates, visual evidence fidelity, environment preflight, artifact serialization, and input-routing safety.

## Recovered Themes

This section captures the recurring problems that appeared often enough to justify a consolidated plan. Each item is written as a concrete workflow lesson rather than a vague category so future GEPA passes can implement it directly.

- [x] Verification-template gaps are the strongest repeated signal: `focused-tests` appeared 4 times, `app-tests` appeared 2 times, and `app-package-tests` appeared 1 time in recoverable reflection evidence.
- [x] Visual evidence remains expensive to rediscover: repeated reflections mention native-terminal screenshot fidelity, 50x15 layout guards, trailing-space preservation in terminal goldens, and exact repro-path capture.
- [x] Environment preflight still causes avoidable retries: recoverable reflections mention missing `docs/node_modules`, fragile SSH host-key paths, and long-running media jobs that need resumable batching.
- [x] Artifact serialization is still fragile: recoverable reflections already warned against concurrent `capture_evidence.py` writes, and recent validation also showed that concurrent `run.json` updates can clobber each other.
- [x] Input-routing safety needs explicit protection: recovered feedback showed that alias or shortcut changes can steal normal text entry when they are not scoped away from compose or prompt surfaces.

## Prioritized Improvement Backlog

This section ranks the workflow changes that would pay down the most repeated friction first. The order favors fixes that should reduce retries across many future runs before more ambitious GEPA search features are added.

- [x] Built reusable remediation templates for `focused-tests`, `app-tests`, `app-package-tests`, and `diff-check` so repeated failures can surface standardized next steps instead of bespoke retry reasoning.
- [x] Added workflow preflight checks for docs dependencies, SSH host-key paths, and resumable long-running media batches so environment failures are caught before feature-level verification starts.
- [x] Serialized `run.json` and evidence-manifest writes so helpers cannot clobber each other when multiple workflow steps run close together.
- [x] Promoted native-terminal repro paths, small-terminal guards, and canonical before/after capture requirements into an explicit visual-evidence gate for TUI-facing work.
- [ ] Add an input-routing safety gate that requires shortcut and alias changes to prove they do not steal text entry on compose, prompt, or editor surfaces.
- [ ] Build a pending-approval queue that collects post-publish self-reflection suggestions across runs so the user can review and batch-approve GEPA changes.

## Execution Order

This section sequences the backlog into a practical rollout. The idea is to stabilize the most repeated sources of wasted effort first, then improve visibility and approval flow, and only after that consider more autonomous GEPA behavior.

- [x] Phase 1: shipped `template-focused-tests-feedback` and `template-app-tests-feedback`, along with adjacent `app-package-tests` and `diff-check` coverage, because verification-template failures were the densest repeated signal in recovered reflections.
- [x] Phase 2: hardened workflow infrastructure with serialized artifact writes and preflight environment checks because those failures waste time before feature-level learning begins.
- [ ] Phase 3: finish the scored TUI-safety gate set by pairing the completed visual-evidence gate with the still-missing input-routing safety gate.
- [ ] Phase 4: add the pending-approval queue so post-publish self-reflection becomes a reviewable backlog instead of scattered report text.
- [ ] Phase 5: measure whether the first four phases reduce retry count, skipped gates, and manual clarification load before expanding into challenger worktrees or autonomous self-editing.

## Approval Prompts

This section converts the plan into direct asks that future sessions can execute without extra archaeology. Each prompt is meant to be approved or deferred explicitly so GEPA remains visible and user-controlled.

- [ ] Approve a verification-template pass focused on `focused-tests` and `app-tests`.
- [x] Approved and completed a workflow-safety pass focused on serialized writes and preflight checks.
- [x] Approved and completed a visual-evidence gate pass focused on canonical TUI before/after capture at `220x50`, `80x24`, and `50x15`.
- [ ] Approve a visibility pass that adds a pending-approval queue for post-publish reflection suggestions.
- [ ] Approve a measurement pass that compares retry counts and follow-up load before and after the first three improvements.
