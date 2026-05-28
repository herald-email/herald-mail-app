# Herald GEPA Remediation Template Adoption

This report measures whether published Herald autopilot self-reflections actually use reusable remediation templates, and whether retry counts differ between matched and unmatched eligible runs.

## Summary

- Generated at: 2026-05-28T17:37:39+00:00
- Published reflections analyzed: 52
- Published runs with templates: 3
- Published adoption rate: 6%
- Eligible runs: 4
- Eligible runs with templates: 3
- Eligible adoption rate: 75%
- Total template matches: 3
- Average retries with templates: 1.00
- Average retries without templates: 0.02
- Eligible retry delta with templates: +0.00

## Template Matches

### Focused test remediation template

- Key: `focused-tests`
- Matched runs: 2
- Average retries: 1.00
- Runs: `20260501-self-reflection-validation`, `20260507-134720-add-an-attachment-section-divider-with-selection-save-hints-and-a-blank-line-before-the-email-body`

### Package-level test remediation template

- Key: `app-package-tests`
- Matched runs: 1
- Average retries: 1.00
- Runs: `20260506-073508-add-timeline-c-compose-discoverability-with-bottom-hint-guard-rails`

## Unmatched Eligible Runs

- `20260526-201311-implement-read-only-scoped-calendar-mcp-tools`: retries 1, actions commit 033eeb0 Add read-only calendar MCP tools, fast-forward merged to main, removed worktree /Users/zoomacode/Developer/mail-processor/.worktrees/20260526-201311-calendar-mcp-readonly, deleted branch codex/autopilot-calendar-mcp-readonly-20260526-201311

## Caveats

- Template matches appear only when self-reflection records them, so older runs without `self_reflection.json` are outside this measurement.
- This is observational. Lower or higher retries on matched runs can reflect task difficulty as much as template usefulness.
