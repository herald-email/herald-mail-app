# Run Schema

The run folder is designed for replay first and optimization second. The schema needs to be understandable by both humans and scripts.

## `run.json`

Top-level fields:

- `schema_version`: Schema identifier for forward compatibility
- `run_id`: Stable identifier for the run folder
- `created_at`, `updated_at`
- `status`: Current lifecycle state
- `mode`: `reflective-single-run`
- `task`: Request, type, slug, and affected surfaces
- `paths`: Repo root, worktree, branch, report path, evolution doc path
- `policy`: Approval mode, verification mode, retry limit
- `preflight`: Required environment checks, latest results, and prepared resources such as run-local SSH host-key paths or resumable media-batch state
- `baseline`: Result of pre-implementation baseline checks
- `plan`: Short summary, whether user questions were needed, and key decisions
- `product_truth`: Whether grounding was required, consulted sources, docs updated before code, and a short grounding summary
- `publication`: Requested publish actions that were actually performed plus a short summary
- `visual_evidence`: Whether the canonical visual gate is required, its current status, required terminal sizes, and the recorded before/after pairs
- `input_routing`: Whether the input-routing safety gate is required, its current status, required text-entry surfaces, and the recorded per-surface routing checks
- `verification`: Required gates and observed results
- `metrics`: Retry count, diff stats, human follow-up flag
- `outcome`: Final outcome summary and remaining risks
- `latest_feedback`: Most recent reflection feedback strings

## `run.json.visual_evidence`

Use this block to make TUI-facing verification explicit instead of implicit. It should track whether the run owes canonical visual evidence and whether that gate is actually closed.

- `required`: Whether the task must close the visual-evidence gate
- `status`: `pending`, `passed`, or `not-needed`
- `required_sizes`: Canonical terminal sizes such as `220x50`, `80x24`, and `50x15`
- `pairs`: Recorded evidence pairs, each including `state_label`, `size`, before/after PNG paths, before/after ANSI-text paths, repro steps, snapshot-fidelity notes, and completion issues if any remain

## `run.json.input_routing`

Use this block to make shortcut and alias safety explicit whenever a TUI task changes keyboard dispatch. It should record which text-entry surfaces were exercised and whether normal typing stayed intact.

- `required`: Whether the run must close the input-routing safety gate
- `status`: `pending`, `passed`, or `not-needed`
- `required_surfaces`: Canonical text-entry surfaces such as `compose`, `prompt`, and `editor`
- `checks`: Recorded routing checks, each including `surface`, `input_sequence`, expected and observed behavior, the proving artifact path, whether text was preserved, repro steps, and completion issues if any remain

## `evidence/manifest.json`

JSON array of evidence objects. Each object should include:

- `id`
- `timestamp`
- `kind`: `command`, `screenshot`, `note`, `artifact`, `link`
- `summary`
- `status`: `pass`, `fail`, `info`, `skip`
- `gate`: Optional verification gate name
- `artifact`: Path to captured output, screenshot, or log

## `preflight.json`

Mirror of the latest `run.json.preflight` block so a future session can inspect environment readiness without diffing the full run document. It should include:

- `status`
- `required_checks`
- `results`
- `resources`

## `reflections/<attempt>.json`

Each reflection document should include:

- `attempt`
- `timestamp`
- `failing_evidence`
- `hypothesis`
- `next_step`
- `decision`: `continue` or `stop`
- `feedback`: Array of natural-language lessons for the next retry

## `score.json`

Scoring is intentionally simple in v1:

- gate completeness
- preflight readiness
- visual-evidence readiness
- input-routing readiness
- baseline cleanliness
- retry efficiency
- human follow-up required
- diff size if known

The score document should expose both numeric values and natural-language reasons so later versions can rank candidates without losing interpretability.

## `self_reflection.json` and `self_reflection.md`

When a run reaches final reporting, the renderer should also produce a self-reflection artifact that summarizes:

- what worked well in the run
- what created drag or uncertainty
- any requested publish actions that were performed
- approval-ready GEPA or workflow changes suggested by the run

These suggestions are advisory until the user explicitly approves an improvement pass.

Each suggested change should include a stable `queue_key` so later queue syncs can deduplicate the same lesson across multiple published runs without guessing from prose alone.

When a run matches a reusable remediation template, the self-reflection artifact should record that matched template and its checklist so future sessions can recover the lesson without rereading every reflection file.

## `pending-approvals.json` and `gepa-pending-approvals.md`

These cross-run artifacts turn post-publish reflection suggestions into a reviewable backlog instead of leaving them buried in individual reports. They should live under `.superpowers/autopilot/state/pending-approvals.json` and `docs/superpowers/gepa-pending-approvals.md`.

The queue should track:

- stable suggestion keys
- current approval status such as `pending`, `approved`, `rejected`, or `implemented`
- first and last seen timestamps
- occurrence count across published runs
- source run references
- the user-facing approval prompt and the decision note when one exists

## Future Compatibility

Later phases can add:

- multiple candidates per task
- challenger worktrees
- Pareto frontier metadata
- cross-run learned prompt updates

Do not rename existing fields for that phase. Add new names under `candidates`, `pareto`, or `optimizer`.
