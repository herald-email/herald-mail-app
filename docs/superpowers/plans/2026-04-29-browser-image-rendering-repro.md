# Browser Image Rendering Repro Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce clear browser-first repro instructions and screenshot proof for Herald's demo-mode full-screen inline image rendering behavior after pressing `z`.

**Architecture:** Build Herald in demo mode, run it through ttyd, and inspect the browser-rendered terminal. Prefer a custom xterm.js frontend with image addon support; fall back to stock ttyd or native iTerm2 only after recording why the browser image path is not enough.

**Tech Stack:** Go, Herald `--demo`, ttyd 1.7.7, xterm.js, `@xterm/addon-image`, browser screenshots, `reports/` Markdown artifacts.

---

## File Structure

- Create: `docs/superpowers/plans/2026-04-29-browser-image-rendering-repro.md`
  - This implementation checklist.
- Create: `reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md`
  - Final repro report with commands, environment, observations, screenshot paths, and fallback notes.
- Create: `reports/browser-image-rendering-repro-*.png`
  - Screenshot proof files captured from the browser or native fallback.
- Optional Create: `reports/ttyd-image-harness/index.html`
  - Disposable local custom ttyd frontend if stock ttyd does not expose iTerm inline image rendering.
- Optional Create: `reports/ttyd-image-harness/package.json`
  - Disposable local package manifest if the custom frontend needs bundled xterm.js dependencies.

### Task 1: Baseline Build And Environment Capture

**Files:**
- Create: `reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md`

- [ ] **Step 1: Build Herald**

Run:

```bash
make build
```

Expected: command exits `0` and creates `bin/herald`.

- [ ] **Step 2: Capture environment facts**

Run:

```bash
{
  echo "# Browser Image Rendering Repro Report"
  echo
  echo "Date: 2026-04-29"
  echo "Repository: /Users/zoomacode/Developer/mail-processor"
  echo "Commit: $(git rev-parse --short HEAD)"
  echo "ttyd: $(ttyd --version)"
  echo "Go: $(go version)"
  echo "Node: $(node --version)"
  echo "npm: $(npm --version)"
  echo
  echo "## Goal"
  echo
  echo "Reproduce Herald demo-mode full-screen inline image rendering after pressing z on the Creative Commons image sampler email."
  echo
} > reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md
```

Expected: report file exists with non-empty environment values.

- [ ] **Step 3: Commit checkpoint if plan-only changes are being preserved**

Run only if this plan file should be committed separately:

```bash
git add docs/superpowers/plans/2026-04-29-browser-image-rendering-repro.md
git commit -m "docs: plan browser image rendering repro"
```

Expected: commit succeeds after repository hooks pass. If this is being treated as an execution artifact, leave it uncommitted and record that in the final response.

### Task 2: Browser Repro With Stock ttyd

**Files:**
- Modify: `reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md`
- Create: `reports/browser-image-rendering-repro-stock-ttyd.png`

- [ ] **Step 1: Start stock ttyd**

Run:

```bash
ttyd -W -p 7681 -t rendererType=canvas -t disableLeaveAlert=true -t disableResizeOverlay=true ./bin/herald --demo
```

Expected: ttyd logs a server URL such as `http://127.0.0.1:7681`.

- [ ] **Step 2: Open the browser URL**

Use the in-app browser or a normal browser to open:

```text
http://127.0.0.1:7681
```

Expected: Herald renders in demo mode with the Timeline tab visible.

- [ ] **Step 3: Navigate to the sampler email**

Send keys:

```text
/Creative Commons image sampler
Enter
Enter
```

If search focus or result selection differs, use `j` and `k` until the highlighted subject is:

```text
Creative Commons image sampler for terminal previews
```

Expected: split preview shows the sampler email and the image hint.

- [ ] **Step 4: Enter full-screen preview**

Send key:

```text
z
```

Expected: Herald enters full-screen preview mode, with status text similar to `z/esc: exit full-screen`.

- [ ] **Step 5: Capture screenshot**

Save the visible browser window as:

```text
reports/browser-image-rendering-repro-stock-ttyd.png
```

Expected: screenshot shows the post-`z` state. If stock ttyd does not render inline images, that is still useful baseline evidence; continue to Task 3.

- [ ] **Step 6: Append stock ttyd notes**

Append:

````markdown
## Stock ttyd Attempt

Command:

```bash
ttyd -W -p 7681 -t rendererType=canvas -t disableLeaveAlert=true -t disableResizeOverlay=true ./bin/herald --demo
```

Browser URL: http://127.0.0.1:7681

Observed after pressing `z`:

- Full-screen mode: yes/no
- Inline raster images visible: yes/no
- Screenshot: `reports/browser-image-rendering-repro-stock-ttyd.png`
````

Expected: report has a baseline browser attempt section.

### Task 3: Browser Repro With Custom xterm.js Image Frontend

**Files:**
- Create: `reports/ttyd-image-harness/index.html`
- Modify: `reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md`
- Create: `reports/browser-image-rendering-repro-custom-xterm.png`

- [ ] **Step 1: Create custom ttyd index**

Create `reports/ttyd-image-harness/index.html` with a minimal xterm.js client. It must load xterm.js, fit support, and image addon support, then connect to ttyd's websocket endpoint.

Use this initial implementation:

```html
<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <title>Herald ttyd Image Repro</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@xterm/xterm/css/xterm.css" />
    <style>
      html, body, #terminal {
        width: 100%;
        height: 100%;
        margin: 0;
        background: #000;
      }
    </style>
  </head>
  <body>
    <div id="terminal"></div>
    <script type="module">
      import { Terminal } from "https://cdn.jsdelivr.net/npm/@xterm/xterm/+esm";
      import { FitAddon } from "https://cdn.jsdelivr.net/npm/@xterm/addon-fit/+esm";
      import { ImageAddon } from "https://cdn.jsdelivr.net/npm/@xterm/addon-image/+esm";

      const terminal = new Terminal({
        cursorBlink: false,
        convertEol: false,
        fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        fontSize: 14,
        rendererType: "canvas",
        scrollback: 5000,
        theme: { background: "#000000", foreground: "#d0d0d0" },
        windowOptions: {
          getWinSizePixels: true,
          getCellSizePixels: true,
          getWinSizeChars: true
        }
      });

      const fitAddon = new FitAddon();
      const imageAddon = new ImageAddon({
        enableSizeReports: true,
        iipSupport: true,
        sixelSupport: true,
        sixelScrolling: true,
        showPlaceholder: true
      });

      terminal.loadAddon(fitAddon);
      terminal.loadAddon(imageAddon);
      terminal.open(document.getElementById("terminal"));
      fitAddon.fit();

      const protocol = location.protocol === "https:" ? "wss:" : "ws:";
      const socket = new WebSocket(`${protocol}//${location.host}/ws`, ["tty"]);
      socket.binaryType = "arraybuffer";

      socket.addEventListener("open", () => {
        const resize = () => {
          fitAddon.fit();
          const msg = JSON.stringify({ columns: terminal.cols, rows: terminal.rows });
          socket.send("1" + msg);
        };
        terminal.onData((data) => socket.send("0" + data));
        terminal.onResize(({ cols, rows }) => socket.send("1" + JSON.stringify({ columns: cols, rows })));
        window.addEventListener("resize", resize);
        resize();
      });

      socket.addEventListener("message", (event) => {
        if (typeof event.data === "string") {
          const command = event.data.slice(0, 1);
          const payload = event.data.slice(1);
          if (command === "0") terminal.write(payload);
          return;
        }
        const bytes = new Uint8Array(event.data);
        if (bytes.length > 1 && bytes[0] === 48) {
          terminal.write(bytes.slice(1));
        }
      });

      socket.addEventListener("close", () => {
        terminal.writeln("");
        terminal.writeln("[ttyd websocket closed]");
      });
    </script>
  </body>
</html>
```

Expected: file exists and can be served by `ttyd -I`.

- [ ] **Step 2: Start ttyd with the custom index and iTerm-compatible environment**

Run:

```bash
TERM_PROGRAM=iTerm.app ttyd -W -p 7682 -I reports/ttyd-image-harness/index.html -t rendererType=canvas -t disableLeaveAlert=true -t disableResizeOverlay=true ./bin/herald --demo
```

Expected: ttyd serves `http://127.0.0.1:7682` and the child Herald process sees `TERM_PROGRAM=iTerm.app`, so Herald may emit OSC 1337 inline image sequences.

- [ ] **Step 3: Open custom browser terminal**

Open:

```text
http://127.0.0.1:7682
```

Expected: Herald renders and browser console does not show module load errors for `@xterm/addon-image`.

- [ ] **Step 4: Navigate and enter full-screen**

Send keys:

```text
/Creative Commons image sampler
Enter
Enter
z
```

Expected: full-screen preview opens and the custom frontend either renders inline images or reveals the specific failure mode.

- [ ] **Step 5: Capture custom frontend screenshot**

Save:

```text
reports/browser-image-rendering-repro-custom-xterm.png
```

Expected: screenshot shows the post-`z` browser terminal state with image rendering behavior visible.

- [ ] **Step 6: Append custom frontend notes**

Append:

````markdown
## Custom xterm.js Image Frontend Attempt

Command:

```bash
TERM_PROGRAM=iTerm.app ttyd -W -p 7682 -I reports/ttyd-image-harness/index.html -t rendererType=canvas -t disableLeaveAlert=true -t disableResizeOverlay=true ./bin/herald --demo
```

Browser URL: http://127.0.0.1:7682

Observed after pressing `z`:

- Full-screen mode: yes/no
- Inline raster images visible: yes/no
- Browser console addon errors: yes/no
- Screenshot: `reports/browser-image-rendering-repro-custom-xterm.png`
````

Expected: report documents whether the browser-first image path succeeded.

### Task 4: Native iTerm2 Fallback If Browser Path Fails

**Files:**
- Modify: `reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md`
- Create: `reports/browser-image-rendering-repro-iterm-fallback.png`

- [ ] **Step 1: Launch native fallback only if needed**

Run only if Task 3 cannot render or capture the needed browser evidence:

```bash
make build
TERM_PROGRAM=iTerm.app ./bin/herald --demo
```

Expected: Herald runs in a native terminal that supports OSC 1337 inline images.

- [ ] **Step 2: Navigate to the sampler and press `z`**

Use the same key path:

```text
/Creative Commons image sampler
Enter
Enter
z
```

Expected: full-screen preview opens and native inline image behavior is visible.

- [ ] **Step 3: Capture fallback screenshot**

Save:

```text
reports/browser-image-rendering-repro-iterm-fallback.png
```

Expected: screenshot proves the post-`z` rendering state in native fallback.

- [ ] **Step 4: Append fallback notes**

Append:

````markdown
## Native iTerm2 Fallback

Used fallback: yes/no

Reason browser path was insufficient:

- Stock ttyd:
- Custom xterm.js frontend:

Screenshot: `reports/browser-image-rendering-repro-iterm-fallback.png`
````

Expected: fallback notes do not replace the browser notes; they explain why fallback was needed.

### Task 5: Final Repro Instructions And Verification

**Files:**
- Modify: `reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md`

- [ ] **Step 1: Add future-debugging instructions**

Append a concise section:

```markdown
## Future Debugging Guidance

1. Build with `make build`.
2. Start the browser harness with the exact ttyd command recorded in the browser attempt section.
3. Open the recorded local URL.
4. Search for `Creative Commons image sampler`.
5. Open the email preview and press `z`.
6. Capture the browser screenshot before changing size or scrolling.
7. If no images appear, verify `TERM_PROGRAM=iTerm.app` reached Herald and that the browser loaded `@xterm/addon-image`.
8. Use tmux captures only for text layout; use browser or iTerm screenshots for raster image placement.
```

Expected: report tells future Codex sessions how to repeat the investigation.

- [ ] **Step 2: Verify artifacts exist**

Run:

```bash
ls -lh reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md reports/browser-image-rendering-repro-*.png
```

Expected: report exists and at least one screenshot exists.

- [ ] **Step 3: Sanity-check report**

Run:

```bash
rg -n "Stock ttyd Attempt|Custom xterm.js Image Frontend Attempt|Future Debugging Guidance|Screenshot:" reports/TEST_REPORT_2026-04-29_browser-image-rendering-repro.md
```

Expected: all major report sections are present.
