#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${HERALD_BIN:-$ROOT/bin/herald}"
VIEW="${HERALD_THEME_SCREENSHOT_VIEW:-timeline}"
WIDTH="${HERALD_THEME_SCREENSHOT_COLS:-140}"
HEIGHT="${HERALD_THEME_SCREENSHOT_ROWS:-42}"
CANVAS="${HERALD_THEME_SCREENSHOT_CANVAS:-1320x900}"
DELAY="${HERALD_THEME_SCREENSHOT_DELAY:-5}"
CHROME="${HERALD_THEME_SCREENSHOT_CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"

case "$VIEW" in
  timeline)
    DEFAULT_OUT_DIR="$ROOT/docs/public/screenshots/themes"
    ;;
  preview)
    DEFAULT_OUT_DIR="$ROOT/docs/public/screenshots/themes/preview"
    ;;
  *)
    echo "error: HERALD_THEME_SCREENSHOT_VIEW must be 'timeline' or 'preview' (got '$VIEW')" >&2
    exit 1
    ;;
esac

OUT_DIR="${HERALD_THEME_SCREENSHOT_DIR:-$DEFAULT_OUT_DIR}"

THEMES=(
  red-black
  crimson
  ember
  ruby-noir
  garnet-console
  jade-signal
  viridian-glass
  forest-crt
  pine-mail
  amber-furnace
  copper-ash
  magma-core
  peacock-ink
  ultramarine-desk
  amethyst-night
  graphite-rose
  olive-circuit
  arctic-signal
  sepia-debug
  ayu-courier
  cobalt-dispatch
  kanagawa-post
  rose-pine-desk
  solar-paper
  tokyo-dusk
  iceberg-queue
  panda-packet
  sonokai-signal
  zenbones-light
  tomorrow-desk
)

if [[ $# -gt 0 ]]; then
  THEMES=("$@")
fi

if [[ "${HERALD_THEME_SCREENSHOT_SKIP_BUILD:-0}" != "1" ]]; then
  (cd "$ROOT" && make build)
fi

if [[ ! -x "$BIN" ]]; then
  echo "error: Herald binary not found or not executable: $BIN" >&2
  exit 1
fi

if ! command -v aha >/dev/null 2>&1; then
  echo "error: aha is required to render ANSI screenshots. Install with: brew install aha" >&2
  exit 1
fi

if [[ ! -x "$CHROME" ]]; then
  echo "error: Google Chrome not found at: $CHROME" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

capture_tmux_png() {
  local session="$1"
  local output="$2"
  local canvas="$3"
  local tmpdir ansi_file html_file

  tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/herald-theme-shot.XXXXXX")"
  ansi_file="$tmpdir/capture.ansi"
  html_file="$tmpdir/capture.html"
  trap 'rm -rf "$tmpdir"' RETURN

  tmux capture-pane -t "$session" -p -e > "$ansi_file"
  {
    cat <<'HTMLEOF'
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
HTMLEOF
    aha --no-header < "$ansi_file"
    cat <<'HTMLEOF'
</body></html>
HTMLEOF
  } > "$html_file"

  "$CHROME" \
    --headless --disable-gpu \
    "--screenshot=$output" \
    "--window-size=${canvas/x/,}" \
    "file://$html_file" >/dev/null 2>&1
}

pane_contains() {
  local session="$1"
  local text="$2"
  tmux capture-pane -t "$session" -p | grep -Fq "$text"
}

wait_for_pane_text() {
  local session="$1"
  local text="$2"
  local timeout="$3"
  local end=$((SECONDS + timeout))
  while (( SECONDS <= end )); do
    if pane_contains "$session" "$text"; then
      return 0
    fi
    sleep 0.2
  done
  return 1
}

for theme in "${THEMES[@]}"; do
  session="herald-theme-${theme//[^a-zA-Z0-9]/-}"
  tmux kill-session -t "$session" 2>/dev/null || true
  launch_cmd="$(printf "%q --demo -theme %q" "$BIN" "$theme")"
  tmux new-session -d -s "$session" -x "$WIDTH" -y "$HEIGHT" "$launch_cmd"
  wait_for_pane_text "$session" "Welcome to Herald Demo" "$DELAY" || sleep 0.5
  tmux send-keys -t "$session" " "
  sleep 0.8
  if pane_contains "$session" "Welcome to Herald Demo"; then
    tmux send-keys -t "$session" " "
    sleep 0.8
  fi
  tmux send-keys -t "$session" "1"
  sleep 0.6
  if [[ "$VIEW" == "preview" ]]; then
    tmux send-keys -t "$session" "l"
    sleep 1
  fi

  tmux capture-pane -t "$session" -p -e > "$OUT_DIR/$theme.ansi.txt"
  capture_tmux_png "$session" "$OUT_DIR/$theme.png" "$CANVAS"
  tmux kill-session -t "$session" 2>/dev/null || true
  echo "wrote $OUT_DIR/$theme.png"
done
