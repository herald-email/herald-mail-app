# iTerm2 Raster Inline Image Placement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Herald's iTerm2 OSC 1337 full-screen preview path render real bounded raster images without losing the pinned preview header or corrupting document flow.

**Architecture:** Keep the existing preview document stream, but add an explicit raster row contract so protocol renderers can state whether the terminal itself advances through reserved image rows. Kitty/Ghostty remains the robust reference path; iTerm2 gets conservative safe cell boxes and terminal-consumed reservation rows so Herald does not double-advance the cursor with blank padding. Verification requires both unit tests and custom ttyd+xterm image-addon screenshots with real raster images.

**Tech Stack:** Go 1.25, Bubble Tea, Lipgloss, existing `internal/app` preview document renderer, `internal/iterm2` OSC 1337 renderer, `internal/kittyimg` renderer, tmux, ttyd custom xterm.js image harness, Playwright/Chrome or manual browser screenshot.

---

## File Structure

- Modify `internal/app/preview_image_renderer.go`
  - Owns image mode detection, cell sizing, raster placement planning, and rendering one preview image block.
  - Add an iTerm2-specific safe sizing function and a `TerminalConsumesRows` flag on `previewImageRenderResult`.
- Modify `internal/app/preview_viewport.go`
  - Owns preview document row materialization and viewport rendering.
  - Add a row flag for terminal-consumed image rows and skip physical blank output for those rows.
- Modify `internal/app/preview_image_renderer_test.go`
  - Add TDD coverage for iTerm2 safe boxes, exact row consumption, and forced raster mode.
- Modify `internal/app/preview_viewport_test.go`
  - Add viewport tests proving iTerm2 terminal-consumed reservation rows count in scroll math without printing duplicate blank rows.
- Modify `internal/app/image_preview_test.go`
  - Add a full-screen regression proving iTerm2 output remains raster, bounded, and not local-link fallback.
- Create `internal/iterm2/render_test.go`
  - Test OSC 1337 output shape and explicit dimensions.
- Write `reports/TEST_REPORT_2026-04-29_iterm2-raster-inline-image-placement.md`
  - Record red repro, tests, custom ttyd+xterm raster evidence, tmux captures, and residual risk.

## Task 0: Worktree And Red Visual Repro

**Files:**
- Read: `docs/superpowers/specs/2026-04-29-iterm2-raster-inline-image-placement-design.md`
- Read: `TUI_TESTPLAN.md`
- Read: `reports/ttyd-image-harness/index.html`
- Artifact: `reports/iterm2-raster-inline-image-placement_2026-04-29/`

- [ ] **Step 1: Create an isolated implementation worktree**

Run:

```bash
git worktree add .worktrees/20260429-iterm2-raster-inline-image-placement \
  -b codex/iterm2-raster-inline-image-placement-20260429
cd .worktrees/20260429-iterm2-raster-inline-image-placement
```

Expected: new worktree on branch `codex/iterm2-raster-inline-image-placement-20260429`.

- [ ] **Step 2: Create the evidence directory**

Run:

```bash
mkdir -p reports/iterm2-raster-inline-image-placement_2026-04-29
```

Expected: the directory exists and is gitignored by `reports/`.

- [ ] **Step 3: Build the red repro binary**

Run:

```bash
go build -o /tmp/herald-iterm-red ./main.go
```

Expected: command exits 0.

- [ ] **Step 4: Capture tmux fallback baseline**

Run:

```bash
tmux kill-session -t herald-iterm-red 2>/dev/null || true
tmux new-session -d -s herald-iterm-red -x 220 -y 50
tmux send-keys -t herald-iterm-red '/tmp/herald-iterm-red --demo' Enter
sleep 5
tmux send-keys -t herald-iterm-red '/'
sleep 0.2
tmux send-keys -t herald-iterm-red 'Creative Commons image sampler for terminal previews'
sleep 0.5
tmux send-keys -t herald-iterm-red Enter
sleep 0.2
tmux send-keys -t herald-iterm-red j
sleep 0.2
tmux send-keys -t herald-iterm-red Enter
sleep 1
tmux send-keys -t herald-iterm-red z
sleep 1
tmux capture-pane -t herald-iterm-red -p > reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_red_220x50.txt
tmux capture-pane -t herald-iterm-red -p -e > reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_red_220x50.ansi.txt
.agents/skills/tui-test/screenshot.sh herald-iterm-red reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_red_220x50.png 1800x1000
```

Expected: capture shows the Creative Commons sampler in full-screen mode with safe fallback links in tmux.

- [ ] **Step 5: Capture custom ttyd+xterm image-addon red screenshot**

Run this in one terminal:

```bash
lsof -ti tcp:7685 | xargs -r kill
TERM_PROGRAM=iTerm.app ttyd -W -p 7685 \
  -I reports/ttyd-image-harness/index.html \
  -t rendererType=canvas \
  -t disableLeaveAlert=true \
  -t disableResizeOverlay=true \
  /tmp/herald-iterm-red --demo
```

Then run the browser capture from another terminal:

```bash
NODE_PATH=/Users/zoomacode/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules \
/Users/zoomacode/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node <<'NODE'
const { chromium } = require("playwright");
(async () => {
  const out = "reports/iterm2-raster-inline-image-placement_2026-04-29";
  const browser = await chromium.launch({
    headless: true,
    executablePath: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  });
  const page = await browser.newPage({ viewport: { width: 1512, height: 1704 }, deviceScaleFactor: 1 });
  await page.goto("http://127.0.0.1:7685", { waitUntil: "networkidle", timeout: 60000 });
  await page.waitForTimeout(5000);
  await page.mouse.click(500, 500);
  await page.keyboard.press("/");
  await page.keyboard.type("Creative Commons image sampler for terminal previews", { delay: 5 });
  await page.waitForTimeout(500);
  await page.keyboard.press("Enter");
  await page.waitForTimeout(250);
  await page.keyboard.press("j");
  await page.waitForTimeout(250);
  await page.keyboard.press("Enter");
  await page.waitForTimeout(1200);
  await page.keyboard.press("z");
  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${out}/browser_red_iterm2_raster.png`, fullPage: true });
  await browser.close();
})();
NODE
```

Expected: `browser_red_iterm2_raster.png` shows the current broken raster placement: the full-screen header/body context is missing or displaced by image output.

- [ ] **Step 6: Stop red repro processes**

Run:

```bash
tmux kill-session -t herald-iterm-red 2>/dev/null || true
lsof -ti tcp:7685 | xargs -r kill
```

Expected: no `ttyd` or tmux repro session remains.

## Task 1: Add Failing iTerm2 Sizing And Row Contract Tests

**Files:**
- Modify: `internal/app/preview_image_renderer_test.go`
- Test: `internal/app/preview_image_renderer_test.go`

- [ ] **Step 1: Add tests for iTerm2 safe landscape sizing and terminal row consumption**

Append this test block after `TestPreviewImageCellSizeBoundsLargeImages` in `internal/app/preview_image_renderer_test.go`:

```go
func TestIterm2PreviewImageCellSizeUsesSafeLandscapeBox(t *testing.T) {
	img := models.InlineImage{ContentID: "landscape", MIMEType: "image/png", Data: tinyPNG(t, 960, 540)}

	size := previewImageCellSizeForMode(previewImageModeIterm2, img, 160, 18)

	if size.Rows != 18 {
		t.Fatalf("iTerm2 landscape rows = %d, want exact row cap 18", size.Rows)
	}
	if size.Width > 72 {
		t.Fatalf("iTerm2 landscape width = %d cells, want conservative safe width <= 72", size.Width)
	}
	if size.Width < 48 {
		t.Fatalf("iTerm2 landscape width = %d cells, want useful raster thumbnail >= 48", size.Width)
	}
}

func TestIterm2PreviewRendererMarksTerminalConsumedRows(t *testing.T) {
	img := models.InlineImage{ContentID: "photo", MIMEType: "image/png", Data: tinyPNG(t, 960, 540)}

	rendered := renderPreviewImageBlock(previewImageRenderRequest{
		Mode:          previewImageModeIterm2,
		Image:         img,
		InnerWidth:    160,
		AvailableRows: 18,
	})

	if !rendered.TerminalConsumesRows {
		t.Fatalf("iTerm2 rendered block should mark terminal-consumed rows: %#v", rendered)
	}
	if rendered.Rows != 18 {
		t.Fatalf("iTerm2 rows = %d, want 18", rendered.Rows)
	}
	if strings.Count(rendered.Content, "\n") != 0 {
		t.Fatalf("iTerm2 content should be a single physical OSC line, got %q", rendered.Content)
	}
}
```

- [ ] **Step 2: Run the focused red tests**

Run:

```bash
go test ./internal/app -run 'Iterm2PreviewImageCellSize|Iterm2PreviewRendererMarks' -count=1
```

Expected: FAIL because `previewImageCellSizeForMode` and `TerminalConsumesRows` do not exist yet.

## Task 2: Implement iTerm2 Safe Cell Sizing And Render Result Contract

**Files:**
- Modify: `internal/app/preview_image_renderer.go`
- Test: `internal/app/preview_image_renderer_test.go`

- [ ] **Step 1: Extend `previewImageRenderResult`**

In `internal/app/preview_image_renderer.go`, replace:

```go
type previewImageRenderResult struct {
	Content string
	Rows    int
}
```

with:

```go
type previewImageRenderResult struct {
	Content              string
	Rows                 int
	TerminalConsumesRows bool
}
```

- [ ] **Step 2: Add mode-aware cell sizing helpers**

In `internal/app/preview_image_renderer.go`, replace the body of `previewImageCellSize` and add the new helpers below it:

```go
func previewImageCellSize(img models.InlineImage, innerW, availableRows int) previewImageSize {
	return previewImageCellSizeForMode(previewImageModeKitty, img, innerW, availableRows)
}

func previewImageCellSizeForMode(mode previewImageMode, img models.InlineImage, innerW, availableRows int) previewImageSize {
	if mode == previewImageModeIterm2 {
		return previewIterm2ImageCellSize(img, innerW, availableRows)
	}
	return previewAspectFitImageCellSize(img, innerW, availableRows, decodedPreviewImageMaxRows)
}

func previewAspectFitImageCellSize(img models.InlineImage, innerW, availableRows, maxRows int) previewImageSize {
	widthCap := innerW - 2
	if widthCap < 1 {
		widthCap = 1
	}
	rowCap := minInt(availableRows, maxRows)
	if rowCap < 1 {
		rowCap = 1
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		rows := availableRows / 2
		if rows < 1 {
			rows = 1
		}
		rows = minInt(rows, maxPreviewImageRows)
		rows = minInt(rows, rowCap)
		return previewImageSize{Width: widthCap, Rows: rows}
	}

	widthCells := (cfg.Width + 7) / 8
	rowCells := (cfg.Height + 15) / 16
	if widthCells < 1 {
		widthCells = 1
	}
	if rowCells < 1 {
		rowCells = 1
	}
	if widthCells > widthCap || rowCells > rowCap {
		if widthCap*rowCells <= rowCap*widthCells {
			rowCells = ceilDivInt(rowCells*widthCap, widthCells)
			widthCells = widthCap
		} else {
			widthCells = ceilDivInt(widthCells*rowCap, rowCells)
			rowCells = rowCap
		}
	}
	if widthCells > widthCap {
		widthCells = widthCap
	}
	if rowCells > rowCap {
		rowCells = rowCap
	}
	if widthCells < 1 {
		widthCells = 1
	}
	if rowCells < 1 {
		rowCells = 1
	}
	return previewImageSize{Width: widthCells, Rows: rowCells}
}

func previewIterm2ImageCellSize(img models.InlineImage, innerW, availableRows int) previewImageSize {
	size := previewAspectFitImageCellSize(img, innerW, availableRows, decodedPreviewImageMaxRows)
	widthCap := innerW - 2
	if widthCap < 1 {
		widthCap = 1
	}
	rowCap := minInt(availableRows, decodedPreviewImageMaxRows)
	if rowCap < 1 {
		rowCap = 1
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return size
	}

	aspectMilli := cfg.Width * 1000 / cfg.Height
	switch {
	case cfg.Width <= 80 && cfg.Height <= 40:
		size.Width = minInt(size.Width, minInt(widthCap, 10))
		size.Rows = minInt(size.Rows, minInt(rowCap, 3))
	case aspectMilli >= 1500:
		size.Rows = minInt(rowCap, maxInt(6, minInt(rowCap, 18)))
		size.Width = minInt(widthCap, maxInt(24, minInt(ceilDivInt(size.Rows*16*cfg.Width, 8*cfg.Height), 72)))
	case aspectMilli <= 800:
		size.Rows = minInt(rowCap, maxInt(8, minInt(rowCap, 16)))
		size.Width = minInt(widthCap, maxInt(12, minInt(ceilDivInt(size.Rows*16*cfg.Width, 8*cfg.Height), 44)))
	default:
		size.Rows = minInt(rowCap, maxInt(8, minInt(rowCap, 14)))
		size.Width = minInt(widthCap, maxInt(18, minInt(ceilDivInt(size.Rows*16*cfg.Width, 8*cfg.Height), 56)))
	}
	if size.Width < 1 {
		size.Width = 1
	}
	if size.Rows < 1 {
		size.Rows = 1
	}
	return size
}
```

- [ ] **Step 3: Add integer helpers**

Add these helpers below `minInt`:

```go
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func ceilDivInt(a, b int) int {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}
```

- [ ] **Step 4: Use mode-aware sizing and terminal-consumed rows**

In `renderPreviewImageBlock`, replace both raster sizing calls:

```go
size := previewImageCellSize(req.Image, req.InnerWidth, req.AvailableRows)
```

with:

```go
size := previewImageCellSizeForMode(mode, req.Image, req.InnerWidth, req.AvailableRows)
```

Then replace the iTerm2 return:

```go
return previewImageRenderResult{Content: rendered, Rows: size.Rows}
```

with:

```go
return previewImageRenderResult{Content: rendered, Rows: size.Rows, TerminalConsumesRows: true}
```

Leave the Kitty return as:

```go
return previewImageRenderResult{Content: strings.TrimRight(rendered, "\n"), Rows: size.Rows}
```

- [ ] **Step 5: Run focused tests**

Run:

```bash
go test ./internal/app -run 'PreviewImageCellSize|Iterm2PreviewRendererMarks|KittyPreviewRenderer' -count=1
```

Expected: PASS for the new sizing/render-result tests and existing Kitty renderer tests.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/app/preview_image_renderer.go internal/app/preview_image_renderer_test.go
git commit -m "fix: plan bounded iterm2 preview image cells"
```

Expected: commit succeeds.

## Task 3: Render iTerm2 Terminal-Consumed Rows Without Duplicate Padding

**Files:**
- Modify: `internal/app/preview_viewport.go`
- Modify: `internal/app/preview_viewport_test.go`
- Test: `internal/app/preview_viewport_test.go`

- [ ] **Step 1: Add a row flag test**

Append this test to `internal/app/preview_viewport_test.go`:

```go
func TestPreviewRowsFromIterm2ImageMarksConsumedRows(t *testing.T) {
	rows := previewRowsFromRenderedImage(previewImageRenderResult{
		Content:              "\x1b]1337;File=inline=1;width=64;height=18:payload\a",
		Rows:                 4,
		TerminalConsumesRows: true,
	}, 80)

	if len(rows) != 4 {
		t.Fatalf("row count = %d, want 4", len(rows))
	}
	if rows[0].TerminalConsumed {
		t.Fatalf("first row must carry the OSC output")
	}
	for i := 1; i < len(rows); i++ {
		if !rows[i].TerminalConsumed {
			t.Fatalf("row %d should be terminal-consumed reservation: %#v", i, rows[i])
		}
	}
}

func TestRenderPreviewDocumentViewportSkipsIterm2ConsumedPhysicalRows(t *testing.T) {
	layout := previewDocumentLayout{
		ImageMode: previewImageModeIterm2,
		Rows: []previewRenderedRow{
			{Content: "before"},
			{Content: "\x1b]1337;File=inline=1;width=64;height=3:payload\a"},
			{TerminalConsumed: true},
			{TerminalConsumed: true},
			{Content: "after"},
		},
		TotalRows: 5,
	}

	rendered := renderPreviewDocumentViewport(layout, 0, 5)

	if rendered.Rows != 5 {
		t.Fatalf("rendered rows = %d, want logical viewport rows 5", rendered.Rows)
	}
	if strings.Count(rendered.Content, "\n") != 2 {
		t.Fatalf("iTerm2 terminal-consumed rows should not print duplicate blank lines, got %q", rendered.Content)
	}
	if !strings.Contains(rendered.Content, "before") || !strings.Contains(rendered.Content, "after") {
		t.Fatalf("viewport lost surrounding text: %q", rendered.Content)
	}
}
```

- [ ] **Step 2: Run the focused red viewport tests**

Run:

```bash
go test ./internal/app -run 'PreviewRowsFromIterm2|SkipsIterm2Consumed' -count=1
```

Expected: FAIL because `previewRenderedRow.TerminalConsumed` does not exist yet.

- [ ] **Step 3: Add `TerminalConsumed` to preview rows**

In `internal/app/preview_viewport.go`, replace:

```go
type previewRenderedRow struct {
	Content string
}
```

with:

```go
type previewRenderedRow struct {
	Content          string
	TerminalConsumed bool
}
```

- [ ] **Step 4: Update image row materialization**

Replace `previewRowsFromRenderedImage` in `internal/app/preview_viewport.go` with:

```go
func previewRowsFromRenderedImage(rendered previewImageRenderResult, innerWidth int) []previewRenderedRow {
	contentLines := strings.Split(rendered.Content, "\n")
	rows := make([]previewRenderedRow, 0, rendered.Rows)
	for i, line := range contentLines {
		if i >= rendered.Rows {
			break
		}
		rows = append(rows, previewRenderedRow{Content: ansi.Truncate(line, innerWidth, "")})
	}
	if rendered.TerminalConsumesRows {
		for len(rows) < rendered.Rows {
			rows = append(rows, previewRenderedRow{TerminalConsumed: true})
		}
		return rows
	}
	for len(rows) < rendered.Rows {
		rows = append(rows, previewRenderedRow{})
	}
	return rows
}
```

- [ ] **Step 5: Update viewport rendering to skip consumed physical rows**

In `renderPreviewDocumentViewportWithVisual`, replace this loop body:

```go
for i := offset; i < end && i < len(layout.Rows); i++ {
	content := layout.Rows[i].Content
	if visualMode && i >= lo && i <= hi {
		content = highlightStyle.Render(content)
	}
	lines = append(lines, content)
}
for len(lines) < visibleRows {
	lines = append(lines, "")
}
content := strings.Join(lines, "\n")
```

with:

```go
hasTerminalConsumedRows := false
for i := offset; i < end && i < len(layout.Rows); i++ {
	row := layout.Rows[i]
	if row.TerminalConsumed {
		hasTerminalConsumedRows = true
		continue
	}
	content := row.Content
	if visualMode && i >= lo && i <= hi {
		content = highlightStyle.Render(content)
	}
	lines = append(lines, content)
}
if !hasTerminalConsumedRows {
	for len(lines) < visibleRows {
		lines = append(lines, "")
	}
}
content := strings.Join(lines, "\n")
```

Keep the return as:

```go
return previewViewportRender{Content: content, Rows: len(lines)}
```

Then change it to:

```go
return previewViewportRender{Content: content, Rows: visibleRows}
```

- [ ] **Step 6: Run viewport tests**

Run:

```bash
go test ./internal/app -run 'PreviewRowsFrom|RenderPreviewDocumentViewport|ClampPreviewScrollOffset' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/app/preview_viewport.go internal/app/preview_viewport_test.go
git commit -m "fix: avoid duplicate iterm2 raster padding rows"
```

Expected: commit succeeds.

## Task 4: Lock Down OSC 1337 Output

**Files:**
- Create: `internal/iterm2/render_test.go`
- Modify: `internal/iterm2/render.go`
- Test: `internal/iterm2/render_test.go`

- [ ] **Step 1: Add OSC output tests**

Create `internal/iterm2/render_test.go`:

```go
package iterm2

import (
	"strings"
	"testing"
)

func TestRenderInlineUsesExplicitCellDimensions(t *testing.T) {
	rendered := RenderInline([]byte("image-bytes"), 64, 18)

	for _, want := range []string{
		"\x1b]1337;File=",
		"inline=1",
		"preserveAspectRatio=1",
		"size=11",
		"width=64",
		"height=18",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered escape missing %q: %q", want, rendered)
		}
	}
}

func TestRenderInlineReturnsSingleOSCLineWithTrailingNewline(t *testing.T) {
	rendered := RenderInline([]byte("image-bytes"), 64, 18)

	if strings.Count(rendered, "\n") != 1 || !strings.HasSuffix(rendered, "\n") {
		t.Fatalf("RenderInline should emit one OSC line plus one trailing newline for callers to trim, got %q", rendered)
	}
}

func TestRenderRequiresItermEnvironmentButRenderInlineDoesNot(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	if got := Render([]byte("image-bytes"), 64, 18); got != "" {
		t.Fatalf("Render without iTerm env = %q, want empty string", got)
	}
	if got := RenderInline([]byte("image-bytes"), 64, 18); !strings.Contains(got, "\x1b]1337;File=") {
		t.Fatalf("RenderInline should support forced protocol mode, got %q", got)
	}
}
```

- [ ] **Step 2: Run the iTerm2 tests**

Run:

```bash
go test ./internal/iterm2 -count=1
```

Expected: PASS. If this fails because current output differs, update `internal/iterm2/render.go` only to preserve explicit `inline`, `preserveAspectRatio`, `size`, `width`, and `height` arguments. Do not add undocumented cursor-control arguments.

- [ ] **Step 3: Commit**

Run:

```bash
git add internal/iterm2/render.go internal/iterm2/render_test.go
git commit -m "test: cover iterm2 inline image escapes"
```

Expected: commit succeeds.

## Task 5: Full-Screen App Regression Tests

**Files:**
- Modify: `internal/app/image_preview_test.go`
- Test: `internal/app/image_preview_test.go`

- [ ] **Step 1: Add a full-screen iTerm2 row-budget regression**

Append this test after `TestTimelineFullScreen_ItermRendersBoundedInlineImage` in `internal/app/image_preview_test.go`:

```go
func TestTimelineFullScreen_ItermDoesNotPrintDuplicateImagePadding(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	m := makeSizedModel(t, 120, 32)
	defer m.cleanup()
	m.activeTab = tabTimeline
	m.focusedPanel = panelPreview
	email := testImageEmail()
	m.timeline.selectedEmail = email
	m.timeline.bodyMessageID = email.MessageID
	m.timeline.body = &models.EmailBody{
		TextHTML: `<p>Before image.</p><img alt="Landscape" src="cid:landscape"><p>After image.</p>`,
		InlineImages: []models.InlineImage{
			{ContentID: "landscape", MIMEType: "image/png", Data: tinyPNG(t, 960, 540)},
		},
	}
	m.timeline.fullScreen = true

	rendered := m.renderFullScreenEmail()
	if !strings.Contains(rendered, "\x1b]1337;File=") {
		t.Fatalf("expected iTerm2 raster escape, got raw:\n%q", rendered)
	}
	if strings.Contains(rendered, "open image") || strings.Contains(rendered, "127.0.0.1") {
		t.Fatalf("iTerm2 target path must not use fallback links, got:\n%s", stripANSI(rendered))
	}
	if got := strings.Count(rendered, "\n\n\n\n\n\n"); got > 0 {
		t.Fatalf("iTerm2 render should not contain long duplicate blank padding runs, got %d in:\n%q", got, rendered)
	}
	assertFitsHeight(t, 32, rendered)
}
```

- [ ] **Step 2: Run the focused app regression**

Run:

```bash
go test ./internal/app -run 'TimelineFullScreen_Iterm|ItermDoesNotPrintDuplicate' -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

Run:

```bash
git add internal/app/image_preview_test.go
git commit -m "test: guard iterm2 full-screen raster flow"
```

Expected: commit succeeds.

## Task 6: Focused And Broad Automated Verification

**Files:**
- Artifact: `reports/iterm2-raster-inline-image-placement_2026-04-29/*.log`

- [ ] **Step 1: Run focused image tests**

Run:

```bash
go test ./internal/app ./internal/iterm2 ./internal/kittyimg \
  -run 'PreviewImage|PreviewDocument|TimelineFullScreen|Iterm|Kitty|CreativeCommons' \
  -count=1 2>&1 | tee reports/iterm2-raster-inline-image-placement_2026-04-29/focused-tests.log
```

Expected: all listed packages pass.

- [ ] **Step 2: Run broad tests**

Run:

```bash
go test ./... 2>&1 | tee reports/iterm2-raster-inline-image-placement_2026-04-29/go-test-all.log
```

Expected: all packages pass.

- [ ] **Step 3: Commit any missed test-only changes**

Run:

```bash
git status --short
```

Expected: no uncommitted code changes. If there are only intended code/test changes, commit them with:

```bash
git add internal/app internal/iterm2 internal/kittyimg
git commit -m "test: finish iterm2 raster regression coverage"
```

Expected: either no commit is needed or the commit succeeds.

## Task 7: Visual Verification With Real Raster Evidence

**Files:**
- Artifact: `reports/iterm2-raster-inline-image-placement_2026-04-29/browser_fixed_iterm2_raster.png`
- Artifact: `reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_220x50.png`
- Artifact: `reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_80x24.png`
- Artifact: `reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_50x15.png`

- [ ] **Step 1: Build the fixed binary**

Run:

```bash
go build -o /tmp/herald-iterm-fixed ./main.go
```

Expected: command exits 0.

- [ ] **Step 2: Capture fixed custom ttyd+xterm raster screenshot**

Run in one terminal:

```bash
lsof -ti tcp:7686 | xargs -r kill
TERM_PROGRAM=iTerm.app ttyd -W -p 7686 \
  -I reports/ttyd-image-harness/index.html \
  -t rendererType=canvas \
  -t disableLeaveAlert=true \
  -t disableResizeOverlay=true \
  /tmp/herald-iterm-fixed --demo -image-protocol=iterm2
```

Run the browser automation:

```bash
NODE_PATH=/Users/zoomacode/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules \
/Users/zoomacode/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node <<'NODE'
const { chromium } = require("playwright");
(async () => {
  const out = "reports/iterm2-raster-inline-image-placement_2026-04-29";
  const browser = await chromium.launch({
    headless: true,
    executablePath: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  });
  const page = await browser.newPage({ viewport: { width: 1512, height: 1704 }, deviceScaleFactor: 1 });
  await page.goto("http://127.0.0.1:7686", { waitUntil: "networkidle", timeout: 60000 });
  await page.waitForTimeout(5000);
  await page.mouse.click(500, 500);
  await page.keyboard.press("/");
  await page.keyboard.type("Creative Commons image sampler for terminal previews", { delay: 5 });
  await page.waitForTimeout(500);
  await page.keyboard.press("Enter");
  await page.waitForTimeout(250);
  await page.keyboard.press("j");
  await page.waitForTimeout(250);
  await page.keyboard.press("Enter");
  await page.waitForTimeout(1200);
  await page.keyboard.press("z");
  await page.waitForTimeout(2000);
  await page.screenshot({ path: `${out}/browser_fixed_iterm2_raster.png`, fullPage: true });
  await browser.close();
})();
NODE
```

Expected: screenshot shows real raster images, not `open image` links, with the full-screen header still visible and images near authored positions.

- [ ] **Step 3: Capture tmux sizes**

Run:

```bash
tmux kill-session -t herald-iterm-fixed 2>/dev/null || true
tmux new-session -d -s herald-iterm-fixed -x 220 -y 50
tmux send-keys -t herald-iterm-fixed '/tmp/herald-iterm-fixed --demo' Enter
sleep 5
tmux send-keys -t herald-iterm-fixed '/'
sleep 0.2
tmux send-keys -t herald-iterm-fixed 'Creative Commons image sampler for terminal previews'
sleep 0.5
tmux send-keys -t herald-iterm-fixed Enter
sleep 0.2
tmux send-keys -t herald-iterm-fixed j
sleep 0.2
tmux send-keys -t herald-iterm-fixed Enter
sleep 1
tmux send-keys -t herald-iterm-fixed z
sleep 1
tmux capture-pane -t herald-iterm-fixed -p > reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_220x50.txt
tmux capture-pane -t herald-iterm-fixed -p -e > reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_220x50.ansi.txt
.agents/skills/tui-test/screenshot.sh herald-iterm-fixed reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_220x50.png 1800x1000
tmux resize-window -t herald-iterm-fixed -x 80 -y 24
sleep 0.5
tmux capture-pane -t herald-iterm-fixed -p > reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_80x24.txt
tmux capture-pane -t herald-iterm-fixed -p -e > reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_80x24.ansi.txt
.agents/skills/tui-test/screenshot.sh herald-iterm-fixed reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_80x24.png 1000x700
tmux resize-window -t herald-iterm-fixed -x 50 -y 15
sleep 0.5
tmux capture-pane -t herald-iterm-fixed -p > reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_50x15.txt
tmux capture-pane -t herald-iterm-fixed -p -e > reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_50x15.ansi.txt
.agents/skills/tui-test/screenshot.sh herald-iterm-fixed reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_50x15.png 800x500
```

Expected: `220x50` and `80x24` show readable full-screen content; `50x15` shows the minimum-size guard.

- [ ] **Step 4: Stop visual verification processes**

Run:

```bash
tmux kill-session -t herald-iterm-fixed 2>/dev/null || true
lsof -ti tcp:7686 | xargs -r kill
```

Expected: no verification tmux session or ttyd server remains.

## Task 8: Final Report And Handoff

**Files:**
- Create: `reports/TEST_REPORT_2026-04-29_iterm2-raster-inline-image-placement.md`

- [ ] **Step 1: Write the final report**

Create `reports/TEST_REPORT_2026-04-29_iterm2-raster-inline-image-placement.md` with this structure:

```markdown
# TUI Test Report — 2026-04-29 iTerm2 Raster Inline Image Placement

## Session
- Mode: demo
- Branch:
- Binary: `/tmp/herald-iterm-fixed`
- Protocols: iTerm2 OSC 1337, Kitty/Ghostty regression, local fallback links
- Harness: `reports/ttyd-image-harness/index.html`
- Sizes: `220x50`, `80x24`, `50x15`

## Red Reproduction
- Browser raster: `reports/iterm2-raster-inline-image-placement_2026-04-29/browser_red_iterm2_raster.png`
- tmux fallback baseline: `reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_red_220x50.png`
- Observation: current iTerm2 raster path displaced the full-screen header/body context.

## Fix Summary
- Added iTerm2 safe cell sizing.
- Added terminal-consumed row accounting for OSC 1337 image blocks.
- Preserved Kitty/Ghostty placement-clearing behavior.

## Verification
- `go test ./internal/app ./internal/iterm2 ./internal/kittyimg -run 'PreviewImage|PreviewDocument|TimelineFullScreen|Iterm|Kitty|CreativeCommons' -count=1`
- `go test ./...`
- Custom ttyd+xterm image-addon fixed screenshot: `reports/iterm2-raster-inline-image-placement_2026-04-29/browser_fixed_iterm2_raster.png`
- tmux `220x50`: `reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_220x50.png`
- tmux `80x24`: `reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_80x24.png`
- tmux `50x15`: `reports/iterm2-raster-inline-image-placement_2026-04-29/tmux_fixed_50x15.png`

## Findings
No open blocking findings if the browser fixed screenshot shows real raster images and visible preview header.

## Residual Risk
iTerm2 remains less robust than Kitty/Ghostty. The accepted behavior is bounded, approximate raster placement rather than pixel-perfect HTML email rendering.
```

- [ ] **Step 2: Check status and diff**

Run:

```bash
git status --short
git diff --check
```

Expected: no whitespace errors; only intended source/test changes are tracked. Report files under `reports/` remain untracked/ignored.

- [ ] **Step 3: Final commit if needed**

Run:

```bash
git status --short
```

Expected: no uncommitted tracked changes. If tracked changes remain, commit them:

```bash
git add internal/app internal/iterm2 internal/kittyimg
git commit -m "fix: stabilize iterm2 raster image placement"
```

Expected: final code commit succeeds.

- [ ] **Step 4: Handoff**

Final response must include:

```markdown
Implemented iTerm2 raster placement stabilization.

Fixed UI screenshot:
![Fixed iTerm2 raster UI](/Users/zoomacode/Developer/mail-processor/.worktrees/20260429-iterm2-raster-inline-image-placement/reports/iterm2-raster-inline-image-placement_2026-04-29/browser_fixed_iterm2_raster.png)

Report: [TEST_REPORT_2026-04-29_iterm2-raster-inline-image-placement.md](/Users/zoomacode/Developer/mail-processor/.worktrees/20260429-iterm2-raster-inline-image-placement/reports/TEST_REPORT_2026-04-29_iterm2-raster-inline-image-placement.md)

Tests:
- `go test ./internal/app ./internal/iterm2 ./internal/kittyimg -run 'PreviewImage|PreviewDocument|TimelineFullScreen|Iterm|Kitty|CreativeCommons' -count=1`
- `go test ./...`
```

Expected: the user sees the fixed raster screenshot path and the test report path.
