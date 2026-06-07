# Herald GEPA Pending Approvals

This document is the visible approval backlog for workflow suggestions recovered from published Herald autopilot runs. It turns scattered post-publish self-reflection into one place where approvals, rejections, and implemented ideas can be reviewed without digging through run folders.

## Snapshot

- Updated at: 2026-06-07T05:16:54+00:00
- Published runs analyzed: 57
- Total queue items: 13
- Pending: 0
- Approved: 0
- Rejected: 0
- Implemented: 13

## How To Update

- Sync from the latest published reflections: `python3 .agents/skills/herald-autopilot/scripts/sync_pending_approvals.py --repo-root .`
- Approve one or more items: `python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py --repo-root . --status approved --key <queue-key>`
- Batch-approve everything still pending: `python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py --repo-root . --status approved --all-pending`

## Pending Items

- No pending items.

## Approved Items

- No approved items.

## Rejected Items

- No rejected items.

## Implemented Items

### template-user-review-followup-settings-hints-feedback

- Queue key: `template-user-review-followup-settings-hints-feedback-11020dbda5`
- Status: implemented
- Seen in runs: 27
- First seen: 2026-05-18T23:21:53+00:00
- Last seen: 2026-05-27T06:17:12+00:00
- Publish actions: commit, commit 033eeb0 Add read-only calendar MCP tools, commit 0592cfb Expose provider freshness metadata, commit 1aae509 Scope FTS mail search rows, commit 219b926 Tag contact enrichment AI work by source, commit 3ce59df Make cleanup scheduling source aware, commit 3d2a98c Migrate deletion lane to source serial work, commit 4c2cf4c Add calendar full event detail timezone foundation, commit 588d668 Store classifications with scoped message refs, commit 68299d4 Migrate active sync coordinator to internal work, commit 69019c7 Add source plugin registry, commit 80ab68d Scope background mail embeddings, commit 853fe85 Add calendar search foundation, commit 8587d56 Add account-level compose signatures, commit f359845 Reconcile source platform roadmap markers, commit fec3a24 Add source-fair AI budget scheduling, deleted branch codex/autopilot-account-aware-compose-sending-20260523-220633, deleted branch codex/autopilot-account-signatures-20260526-224222, deleted branch codex/autopilot-active-collection-sync-work-coordinator-20260526-210426, deleted branch codex/autopilot-ai-global-budget-20260526-225307, deleted branch codex/autopilot-calendar-mcp-readonly-20260526-201311, deleted branch codex/autopilot-contact-enrichment-source-tags-20260526-230806, deleted branch codex/autopilot-fts-source-scope-20260526-222846, deleted branch codex/autopilot-roadmap-checkbox-reconciliation-20260526-231712, deleted branch codex/autopilot-scoped-ai-indexing-20260526-214024, deleted branch codex/autopilot-scoped-classification-results-20260526-212551, deleted branch codex/autopilot-source-aware-cleanup-20260526-215720, deleted branch codex/autopilot-source-freshness-metadata-20260526-221911, deleted branch codex/autopilot-source-plugin-registry-20260526-220729, deleted branch codex/autopilot-source-serial-deletion-lane-20260526-211454, fast-forward merged to local main, fast-forward merged to main, left in local worktree for review/finish-development, local commit 0e42abc on codex/autopilot-account-aware-compose-sending-20260523-220633, local commits only, merge, merge main fast-forward to 1aae509, merge main fast-forward to 8587d56, merged to local main at 0e42abc, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260523-220633-account-aware-compose-sending, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-201311-calendar-mcp-readonly, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-210426-active-collection-sync-work-coordinator, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-211454-source-serial-deletion-lane, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-212551-scoped-classification-results, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-214024-scoped-ai-indexing, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-215720-source-aware-cleanup, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-220729-source-plugin-registry, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-221911-source-freshness-metadata, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-222846-fts-source-scope, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-224222-account-signatures, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-225307-ai-global-budget, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-230806-contact-enrichment-source-tags, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-231712-roadmap-checkbox-reconciliation
- Why: The next best improvement is to extend template coverage to the next repeated failure class instead of re-implementing an existing template.
- Approval prompt: Approve exploring `template-user-review-followup-settings-hints-feedback` as the next explicit GEPA improvement pass.
- Decision: implemented at 2026-05-27T14:55:15+00:00
- Note: Implemented by user request: added reusable remediation templates for user-review settings hints and commit-hook make test failures.
- Source runs:
- `20260526-231712-reconcile-source-platform-roadmap-checkboxes` at 2026-05-27T06:17:12+00:00 via commit f359845 Reconcile source platform roadmap markers, fast-forward merged to local main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-231712-roadmap-checkbox-reconciliation, deleted branch codex/autopilot-roadmap-checkbox-reconciliation-20260526-231712
- `20260526-230806-source-tag-contact-enrichment-ai-work` at 2026-05-27T06:08:06+00:00 via commit 219b926 Tag contact enrichment AI work by source, fast-forward merged to local main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-230806-contact-enrichment-source-tags, deleted branch codex/autopilot-contact-enrichment-source-tags-20260526-230806
- `20260526-225307-add-global-ai-budget-source-fairness` at 2026-05-27T05:53:07+00:00 via commit fec3a24 Add source-fair AI budget scheduling, fast-forward merged to local main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-225307-ai-global-budget, deleted branch codex/autopilot-ai-global-budget-20260526-225307
- `20260526-224222-add-account-level-compose-signatures-for-mail-sources` at 2026-05-27T05:42:22+00:00 via commit 8587d56 Add account-level compose signatures, merge main fast-forward to 8587d56, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-224222-account-signatures, deleted branch codex/autopilot-account-signatures-20260526-224222
- `20260526-222846-scope-fts-mail-search-rows` at 2026-05-27T05:28:46+00:00 via commit 1aae509 Scope FTS mail search rows, merge main fast-forward to 1aae509, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-222846-fts-source-scope, deleted branch codex/autopilot-fts-source-scope-20260526-222846

### Require doc-first feature grounding

- Queue key: `require-doc-first-feature-grounding-ef7c7bc4af`
- Status: implemented
- Seen in runs: 21
- First seen: 2026-05-01T16:45:45+00:00
- Last seen: 2026-06-06T02:04:15+00:00
- Publish actions: branch-delete, commit, commit 0592cfb Expose provider freshness metadata, commit 1aae509 Scope FTS mail search rows, commit 219b926 Tag contact enrichment AI work by source, commit 3ce59df Make cleanup scheduling source aware, commit 3d2a98c Migrate deletion lane to source serial work, commit 588d668 Store classifications with scoped message refs, commit 68299d4 Migrate active sync coordinator to internal work, commit 69019c7 Add source plugin registry, commit 80ab68d Scope background mail embeddings, commit 8587d56 Add account-level compose signatures, commit 9e6c1d4 Add docs copy drift checker, commit fec3a24 Add source-fair AI budget scheduling, deleted branch codex/autopilot-account-signatures-20260526-224222, deleted branch codex/autopilot-active-collection-sync-work-coordinator-20260526-210426, deleted branch codex/autopilot-ai-global-budget-20260526-225307, deleted branch codex/autopilot-contact-enrichment-source-tags-20260526-230806, deleted branch codex/autopilot-fts-source-scope-20260526-222846, deleted branch codex/autopilot-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-20260605-190045, deleted branch codex/autopilot-scoped-ai-indexing-20260526-214024, deleted branch codex/autopilot-scoped-classification-results-20260526-212551, deleted branch codex/autopilot-source-aware-cleanup-20260526-215720, deleted branch codex/autopilot-source-freshness-metadata-20260526-221911, deleted branch codex/autopilot-source-plugin-registry-20260526-220729, deleted branch codex/autopilot-source-serial-deletion-lane-20260526-211454, fast-forward merged to local main, fast-forward merged to main, merge, merge main fast-forward to 1aae509, merge main fast-forward to 8587d56, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-210426-active-collection-sync-work-coordinator, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-211454-source-serial-deletion-lane, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-212551-scoped-classification-results, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-214024-scoped-ai-indexing, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-215720-source-aware-cleanup, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-220729-source-plugin-registry, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-221911-source-freshness-metadata, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-222846-fts-source-scope, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-224222-account-signatures, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-225307-ai-global-budget, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-230806-contact-enrichment-source-tags, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy, worktree-delete
- Why: Feature work is safer when VISION, ARCHITECTURE, and specs are updated before code rather than only consulted during implementation.
- Approval prompt: Approve a stricter doc-first gate for non-trivial feature runs.
- Decision: implemented at 2026-05-07T18:42:31+00:00
- Note: Approved by user on 2026-05-07 and incorporated into Herald GEPA workflow guidance/templates.
- Source runs:
- `20260605-190415-provider-choice-cards` at 2026-06-06T02:04:15+00:00 via commit, merge
- `20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy` at 2026-06-06T02:00:45+00:00 via commit 9e6c1d4 Add docs copy drift checker, fast-forward merged to main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy, deleted branch codex/autopilot-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-20260605-190045
- `20260526-230806-source-tag-contact-enrichment-ai-work` at 2026-05-27T06:08:06+00:00 via commit 219b926 Tag contact enrichment AI work by source, fast-forward merged to local main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-230806-contact-enrichment-source-tags, deleted branch codex/autopilot-contact-enrichment-source-tags-20260526-230806
- `20260526-225307-add-global-ai-budget-source-fairness` at 2026-05-27T05:53:07+00:00 via commit fec3a24 Add source-fair AI budget scheduling, fast-forward merged to local main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-225307-ai-global-budget, deleted branch codex/autopilot-ai-global-budget-20260526-225307
- `20260526-224222-add-account-level-compose-signatures-for-mail-sources` at 2026-05-27T05:42:22+00:00 via commit 8587d56 Add account-level compose signatures, merge main fast-forward to 8587d56, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-224222-account-signatures, deleted branch codex/autopilot-account-signatures-20260526-224222

### two-candidate-worktree-trial

- Queue key: `two-candidate-worktree-trial-f38557be73`
- Status: implemented
- Seen in runs: 13
- First seen: 2026-05-29T05:46:13+00:00
- Last seen: 2026-06-07T01:44:47+00:00
- Publish actions: branch-delete, commit, commit 68cf842 Refs #32, commit 91a1f10 Cache clipped native image previews, commit 96ab2e2 Make demo mode the docs front door, commit 9e6c1d4 Add docs copy drift checker, commit 9ee4508 Add Gmail API history sync, commit a6435fa Harden Gmail API mail source, deleted branch codex/autopilot-address-github-issue-50-https-github-com-herald-email-herald-mail-app-issues-50-20260605-225555, deleted branch codex/autopilot-demo-newsletter-preview-scroll-lag-20260604-112556, deleted branch codex/autopilot-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-20260605-190045, fast-forward merged to local main, fast-forward merged to main, merge, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260604-112556-demo-newsletter-preview-scroll-lag, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260605-225555-address-github-issue-50-https-github-com-herald-email-herald-mail-app-issues-50-address-github-issue-50-https-github-com-herald-email-herald-mail-app-issues-50, worktree-remove
- Why: A narrow challenger-worktree experiment is the next step toward true GEPA-style search once the baseline is trustworthy.
- Approval prompt: Approve exploring `two-candidate-worktree-trial` as the next explicit GEPA improvement pass.
- Decision: implemented at 2026-06-07T05:15:56+00:00
- Note: Implemented by user request: added docs-build remediation, settings/hints alias, and two-candidate trial guidance.
- Source runs:
- `20260606-184447-make-the-chat-drawer-focusable-using-mouse-like-other-blocks` at 2026-06-07T01:44:47+00:00 via commit, merge
- `20260606-123300-fix-issue-66-rework-ai-settings-and-first-run-wizard-around-compact-provider-presets-and-per-capability-model-choices` at 2026-06-06T19:33:00+00:00 via commit, merge
- `20260605-225555-address-github-issue-50-https-github-com-herald-email-herald-mail-app-issues-50` at 2026-06-06T05:55:55+00:00 via commit 96ab2e2 Make demo mode the docs front door, fast-forward merged to local main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260605-225555-address-github-issue-50-https-github-com-herald-email-herald-mail-app-issues-50-address-github-issue-50-https-github-com-herald-email-herald-mail-app-issues-50, deleted branch codex/autopilot-address-github-issue-50-https-github-com-herald-email-herald-mail-app-issues-50-20260605-225555
- `20260605-190415-provider-choice-cards` at 2026-06-06T02:04:15+00:00 via commit, merge
- `20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy` at 2026-06-06T02:00:45+00:00 via commit 9e6c1d4 Add docs copy drift checker, fast-forward merged to main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy, deleted branch codex/autopilot-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-20260605-190045

### template-green-demo-key-overlay-app-attempt1-feedback

- Queue key: `template-green-demo-key-overlay-app-attempt1-feedback-b19c0ba4a6`
- Status: implemented
- Seen in runs: 8
- First seen: 2026-05-07T19:30:58+00:00
- Last seen: 2026-05-15T23:00:24+00:00
- Publish actions: branch-delete, commit, merge, worktree-delete
- Why: The next best improvement is to extend template coverage to the next repeated failure class instead of re-implementing an existing template.
- Approval prompt: Approve exploring `template-green-demo-key-overlay-app-attempt1-feedback` as the next explicit GEPA improvement pass.
- Decision: implemented at 2026-05-18T20:16:26+00:00
- Note: Implemented by user request: added demo-key-overlay remediation template.
- Source runs:
- `20260515-160024-modifier-aware-key-hints` at 2026-05-15T23:00:24+00:00 via commit, merge, worktree-delete, branch-delete
- `20260515-155701-preview-cache-policy-pruning` at 2026-05-15T22:57:01+00:00 via commit, merge, worktree-delete, branch-delete
- `20260507-162440-centralize-tui-color-hues-in-internal-app-theme-roles` at 2026-05-07T23:24:40+00:00 via commit, merge, worktree-delete
- `20260507-162410-make-tui-key-hints-consistently-come-from-the-global-keyboard-config` at 2026-05-07T23:24:10+00:00 via commit, merge, worktree-delete
- `20260507-162311-check-changes-between-the-most-recent-tag-and-head-and-update-documentation-if-needed` at 2026-05-07T23:23:11+00:00 via commit, merge, worktree-delete

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

### measure-remediation-template-adoption

- Queue key: `measure-remediation-template-adoption-64aca1bc95`
- Status: implemented
- Seen in runs: 3
- First seen: 2026-05-27T18:39:07+00:00
- Last seen: 2026-05-28T16:49:06+00:00
- Publish actions: commit 9fe638c Align calendar timeframes to calendar ranges, deleted branch codex/autopilot-calendar-timeframe-ranges-20260528-094906, fast-forward merged to main, local branch/worktree left for handoff, local commit 46f3f94, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260528-094906-calendar-timeframe-ranges
- Why: Once the main repeated failure classes have templates, the next leverage comes from checking whether runs actually use them and whether retries decline.
- Approval prompt: Approve exploring `measure-remediation-template-adoption` as the next explicit GEPA improvement pass.
- Decision: implemented at 2026-05-28T17:36:23+00:00
- Note: Implemented by user request: added remediation-template adoption measurement report and optimizer evidence.
- Source runs:
- `20260528-094906-fix-calendar-timeframe-ranges-so-month-windows-use-calendar-months-and-week-windows-use-monday-sunday` at 2026-05-28T16:49:06+00:00 via commit 9fe638c Align calendar timeframes to calendar ranges, fast-forward merged to main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260528-094906-calendar-timeframe-ranges, deleted branch codex/autopilot-calendar-timeframe-ranges-20260528-094906
- `20260527-235216-implement-calendar-design-parity-roadmap-screens-01-04-plus-notes-rsvp-invitation-settings-hardening` at 2026-05-28T06:52:16+00:00 via local commit 46f3f94
- `20260527-113907-implement-mail-app-style-multi-account-folder-sidebar` at 2026-05-27T18:39:07+00:00 via local branch/worktree left for handoff

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

### Template `docs-build` remediation guidance

- Queue key: `template-docs-build-remediation-guidance-f50055c9cb`
- Status: implemented
- Seen in runs: 2
- First seen: 2026-06-06T02:00:45+00:00
- Last seen: 2026-06-06T02:04:15+00:00
- Publish actions: commit, commit 9e6c1d4 Add docs copy drift checker, deleted branch codex/autopilot-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-20260605-190045, fast-forward merged to main, merge, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy
- Why: This run needed retries or explicit feedback that could become reusable autopilot guidance.
- Approval prompt: Approve turning the `docs-build` lesson from this run into a reusable GEPA workflow template.
- Decision: implemented at 2026-06-07T05:15:56+00:00
- Note: Implemented by user request: added docs-build remediation, settings/hints alias, and two-candidate trial guidance.
- Source runs:
- `20260605-190415-provider-choice-cards` at 2026-06-06T02:04:15+00:00 via commit, merge
- `20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy` at 2026-06-06T02:00:45+00:00 via commit 9e6c1d4 Add docs copy drift checker, fast-forward merged to main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260605-190045-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy, deleted branch codex/autopilot-issue-56-add-docs-copy-drift-smoke-test-for-stale-product-copy-20260605-190045

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

### Template `a repeated failure mode` remediation guidance

- Queue key: `template-a-repeated-failure-mode-remediation-guidance-a119868f77`
- Status: implemented
- Seen in runs: 1
- First seen: 2026-05-15T23:00:24+00:00
- Last seen: 2026-05-15T23:00:24+00:00
- Publish actions: branch-delete, commit, merge, worktree-delete
- Why: This run needed retries or explicit feedback that could become reusable autopilot guidance.
- Approval prompt: Approve turning the `a repeated failure mode` lesson from this run into a reusable GEPA workflow template.
- Decision: implemented at 2026-05-16T00:54:27+00:00
- Note: Implemented by user request: final handoffs now require runnable cd/build/launch commands.
- Source runs:
- `20260515-160024-modifier-aware-key-hints` at 2026-05-15T23:00:24+00:00 via commit, merge, worktree-delete, branch-delete

### Template `commit hook make test` remediation guidance

- Queue key: `template-commit-hook-make-test-remediation-guidance-a04c85775e`
- Status: implemented
- Seen in runs: 1
- First seen: 2026-05-27T03:13:11+00:00
- Last seen: 2026-05-27T03:13:11+00:00
- Publish actions: commit 033eeb0 Add read-only calendar MCP tools, deleted branch codex/autopilot-calendar-mcp-readonly-20260526-201311, fast-forward merged to main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-201311-calendar-mcp-readonly
- Why: This run needed retries or explicit feedback that could become reusable autopilot guidance.
- Approval prompt: Approve turning the `commit hook make test` lesson from this run into a reusable GEPA workflow template.
- Decision: implemented at 2026-05-27T14:55:15+00:00
- Note: Implemented by user request: added reusable remediation templates for user-review settings hints and commit-hook make test failures.
- Source runs:
- `20260526-201311-implement-read-only-scoped-calendar-mcp-tools` at 2026-05-27T03:13:11+00:00 via commit 033eeb0 Add read-only calendar MCP tools, fast-forward merged to main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-201311-calendar-mcp-readonly, deleted branch codex/autopilot-calendar-mcp-readonly-20260526-201311

### User-review follow-up settings hints template

- Queue key: `user-review-follow-up-settings-hints-template-d8172ef566`
- Status: implemented
- Seen in runs: 1
- First seen: 2026-05-29T05:46:13+00:00
- Last seen: 2026-05-29T05:46:13+00:00
- Publish actions: commit, merge, worktree-remove
- Why: Repeated user follow-up around settings and bottom hints shows that a green implementation can still miss the exact review path, visible hint contract, or text the user expected to survive.
- Approval prompt: Approve keeping the user-review follow-up settings hints template as a default GEPA retry aid.
- Decision: implemented at 2026-06-07T05:15:56+00:00
- Note: Implemented by user request: added docs-build remediation, settings/hints alias, and two-candidate trial guidance.
- Source runs:
- `20260528-224613-improve-calendar-week-time-grid-density-on-tall-screens-and-keep-long-event-blocks-uninterrupted-by-guide-dots` at 2026-05-29T05:46:13+00:00 via commit, merge, worktree-remove
