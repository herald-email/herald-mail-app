---
name: approve-gepa-changes
description: Use when approving, rejecting, or marking implemented Herald GEPA pending approval items, including requests like "approve GEPA changes", "approve all pending GEPA", or approval of a specific queue key from docs/superpowers/gepa-pending-approvals.md.
---

# Approve GEPA Changes

Use this skill for Herald's GEPA approval lane. Approval records a user's decision about workflow suggestions recovered from autopilot self-reflection; it does not implement the approved improvement unless the user separately asks for implementation.

## Sources

Use the existing Herald autopilot queue tooling:

- Visible backlog: `docs/superpowers/gepa-pending-approvals.md`
- State file: `.superpowers/autopilot/state/pending-approvals.json`
- Sync script: `.agents/skills/herald-autopilot/scripts/sync_pending_approvals.py`
- Decision script: `.agents/skills/herald-autopilot/scripts/update_pending_approvals.py`

Do not manually edit the state JSON or generated markdown queue except to repair tooling after a script failure.

## Preflight

Run from the repo root:

```bash
git status --short
git diff -- docs/superpowers/gepa-pending-approvals.md
```

If `docs/superpowers/gepa-pending-approvals.md` is already dirty, inspect the diff before running queue scripts. Proceed when it is only a generated queue refresh. Ask before overwriting if it contains unrelated hand-written edits.

Refresh the queue from published run reflections:

```bash
python3 .agents/skills/herald-autopilot/scripts/sync_pending_approvals.py --repo-root .
sed -n '1,140p' docs/superpowers/gepa-pending-approvals.md
```

Summarize the current pending items by title, queue key, why, approval prompt, and number of source runs.

## Choosing Targets

Interpret user intent conservatively:

- If the user gives a queue key, update that key.
- If the user says "approve all pending", use `--all-pending`.
- If the user says "approve GEPA changes" and exactly one item is pending, approve that one.
- If multiple items are pending and the user did not say "all", show the pending list and ask which keys to approve.
- Use `rejected`, `implemented`, or `pending` only when the user explicitly asks for that status.

## Apply Decision

Approve specific keys:

```bash
python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py \
  --repo-root . \
  --status approved \
  --key "<queue-key>" \
  --note "Approved by user: <brief source phrase>"
```

Approve every pending item:

```bash
python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py \
  --repo-root . \
  --status approved \
  --all-pending \
  --note "Approved by user: <brief source phrase>"
```

For other explicit decisions, replace `approved` with `rejected`, `implemented`, or `pending`. Keep notes short and factual.

## Verify

Read back the updated queue:

```bash
sed -n '1,180p' docs/superpowers/gepa-pending-approvals.md
git diff -- docs/superpowers/gepa-pending-approvals.md
```

Confirm:

- selected keys moved to the requested status section
- snapshot counts changed as expected
- decision note appears when the status is not `pending`
- no unrelated files were modified except the generated `.superpowers` state file and `docs/superpowers/gepa-pending-approvals.md`

Do not commit approval changes unless the user asks.

## Final Response

Report:

- approved, rejected, implemented, or reset queue keys
- updated pending and approved counts
- files changed by the queue tooling
- whether a commit was created
