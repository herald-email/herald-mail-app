# Test Report — 2026-03-23

**Plan:** TUI_TESTPLAN.md
**Binary:** built from HEAD (`64e6707`)
**Sizes tested:** 220×50, 80×24, 50×15
**Test emails sent to:** zoomacode@pm.me (3 emails, all received and parsed correctly)

---

## Bugs

### B1 — Timeline Sender/Subject columns collapse to 1 char at 80×24 with sidebar open
**Severity: Medium | TC-05**

At 80×24 with the sidebar visible, the layout math leaves only 6 chars of variable space for Sender+Subject combined. Integer division gives tSenderWidth=1 and tSubjectWidth=5, producing:

```
│ …  Subj…  Date              Size KB  Att  Tag  │
│ …  zoom…  26-03-22 09:02    35.9     N         │
```

The Sender column is a single `…` — completely unusable. The 80×24 terminal with sidebar hidden works correctly (Sender=11 cols, Subject=29 cols).

**Root cause:** `sidebarExtra` = 30 leaves `timelineVariable` = 80 − 74 = 6. Per-column minimums are only enforced when `timelineVariable >= 24`, so they never fire here.

**Fix:** Auto-hide the sidebar when `timelineVariable < 20` after layout calculation, or render a "Too narrow — press `f` to hide sidebar" hint in the table area.

---

### B2 — Zero-width tracking characters render as visible dots in email body preview
**Severity: Low | TC-04 (email #18 and others)**

HTML-only emails stripped to plain text retain Unicode zero-width/invisible characters used as email tracking spacers (U+034F COMBINING GRAPHEME JOINER, U+200B ZERO WIDTH SPACE, U+FEFF BOM, etc.). In the terminal these appear as visible `͏ ` dots:

```
  ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏ ͏
```

These waste several lines of the preview panel and are meaningless to the reader.

**Fix:** Strip Unicode formatting/invisible characters from body text before wrapping. Characters in ranges: Cc (control), Cf (format), Cs (surrogate), Co (private use), Cn (unassigned) that are not whitespace should be removed or replaced.

---

## UX Issues

### U1 — Global toggle keys (`c`, `l`, `f`, `r`) do not work while Compose text fields are focused
**Severity: Low | TC-09, TC-11**

When the Compose tab is active with a text input (To, Subject, or Body) focused, pressing `c` types the letter "c" into the field rather than toggling the chat panel. Same for `l` (logs), `f` (sidebar), `r` (refresh). Only the explicitly-intercepted keys (`1`, `2`, `3`, `tab`, `ctrl+s`, `ctrl+p`, `esc`) work as global actions.

This is a design consequence of text inputs capturing all characters, but it creates an inconsistency: the key hint bar shows `c: chat` at the bottom, yet the key doesn't work on the Compose tab.

**Suggestion:** Either remove the `c`/`l`/`f` hints from the Compose tab key bar, or add these keys to `handleComposeKey` (only practical for `f` and `l` since they're rarely typed; `c`, `r` are too common to safely intercept).

---

### U2 — Body fetch latency: "Loading…" visible for >5 seconds on some emails
**Severity: Low | TC-03, TC-04**

Several emails in the TC-04 iteration still showed "Loading…" after a 3-second wait. The fetch completed at ~5–8 seconds. This is IMAP server latency (ProtonMail Bridge), not a code issue. The "Loading…" indicator itself works correctly.

**Suggestion:** No code fix needed; behaviour is correct. Could add a progress indicator (dots animation) to the loading state for better feedback.

---

## Passed Test Cases

| TC  | Description                        | Result |
|-----|------------------------------------|--------|
| 01  | App startup and initial render     | PASS   |
| 02  | Tab switching (1/2/3)              | PASS   |
| 03  | Timeline preview open/close        | PASS   |
| 04  | Iterate first 20 emails            | PASS — no crashes; thread expansion works; Unicode subjects/bodies render correctly |
| 05  | Sidebar toggle at 80×24            | PASS (no sidebar) / **BUG B1** (with sidebar) |
| 07  | Cleanup selection + checkmarks     | PASS — ✓ marks and `2 messages selected` status bar work |
| 08  | Domain mode toggle                 | PASS   |
| 09  | AI chat panel                      | PASS from Timeline; see U1 for Compose conflict |
| 10  | Log viewer overlay                 | PASS — colour-coded entries, opens/closes cleanly |
| 11  | Compose tab fields and Ctrl+P      | PASS   |
| 12  | Minimum size guard (50×15)         | PASS — "Terminal too narrow (50 cols)" message shown |
| 13  | Resize 220→80→220 with preview     | PASS — layout reflows cleanly |
| 14  | Refresh (`r`)                      | PASS   |
| 15  | Reply pre-fill (`R`)               | PASS — To and `Re:` Subject correctly populated; Unicode subject preserved |

### Test email verification

| Email | Subject | Delivered | Timeline render | Preview body |
|-------|---------|-----------|-----------------|--------------|
| 1 | `TUI Test Email - Plain Text` | ✓ | ✓ sender + subject | ✓ |
| 2 | `TUI Test - Unicode café résumé 日本語` | ✓ | ✓ full Unicode, no truncation needed | ✓ including Japanese |
| 3 | `TUI Test - This is an extremely long subject line…` | ✓ | ✓ truncated with `…`, no garbled chars | ✓ |

Reply pre-fill for email 2: `Re: TUI Test - Unicode café résumé 日本語` populated correctly in Subject field.
