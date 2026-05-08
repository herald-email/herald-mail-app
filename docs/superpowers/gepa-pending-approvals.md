# Herald GEPA Pending Approvals

This document is the visible approval backlog for workflow suggestions recovered from published Herald autopilot runs. It turns scattered post-publish self-reflection into one place where approvals, rejections, and implemented ideas can be reviewed without digging through run folders.

## Snapshot

- Updated at: 2026-05-07T23:51:13+00:00
- Published runs analyzed: 11
- Total queue items: 6
- Pending: 1
- Approved: 0
- Rejected: 0
- Implemented: 5

## How To Update

- Sync from the latest published reflections: `python3 .agents/skills/herald-autopilot/scripts/sync_pending_approvals.py --repo-root .`
- Approve one or more items: `python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py --repo-root . --status approved --key <queue-key>`
- Batch-approve everything still pending: `python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py --repo-root . --status approved --all-pending`

## Pending Items

### template-green-demo-key-overlay-app-attempt1-feedback

- Queue key: `template-green-demo-key-overlay-app-attempt1-feedback-b19c0ba4a6`
- Status: pending
- Seen in runs: 5
- First seen: 2026-05-07T19:30:58+00:00
- Last seen: 2026-05-07T23:24:40+00:00
- Publish actions: commit, merge, worktree-delete
- Why: The next best improvement is to extend template coverage to the next repeated failure class instead of re-implementing an existing template.
- Approval prompt: Approve exploring `template-green-demo-key-overlay-app-attempt1-feedback` as the next explicit GEPA improvement pass.
- Source runs:
- `20260507-162440-centralize-tui-color-hues-in-internal-app-theme-roles` at 2026-05-07T23:24:40+00:00 via commit, merge
- `20260507-162311-check-changes-between-the-most-recent-tag-and-head-and-update-documentation-if-needed` at 2026-05-07T23:23:11+00:00 via commit, merge, worktree-delete
- `20260507-145108` at 2026-05-07T21:51:36+00:00 via commit, merge
- `20260507-134720-add-an-attachment-section-divider-with-selection-save-hints-and-a-blank-line-before-the-email-body` at 2026-05-07T20:47:20+00:00 via commit, merge, worktree-delete
- `20260507-123058-demo-onboarding-emails` at 2026-05-07T19:30:58+00:00 via commit

## Approved Items

- No approved items.

## Rejected Items

- No rejected items.

## Implemented Items

### template-user-repro-after-ed02a1d-feedback

- Queue key: `template-user-repro-after-ed02a1d-feedback-7fde7140ee`
- Status: implemented
- Seen in runs: 4
- First seen: 2026-05-05T04:22:03+00:00
- Last seen: 2026-05-06T17:51:48+00:00
- Publish actions: branch-delete, commit, merge, worktree-delete
- Why: The next best improvement is to extend template coverage to the next repeated failure class instead of re-implementing an existing template.
- Approval prompt: Approve exploring `template-user-repro-after-ed02a1d-feedback` as the next explicit GEPA improvement pass.
- Decision: implemented at 2026-05-07T18:42:31+00:00
- Note: Approved by user on 2026-05-07 and incorporated into Herald GEPA workflow guidance/templates.
- Source runs:
- `20260506-105148-settings-bottom-line` at 2026-05-06T17:51:48+00:00 via commit, merge
- `20260506-073508-add-timeline-c-compose-discoverability-with-bottom-hint-guard-rails` at 2026-05-06T14:35:08+00:00 via commit, merge
- `20260505-111520-make-cleanup-rules-configuration-windows-compact-centered-overlays-like-settings-and-help` at 2026-05-05T18:15:20+00:00 via commit, merge, worktree-delete, branch-delete
- `20260504-212203-implement-timeline-reading-first-redesign-remove-size-kb-and-att-columns-add-subject-attachment-marker-human-local-dates-improved-sender-subject-allocation-and-distinct-table-headers` at 2026-05-05T04:22:03+00:00 via commit, merge, worktree-delete

### Require doc-first feature grounding

- Queue key: `require-doc-first-feature-grounding-ef7c7bc4af`
- Status: implemented
- Seen in runs: 4
- First seen: 2026-05-01T16:45:45+00:00
- Last seen: 2026-05-07T23:24:40+00:00
- Publish actions: commit, merge, worktree-delete
- Why: Feature work is safer when VISION, ARCHITECTURE, and specs are updated before code rather than only consulted during implementation.
- Approval prompt: Approve a stricter doc-first gate for non-trivial feature runs.
- Decision: implemented at 2026-05-07T18:42:31+00:00
- Note: Approved by user on 2026-05-07 and incorporated into Herald GEPA workflow guidance/templates.
- Source runs:
- `20260507-162440-centralize-tui-color-hues-in-internal-app-theme-roles` at 2026-05-07T23:24:40+00:00 via commit, merge
- `20260507-134720-add-an-attachment-section-divider-with-selection-save-hints-and-a-blank-line-before-the-email-body` at 2026-05-07T20:47:20+00:00 via commit, merge, worktree-delete
- `20260507-123058-demo-onboarding-emails` at 2026-05-07T19:30:58+00:00 via commit
- `20260501-self-reflection-validation` at 2026-05-01T16:45:45+00:00 via commit

### template-red-compose-comma-alias-feedback

- Queue key: `template-red-compose-comma-alias-feedback-f72a614404`
- Status: implemented
- Seen in runs: 2
- First seen: 2026-05-01T16:45:45+00:00
- Last seen: 2026-05-01T16:55:28+00:00
- Publish actions: commit
- Why: The next best improvement is to extend template coverage to the next repeated failure class instead of re-implementing an existing template.
- Approval prompt: Approve exploring `template-red-compose-comma-alias-feedback` as the next explicit GEPA improvement pass.
- Decision: implemented at 2026-05-07T18:42:31+00:00
- Note: Approved by user on 2026-05-07 and incorporated into Herald GEPA workflow guidance/templates.
- Source runs:
- `20260501-095528-issue-16-support-japanese-ime-composition-for-layout-independent-shortcuts-https-github-com-herald-email-herald-mail-app-issues-16` at 2026-05-01T16:55:28+00:00 via commit
- `20260501-self-reflection-validation` at 2026-05-01T16:45:45+00:00 via commit

### Focused test remediation template

- Queue key: `focused-test-remediation-template-f643092950`
- Status: implemented
- Seen in runs: 2
- First seen: 2026-05-01T16:45:45+00:00
- Last seen: 2026-05-07T20:47:20+00:00
- Publish actions: commit, merge, worktree-delete
- Why: Repeated run history shows focused test failures usually come from stale expectations, overspecified assertions, or missing adjacent regression coverage rather than the core feature direction being wrong.
- Approval prompt: Approve keeping the focused-tests remediation template as a default GEPA retry aid.
- Decision: implemented at 2026-05-07T18:42:31+00:00
- Note: Approved by user on 2026-05-07 and incorporated into Herald GEPA workflow guidance/templates.
- Source runs:
- `20260507-134720-add-an-attachment-section-divider-with-selection-save-hints-and-a-blank-line-before-the-email-body` at 2026-05-07T20:47:20+00:00 via commit, merge, worktree-delete
- `20260501-self-reflection-validation` at 2026-05-01T16:45:45+00:00 via commit

### Package-level test remediation template

- Queue key: `package-level-test-remediation-template-578d17f670`
- Status: implemented
- Seen in runs: 1
- First seen: 2026-05-06T14:35:08+00:00
- Last seen: 2026-05-06T14:35:08+00:00
- Publish actions: commit, merge
- Why: Package-level failures usually indicate a narrow contract or snapshot expectation drift that should be repaired without exploding the retry scope.
- Approval prompt: Approve keeping the app-package-tests remediation template as a default GEPA retry aid.
- Decision: implemented at 2026-05-07T18:42:31+00:00
- Note: Approved by user on 2026-05-07 and incorporated into Herald GEPA workflow guidance/templates.
- Source runs:
- `20260506-073508-add-timeline-c-compose-discoverability-with-bottom-hint-guard-rails` at 2026-05-06T14:35:08+00:00 via commit, merge
