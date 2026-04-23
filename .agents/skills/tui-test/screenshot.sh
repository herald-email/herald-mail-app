#!/usr/bin/env bash
# screenshot.sh — Capture a tmux pane as a rendered PNG screenshot.
#
# Usage:
#   ./screenshot.sh <session> <output.png> [window-size]
#
# Examples:
#   ./screenshot.sh test /tmp/tui.png              # Default 1400x800 browser window
#   ./screenshot.sh test /tmp/tui.png 1600x900     # Larger capture
#
# Prerequisites:
#   brew install aha    # ANSI-to-HTML converter
#   Google Chrome       # Headless screenshot renderer
#
# How it works:
#   1. tmux capture-pane with -e (ANSI escape codes preserved)
#   2. aha converts ANSI to HTML spans with inline color styles
#   3. Wrapped in a dark-background HTML page with monospace font
#   4. Chrome headless renders to PNG
#
# The resulting PNG shows exactly what a human would see in the terminal,
# including colors, bold text, and background highlights.

set -euo pipefail

SESSION="${1:?Usage: screenshot.sh <tmux-session> <output.png> [window-size]}"
OUTPUT="${2:?Usage: screenshot.sh <tmux-session> <output.png> [window-size]}"
WINSIZE="${3:-1400x800}"

mkdir -p "$(dirname "$OUTPUT")"

# Check dependencies
command -v aha >/dev/null 2>&1 || { echo "ERROR: aha not found. Install with: brew install aha" >&2; exit 1; }
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
[ -x "$CHROME" ] || { echo "ERROR: Google Chrome not found at $CHROME" >&2; exit 1; }

# Temporary files (cleaned up on exit)
ANSI_FILE=$(mktemp /tmp/tui_ansi.XXXXXX)
HTML_FILE=$(mktemp /tmp/tui_html.XXXXXX.html)
trap 'rm -f "$ANSI_FILE" "$HTML_FILE"' EXIT

# 1. Capture with ANSI codes
tmux capture-pane -t "$SESSION" -p -e > "$ANSI_FILE"

# 2. Convert ANSI → HTML body
AHA_BODY=$(cat "$ANSI_FILE" | aha --no-header)

# 3. Wrap in styled HTML page
cat > "$HTML_FILE" << HTMLEOF
<!DOCTYPE html>
<html>
<head><style>
body {
  background: #1a1a2e;
  color: #d4d4d4;
  font-family: 'Menlo', 'Monaco', 'Courier New', monospace;
  font-size: 15px;
  line-height: 1.35;
  padding: 16px 20px;
  margin: 0;
  white-space: pre;
  -webkit-font-smoothing: antialiased;
}
</style></head>
<body>
${AHA_BODY}
</body></html>
HTMLEOF

# 4. Headless Chrome screenshot
"$CHROME" \
  --headless --disable-gpu \
  "--screenshot=$OUTPUT" \
  "--window-size=${WINSIZE/x/,}" \
  "file://$HTML_FILE" 2>/dev/null

echo "$OUTPUT"
