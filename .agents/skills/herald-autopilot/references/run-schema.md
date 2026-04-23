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
- `baseline`: Result of pre-implementation baseline checks
- `plan`: Short summary, whether user questions were needed, and key decisions
- `product_truth`: Whether grounding was required, consulted sources, docs updated before code, and a short grounding summary
- `verification`: Required gates and observed results
- `metrics`: Retry count, diff stats, human follow-up flag
- `outcome`: Final outcome summary and remaining risks
- `latest_feedback`: Most recent reflection feedback strings

## `evidence/manifest.json`

JSON array of evidence objects. Each object should include:

- `id`
- `timestamp`
- `kind`: `command`, `screenshot`, `note`, `artifact`, `link`
- `summary`
- `status`: `pass`, `fail`, `info`, `skip`
- `gate`: Optional verification gate name
- `artifact`: Path to captured output, screenshot, or log

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
- baseline cleanliness
- retry efficiency
- human follow-up required
- diff size if known

The score document should expose both numeric values and natural-language reasons so later versions can rank candidates without losing interpretability.

## Future Compatibility

Phase 2 can add:

- multiple candidates per task
- challenger worktrees
- Pareto frontier metadata
- cross-run learned prompt updates

Do not rename existing fields for that phase. Add new names under `candidates`, `pareto`, or `optimizer`.
