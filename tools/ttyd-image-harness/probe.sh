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
  PROBE_TARGET     demo-image-sampler, timeline-remote-reveal, or timeline-image-clear-on-navigation (default: demo-image-sampler)
  SEARCH_QUERY     Herald search query (default: Step 5: View inline images in full screen)
  RENDERER_TYPE    ttyd renderer for stock mode (default: canvas)
  IMAGE_PROTOCOL   Herald image protocol (default: iterm2)
  PROBE_VIEWPORT_WIDTH   Browser viewport width override
  PROBE_VIEWPORT_HEIGHT  Browser viewport height override
  HERALD_THEME     Optional Herald app theme, e.g. jade-signal
  HERALD_PRESERVE_NO_COLOR  Set to 1 only when intentionally testing NO_COLOR
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
PROBE_TARGET="${PROBE_TARGET:-demo-image-sampler}"
SEARCH_QUERY="${SEARCH_QUERY:-Step 5: View inline images in full screen}"
RENDERER_TYPE="${RENDERER_TYPE:-canvas}"
IMAGE_PROTOCOL="${IMAGE_PROTOCOL:-iterm2}"
HERALD_THEME="${HERALD_THEME:-}"
HERALD_BIN="${HERALD_BIN:-./bin/herald}"
TTYD_BIN="${TTYD_BIN:-ttyd}"
NODE_BIN="${NODE_BIN:-node}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
EVIDENCE_DIR="${EVIDENCE_DIR:-reports/ttyd-image-preview_$(date +%F_%H%M%S)}"
SCREENSHOT_PATH="$EVIDENCE_DIR/ttyd-image-preview.png"
BEFORE_SCREENSHOT_PATH="$EVIDENCE_DIR/ttyd-image-before-navigation.png"
METRICS_PATH="$EVIDENCE_DIR/ttyd-image-preview-metrics.json"
METADATA_PATH="$EVIDENCE_DIR/ttyd-image-preview-metadata.json"
TTYD_LOG="$EVIDENCE_DIR/ttyd.log"
GIT_SHA="$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"

mkdir -p "$EVIDENCE_DIR"

case "$PROBE_TARGET" in
  demo-image-sampler|timeline-remote-reveal|timeline-image-clear-on-navigation)
    ;;
  *)
    echo "PROBE_TARGET must be 'demo-image-sampler', 'timeline-remote-reveal', or 'timeline-image-clear-on-navigation' for this harness slice." >&2
    exit 2
    ;;
esac

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

herald_env=(env)
if [ "${HERALD_PRESERVE_NO_COLOR:-0}" != "1" ]; then
  herald_env+=(-u NO_COLOR COLORTERM="${COLORTERM:-truecolor}")
fi

"$TTYD_BIN" \
  "${ttyd_args[@]}" \
  "${herald_env[@]}" "$HERALD_BIN" "${herald_args[@]}" \
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
BEFORE_SCREENSHOT_PATH="$BEFORE_SCREENSHOT_PATH" \
METADATA_PATH="$METADATA_PATH" \
METRICS_PATH="$METRICS_PATH" \
CHROME_BIN="$CHROME_BIN" \
PROBE_TARGET="$PROBE_TARGET" \
SEARCH_QUERY="$SEARCH_QUERY" \
TTYD_MODE="$TTYD_MODE" \
IMAGE_PROTOCOL="$IMAGE_PROTOCOL" \
HERALD_THEME="$HERALD_THEME" \
PROBE_VIEWPORT_WIDTH="${PROBE_VIEWPORT_WIDTH:-}" \
PROBE_VIEWPORT_HEIGHT="${PROBE_VIEWPORT_HEIGHT:-}" \
GIT_SHA="$GIT_SHA" \
"$NODE_BIN" <<'NODE'
const { chromium } = require("playwright");
const fs = require("fs");

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const visibleText = async (page, limit = 4000) =>
  page.evaluate((n) => document.body.innerText.slice(0, n), limit);
const waitForVisibleText = async (page, predicate, label, timeoutMs = 10000) => {
  const deadline = Date.now() + timeoutMs;
  let lastText = "";
  while (Date.now() < deadline) {
    lastText = await visibleText(page, 12000);
    if (predicate(lastText)) return lastText;
    await sleep(250);
  }
  throw new Error(`timed out waiting for ${label}; visible text head was: ${lastText.slice(0, 1000)}; tail was: ${lastText.slice(-2000)}`);
};

(async () => {
  const browser = await chromium.launch({
    executablePath: process.env.CHROME_BIN,
    headless: true,
  });
  const target = process.env.PROBE_TARGET;
  const defaultViewport =
    target === "timeline-remote-reveal" || target === "timeline-image-clear-on-navigation"
      ? { width: 1700, height: 1800 }
      : { width: 1300, height: 1000 };
  const viewport = {
    width: Number(process.env.PROBE_VIEWPORT_WIDTH || defaultViewport.width),
    height: Number(process.env.PROBE_VIEWPORT_HEIGHT || defaultViewport.height),
  };
  const page = await browser.newPage({
    viewport,
    deviceScaleFactor: 1,
  });

  const logs = [];
  page.on("console", (msg) => logs.push(`${msg.type()}: ${msg.text()}`));
  page.on("pageerror", (err) => logs.push(`pageerror: ${err.message}`));

  await page.goto(process.env.URL, { waitUntil: "domcontentloaded" });
  await waitForVisibleText(
    page,
    (text) => text.includes("Timeline") && text.includes("Herald Welcome") && text.includes("Step 1: Move around your inbox"),
    "Herald demo Timeline readiness",
  );

  await page.keyboard.press("Escape");
  await sleep(400);

  if (target === "timeline-image-clear-on-navigation") {
    for (let i = 0; i < 5; i++) await page.keyboard.press("ArrowDown");
    await sleep(500);
    await page.keyboard.press("Enter");
    await waitForVisibleText(
      page,
      (text) =>
        text.includes("From: Herald Image Lab") &&
        text.includes("Subj: Step 5: View inline images in full screen") &&
        text.includes("image(s) shown below"),
      "Step 5 split preview with inline images",
      12000,
    );
    await sleep(6000);
    await page.screenshot({ path: process.env.BEFORE_SCREENSHOT_PATH, fullPage: true });
    await page.keyboard.press("ArrowDown");
    await waitForVisibleText(
      page,
      (text) =>
        text.includes("From: Herald Cleanup Coach") &&
        text.includes("Subj: Step 6: Clean up senders and domains safely"),
      "Step 6 split preview after list navigation",
      12000,
    );
    await sleep(2500);
  } else {
    await page.keyboard.press("/");
    await sleep(250);
    await page.keyboard.type(process.env.SEARCH_QUERY, { delay: 5 });
    await page.keyboard.press("Enter");
    await waitForVisibleText(
      page,
      (text) =>
        text.includes(process.env.SEARCH_QUERY) &&
        !text.includes("Search: 0 results") &&
        text.includes("Herald Image Lab") &&
        !text.includes("Herald Welcome"),
      `search results for ${JSON.stringify(process.env.SEARCH_QUERY)}`,
    );
    await sleep(900);
    await page.keyboard.press("Enter");
    await sleep(1200);
  }

  const openedText = await visibleText(page);
  if (target !== "timeline-image-clear-on-navigation") {
    if (
      !openedText.includes(process.env.SEARCH_QUERY) ||
      !openedText.includes("Herald Image Lab") ||
      openedText.includes("Search: 0 results")
    ) {
      throw new Error(`failed to reach demo image email for query ${JSON.stringify(process.env.SEARCH_QUERY)}; visible text was: ${openedText.slice(0, 1000)}`);
    }
  }

  if (target === "timeline-remote-reveal") {
    if (!openedText.includes("linked image(s)") || !openedText.includes("press o to reveal")) {
      throw new Error(`split preview did not show the remote-image reveal hint; visible text was: ${openedText.slice(0, 1400)}`);
    }
    const inputState = await page.evaluate(() => window.heraldHarnessInput("o"));
    logs.push(`harness input state: ${inputState}`);
    console.log(`harness input state: ${inputState}`);
    await sleep(6000);
  } else if (target === "demo-image-sampler") {
    await page.keyboard.press("z");
    await sleep(1000);
    for (let i = 0; i < 16; i++) await page.keyboard.press("ArrowDown");
    await sleep(6000);
  }

  await page.screenshot({ path: process.env.SCREENSHOT_PATH, fullPage: true });
  const bodyText = await visibleText(page, 2200);
  fs.writeFileSync(
    process.env.METADATA_PATH,
    JSON.stringify(
      {
        "probeTarget": process.env.PROBE_TARGET,
        "searchQuery": process.env.SEARCH_QUERY,
        "imageProtocol": process.env.IMAGE_PROTOCOL,
        "theme": process.env.HERALD_THEME || null,
        "ttydMode": process.env.TTYD_MODE,
        "browserPath": process.env.CHROME_BIN,
        "gitSHA": process.env.GIT_SHA,
        "url": process.env.URL,
        "viewport": viewport,
        "screenshot": process.env.SCREENSHOT_PATH,
        "beforeScreenshot": target === "timeline-image-clear-on-navigation" ? process.env.BEFORE_SCREENSHOT_PATH : null,
        "metrics": process.env.METRICS_PATH,
        "visibleTextSample": bodyText,
      },
      null,
      2,
    ) + "\n",
  );
  console.log(JSON.stringify({ screenshot: process.env.SCREENSHOT_PATH, bodyText, logs }, null, 2));
  await browser.close();
})();
NODE

SCREENSHOT_PATH="$SCREENSHOT_PATH" BEFORE_SCREENSHOT_PATH="$BEFORE_SCREENSHOT_PATH" METRICS_PATH="$METRICS_PATH" TTYD_MODE="$TTYD_MODE" HERALD_THEME="$HERALD_THEME" PROBE_TARGET="$PROBE_TARGET" "$PYTHON_BIN" <<'PY'
from __future__ import annotations

import json
import os
import sys
from pathlib import Path

from PIL import Image

screenshot = Path(os.environ["SCREENSHOT_PATH"])
before_screenshot = Path(os.environ["BEFORE_SCREENSHOT_PATH"])
metrics_path = Path(os.environ["METRICS_PATH"])
ttyd_mode = os.environ["TTYD_MODE"]
herald_theme = os.environ.get("HERALD_THEME", "")
probe_target = os.environ["PROBE_TARGET"]

def analyze(path: Path):
    image = Image.open(path).convert("RGB")
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
    return {
        "screenshot": str(path),
        "image_size": {"width": width, "height": height},
        "large_image_components": large_images[:10],
        "large_raster_area": large_raster_area,
        "chart_cell_components": chart_cells[:20],
        "solid_color_blocks": solid_color_blocks[:30],
        "solid_chart_blocks": solid_chart_blocks[:20],
        "chart_evidence_count": chart_evidence_count,
        "component_count": len(components),
    }

current = analyze(screenshot)

if probe_target == "timeline-image-clear-on-navigation":
    before = analyze(before_screenshot)
    after = current
    ok = (
        before["large_raster_area"] >= 10000
        and before["component_count"] >= 8
        and after["large_raster_area"] <= max(9000, before["large_raster_area"] * 0.15)
    )
    metrics = {
        "mode": ttyd_mode,
        "theme": herald_theme or None,
        "target": probe_target,
        "before": before,
        "after": after,
        "pass": ok,
    }
    metrics_path.write_text(json.dumps(metrics, indent=2) + "\n")
    print(json.dumps(metrics, indent=2))
    if not ok:
        sys.exit(1)
    sys.exit(0)

large_raster_area = current["large_raster_area"]
chart_evidence_count = current["chart_evidence_count"]
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
	**current,
	"pass": ok,
}
metrics_path.write_text(json.dumps(metrics, indent=2) + "\n")
print(json.dumps(metrics, indent=2))
if not ok:
    sys.exit(1)
PY

echo "ttyd image preview screenshot: $SCREENSHOT_PATH"
echo "ttyd image preview metrics:    $METRICS_PATH"
echo "ttyd image preview metadata:   $METADATA_PATH"
