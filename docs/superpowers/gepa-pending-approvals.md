# Herald GEPA Pending Approvals

This document is the visible approval backlog for workflow suggestions recovered from published Herald autopilot runs. It turns scattered post-publish self-reflection into one place where approvals, rejections, and implemented ideas can be reviewed without digging through run folders.

## Snapshot

- Updated at: 2026-05-01T22:30:48+00:00
- Published runs analyzed: 2
- Total queue items: 3
- Pending: 3
- Approved: 0
- Rejected: 0
- Implemented: 0

## How To Update

- Sync from the latest published reflections: `python3 .agents/skills/herald-autopilot/scripts/sync_pending_approvals.py --repo-root .`
- Approve one or more items: `python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py --repo-root . --status approved --key <queue-key>`
- Batch-approve everything still pending: `python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py --repo-root . --status approved --all-pending`

## Pending Items

### template-red-compose-comma-alias-feedback

- Queue key: `template-red-compose-comma-alias-feedback-f72a614404`
- Status: pending
- Seen in runs: 2
- First seen: 2026-05-01T16:45:45+00:00
- Last seen: 2026-05-01T16:55:28+00:00
- Publish actions: commit
- Why: The next best improvement is to extend template coverage to the next repeated failure class instead of re-implementing an existing template.
- Approval prompt: Approve exploring `template-red-compose-comma-alias-feedback` as the next explicit GEPA improvement pass.
- Source runs:
- `20260501-095528-issue-16-support-japanese-ime-composition-for-layout-independent-shortcuts-https-github-com-herald-email-herald-mail-app-issues-16` at 2026-05-01T16:55:28+00:00 via commit
- `20260501-self-reflection-validation` at 2026-05-01T16:45:45+00:00 via commit

### Focused test remediation template

- Queue key: `focused-test-remediation-template-f643092950`
- Status: pending
- Seen in runs: 1
- First seen: 2026-05-01T16:45:45+00:00
- Last seen: 2026-05-01T16:45:45+00:00
- Publish actions: commit
- Why: Repeated run history shows focused test failures usually come from stale expectations, overspecified assertions, or missing adjacent regression coverage rather than the core feature direction being wrong.
- Approval prompt: Approve keeping the focused-tests remediation template as a default GEPA retry aid.
- Source runs:
- `20260501-self-reflection-validation` at 2026-05-01T16:45:45+00:00 via commit

### Require doc-first feature grounding

- Queue key: `require-doc-first-feature-grounding-ef7c7bc4af`
- Status: pending
- Seen in runs: 1
- First seen: 2026-05-01T16:45:45+00:00
- Last seen: 2026-05-01T16:45:45+00:00
- Publish actions: commit
- Why: Feature work is safer when VISION, ARCHITECTURE, and specs are updated before code rather than only consulted during implementation.
- Approval prompt: Approve a stricter doc-first gate for non-trivial feature runs.
- Source runs:
- `20260501-self-reflection-validation` at 2026-05-01T16:45:45+00:00 via commit

## Approved Items

- No approved items.

## Rejected Items

- No rejected items.

## Implemented Items

- No implemented items.
