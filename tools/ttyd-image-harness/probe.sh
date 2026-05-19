#!/usr/bin/env bash
# Run Herald's deterministic inline-image preview in ttyd and capture a browser
# screenshot that proves xterm.js painted terminal raster images.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

usage() {
  cat <<'EOF'
Usage: tools/ttyd-image-harness/probe.sh

Environment:
  PORT             ttyd port (default: 7682)
  HOST             ttyd bind host (default: 127.0.0.1)
  TTYD_MODE        custom or stock (default: custom)
  RENDERER_TYPE    ttyd renderer for stock mode (default: canvas)
  IMAGE_PROTOCOL   Herald image protocol (default: iterm2)
  HERALD_THEME     Optional Herald app theme, e.g. jade-signal
  EVIDENCE_DIR     output directory under reports/
  HERALD_BIN       Herald binary path (default: ./bin/herald)
  CHROME_BIN       Chrome/Chromium executable path
  NODE_BIN         Node executable (default: node)
  PYTHON_BIN       Python executable with Pillow (default: python3)
EOF
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
  "")
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

PORT="${PORT:-7682}"
HOST="${HOST:-127.0.0.1}"
TTYD_MODE="${TTYD_MODE:-custom}"
RENDERER_TYPE="${RENDERER_TYPE:-canvas}"
IMAGE_PROTOCOL="${IMAGE_PROTOCOL:-iterm2}"
HERALD_THEME="${HERALD_THEME:-}"
HERALD_BIN="${HERALD_BIN:-./bin/herald}"
TTYD_BIN="${TTYD_BIN:-ttyd}"
NODE_BIN="${NODE_BIN:-node}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
EVIDENCE_DIR="${EVIDENCE_DIR:-reports/ttyd-image-preview_$(date +%F_%H%M%S)}"
SCREENSHOT_PATH="$EVIDENCE_DIR/ttyd-image-preview.png"
METRICS_PATH="$EVIDENCE_DIR/ttyd-image-preview-metrics.json"
TTYD_LOG="$EVIDENCE_DIR/ttyd.log"

mkdir -p "$EVIDENCE_DIR"

if [ ! -x "$HERALD_BIN" ]; then
  go build -o ./bin/herald ./cmd/herald
  HERALD_BIN="./bin/herald"
fi

command -v "$TTYD_BIN" >/dev/null 2>&1 || {
  echo "ttyd is required. Install with: brew install ttyd" >&2
  exit 2
}

if ! "$NODE_BIN" -e "require.resolve('playwright')" >/dev/null 2>&1; then
  CODEX_NODE_MODULES="$HOME/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules"
  if [ -d "$CODEX_NODE_MODULES" ]; then
    export NODE_PATH="${NODE_PATH:+$NODE_PATH:}$CODEX_NODE_MODULES"
  fi
fi

"$NODE_BIN" -e "require.resolve('playwright')" >/dev/null 2>&1 || {
  echo "Playwright for Node is required. Set NODE_PATH or install playwright." >&2
  exit 2
}

if [ -z "${CHROME_BIN:-}" ]; then
  for candidate in \
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" \
    "/Applications/Chromium.app/Contents/MacOS/Chromium"; do
    if [ -x "$candidate" ]; then
      CHROME_BIN="$candidate"
      break
    fi
  done
fi

if [ -z "${CHROME_BIN:-}" ] || [ ! -x "$CHROME_BIN" ]; then
  echo "Chrome/Chromium is required. Set CHROME_BIN to the browser executable." >&2
  exit 2
fi

"$PYTHON_BIN" - <<'PY' >/dev/null
from PIL import Image  # noqa: F401
PY

case "$TTYD_MODE" in
  stock)
    ttyd_args=(
      -i "$HOST"
      -p "$PORT"
      -W
      -t enableSixel=true
      -t rendererType="$RENDERER_TYPE"
      -t disableLeaveAlert=true
      -t disableResizeOverlay=true
    )
    ;;
  custom)
    ttyd_args=(
      -I tools/ttyd-image-harness/index.html
      -i "$HOST"
      -p "$PORT"
      -W
      -t disableLeaveAlert=true
      -t disableResizeOverlay=true
    )
    ;;
  *)
    echo "TTYD_MODE must be 'stock' or 'custom'." >&2
    exit 2
    ;;
esac

herald_args=(-debug -demo -image-protocol="$IMAGE_PROTOCOL")
if [ -n "$HERALD_THEME" ]; then
  herald_args+=(-theme "$HERALD_THEME")
fi

"$TTYD_BIN" \
  "${ttyd_args[@]}" \
  "$HERALD_BIN" "${herald_args[@]}" \
  >"$TTYD_LOG" 2>&1 &
TTYD_PID=$!

cleanup() {
  kill "$TTYD_PID" 2>/dev/null || true
  wait "$TTYD_PID" 2>/dev/null || true
}
trap cleanup EXIT

for _ in $(seq 1 30); do
  if nc -z "$HOST" "$PORT" 2>/dev/null; then
    break
  fi
  sleep 0.2
done

if ! nc -z "$HOST" "$PORT" 2>/dev/null; then
  echo "ttyd did not start on $HOST:$PORT" >&2
  cat "$TTYD_LOG" >&2
  exit 1
fi

URL="http://$HOST:$PORT" \
SCREENSHOT_PATH="$SCREENSHOT_PATH" \
CHROME_BIN="$CHROME_BIN" \
"$NODE_BIN" <<'NODE'
const { chromium } = require("playwright");

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROME_BIN,
    headless: true,
  });
  const page = await browser.newPage({
    viewport: { width: 1300, height: 1000 },
    deviceScaleFactor: 1,
  });

  const logs = [];
  page.on("console", (msg) => logs.push(`${msg.type()}: ${msg.text()}`));
  page.on("pageerror", (err) => logs.push(`pageerror: ${err.message}`));

  await page.goto(process.env.URL, { waitUntil: "domcontentloaded" });
  await sleep(2500);

  await page.keyboard.press("Escape");
  await sleep(400);
  await page.keyboard.press("/");
  await sleep(250);
  await page.keyboard.type("Step 5");
  await sleep(1200);
  await page.keyboard.press("Enter");
  await sleep(900);
  await page.keyboard.press("Enter");
  await sleep(1200);
  await page.keyboard.press("z");
  await sleep(1000);
  for (let i = 0; i < 16; i++) await page.keyboard.press("ArrowDown");
  await sleep(6000);

  await page.screenshot({ path: process.env.SCREENSHOT_PATH, fullPage: true });
  const bodyText = await page.evaluate(() => document.body.innerText.slice(0, 1600));
  console.log(JSON.stringify({ screenshot: process.env.SCREENSHOT_PATH, bodyText, logs }, null, 2));
  await browser.close();
})();
NODE

SCREENSHOT_PATH="$SCREENSHOT_PATH" METRICS_PATH="$METRICS_PATH" TTYD_MODE="$TTYD_MODE" HERALD_THEME="$HERALD_THEME" "$PYTHON_BIN" <<'PY'
from __future__ import annotations

import json
import os
import sys
from pathlib import Path

from PIL import Image

screenshot = Path(os.environ["SCREENSHOT_PATH"])
metrics_path = Path(os.environ["METRICS_PATH"])
ttyd_mode = os.environ["TTYD_MODE"]
herald_theme = os.environ.get("HERALD_THEME", "")
image = Image.open(screenshot).convert("RGB")
width, height = image.size
pixels = image.load()

mask = set()
for y in range(height):
    for x in range(width):
        r, g, b = pixels[x, y]
        mx = max(r, g, b)
        mn = min(r, g, b)
        if (mx - mn > 50 and mx > 90) or (mx > 180 and mn < 130):
            mask.add((x, y))

seen = set()
components = []
for point in list(mask):
    if point in seen:
        continue
    stack = [point]
    seen.add(point)
    xs = []
    ys = []
    for x, y in stack:
        xs.append(x)
        ys.append(y)
        for nx, ny in ((x + 1, y), (x - 1, y), (x, y + 1), (x, y - 1)):
            if (nx, ny) in mask and (nx, ny) not in seen:
                seen.add((nx, ny))
                stack.append((nx, ny))
    area = len(xs)
    x0, x1 = min(xs), max(xs)
    y0, y1 = min(ys), max(ys)
    comp_width = x1 - x0 + 1
    comp_height = y1 - y0 + 1
    if area > 500 and comp_width > 20 and comp_height > 20:
        components.append(
            {
                "area": area,
                "x0": x0,
                "y0": y0,
                "x1": x1,
                "y1": y1,
                "width": comp_width,
                "height": comp_height,
            }
        )

large_images = [
    comp
    for comp in components
    if comp["area"] >= 10000 and comp["width"] >= 100 and comp["height"] >= 80
]
chart_cells = [
    comp
    for comp in components
    if 1200 <= comp["area"] <= 5000
    and 25 <= comp["width"] <= 90
    and 25 <= comp["height"] <= 90
    and comp["y0"] < int(height * 0.55)
]

def detect_solid_color_blocks():
    bg = pixels[max(0, width - 20), height // 2]
    win = 20
    step = 4
    sample = 3
    candidates = []
    x_limit = min(width, 620)
    y_limit = min(int(height * 0.45), height - win)
    for y in range(120, y_limit, step):
        for x in range(0, x_limit - win, step):
            vals = []
            off_background = 0
            for yy in range(y, y + win, sample):
                for xx in range(x, x + win, sample):
                    rgb = pixels[xx, yy]
                    vals.append(rgb)
                    if sum(abs(rgb[i] - bg[i]) for i in range(3)) > 55:
                        off_background += 1
            sample_count = len(vals)
            if sample_count == 0 or off_background / sample_count < 0.86:
                continue
            avg = [sum(v[i] for v in vals) / sample_count for i in range(3)]
            variance = sum(
                sum((v[i] - avg[i]) ** 2 for i in range(3)) for v in vals
            ) / sample_count
            if variance < 900 and sum(abs(avg[i] - bg[i]) for i in range(3)) > 70:
                candidates.append((x + win // 2, y + win // 2))

    clusters = []
    for cx, cy in candidates:
        for cluster in clusters:
            if abs(cluster["cx"] - cx) < 22 and abs(cluster["cy"] - cy) < 22:
                cluster["points"].append((cx, cy))
                cluster["cx"] = sum(px for px, _ in cluster["points"]) / len(cluster["points"])
                cluster["cy"] = sum(py for _, py in cluster["points"]) / len(cluster["points"])
                break
        else:
            clusters.append({"cx": cx, "cy": cy, "points": [(cx, cy)]})

    solid = [
        {"x": round(cluster["cx"]), "y": round(cluster["cy"]), "hits": len(cluster["points"])}
        for cluster in clusters
        if len(cluster["points"]) >= 10
    ]
    rows = {}
    for block in solid:
        if block["x"] > 420 or block["y"] < 140 or block["y"] > 330:
            continue
        row_key = round(block["y"] / 32)
        rows.setdefault(row_key, []).append(block)
    if not rows:
        return solid, []
    chart_row = max(rows.values(), key=len)
    return solid, sorted(chart_row, key=lambda block: block["x"])

solid_color_blocks, solid_chart_blocks = detect_solid_color_blocks()
large_raster_area = sum(comp["area"] for comp in large_images)
chart_evidence_count = max(len(chart_cells), len(solid_chart_blocks))
if ttyd_mode == "custom":
    # The custom harness loads @xterm/addon-image directly and should paint the
    # color chart plus enough large photo area to prove native raster rendering.
    ok = large_raster_area >= 45000 and chart_evidence_count >= 4
else:
    # Stock ttyd is intentionally a smoke test for the exact manual command.
    # xterm.js may relocate or omit later overlays, so require only that browser
    # raster bytes visibly paint at least the color chart plus one large image.
    ok = large_raster_area >= 10000 and chart_evidence_count >= 4
metrics = {
    "mode": ttyd_mode,
    "theme": herald_theme or None,
    "screenshot": str(screenshot),
    "image_size": {"width": width, "height": height},
    "large_image_components": large_images[:10],
    "large_raster_area": large_raster_area,
    "chart_cell_components": chart_cells[:20],
    "solid_color_blocks": solid_color_blocks[:30],
    "solid_chart_blocks": solid_chart_blocks[:20],
    "chart_evidence_count": chart_evidence_count,
    "component_count": len(components),
    "pass": ok,
}
metrics_path.write_text(json.dumps(metrics, indent=2) + "\n")
print(json.dumps(metrics, indent=2))
if not ok:
    sys.exit(1)
PY

echo "ttyd image preview screenshot: $SCREENSHOT_PATH"
echo "ttyd image preview metrics:    $METRICS_PATH"
