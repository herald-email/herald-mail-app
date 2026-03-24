## Test Run — 2026-03-23 — SSH server (cmd/ssh-server)

**Plan:** SSH_TESTPLAN.md
**Binary:** built from HEAD, output to `/tmp/ssh-server-test`
**Method:** tmux session `ssh_test`, pane 0 = server, pane 1 = SSH client, pane 2 = second SSH client (TC-SS-08)
**Cache:** populated (INBOX ~5132 emails)

---

### Bugs

None found.

---

### UX Issues

| ID | TC     | Description                                                                                                                   | Suggestion                                                                                                        |
|----|--------|-------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|
| U1 | SS-01  | Server prints no log line when a client connects or disconnects. Only startup lines appear.                                   | Log `"new connection from <addr>"` and `"connection closed from <addr>"` to help diagnose multi-session issues.  |
| U2 | SS-04  | Tab bar disappears at 80-column width — only tab numbers remain, labels are truncated entirely.                               | Pre-existing local TUI issue (same behaviour without SSH). Not SSH-specific. File against TUI rendering.          |
| U3 | SS-05  | In the Timeline tab, pressing `j`/`k` when the sidebar is focused moves the email table cursor instead of the sidebar cursor. `j`/`k` are only routed to `handleNavigation` for non-Timeline tabs. Sidebar navigation works correctly in the Cleanup tab. | In `app.go` `case "down", "j":`, add a `m.focusedPanel == panelSidebar` check before routing to `timelineTable`. |

---

### All Good

- **TC-SS-01** Connection and initial render: PASS — Tab bar, folder sidebar, email table, and status bar all rendered correctly. No garbled bytes. Server logs startup lines on launch.
- **TC-SS-02** All tabs accessible: PASS — `1`/`2`/`3` switch tabs; each tab renders its correct layout (Timeline list, Compose fields, Cleanup two-panel). No layout corruption on switching.
- **TC-SS-03** Email navigation and body preview: PASS — `j` navigates the timeline table. Enter on a non-thread row opens a split-screen body preview with From/Date/Subject header and plain-text body. `j`/`k` scroll the preview body; scroll indicator updates. Escape closes preview and restores full-width timeline; cursor unchanged.
- **TC-SS-04** Terminal resize propagates: PASS — Resizing pane to 80×24 reflows layout; tab bar narrows (labels hidden — see U2), columns narrow, no overflow. Sidebar auto-hides below a width threshold. Restoring to 220×50 brings full layout back including sidebar; no stale artefacts.
- **TC-SS-05** Folder switch via sidebar: PASS — In Cleanup tab (where sidebar `j`/`k` navigation works), Shift+Tab focuses the sidebar (purple highlight on current folder). `j` moves cursor to Sent. Enter switches folder; status bar updates to "Sent | 4 unread / 444 total". Note: sidebar `j`/`k` does not work in Timeline tab (see U3).
- **TC-SS-06** Graceful quit and server persistence: PASS — Pressing `q` closes the SSH connection; shell prompt returns in client pane with no error output. Server pane shows no crash. Second connection immediately after succeeds and TUI loads normally.
- **TC-SS-07** Host key persistence across server restart: PASS — After Ctrl-C and restart, connecting without `-o StrictHostKeyChecking=no` raises no "REMOTE HOST IDENTIFICATION HAS CHANGED" warning. Same ed25519 key fingerprint (`AAAAC3NzaC1lZDI1NTE5AAAAIBAMTaWt+GQ78oAu/WUDhYRfDyR5I7dV5fLC2gHOU6MF`) reused from `.ssh/host_ed25519`. TUI loads normally.
- **TC-SS-08** Multiple simultaneous sessions: PASS — Two SSH sessions connect concurrently. Switching session 1 to Compose tab leaves session 2 on Timeline. Sessions render and navigate independently; no data corruption or crash in either.
- **TC-SS-09** Missing config file: PASS — `./ssh-server-test -config /nonexistent/proton.yaml` prints `"Failed to load config: config file permission check failed: stat /nonexistent/proton.yaml: no such file or directory"` to stderr and exits with code 1. No panic, no stack trace.
