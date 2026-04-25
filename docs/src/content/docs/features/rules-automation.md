---
title: Rules and Automation
description: Use automation rules, custom prompts, cleanup rules, scheduled cleanup, and dry-run mode.
---

Rules and automation turn repeated cleanup decisions into saved behavior. Herald exposes automation through the Cleanup tab rule editor, custom prompt editor, cleanup manager, schedule settings, MCP tools, and dry-run status.

## Overview

Use `W` for future-mail automation rules, `P` for reusable AI prompts, and `C` for cleanup rules that target older mail by sender or domain. These are shipped user-facing features backed by Herald's rule and cleanup-rule storage.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Automation rule editor | `Automation Rule` form with trigger group, action multiselect, action detail fields, and saved-rule summary. |
| Trigger fields | Trigger type sender, domain, or AI category; trigger value such as address, domain, or category. |
| Action list | Desktop notification, move, archive, delete, webhook POST, and shell command. |
| Action detail fields | Destination folder, webhook URL/body, shell command, notification title/body. |
| Prompt editor | Name, output variable, system prompt, user template, and saved-prompt summary. |
| Cleanup manager list | Saved cleanup rules with name, match type, match value, action, older-than days, enabled state, and last run. |
| Cleanup manager edit form | Rule name, match type, match value, action, older-than days, and enabled toggle. |
| Status bar | Run results, dry-run marker, deletion/archive progress, and error messages. |

<!-- HERALD_SCREENSHOT id="automation-rule-editor" page="rules-automation" alt="Automation rule editor form" state="demo mode, 120x40, Cleanup tab, W overlay open" desc="Shows trigger type/value, action selection, details fields, saved rule summary, and form help." capture="tmux demo 120x40; ./bin/herald --demo; press 3; press W" -->

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `W` | Cleanup | Rule editor closed. | Opens automation rule editor, prefilled from focused sender/domain when available. |
| `esc` | Rule editor | Form not completed. | Cancels and closes the rule editor. |
| `P` | Main UI | Rule editor, prompt editor, and settings closed. | Opens custom prompt editor. |
| `esc` | Prompt editor | Form not completed. | Cancels and closes the prompt editor. |
| `C` | Cleanup | Cleanup manager closed. | Opens cleanup manager. |
| `n` | Cleanup manager list | Manager open. | Creates a new cleanup rule. |
| `enter` | Cleanup manager list | A rule exists. | Edits selected cleanup rule. |
| `d` / `D` | Cleanup manager list | A rule exists. | Deletes selected cleanup rule. |
| `r` | Cleanup manager list | Manager open. | Runs all cleanup rules immediately. |
| `j` / `down` | Cleanup manager list | Manager open. | Moves down. |
| `k` / `up` | Cleanup manager list | Manager open. | Moves up. |
| `esc` | Cleanup manager | Manager open. | Closes list or cancels edit form back to list. |

## Workflows

### Create a Future-Mail Automation Rule

1. Open Cleanup with `3`.
2. Focus a sender/domain row when you want prefilled context.
3. Press `W`.
4. Choose trigger type.
5. Enter trigger value.
6. Select one or more actions.
7. Fill details for move, webhook, command, or notification actions.
8. Complete the form to save.

### Create a Custom AI Prompt

1. Press `P`.
2. Enter a name.
3. Optionally enter an output variable.
4. Write system instructions.
5. Write a user template using placeholders such as `{{.Sender}}`, `{{.Subject}}`, and `{{.Body}}`.
6. Complete the form to save.

Custom prompts are reusable instructions. A rule or MCP tool must invoke a saved prompt before it produces results.

### Create a Cleanup Rule

1. Open Cleanup with `3`.
2. Press `C`.
3. Press `n`.
4. Fill rule name, match type, match value, action, older-than days, and enabled state.
5. Save the form.
6. Press `r` in the manager list to run all rules immediately, or rely on configured scheduling.

### Configure Scheduled Cleanup

1. Press `S` to open settings.
2. Set cleanup schedule hours in the sync/cleanup section.
3. Save settings.
4. Reopen Cleanup manager with `C` to review enabled rules.

## States

| State | What happens |
| --- | --- |
| No saved automation rules | Rule editor summary says none yet. |
| Saved automation rules | Rule editor summary shows a few saved rules and a count of additional rules. |
| Prompt validation | Prompt name is required. |
| No cleanup rules | Cleanup manager explains that `n` creates one. |
| Disabled cleanup rule | Manager list marks the rule disabled and scheduled runs skip it. |
| Run all | Cleanup manager emits an immediate run request for all rules. |
| Dry-run mode | Status shows `[DRY RUN]` so rule effects can be inspected without live mutation when launched in that mode. |
| AI unavailable | AI-category triggers and custom prompt execution cannot classify new content. |
| Dangerous actions | Delete, shell command, and webhook actions can affect mail or external systems. |

## Data And Privacy

Rules are stored in Herald's backend and may include trigger values, destination folders, webhook URLs, webhook body templates, shell commands, notification text, and custom AI prompts. Cleanup rules can delete or archive matching mail. Webhooks send configured email-derived data to external URLs. Shell commands run locally with environment variables derived from matching mail.

## Troubleshooting

If a rule does not match, compare trigger type and value with the exact sender/domain/category shown in Cleanup.

If a custom prompt saves but appears to do nothing, remember that saved prompts are invoked by rules or MCP tools; saving alone does not run a prompt.

If cleanup rules do not run on schedule, check `cleanup.schedule_hours` and confirm the app or daemon surface responsible for scheduled work is active.

If experimenting with delete rules, use dry-run mode when available and verify the status bar before enabling live cleanup.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="automation-prompt-editor" page="rules-automation" alt="Custom prompt editor form" state="demo mode, 120x40, prompt editor open" desc="Shows prompt identity fields, system prompt, user template, template-variable descriptions, and saved prompt summary." capture="tmux demo 120x40; ./bin/herald --demo; press P" -->

<!-- HERALD_SCREENSHOT id="automation-cleanup-manager-list" page="rules-automation" alt="Cleanup manager rule list" state="demo mode, 120x40, Cleanup manager list" desc="Shows saved cleanup rule rows or empty state, n/enter/d/r/esc controls, and last run details when present." capture="tmux demo 120x40; ./bin/herald --demo; press 3; press C" -->

<!-- HERALD_SCREENSHOT id="automation-cleanup-manager-edit" page="rules-automation" alt="Cleanup manager edit form" state="demo mode, 120x40, new cleanup rule form" desc="Shows cleanup rule name, match type, match value, action, older-than days, enabled field, and cancel hint." capture="tmux demo 120x40; ./bin/herald --demo; press 3; press C; press n" -->

## Related Pages

- [Cleanup](/using-herald/cleanup/)
- [AI Features](/features/ai/)
- [Destructive Actions](/features/destructive-actions/)
- [MCP Server](/advanced/mcp/)
- [Config Reference](/reference/config/)
