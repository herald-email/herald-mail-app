#!/usr/bin/env bash
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$ROOT" || exit 1

STAMP="$(date +%F_%H%M%S)"
THEME="${HERALD_PRE_RELEASE_THEME:-jade-signal}"
IMAGE_PROTOCOL="${HERALD_PRE_RELEASE_IMAGE_PROTOCOL:-iterm2}"
PORT_BASE="${HERALD_PRE_RELEASE_PORT_BASE:-7682}"
EVIDENCE_DIR="${HERALD_PRE_RELEASE_EVIDENCE_DIR:-$ROOT/reports/pre-release-gate_$STAMP}"
REPORT="$EVIDENCE_DIR/TEST_REPORT_${STAMP}_pre-release-gate.md"
BIN="$ROOT/bin/herald"
SSH_BIN="$ROOT/bin/herald-ssh-server"
MCP_BIN="$ROOT/bin/herald-mcp-server"
SCREENSHOT="$ROOT/.agents/skills/tui-test/screenshot.sh"

mkdir -p "$EVIDENCE_DIR"

FAILURES=0
declare -a STEP_ROWS=()

record_step() {
  local status="$1"
  local name="$2"
  local log="$3"
  STEP_ROWS+=("- [$status] $name: \`$log\`")
}

run_cmd_step() {
  local name="$1"
  local slug="$2"
  local cmd="$3"
  local log="$EVIDENCE_DIR/$slug.log"

  printf '==> %s\n' "$name"
  if bash -lc "$cmd" >"$log" 2>&1; then
    record_step "PASS" "$name" "$log"
  else
    local status=$?
    record_step "FAIL" "$name (exit $status)" "$log"
    FAILURES=$((FAILURES + 1))
  fi
}

run_func_step() {
  local name="$1"
  local slug="$2"
  local func="$3"
  local log="$EVIDENCE_DIR/$slug.log"

  printf '==> %s\n' "$name"
  if "$func" >"$log" 2>&1; then
    record_step "PASS" "$name" "$log"
  else
    local status=$?
    record_step "FAIL" "$name (exit $status)" "$log"
    FAILURES=$((FAILURES + 1))
  fi
}

preflight_dependencies() {
  local missing=0
  for cmd in go make tmux ttyd ssh nc node python3; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      echo "missing required command: $cmd"
      missing=1
    fi
  done

  if ! command -v aha >/dev/null 2>&1; then
    echo "missing required command: aha (install with: brew install aha)"
    missing=1
  fi

  if ! node -e "require.resolve('playwright')" >/dev/null 2>&1; then
    local codex_node_modules="$HOME/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules"
    if [ -d "$codex_node_modules" ]; then
      export NODE_PATH="${NODE_PATH:+$NODE_PATH:}$codex_node_modules"
    fi
  fi
  if ! node -e "require.resolve('playwright')" >/dev/null 2>&1; then
    echo "missing required Node module: playwright"
    missing=1
  fi

  if ! python3 - <<'PY' >/dev/null 2>&1
from PIL import Image  # noqa: F401
PY
  then
    echo "missing required Python module: Pillow"
    missing=1
  fi

  local screenshot_chrome="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
  if [ ! -x "$screenshot_chrome" ]; then
    echo "missing Google Chrome executable required by .agents/skills/tui-test/screenshot.sh: $screenshot_chrome"
    missing=1
  else
    echo "tmux screenshot browser: $screenshot_chrome"
  fi

  local chrome=""
  for candidate in \
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" \
    "/Applications/Chromium.app/Contents/MacOS/Chromium"; do
    if [ -x "$candidate" ]; then
      chrome="$candidate"
      break
    fi
  done
  if [ -z "$chrome" ]; then
    echo "missing Chrome/Chromium executable for screenshots"
    missing=1
  else
    echo "ttyd proof browser: $chrome"
  fi

  return "$missing"
}

wait_for_port() {
  local host="$1"
  local port="$2"
  local tries="${3:-40}"
  for _ in $(seq 1 "$tries"); do
    if nc -z "$host" "$port" 2>/dev/null; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

capture_themed_tui() {
  local out="$EVIDENCE_DIR/tmux-themed-preview"
  mkdir -p "$out"

  for size in 220x50 80x24 50x15; do
    local cols="${size%x*}"
    local rows="${size#*x}"
    local session="herald-prerelease-${size//x/-}"
    tmux kill-session -t "$session" 2>/dev/null || true
    tmux new-session -d -s "$session" -x "$cols" -y "$rows" "$BIN --demo -theme $THEME"
    sleep 2

    if [ "$size" != "50x15" ]; then
      tmux send-keys -t "$session" Escape
      sleep 0.4
      tmux send-keys -t "$session" "/"
      sleep 0.2
      tmux send-keys -t "$session" "Step 5"
      sleep 1.1
      tmux send-keys -t "$session" Enter
      sleep 0.8
      tmux send-keys -t "$session" Enter
      sleep 1
      tmux send-keys -t "$session" "z"
      sleep 0.8
      for _ in $(seq 1 10); do
        tmux send-keys -t "$session" Down
      done
      sleep 1
    fi

    tmux capture-pane -t "$session" -p >"$out/$size.txt"
    tmux capture-pane -t "$session" -p -e >"$out/$size.ansi.txt"
    "$SCREENSHOT" "$session" "$out/$size.png" 1600x900 >/dev/null
    tmux kill-session -t "$session" 2>/dev/null || true
    echo "captured $size -> $out/$size.png"
  done
}

run_ttyd_image_probe_default() {
  PORT="$PORT_BASE" \
    IMAGE_PROTOCOL="$IMAGE_PROTOCOL" \
    HERALD_BIN="$BIN" \
    EVIDENCE_DIR="$EVIDENCE_DIR/ttyd-image-default" \
    tools/ttyd-image-harness/probe.sh
}

run_ttyd_image_probe_themed() {
  PORT="$((PORT_BASE + 2))" \
    IMAGE_PROTOCOL="$IMAGE_PROTOCOL" \
    HERALD_THEME="$THEME" \
    HERALD_BIN="$BIN" \
    EVIDENCE_DIR="$EVIDENCE_DIR/ttyd-image-themed" \
    tools/ttyd-image-harness/probe.sh
}

run_ssh_smoke() {
  local port="$((PORT_BASE + 20))"
  local host_key="$EVIDENCE_DIR/ssh_host_ed25519"
  local server_log="$EVIDENCE_DIR/ssh-server.log"
  local client_log="$EVIDENCE_DIR/ssh-client.log"

  "$SSH_BIN" -demo -addr "127.0.0.1:$port" -host-key "$host_key" >"$server_log" 2>&1 &
  local server_pid=$!
  cleanup_ssh() {
    if [ -n "${server_pid:-}" ]; then
      kill "$server_pid" 2>/dev/null || true
      wait "$server_pid" 2>/dev/null || true
    fi
  }

  wait_for_port 127.0.0.1 "$port" 60 || {
    echo "SSH server did not listen on 127.0.0.1:$port"
    cat "$server_log" || true
    cleanup_ssh
    return 1
  }

  ssh \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -p "$port" \
    127.0.0.1 >"$client_log" 2>&1 &
  local client_pid=$!
  sleep 3
  if kill -0 "$client_pid" 2>/dev/null; then
    kill "$client_pid" 2>/dev/null || true
    wait "$client_pid" 2>/dev/null || true
  else
    wait "$client_pid" || true
  fi

  if grep -Eiq "connection refused|permission denied|host key verification failed" "$client_log"; then
    cat "$client_log"
    cleanup_ssh
    return 1
  fi
  cleanup_ssh
  echo "SSH smoke connected to 127.0.0.1:$port"
}

run_mcp_smoke() {
  local out="$EVIDENCE_DIR/mcp-tools-list.json"
  printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' | "$MCP_BIN" --demo >"$out"
  grep -q "list_recent_emails" "$out"
  echo "MCP tools/list wrote $out"
}

write_report() {
  {
    echo "# Pre-Release Gate Report - $STAMP"
    echo
    echo "## Configuration"
    echo
    echo "- Repo: $ROOT"
    echo "- Theme: $THEME"
    echo "- Image protocol: $IMAGE_PROTOCOL"
    echo "- Port base: $PORT_BASE"
    echo "- Evidence: $EVIDENCE_DIR"
    echo
    echo "## Results"
    echo
    for row in "${STEP_ROWS[@]}"; do
      echo "$row"
    done
    echo
    echo "## Key Evidence"
    echo
    echo "- Tmux themed captures: \`$EVIDENCE_DIR/tmux-themed-preview\`"
    echo "- Default image probe: \`$EVIDENCE_DIR/ttyd-image-default/ttyd-image-preview.png\`"
    echo "- Themed image probe: \`$EVIDENCE_DIR/ttyd-image-themed/ttyd-image-preview.png\`"
    echo "- MCP tools output: \`$EVIDENCE_DIR/mcp-tools-list.json\`"
    echo
    if [ "$FAILURES" -eq 0 ]; then
      echo "Result: PASS"
    else
      echo "Result: FAIL ($FAILURES failing step(s))"
    fi
  } >"$REPORT"
  echo "pre-release report: $REPORT"
}

run_func_step "Dependency preflight" "00-dependency-preflight" preflight_dependencies
run_cmd_step "make test" "01-make-test" "make test"
run_cmd_step "make vet" "02-make-vet" "make vet"
run_cmd_step "Build release smoke binaries" "03-build-binaries" "make build build-ssh build-mcp"
run_func_step "Themed TUI tmux captures" "04-themed-tui-captures" capture_themed_tui
run_func_step "Default custom ttyd image proof" "05-ttyd-image-default" run_ttyd_image_probe_default
run_func_step "Themed custom ttyd image proof" "06-ttyd-image-themed" run_ttyd_image_probe_themed
run_func_step "SSH demo smoke" "07-ssh-smoke" run_ssh_smoke
run_func_step "MCP demo tools/list smoke" "08-mcp-smoke" run_mcp_smoke

write_report

if [ "$FAILURES" -ne 0 ]; then
  exit 1
fi
