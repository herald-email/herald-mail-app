# SSH Server Test Plan — Herald

Manual QA checklist for verifying that the SSH server delivers a fully functional TUI over an SSH connection.
Run this after any change to `herald ssh`, `internal/sshserver/`, `cmd/herald-ssh-server/`, connection handling, or TUI rendering.

---

## Setup

### 1. Build the Herald binary

```bash
go build -o /tmp/herald-test ./main.go
go build -o /tmp/herald-ssh-server-test ./cmd/herald-ssh-server  # compatibility wrapper
```

### 2. Start the server in a tmux pane

```bash
# Pane A — server
tmux new-session -d -s ssh_test
tmux send-keys -t ssh_test '/tmp/herald-test ssh -config proton.yaml -addr :2222' Enter
sleep 2   # wait for server to initialise
```

### 3. Open a second pane for the SSH client

```bash
# Pane B — client
tmux split-window -t ssh_test -h
tmux send-keys -t ssh_test:0.1 'ssh -p 2222 -o StrictHostKeyChecking=no -o LogLevel=ERROR localhost' Enter
sleep 6   # wait for TUI to load
```

### 4. Capture screenshots from the client pane

```bash
tmux capture-pane -t ssh_test:0.1 -p -e > /tmp/ssh_cap.txt
cat /tmp/ssh_cap.txt
```

### 5. Send keystrokes to the client pane

```bash
# Single key
tmux send-keys -t ssh_test:0.1 'j' ''

# Enter
tmux send-keys -t ssh_test:0.1 '' ''

# Escape
tmux send-keys -t ssh_test:0.1 '' ''

sleep 0.3
```

### 6. Resize the client pane

```bash
tmux resize-pane -t ssh_test:0.1 -x 80 -y 24
sleep 0.3
tmux capture-pane -t ssh_test:0.1 -p -e > /tmp/ssh_cap_80.txt
```

### 7. Teardown

```bash
# Quit TUI in client pane
tmux send-keys -t ssh_test:0.1 'q' ''
sleep 1

# Kill server
tmux send-keys -t ssh_test:0.0 'C-c' ''
sleep 1

tmux kill-session -t ssh_test
```

---

## Prerequisites

| Item | Required |
|------|----------|
| `proton.yaml` present and valid | Yes |
| `email_cache.db` populated (run TUI first) | Recommended |
| Port 2222 free on localhost | Yes |
| `ssh` client available | Yes |
| Ollama running (for AI test cases) | TC-SS-12 semantic path, TC-SS-17 |

---

## What to Look for in Every Capture

- **Rendering fidelity** — tab bar, table borders, and status bar appear correctly; no garbled escape sequences
- **Overflow** — no lines wrap past the right edge of the SSH pane
- **Truncation** — long subjects and senders end with `…`, not broken bytes
- **Responsiveness** — key presses produce visible changes within ~0.5 seconds
- **No crash output** — no raw Go stack trace visible in either server or client pane

---

## Test Cases

### TC-SS-00 — CLI discovery and compatibility wrapper

**Steps:**
```bash
/tmp/herald-test --help
/tmp/herald-test ssh --version
/tmp/herald-ssh-server-test --version
```

**Expect:**
- Root help advertises `herald ssh`.
- `herald ssh --version` exits successfully without starting a listener.
- Legacy `herald-ssh-server --version` exits successfully and remains available for existing scripts.

### TC-SS-01 — Successful connection and initial render

**Steps:**
1. Follow Setup steps 1–4.
2. Capture screenshot after TUI loads.

**Expect:**
- Tab bar visible: `1  Timeline  2  Compose  3  Cleanup`
- Timeline table populated or "No emails" message shown
- Folder sidebar visible on the left
- Status bar at the bottom shows folder name and email count
- No garbled bytes or raw escape sequences
- Server pane (Pane A) shows a connection log line

---

### TC-SS-02 — All tabs accessible over SSH

**Steps:**
1. Connect (TC-SS-01 setup).
2. Press `1` → capture.
3. Press `2` → capture.
4. Press `3` → capture.
5. Press `1` to return to Timeline.

**Expect:**
- Each tab highlights correctly in the tab bar
- Timeline: chronological list with Sender, Subject, Date columns
- Compose: To / Subject fields and body textarea visible
- Cleanup: two-panel layout (senders left, messages right)
- No layout corruption when switching

---

### TC-SS-03 — Email navigation and body preview

**Steps:**
1. Switch to Timeline (`1`).
2. Press `j` five times.
3. Press Enter.
4. Wait 2 seconds for body to load.
5. Capture screenshot.
6. Press `j` three times to scroll the preview body.
7. Capture screenshot.
8. Press Escape.
9. Capture screenshot.

**Expect (step 5 — preview open):**
- Screen splits: timeline table left, preview panel right
- Preview header shows From, Date, Subject
- Body text visible (or "Loading…" briefly)
- No column overflow in either panel

**Expect (step 7 — after scroll):**
- Body scrolls down; scroll indicator updates (`line N/M  XX%`)
- Timeline table cursor does not change

**Expect (step 9 — preview closed):**
- Layout returns to full-width timeline
- Cursor remains on the previously selected email

---

### TC-SS-04 — Terminal resize propagates

**Steps:**
1. Start with client pane at 220×50, capture.
2. Resize pane to 80×24: `tmux resize-pane -t ssh_test:0.1 -x 80 -y 24`
3. Wait 0.5 seconds, capture.
4. Resize back to 220×50.
5. Wait 0.5 seconds, capture.

**Expect (step 3 — 80×24):**
- Layout reflows; columns narrow proportionally
- No overflow; tab bar and status bar still visible
- Sidebar may auto-hide if too narrow

**Expect (step 5 — 220×50 restored):**
- Full layout restored; sidebar reappears if it was auto-hidden
- No stale artefacts from the previous size

---

### TC-SS-05 — Folder switch via sidebar

**Steps:**
1. Ensure sidebar is visible (press `f` if needed).
2. Press `Tab` until sidebar is focused.
3. Press `j` to move to a different folder.
4. Press Enter to switch.
5. Wait 3 seconds, capture.

**Expect:**
- Status bar updates with the new folder name
- Email list repopulates for the new folder
- Empty folder shows "No emails in this folder", not blank rows

---

### TC-SS-06 — Graceful quit and server persistence

**Steps:**
1. Connect (TC-SS-01 setup).
2. Press `q` in the client pane.
3. Capture client pane immediately after.
4. Wait 1 second; attempt a second connection:
   ```bash
   tmux send-keys -t ssh_test:0.1 'ssh -p 2222 -o StrictHostKeyChecking=no -o LogLevel=ERROR localhost' Enter
   sleep 5
   ```
5. Capture the new session.

**Expect (step 3):**
- SSH connection closes cleanly; shell prompt returns in client pane
- No error output in server pane

**Expect (step 5):**
- Second connection succeeds and TUI loads normally
- Server has not crashed or hung

---

### TC-SS-07 — Host key persistence across server restart

**Steps:**
1. Stop the server (Ctrl-C in Pane A).
2. Note the host key fingerprint shown when first connecting (or check `.ssh/host_ed25519.pub`).
3. Restart the server:
   ```bash
  tmux send-keys -t ssh_test:0.0 '/tmp/herald-test ssh -config proton.yaml -addr :2222' Enter
   sleep 2
   ```
4. Connect again from the client pane.

**Expect:**
- No "REMOTE HOST IDENTIFICATION HAS CHANGED" SSH warning
- TUI loads normally — same experience as the first connection

---

### TC-SS-08 — Multiple simultaneous sessions

**Steps:**
1. Start server (Pane A).
2. Open client session 1 in Pane B.
3. Open a third tmux pane and connect a second client:
   ```bash
   tmux split-window -t ssh_test -v
   tmux send-keys -t ssh_test:0.2 'ssh -p 2222 -o StrictHostKeyChecking=no -o LogLevel=ERROR localhost' Enter
   sleep 5
   ```
4. In session 1, press `j` several times.
5. Capture both client panes.

**Expect:**
- Both sessions render independently; navigation in one does not affect the other
- Server pane shows two connection log lines
- No crash or data corruption in either session

---

### TC-SS-09 — Missing config file error handling

**Steps:**
1. Start server with a non-existent config path:
   ```bash
   /tmp/herald-test ssh -config /nonexistent/proton.yaml -addr :2223
   ```
2. Capture terminal output.

**Expect:**
- Server prints a clear error message (e.g. `failed to load config: …`)
- Process exits with a non-zero status code
- No panic or stack trace

---

---

### TC-SS-10 — Deletion confirmation over SSH

**Steps:**
1. Connect via SSH (TC-SS-01 setup).
2. Switch to Timeline (`1`), navigate to an email.
3. Press `D`.
4. Capture screenshot.
5. Press `n`, capture screenshot.
6. Press `D` again, press `y`.
7. Capture screenshot after reload.

**Expect (step 4):**
- Status bar turns red with subject and `[y] confirm  [n/Esc] cancel`
- No layout corruption in the SSH terminal

**Expect (step 5):**
- Status bar returns to normal; email still present

**Expect (step 7):**
- Email removed; timeline reloads cleanly

---

### TC-SS-11 — Archive email over SSH

**Steps:**
1. Connect via SSH, switch to Timeline.
2. Press `e` on an email.
3. Capture screenshot (confirmation).
4. Press `y`.
5. Wait 3 seconds, capture screenshot.

**Expect:**
- Confirmation bar shows "Archive …?"
- After `y`: email removed from timeline
- Sidebar folder counts update (Archive count increases)
- No crash or garbled output

---

### TC-SS-12 — Search over SSH (keyword, body, cross-folder, semantic)

**Steps:**
1. Connect via SSH, switch to Timeline.
2. Press `/`, type `github`.
3. Capture screenshot (filtered results).
4. Clear with Esc.
5. Press `/`, type `/b invoice` (body search).
6. Capture screenshot.
7. Press `/`, type `/* hello` (cross-folder).
8. Capture screenshot.
9. Press `/`, type `? swiftui performance` (semantic search).
10. Capture screenshot.
11. Press `/`, type a term that doesn't exist in this folder (e.g. `zzznomatch`).
12. Capture screenshot.

**Expect (steps 3, 6, 8, 10):**
- Key hint bar shows search input text
- Timeline updates in real time as text is typed
- No rendering artifacts or overflowing lines
- Source tags visible in status bar
- Semantic queries either return bounded results or a clear AI/degraded-state message; they must not wedge the SSH render.

**Expect (step 12 — zero results):**
- Status bar shows `Search: 0 results`
- Key hint bar shows: `No results in this folder — try: /* zzznomatch`

---

### TC-SS-13 — Background polling notification over SSH

**Steps:**
1. Connect via SSH, wait for load to complete.
2. Observe status bar for `↻ Ns` countdown.
3. (Optional) Send a new email from another client.
4. Wait for countdown to reach 0.
5. Capture screenshot.

**Expect:**
- `↻ Ns` visible and counting down
- After poll: new email appears without manual refresh (if sent in step 3)
- No crash, freeze, or layout corruption during poll

---

### TC-SS-14 — Full-screen email view over SSH

**Steps:**
1. Connect via SSH (TC-SS-01 setup).
2. Switch to Timeline (`1`), open body preview on any email.
3. Wait for body to load.
4. Press `z` to enter full-screen.
5. Capture screenshot.
6. Press `j` several times to scroll.
7. Capture screenshot.
8. Press `z` to exit; capture screenshot.
9. Re-enter with `z`, then press Escape; capture screenshot.

**Expect (step 5 — full-screen over SSH):**
- Tab bar, sidebar, and timeline table hidden
- Email body fills the entire SSH pane width and height
- From / Date / Subject header at top; scroll indicator at bottom
- No garbled escape sequences or rendering artifacts

**Expect (step 7 — scrolled):**
- Body scrolls; scroll indicator updates; timeline cursor unchanged

**Expect (steps 8 and 9 — exit):**
- Split layout restored; no blank panels or lingering full-screen state

---

### TC-SS-15 — Attachment display over SSH

**Prerequisites:** An email with at least one attachment present in the mailbox.

**Steps:**
1. Connect via SSH (TC-SS-01 setup).
2. Switch to Timeline (`1`), locate an email with attachment indicator.
3. Press Enter to open the body preview; wait for load.
4. Capture screenshot.
5. Tab to focus the preview panel.
6. Press `s`.
7. Capture screenshot (save-path prompt).
8. Create a file at the prompted save path, press `Enter`, and capture screenshot.
9. Press Escape to cancel.

**Expect (step 4):**
- `[attach] filename  mime/type  X KB` label visible below body
- Key hint bar shows `s: save attachment`
- No layout corruption or garbled output over SSH

**Expect (step 7):**
- Save-path input appears with pre-filled `~/Downloads/<filename>`
- If that path already exists on the server, the input is pre-filled with the next available filename and a warning is visible
- No crash or rendering artifacts

**Expect (step 8):**
- Existing file contents are not overwritten
- Prompt remains open with a suggested non-conflicting filename
- Warning text explains that the requested path already exists

**Expect (step 9):**
- Prompt dismissed; preview returns to normal state

---

### TC-SS-16 — Unsubscribe over SSH

**Steps:**
1. Connect via SSH (TC-SS-01 setup).
2. Switch to Timeline (`1`), open body preview on a newsletter email.
3. Wait for body to load; Tab to focus preview panel.
4. If `u: unsubscribe` is visible in key hints, press `u`.
5. Capture screenshot (confirmation bar).
6. Press `n` to cancel; capture screenshot.

**Expect (step 5):**
- Orange confirmation bar visible: `Unsubscribe from <sender>?  [y] confirm  [n/Esc] cancel`
- No rendering artifacts or garbled escape sequences over SSH

**Expect (step 6):**
- Status bar returns to normal; preview unchanged

---

### TC-SS-17 — Compose AI assistant over SSH

**Prerequisites:** AI backend configured and reachable for the server config under test.

**Steps:**
1. Connect via SSH and switch to Compose (`2`).
2. Ensure the draft body contains text.
3. Press `Ctrl+G` to open the AI assistant.
4. Capture the SSH pane.
5. Trigger one quick action (`1`-`5`) or enter a prompt and press `Enter`.
6. Wait for a suggestion or bounded error and capture again.
7. Press `Ctrl+J` to request a subject suggestion, then press `Tab` to accept it.
8. Repeat once with AI unavailable or misconfigured.

**Expect:**
- The AI panel opens without corrupting the SSH pane layout, tab bar, or status bar.
- Success path shows loading and then a visible AI suggestion or diff area.
- Subject-hint acceptance works over SSH and does not leave stale overlay state behind.
- Failure path stays readable and bounded; no panic, raw escape corruption, or wedged session.

---

## Result Format

After completing all test cases, write up findings using this structure:

```
## Test Run — <date> — SSH server <version>

### Bugs

| ID  | Severity | TC     | Description                        | Steps to reproduce |
|-----|----------|--------|------------------------------------|--------------------|
| B1  | High     | SS-04  | Layout not redrawn after resize    | TC-SS-04 step 3    |

### UX Issues

| ID  | TC     | Description                                | Suggestion                        |
|-----|--------|--------------------------------------------|-----------------------------------|
| U1  | SS-01  | No log line when client disconnects        | Add disconnect log to server pane |

### All Good

List test cases that passed with no issues:
- TC-SS-01 Connection and render: PASS
- TC-SS-02 Tab switching: PASS
- ...
```

Only open a bug or UX item when something is clearly wrong or clearly improvable.
