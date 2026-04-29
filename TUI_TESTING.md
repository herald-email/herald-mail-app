# Bubbletea TUI Agent Automation

Guide for building AI agent harnesses that can run, screenshot, and interact with Bubbletea TUI applications programmatically.

## Overview

A TUI app renders via ANSI escape sequences to a terminal. To let an AI agent drive it, you need:

1. A way to run the TUI (either in-process or in a PTY)
2. A virtual terminal emulator to parse escape codes into a screen buffer
3. A way to capture the screen as text ("screenshot")
4. A way to send keystrokes and mouse events

The Charm ecosystem (`github.com/charmbracelet/x`) provides all of these as Go packages.

## Architecture Decision

There are three approaches. Choose based on your constraints.

### Option A: In-Process via `teatest` (White-Box)

Drive the `tea.Model` directly. No subprocess, no PTY. Best when you own the source code and want tight control.

**Pros:** Fastest, no process management, direct access to model state.
**Cons:** Requires importing the model. The `View()` method is not directly exposed on `TestModel` for alt-screen apps — you must read from the output stream. Experimental API, may change.

**Package:** `github.com/charmbracelet/x/exp/teatest`

### Option B: PTY + Virtual Terminal (Black-Box)

Spawn the compiled binary in a pseudo-terminal, pipe output through a virtual terminal emulator, read the screen buffer. Best when treating the binary as an opaque executable.

**Pros:** Works with any TUI binary. Agent sees exactly what a user sees. Robust for alt-screen/full-screen apps.
**Cons:** More setup. Need to manage process lifecycle and handle async rendering.

**Packages:**

- `github.com/charmbracelet/x/vt` — Virtual terminal emulator
- `github.com/creack/pty` — PTY allocation (Unix)
- `github.com/charmbracelet/x/xpty` — Cross-platform PTY (alternative)

### Option C: tmux (Black-Box, No Go Dependencies)

Run the TUI in a headless tmux session. Use `capture-pane` for screenshots, `send-keys` for input. Works from Go, bash, or any language that can shell out.

**Pros:** Zero library dependencies. Works with any binary. Easy to debug interactively (attach to the session). Great for CI. Shell-scriptable test scenarios.
**Cons:** Requires tmux installed. Coarser timing control. Output capture is text-only (no cell-level metadata). Slightly slower due to process spawning per command.

## Package Reference

### `github.com/charmbracelet/x/vt`

Full virtual terminal emulator. Parses ANSI/VT100+ escape sequences and maintains an in-memory screen.

#### Core Types

```go
// Emulator is the main type. NOT concurrency-safe.
e := vt.NewEmulator(80, 24) // width, height in cells

// SafeEmulator wraps Emulator with a mutex. Use this from agent goroutines.
se := vt.NewSafeEmulator(80, 24)
```

#### Writing Output (TUI → Emulator)

Feed the TUI's raw stdout into the emulator. The emulator parses escape codes and updates its screen buffer.

```go
// Write raw terminal output (from PTY or pipe) into the emulator
n, err := e.Write(data) // implements io.Writer

// Or write a string directly
n, err := e.WriteString(s)
```

#### Reading the Screen ("Screenshot")

```go
// Plain text — all cells joined into lines, trailing whitespace trimmed.
// This is what you send to the LLM.
screenshot := e.String()

// With ANSI styles/colors preserved (useful for debugging, not for LLM).
rendered := e.Render()

// Individual cell access
cell := e.CellAt(x, y) // returns *uv.Cell or nil if out of bounds

// Screen dimensions
w := e.Width()
h := e.Height()

// Cursor position
pos := e.CursorPosition() // returns uv.Position with X, Y fields

// Check if in alternate screen mode (most full-screen TUIs use this)
alt := e.IsAltScreen()
```

#### Sending Input (Agent → TUI)

The emulator has an internal input pipe. For PTY-based setups, write directly to the PTY master fd instead.

```go
// Send a key event
e.SendKey(vt.KeyPressEvent{Code: vt.KeyDown})
e.SendKey(vt.KeyPressEvent{Code: vt.KeyEnter})
e.SendKey(vt.KeyPressEvent{Code: 'q'}) // regular character

// With modifiers
e.SendKey(vt.KeyPressEvent{Code: 'c', Mod: vt.ModCtrl}) // Ctrl+C

// Send multiple keys
e.SendKeys(
    vt.KeyPressEvent{Code: vt.KeyDown},
    vt.KeyPressEvent{Code: vt.KeyDown},
    vt.KeyPressEvent{Code: vt.KeyEnter},
)

// Send arbitrary text (types each rune)
e.SendText("hello world")

// Paste text (uses bracketed paste if enabled)
e.Paste("pasted content")

// Send mouse events
e.SendMouse(vt.MouseClick{Button: vt.MouseLeft, X: 10, Y: 5})
e.SendMouse(vt.MouseRelease{Button: vt.MouseLeft, X: 10, Y: 5})
e.SendMouse(vt.MouseWheel{Button: vt.MouseWheelDown, X: 10, Y: 5})
e.SendMouse(vt.MouseMotion{X: 15, Y: 8})

// Get the input pipe for raw writes (for PTY-less usage)
inputPipe := e.InputPipe() // returns io.Writer
```

#### Key Constants

```go
// Navigation
vt.KeyUp, vt.KeyDown, vt.KeyLeft, vt.KeyRight
vt.KeyHome, vt.KeyEnd, vt.KeyPgUp, vt.KeyPgDown

// Editing
vt.KeyEnter, vt.KeyReturn   // same key
vt.KeyBackspace, vt.KeyDelete, vt.KeyInsert
vt.KeyTab, vt.KeyEscape, vt.KeySpace

// Function keys
vt.KeyF1 through vt.KeyF63

// Modifiers (combine with | on KeyPressEvent.Mod)
vt.ModShift, vt.ModAlt, vt.ModCtrl, vt.ModMeta
```

#### Mouse Button Constants

```go
vt.MouseLeft, vt.MouseMiddle, vt.MouseRight
vt.MouseWheelUp, vt.MouseWheelDown, vt.MouseWheelLeft, vt.MouseWheelRight
vt.MouseBackward, vt.MouseForward
```

#### Lifecycle

```go
e.Resize(120, 40)  // resize the virtual terminal
e.Close()          // close the terminal
```

#### Callbacks (Optional)

React to terminal events:

```go
e.SetCallbacks(vt.Callbacks{
    Bell:             func() { /* TUI rang the bell */ },
    Title:            func(t string) { /* title changed */ },
    AltScreen:        func(on bool) { /* entered/exited alt screen */ },
    CursorPosition:   func(old, new uv.Position) { /* cursor moved */ },
    CursorVisibility: func(visible bool) { /* cursor shown/hidden */ },
})
```

### `github.com/charmbracelet/x/exp/teatest`

In-process testing harness for Bubbletea models.

```go
import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/x/exp/teatest"
)

// Create test model (requires a *testing.T — can be faked for non-test usage)
tm := teatest.NewTestModel(t, yourModel,
    teatest.WithInitialTermSize(80, 24),
)

// Send messages (key presses, custom messages, etc.)
tm.Send(tea.KeyMsg{Type: tea.KeyDown})
tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
tm.Type("hello") // convenience for typing text

// Read output (returns an io.Reader that streams rendered frames)
reader := tm.Output()

// Wait for specific content to appear
teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
    return strings.Contains(string(bts), "expected text")
})

// Get final output after program exits
tm.Quit()
finalOutput := tm.FinalOutput(t) // returns io.Reader, blocks until exit

// Get the final model state (for inspecting internal state)
finalModel := tm.FinalModel(t) // returns tea.Model

// Access the underlying tea.Program
program := tm.GetProgram()
```

### `github.com/creack/pty`

PTY allocation for Unix. Use this to spawn TUI binaries.

```go
import (
    "os/exec"
    "github.com/creack/pty"
)

cmd := exec.Command("./my-tui")
cmd.Env = append(os.Environ(), "TERM=xterm-256color")

// Start with specific size
ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
    Rows: 24,
    Cols: 80,
})
defer ptmx.Close()

// ptmx is an *os.File — read for output, write for input
// Read: ptmx.Read(buf) → TUI's stdout/stderr
// Write: ptmx.Write([]byte("\x1b[B")) → send Down arrow to TUI

// Resize
pty.Setsize(ptmx, &pty.Winsize{Rows: 40, Cols: 120})
```

## Implementation Patterns

### Pattern 1: Black-Box Agent Loop (PTY + vt)

Recommended for most agent scenarios. Spawn the binary, capture the screen, send actions.

```go
package agent

import (
    "os"
    "os/exec"
    "time"

    "github.com/charmbracelet/x/vt"
    "github.com/creack/pty"
)

const (
    termWidth  = 80
    termHeight = 24
    settleTime = 150 * time.Millisecond // wait for TUI to finish rendering
)

type TUIAgent struct {
    cmd   *exec.Cmd
    ptmx  *os.File
    term  *vt.SafeEmulator
    done  chan struct{}
}

// Start launches the TUI binary in a PTY and begins piping output
// to the virtual terminal emulator.
func Start(binary string, args ...string) (*TUIAgent, error) {
    cmd := exec.Command(binary, args...)
    cmd.Env = append(os.Environ(), "TERM=xterm-256color")

    ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
        Rows: termHeight,
        Cols: termWidth,
    })
    if err != nil {
        return nil, err
    }

    term := vt.NewSafeEmulator(termWidth, termHeight)
    agent := &TUIAgent{
        cmd:  cmd,
        ptmx: ptmx,
        term: term,
        done: make(chan struct{}),
    }

    // Pipe PTY output into the virtual terminal emulator.
    // This goroutine runs until the PTY is closed.
    go func() {
        defer close(agent.done)
        buf := make([]byte, 4096)
        for {
            n, err := ptmx.Read(buf)
            if err != nil {
                return
            }
            term.Write(buf[:n])
        }
    }()

    // Wait for initial render
    time.Sleep(settleTime)
    return agent, nil
}

// Screenshot returns the current screen content as plain text.
// This is what you pass to the LLM.
func (a *TUIAgent) Screenshot() string {
    return a.term.String()
}

// ScreenshotWithANSI returns screen content with ANSI escape codes
// for colors and styles. Useful for debugging.
func (a *TUIAgent) ScreenshotWithANSI() string {
    return a.term.Render()
}

// SendKey sends a single key press to the TUI.
func (a *TUIAgent) SendKey(code rune, mod vt.KeyMod) {
    // For PTY-based approach, write raw bytes to ptmx.
    // The vt package's SendKey is for emulator-internal input.
    // Convert key to ANSI escape sequence and write to PTY.
    a.term.SendKey(vt.KeyPressEvent{Code: code, Mod: mod})
    // Read the encoded key from the emulator's output
    buf := make([]byte, 64)
    n, _ := a.term.Read(buf)
    if n > 0 {
        a.ptmx.Write(buf[:n])
    }
}

// SendRawBytes sends raw bytes directly to the PTY.
// Use for escape sequences or text input.
func (a *TUIAgent) SendRawBytes(b []byte) {
    a.ptmx.Write(b)
}

// SendText types text into the TUI character by character.
func (a *TUIAgent) SendText(text string) {
    a.ptmx.Write([]byte(text))
}

// WaitForSettle waits for the TUI to finish rendering after input.
func (a *TUIAgent) WaitForSettle() {
    time.Sleep(settleTime)
}

// CursorPosition returns the current cursor position.
func (a *TUIAgent) CursorPosition() (x, y int) {
    pos := a.term.CursorPosition()
    return pos.X, pos.Y
}

// IsAltScreen returns true if the TUI is in alternate screen mode.
func (a *TUIAgent) IsAltScreen() bool {
    return a.term.IsAltScreen()
}

// Close terminates the TUI process and cleans up.
func (a *TUIAgent) Close() error {
    a.ptmx.Close()
    <-a.done
    return a.cmd.Wait()
}
```

**Agent loop using the harness:**

```go
func RunAgentLoop(binary string, task string) error {
    agent, err := Start(binary)
    if err != nil {
        return err
    }
    defer agent.Close()

    for i := 0; i < maxIterations; i++ {
        // 1. Capture screen
        screen := agent.Screenshot()

        // 2. Ask LLM what to do
        action := askLLM(screen, task)

        // 3. Check if done
        if action.Done {
            return nil
        }

        // 4. Execute action
        switch action.Type {
        case "key":
            agent.SendRawBytes(action.RawBytes)
        case "text":
            agent.SendText(action.Text)
        }

        // 5. Wait for render
        agent.WaitForSettle()
    }
    return fmt.Errorf("max iterations reached")
}
```

### Pattern 2: In-Process Agent (teatest)

When you own the model and want to avoid subprocess overhead.

```go
package agent

import (
    "io"
    "strings"
    "testing"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/x/exp/teatest"
)

type InProcessAgent struct {
    tm     *teatest.TestModel
    output *strings.Builder
}

// StartInProcess creates an agent that drives the model directly.
// NOTE: teatest requires a *testing.T. For non-test use, create a
// stub or use testing.TB interface.
func StartInProcess(t *testing.T, model tea.Model) *InProcessAgent {
    tm := teatest.NewTestModel(t, model,
        teatest.WithInitialTermSize(80, 24),
    )

    return &InProcessAgent{
        tm:     tm,
        output: &strings.Builder{},
    }
}

// WaitForContent blocks until the given text appears in the output.
func (a *InProcessAgent) WaitForContent(t *testing.T, text string) {
    teatest.WaitFor(t, a.tm.Output(), func(bts []byte) bool {
        return strings.Contains(string(bts), text)
    })
}

// SendKey sends a Bubbletea key message.
func (a *InProcessAgent) SendKey(keyType tea.KeyType) {
    a.tm.Send(tea.KeyMsg{Type: keyType})
}

// SendRunes sends character input.
func (a *InProcessAgent) SendRunes(s string) {
    a.tm.Type(s)
}

// Quit tells the program to exit and returns the final output.
func (a *InProcessAgent) Quit(t *testing.T) string {
    a.tm.Quit()
    out, _ := io.ReadAll(a.tm.FinalOutput(t))
    return string(out)
}
```

### Pattern 3: tmux Testing Harness

tmux provides a complete testing solution with no Go library dependencies. It handles PTY allocation, terminal emulation, and screen capture internally. You interact with it via CLI commands.

#### Prerequisites

```bash
# macOS
brew install tmux

# Ubuntu/Debian
apt install tmux

# Verify
tmux -V  # should be 3.0+ for best compatibility
```

#### Core tmux Commands for TUI Testing

**Session lifecycle:**

```bash
# Start TUI in a detached session with fixed dimensions
tmux new-session -d -s test -x 80 -y 24 './my-tui-app --some-flag'

# Start with environment variables
tmux new-session -d -s test -x 80 -y 24 -e TERM=xterm-256color -e MY_VAR=value './my-tui-app'

# Kill the session when done
tmux kill-session -t test

# Check if session exists (useful in cleanup)
tmux has-session -t test 2>/dev/null && tmux kill-session -t test
```

**Sending input:**

```bash
# Send named keys
tmux send-keys -t test Down
tmux send-keys -t test Enter
tmux send-keys -t test Escape
tmux send-keys -t test Tab
tmux send-keys -t test BSpace      # Backspace
tmux send-keys -t test Space
tmux send-keys -t test C-c         # Ctrl+C
tmux send-keys -t test C-d         # Ctrl+D
tmux send-keys -t test C-z         # Ctrl+Z
tmux send-keys -t test F1          # Function keys F1-F12
tmux send-keys -t test Up Up Enter # Multiple keys in one call

# Send literal text (no key interpretation)
tmux send-keys -t test -l 'hello world'

# IMPORTANT: -l flag matters!
# Without -l: "C-c" is interpreted as Ctrl+C
# With -l:    "C-c" is typed literally as the characters C, -, c
```

**Capturing the screen:**

```bash
# Capture visible pane content as plain text
tmux capture-pane -t test -p

# Capture with trailing whitespace preserved (important for layout testing)
tmux capture-pane -t test -p -e  # includes escape sequences (colors)

# Capture to a file
tmux capture-pane -t test -p > screenshot.txt

# Capture specific line range (0-indexed from top of visible area)
tmux capture-pane -t test -p -S 0 -E 5   # first 6 lines only

# Capture including scrollback history
tmux capture-pane -t test -p -S -1000     # last 1000 lines of scrollback

# Strip trailing blank lines (useful for comparison)
tmux capture-pane -t test -p | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}'
```

**Other useful commands:**

```bash
# Resize the terminal mid-test
tmux resize-window -t test -x 120 -y 40

# Wait for a specific duration (tmux 3.2+)
tmux wait-for -S done  # signal
tmux wait-for done      # wait for signal

# Attach for interactive debugging (opens the session in your terminal)
tmux attach -t test

# Detach back: press Ctrl+B then D

# List all sessions
tmux list-sessions

# Check if the TUI process is still running
tmux list-panes -t test -F '#{pane_dead}'  # 0 = alive, 1 = dead
```

#### tmux Key Name Reference

| Key             | tmux Name                     | Notes            |
| --------------- | ----------------------------- | ---------------- |
| Arrow keys      | `Up`, `Down`, `Left`, `Right` |                  |
| Enter           | `Enter`                       |                  |
| Escape          | `Escape`                      |                  |
| Tab             | `Tab`                         |                  |
| Shift+Tab       | `BTab`                        |                  |
| Backspace       | `BSpace`                      |                  |
| Delete          | `DC`                          |                  |
| Insert          | `IC`                          |                  |
| Home            | `Home`                        |                  |
| End             | `End`                         |                  |
| Page Up         | `PPage`                       |                  |
| Page Down       | `NPage`                       |                  |
| Space           | `Space`                       | Or use `-l ' '`  |
| F1-F12          | `F1` through `F12`            |                  |
| Ctrl+letter     | `C-a` through `C-z`           | Lowercase letter |
| Alt+letter      | `M-a` through `M-z`           |                  |
| Ctrl+Alt+letter | `C-M-a`                       | Combine prefixes |

#### Shell Script Test Scenarios

A reusable test harness in bash:

```bash
#!/usr/bin/env bash
# test_tui.sh — shell-based TUI test harness
set -euo pipefail

BINARY="./my-tui-app"
SESSION="tui-test-$$"  # unique per process to allow parallel runs
WIDTH=80
HEIGHT=24
SETTLE=0.2  # seconds to wait after each input

# ── Helpers ──────────────────────────────────────────────────────

start_tui() {
    local cmd="${1:-$BINARY}"
    tmux new-session -d -s "$SESSION" -x "$WIDTH" -y "$HEIGHT" "$cmd"
    sleep 0.5  # wait for initial render
}

kill_tui() {
    tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap kill_tui EXIT  # always cleanup

screenshot() {
    tmux capture-pane -t "$SESSION" -p
}

send_keys() {
    tmux send-keys -t "$SESSION" "$@"
    sleep "$SETTLE"
}

send_text() {
    tmux send-keys -t "$SESSION" -l "$1"
    sleep "$SETTLE"
}

# Assert that the screen contains a string
assert_screen_contains() {
    local expected="$1"
    local screen
    screen=$(screenshot)
    if echo "$screen" | grep -qF "$expected"; then
        echo "  ✓ Found: $expected"
    else
        echo "  ✗ NOT found: $expected"
        echo "  Screen was:"
        echo "$screen" | sed 's/^/    /'
        return 1
    fi
}

# Assert that the screen does NOT contain a string
assert_screen_not_contains() {
    local unexpected="$1"
    local screen
    screen=$(screenshot)
    if echo "$screen" | grep -qF "$unexpected"; then
        echo "  ✗ Unexpectedly found: $unexpected"
        echo "  Screen was:"
        echo "$screen" | sed 's/^/    /'
        return 1
    else
        echo "  ✓ Correctly absent: $unexpected"
    fi
}

# Save screenshot to golden file
save_golden() {
    local name="$1"
    screenshot > "testdata/${name}.golden"
    echo "  Saved golden: testdata/${name}.golden"
}

# Compare screenshot against golden file
assert_matches_golden() {
    local name="$1"
    local golden="testdata/${name}.golden"
    local actual
    actual=$(screenshot)

    if [ ! -f "$golden" ]; then
        echo "  Golden file missing: $golden"
        echo "  Run with UPDATE_GOLDEN=1 to create it"
        echo "$actual" > "$golden"
        return 1
    fi

    if diff -u "$golden" <(echo "$actual") > /dev/null 2>&1; then
        echo "  ✓ Matches golden: $name"
    else
        echo "  ✗ Golden mismatch: $name"
        diff -u "$golden" <(echo "$actual") | head -30
        if [ "${UPDATE_GOLDEN:-0}" = "1" ]; then
            echo "$actual" > "$golden"
            echo "  Updated golden file"
        fi
        return 1
    fi
}

# Wait until screen contains a string (with timeout)
wait_for_content() {
    local expected="$1"
    local timeout="${2:-5}"
    local elapsed=0
    while [ "$elapsed" -lt "$timeout" ]; do
        if screenshot | grep -qF "$expected"; then
            return 0
        fi
        sleep 0.2
        elapsed=$((elapsed + 1))
    done
    echo "  ✗ Timed out waiting for: $expected"
    return 1
}

# ── Test Cases ───────────────────────────────────────────────────

test_initial_screen() {
    echo "TEST: initial screen renders correctly"
    start_tui
    assert_screen_contains "Welcome"
    assert_screen_contains "Press Enter to continue"
    assert_matches_golden "initial_screen"
}

test_navigation() {
    echo "TEST: arrow keys navigate the list"
    start_tui
    wait_for_content "Item 1"

    send_keys Down
    assert_screen_contains "> Item 2"   # cursor moved

    send_keys Down Down
    assert_screen_contains "> Item 4"

    send_keys Up
    assert_screen_contains "> Item 3"
}

test_text_input() {
    echo "TEST: typing in a text field"
    start_tui
    wait_for_content "Search:"

    send_text "hello world"
    assert_screen_contains "hello world"

    # Test backspace
    send_keys BSpace BSpace BSpace BSpace BSpace  # delete "world"
    assert_screen_contains "hello"
    assert_screen_not_contains "world"
}

test_form_submission() {
    echo "TEST: filling and submitting a form"
    start_tui
    wait_for_content "Name:"

    send_text "John"
    send_keys Tab
    send_text "john@example.com"
    send_keys Tab
    send_keys Enter  # submit

    wait_for_content "Success"
    assert_screen_contains "John"
}

test_resize() {
    echo "TEST: TUI responds to terminal resize"
    start_tui
    wait_for_content "Ready"

    tmux resize-window -t "$SESSION" -x 40 -y 12
    sleep "$SETTLE"
    # Verify the TUI adapted its layout
    assert_screen_contains "Ready"
}

test_quit() {
    echo "TEST: q key quits the application"
    start_tui
    wait_for_content "Ready"

    send_keys q
    sleep 0.5

    # Check the process exited
    local dead
    dead=$(tmux list-panes -t "$SESSION" -F '#{pane_dead}' 2>/dev/null || echo "1")
    if [ "$dead" = "1" ]; then
        echo "  ✓ Process exited"
    else
        echo "  ✗ Process still running"
        return 1
    fi
}

# ── Runner ───────────────────────────────────────────────────────

run_tests() {
    local failed=0
    local total=0

    for test_fn in $(declare -F | awk '/test_/ {print $3}'); do
        total=$((total + 1))
        kill_tui  # clean slate

        if $test_fn; then
            echo ""
        else
            failed=$((failed + 1))
            echo "  FAILED"
            echo ""
        fi
        kill_tui
    done

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Results: $((total - failed))/$total passed"
    [ "$failed" -eq 0 ]
}

# Create testdata dir for golden files
mkdir -p testdata

run_tests
```

**Usage:**

```bash
# Run tests
./test_tui.sh

# Update golden files when output intentionally changes
UPDATE_GOLDEN=1 ./test_tui.sh

# Debug a failing test — attach to see what the TUI looks like
# (add a `sleep 999` or `read` in the test, then in another terminal:)
tmux attach -t tui-test-12345
```

#### Go Test Integration with tmux

Use tmux from `go test` with proper cleanup and parallel support:

```go
package myapp_test

import (
    "fmt"
    "os"
    "os/exec"
    "strings"
    "testing"
    "time"
)

type tmuxHarness struct {
    session string
    t       *testing.T
    width   int
    height  int
}

func newTmuxHarness(t *testing.T, binary string, width, height int) *tmuxHarness {
    t.Helper()

    // Check tmux is available
    if _, err := exec.LookPath("tmux"); err != nil {
        t.Skip("tmux not found, skipping integration test")
    }

    // Unique session name for parallel test safety
    session := fmt.Sprintf("test-%s-%d", t.Name(), os.Getpid())
    // tmux session names can't have dots or colons
    session = strings.NewReplacer(".", "-", ":", "-", "/", "-").Replace(session)

    h := &tmuxHarness{session: session, t: t, width: width, height: height}

    err := exec.Command("tmux", "new-session", "-d",
        "-s", session,
        "-x", fmt.Sprint(width),
        "-y", fmt.Sprint(height),
        binary,
    ).Run()
    if err != nil {
        t.Fatalf("failed to start tmux session: %v", err)
    }

    // Cleanup on test end
    t.Cleanup(func() {
        exec.Command("tmux", "kill-session", "-t", session).Run()
    })

    // Wait for initial render
    time.Sleep(500 * time.Millisecond)
    return h
}

func (h *tmuxHarness) screenshot() string {
    h.t.Helper()
    out, err := exec.Command("tmux", "capture-pane", "-t", h.session, "-p").Output()
    if err != nil {
        h.t.Fatalf("capture-pane failed: %v", err)
    }
    return string(out)
}

func (h *tmuxHarness) sendKeys(keys ...string) {
    h.t.Helper()
    args := append([]string{"send-keys", "-t", h.session}, keys...)
    if err := exec.Command("tmux", args...).Run(); err != nil {
        h.t.Fatalf("send-keys failed: %v", err)
    }
    time.Sleep(200 * time.Millisecond)
}

func (h *tmuxHarness) sendText(text string) {
    h.t.Helper()
    if err := exec.Command("tmux", "send-keys", "-t", h.session, "-l", text).Run(); err != nil {
        h.t.Fatalf("send-keys -l failed: %v", err)
    }
    time.Sleep(200 * time.Millisecond)
}

func (h *tmuxHarness) resize(width, height int) {
    h.t.Helper()
    exec.Command("tmux", "resize-window", "-t", h.session,
        "-x", fmt.Sprint(width),
        "-y", fmt.Sprint(height),
    ).Run()
    h.width = width
    h.height = height
    time.Sleep(200 * time.Millisecond)
}

func (h *tmuxHarness) waitForContent(text string, timeout time.Duration) bool {
    h.t.Helper()
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if strings.Contains(h.screenshot(), text) {
            return true
        }
        time.Sleep(100 * time.Millisecond)
    }
    return false
}

func (h *tmuxHarness) requireContent(text string) {
    h.t.Helper()
    if !strings.Contains(h.screenshot(), text) {
        h.t.Errorf("expected screen to contain %q\n\nActual screen:\n%s", text, h.screenshot())
    }
}

func (h *tmuxHarness) requireNoContent(text string) {
    h.t.Helper()
    if strings.Contains(h.screenshot(), text) {
        h.t.Errorf("expected screen NOT to contain %q\n\nActual screen:\n%s", text, h.screenshot())
    }
}

// --- Tests ---

func TestTUI_InitialRender(t *testing.T) {
    h := newTmuxHarness(t, "./my-tui-app", 80, 24)
    h.requireContent("Welcome")
}

func TestTUI_Navigation(t *testing.T) {
    h := newTmuxHarness(t, "./my-tui-app", 80, 24)
    h.waitForContent("Item 1", 3*time.Second)

    h.sendKeys("Down", "Down")
    h.requireContent("> Item 3")
}

func TestTUI_TextInput(t *testing.T) {
    h := newTmuxHarness(t, "./my-tui-app", 80, 24)
    h.waitForContent("Search:", 3*time.Second)

    h.sendText("query term")
    h.requireContent("query term")

    // Clear and retype
    for i := 0; i < 10; i++ {
        h.sendKeys("BSpace")
    }
    h.sendText("new query")
    h.requireContent("new query")
    h.requireNoContent("query term")
}

func TestTUI_Resize(t *testing.T) {
    h := newTmuxHarness(t, "./my-tui-app", 80, 24)
    h.waitForContent("Ready", 3*time.Second)

    h.resize(40, 12)
    h.requireContent("Ready") // still renders after resize
}
```

**Build and run:**

```bash
# Build the binary first (tests need the compiled binary)
go build -o my-tui-app .

# Run integration tests
go test -v -run TestTUI -count=1

# Skip tmux tests in environments without tmux
# (the harness auto-skips via t.Skip)
```

#### CI Configuration

tmux tests work in most CI environments. tmux runs headless by default.

**GitHub Actions:**

```yaml
name: TUI Integration Tests
on: [push, pull_request]
jobs:
    test:
        runs-on: ubuntu-latest
        steps:
            - uses: actions/checkout@v4
            - uses: actions/setup-go@v5
              with:
                  go-version: "1.22"
            - name: Install tmux
              run: sudo apt-get install -y tmux
            - name: Build
              run: go build -o my-tui-app .
            - name: Run TUI tests
              run: go test -v -run TestTUI -count=1 -timeout 60s
```

**Makefile integration:**

```makefile
.PHONY: test test-tui test-unit

test: test-unit test-tui

test-unit:
	go test -v -run 'Test[^T][^U][^I]' ./...

test-tui: build
	go test -v -run TestTUI -count=1 -timeout 60s

# Or with the shell script approach
test-tui-sh: build
	./test_tui.sh

build:
	go build -o my-tui-app .
```

#### Golden File Testing with tmux

For snapshot-based testing, capture the full screen and compare against known-good output.

```bash
# Directory layout:
# testdata/
#   initial_screen.golden
#   after_navigation.golden
#   search_results.golden
```

**Creating golden files:**

```bash
# Start the app, get it to the desired state, then capture:
tmux new-session -d -s golden -x 80 -y 24 './my-tui-app'
sleep 0.5
tmux capture-pane -t golden -p > testdata/initial_screen.golden

tmux send-keys -t golden Down Down Enter
sleep 0.2
tmux capture-pane -t golden -p > testdata/after_navigation.golden

tmux kill-session -t golden
```

**Comparing in Go tests:**

```go
func (h *tmuxHarness) assertGolden(name string) {
    h.t.Helper()
    golden := filepath.Join("testdata", name+".golden")
    actual := h.screenshot()

    if os.Getenv("UPDATE_GOLDEN") == "1" {
        os.MkdirAll("testdata", 0o755)
        os.WriteFile(golden, []byte(actual), 0o644)
        return
    }

    expected, err := os.ReadFile(golden)
    if err != nil {
        h.t.Fatalf("golden file %s not found (run with UPDATE_GOLDEN=1): %v", golden, err)
    }

    if string(expected) != actual {
        h.t.Errorf("screen does not match golden file %s\n\nExpected:\n%s\n\nActual:\n%s",
            golden, string(expected), actual)
    }
}
```

```bash
# Update golden files when output intentionally changes
UPDATE_GOLDEN=1 go test -v -run TestTUI -count=1
```

#### tmux Pitfalls and Tips

**Timing is everything.** tmux `send-keys` returns immediately, before the TUI processes the input. Always sleep after sending input. Start with 200ms; lower it if tests are slow, raise it if they're flaky.

**Session name collisions.** If tests run in parallel, each needs a unique session name. Use `t.Name()` + PID, or a UUID. If a test crashes without cleanup, stale sessions can cause the next run to fail. Add `tmux kill-server` to your CI setup step as a safety valve, or prefix cleanup:

```bash
# Kill any leftover test sessions before running
tmux list-sessions 2>/dev/null | grep '^test-' | cut -d: -f1 | xargs -I{} tmux kill-session -t {}
```

**Capture is a point-in-time snapshot.** `capture-pane` reads the current state of tmux's internal screen buffer. If the TUI is mid-render, you may get a partial frame. Always wait for settle first.

**Terminal raster image protocols.** tmux captures are still required for layout, key routing, fallback links, and escape-sequence checks, but tmux cannot prove actual raster placement for protocols such as iTerm2 OSC 1337, Kitty graphics, or Sixel. For changes that affect inline raster images, run the demo in a real compatible terminal as well, capture screenshots, record the terminal app/version and selected graphics mode, and verify native scrollback does not show images displacing pinned preview chrome.

**Browser raster image repro with ttyd + xterm.js.** Stock ttyd is useful for proving the Herald key flow, but its bundled frontend may not render iTerm2 OSC 1337 inline images. For browser proof, serve a custom ttyd index that loads `@xterm/addon-image`, then run Herald with `TERM_PROGRAM=iTerm.app` so Herald emits the iTerm2 inline image protocol:

```bash
make build
mkdir -p reports/ttyd-image-harness
$EDITOR reports/ttyd-image-harness/index.html
TERM_PROGRAM=iTerm.app ttyd -W -p 7682 \
  -I reports/ttyd-image-harness/index.html \
  -t rendererType=canvas \
  -t disableLeaveAlert=true \
  -t disableResizeOverlay=true \
  ./bin/herald --demo
```

The custom page must load xterm.js, `@xterm/addon-fit`, and `@xterm/addon-image`, with image addon options such as `iipSupport: true` and `sixelSupport: true`. ttyd also requires a specific websocket handshake: fetch `/token`, connect to `/ws` with the `tty` subprotocol, and send the first websocket frame as raw JSON:

```json
{"AuthToken":"","columns":120,"rows":40}
```

After that initial frame, send terminal input as `0` + input bytes and resize messages as `1` + `{"columns":120,"rows":40}`. If the browser page is blank and ttyd logs a websocket connection but no `started process`, the initial JSON handshake is probably missing. Once the custom client renders, open the demo email `Creative Commons image sampler for terminal previews`, press `z`, save a browser screenshot under `reports/`, and record the ttyd command, browser, addon status, and whether raster output displaced preview chrome.

**Trailing whitespace varies.** `capture-pane -p` strips trailing spaces per line but preserves blank lines up to the terminal height. For golden file comparison, decide whether to normalize this:

```bash
# Normalize: strip trailing whitespace and blank lines at end
tmux capture-pane -t test -p | sed 's/[[:space:]]*$//' | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}'
```

**Alt-screen apps.** Bubbletea apps using `tea.WithAltScreen()` enter the alternate screen buffer. `capture-pane -p` correctly captures this. But if the app exits and returns to the normal screen, a capture right after will show the normal (empty) screen, not the last alt-screen frame.

**Mouse events.** tmux 3.0+ supports sending mouse events, but the syntax is complex. For TUI testing, prefer keyboard-driven test scenarios. If you must test mouse interaction, use the PTY + vt approach instead.

**TERM variable.** tmux sets `TERM=tmux-256color` by default inside sessions. Some Bubbletea apps check `TERM` for feature detection. If your app behaves differently, override it:

```bash
tmux new-session -d -s test -x 80 -y 24 -e TERM=xterm-256color './my-tui-app'
```

**Debugging interactively.** The biggest advantage of tmux is that you can attach to a test session and see exactly what the TUI looks like. Add a sleep or breakpoint in your test, then `tmux attach -t <session>` from another terminal. Press `Ctrl+B, D` to detach without killing it.

## Common ANSI Escape Sequences for Raw PTY Input

When using PTY-based approaches and writing raw bytes:

```go
var keys = map[string][]byte{
    "up":        []byte("\x1b[A"),
    "down":      []byte("\x1b[B"),
    "right":     []byte("\x1b[C"),
    "left":      []byte("\x1b[D"),
    "home":      []byte("\x1b[H"),
    "end":       []byte("\x1b[F"),
    "pgup":      []byte("\x1b[5~"),
    "pgdown":    []byte("\x1b[6~"),
    "insert":    []byte("\x1b[2~"),
    "delete":    []byte("\x1b[3~"),
    "enter":     []byte("\r"),
    "tab":       []byte("\t"),
    "escape":    []byte("\x1b"),
    "backspace": []byte("\x7f"),
    "ctrl+c":    []byte("\x03"),
    "ctrl+d":    []byte("\x04"),
    "ctrl+z":    []byte("\x1a"),
    "ctrl+l":    []byte("\x0c"),
    "f1":        []byte("\x1bOP"),
    "f2":        []byte("\x1bOQ"),
    "f3":        []byte("\x1bOR"),
    "f4":        []byte("\x1bOS"),
    "f5":        []byte("\x1b[15~"),
    "f6":        []byte("\x1b[17~"),
    "f7":        []byte("\x1b[18~"),
    "f8":        []byte("\x1b[19~"),
    "f9":        []byte("\x1b[20~"),
    "f10":       []byte("\x1b[21~"),
    "f11":       []byte("\x1b[23~"),
    "f12":       []byte("\x1b[24~"),
}
```

## LLM Prompt Design for the Agent

When sending screenshots to the LLM, include structural context:

```
You are controlling a terminal UI application.

Screen dimensions: 80 columns x 24 rows.
Current screen content:
---
{screenshot}
---
Cursor position: row {y}, column {x}
Alternate screen: {yes/no}

Your task: {task_description}

Respond with a JSON action:
- {"type": "key", "key": "enter"} — press a key
- {"type": "key", "key": "down", "count": 3} — press a key multiple times
- {"type": "text", "text": "hello"} — type text
- {"type": "done", "result": "..."} — task complete

Available keys: up, down, left, right, enter, escape, tab, backspace,
space, home, end, pgup, pgdown, delete, ctrl+c, ctrl+d, f1-f12
```

## Important Implementation Notes

### Settle Time / Debounce

TUI apps render asynchronously. After sending input, wait before taking a screenshot. A Bubbletea app may emit multiple frames as different components update.

- **Minimum wait:** 100-150ms after each input
- **Smarter approach:** watch for output to stop changing. Read from the PTY/output stream in a loop, and once no new data arrives for N milliseconds, take the screenshot.

```go
func waitForSettle(ptmx *os.File, term *vt.SafeEmulator, timeout time.Duration) {
    deadline := time.After(timeout)
    buf := make([]byte, 4096)
    for {
        select {
        case <-deadline:
            return
        default:
            // Non-blocking read with short timeout
            ptmx.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
            n, err := ptmx.Read(buf)
            if n > 0 {
                term.Write(buf[:n])
                continue // got data, keep waiting
            }
            if err != nil {
                return // settled — no more data
            }
        }
    }
}
```

### Environment Variables

Always set these when spawning a TUI in a PTY:

```go
cmd.Env = append(os.Environ(),
    "TERM=xterm-256color",  // enables color and modern escape sequences
    "NO_COLOR=",            // clear this — we want colors for proper rendering
    "LANG=en_US.UTF-8",    // unicode support
)
```

### Alternate Screen Mode

Most Bubbletea apps use alternate screen (full-screen mode). The virtual terminal emulator handles this transparently. `e.String()` always returns the currently active screen.

You can detect alt-screen transitions via callbacks:

```go
e.SetCallbacks(vt.Callbacks{
    AltScreen: func(entered bool) {
        if entered {
            // App went full-screen
        } else {
            // App returned to normal screen
        }
    },
})
```

### Concurrency

- `vt.Emulator` is NOT thread-safe. Don't read and write from different goroutines.
- `vt.SafeEmulator` wraps it with a mutex. Use this when the PTY reader goroutine writes and the agent loop reads.
- For `teatest`, all operations go through the `TestModel` which handles synchronization internally.

### Window Resize

To simulate terminal resize (e.g., testing responsive layouts):

```go
// PTY approach
pty.Setsize(ptmx, &pty.Winsize{Rows: 40, Cols: 120})
term.Resize(120, 40)

// teatest approach
tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
```

## Go Module Dependencies

```
go get github.com/charmbracelet/x/vt@latest
go get github.com/charmbracelet/x/exp/teatest@latest
go get github.com/charmbracelet/bubbletea@latest
go get github.com/creack/pty@latest
```

Note: The `charmbracelet/x` packages are experimental (no tagged stable versions). Pin to a specific commit hash in production:

```
go get github.com/charmbracelet/x/vt@f2fb44a
```

## Quick Decision Matrix

| Scenario                            | Approach   | Packages            |
| ----------------------------------- | ---------- | ------------------- |
| Own the source, want speed          | In-process | `teatest`           |
| Black-box binary, Unix only         | PTY + vt   | `creack/pty` + `vt` |
| Black-box, cross-platform           | PTY + vt   | `xpty` + `vt`       |
| Integration tests with golden files | tmux       | (shell out)         |
| CI-friendly black-box tests         | tmux       | (shell out)         |
| Shell-scriptable test scenarios     | tmux       | bash + tmux         |
| Quick prototype, no Go deps         | tmux       | (shell out)         |
| AI agent driving a TUI              | PTY + vt   | `creack/pty` + `vt` |
| AI agent, simple setup              | tmux       | (shell out)         |

## Debugging Tips

1. **Log the raw PTY output** to a file to replay terminal sessions
2. **Use `e.Render()`** instead of `e.String()` to see ANSI codes in captured output
3. **Check `e.IsAltScreen()`** if screenshots appear empty — the app might not have entered alt-screen yet
4. **Increase settle time** if screenshots show partial renders
5. **Log cursor position** to understand where the TUI expects input
6. **Use callbacks** (`Bell`, `Title`, `CursorPosition`) to trace TUI state changes without polling
