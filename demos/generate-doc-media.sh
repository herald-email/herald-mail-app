#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

for tool in vhs ffmpeg; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "missing required tool: $tool" >&2
		exit 1
	fi
done

if [[ ! -x ./bin/herald ]]; then
	echo "missing ./bin/herald; run make build first" >&2
	exit 1
fi

if [[ ! -x ./bin/herald-mcp-server ]]; then
	echo "missing ./bin/herald-mcp-server; run make build-mcp first" >&2
	exit 1
fi

TMP_DIR="${HERALD_DOC_MEDIA_TMP:-tmp/docs-media}"
SCREENSHOT_DIR="docs/public/screenshots"
DOC_DEMO_DIR="docs/public/demo"
ATTACHMENT_FIXTURE="$TMP_DIR/demo-attachment.txt"
ONLY_LIST=",${HERALD_DOC_MEDIA_ONLY:-},"
INCLUDE_RASTER="${HERALD_DOC_MEDIA_INCLUDE_RASTER:-0}"

rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR" "$SCREENSHOT_DIR" "$DOC_DEMO_DIR" assets/demo
printf 'Herald demo attachment for documentation screenshots.\n' >"$ATTACHMENT_FIXTURE"

should_capture() {
	local id="$1"
	[[ "$ONLY_LIST" == ",," || "$ONLY_LIST" == *",$id,"* ]]
}

run_demo_gifs() {
	local tape
	for tape in demos/*.tape; do
		if [[ "$(basename "$tape")" == "image-preview.tape" && "$INCLUDE_RASTER" != "1" ]]; then
			echo "==> skip $tape (set HERALD_DOC_MEDIA_INCLUDE_RASTER=1 after confirming raster capture support)"
			continue
		fi
		echo "==> vhs $tape"
		vhs "$tape"
	done

	cp assets/demo/*.gif "$DOC_DEMO_DIR"/
}

write_tape_header() {
	local out="$1"
	local width="$2"
	local height="$3"

	{
		printf 'Output %s\n\n' "$out"
		printf 'Set Theme "Builtin Solarized Dark"\n'
		printf 'Set FontSize 13\n'
		printf 'Set Width %s\n' "$width"
		printf 'Set Height %s\n' "$height"
		printf 'Set Padding 8\n\n'
	} >"$CURRENT_TAPE"
}

extract_final_frame() {
	local gif="$1"
	local png="$2"
	local frames_dir="$TMP_DIR/frames-$(basename "$png" .png)"
	local last_frame

	if ffmpeg -y -loglevel error -sseof -0.1 -i "$gif" -frames:v 1 "$png"; then
		if [[ -s "$png" ]]; then
			return 0
		fi
	fi

	rm -rf "$frames_dir"
	mkdir -p "$frames_dir"
	ffmpeg -y -loglevel error -i "$gif" "$frames_dir/frame-%05d.png"
	last_frame="$(find "$frames_dir" -name 'frame-*.png' -print | sort | tail -n 1)"
	if [[ -z "$last_frame" ]]; then
		echo "failed to extract a frame from $gif" >&2
		exit 1
	fi
	cp "$last_frame" "$png"
	rm -rf "$frames_dir"
}

capture_tui() {
	local id="$1"
	local actions="${2:-}"
	local width="${3:-1400}"
	local height="${4:-700}"
	local start_cmd="${5:-./bin/herald --demo}"
	local gif="$TMP_DIR/$id.gif"
	local png="$SCREENSHOT_DIR/$id.png"
	CURRENT_TAPE="$TMP_DIR/$id.tape"

	if ! should_capture "$id"; then
		return 0
	fi

	echo "==> screenshot $id"
	write_tape_header "$gif" "$width" "$height"
	{
		printf 'Type "%s"\n' "$start_cmd"
		printf 'Enter\n'
		printf 'Sleep 3s\n\n'
		if [[ -n "$actions" ]]; then
			printf '%s\n\n' "$actions"
		fi
		printf 'Sleep 1s\n'
	} >>"$CURRENT_TAPE"

	vhs "$CURRENT_TAPE"
	extract_final_frame "$gif" "$png"
}

capture_shell() {
	local id="$1"
	local actions="$2"
	local width="${3:-1400}"
	local height="${4:-700}"
	local gif="$TMP_DIR/$id.gif"
	local png="$SCREENSHOT_DIR/$id.png"
	CURRENT_TAPE="$TMP_DIR/$id.tape"

	if ! should_capture "$id"; then
		return 0
	fi

	echo "==> screenshot $id"
	write_tape_header "$gif" "$width" "$height"
	{
		printf 'Set Shell "bash"\n\n'
		printf '%s\n\n' "$actions"
		printf 'Sleep 1s\n'
	} >>"$CURRENT_TAPE"

	vhs "$CURRENT_TAPE"
	extract_final_frame "$gif" "$png"
}

copy_shot() {
	local src="$1"
	local dest="$2"
	if ! should_capture "$dest"; then
		return 0
	fi
	echo "==> screenshot $dest (alias of $src)"
	cp "$SCREENSHOT_DIR/$src.png" "$SCREENSHOT_DIR/$dest.png"
}

if [[ "$ONLY_LIST" == ",," || "$ONLY_LIST" == *",demo-gifs,"* ]]; then
	run_demo_gifs
fi

capture_tui "wizard-account-type" "" 1400 700 "rm -f /tmp/herald-docs-wizard.yaml && ./bin/herald -config /tmp/herald-docs-wizard.yaml"
copy_shot "wizard-account-type" "overview-first-launch"

capture_tui "timeline-main-list" $'Type "1"\nSleep 0.8s'
copy_shot "timeline-main-list" "demo-mode-timeline"
copy_shot "timeline-main-list" "getting-started-main-tui"
copy_shot "timeline-main-list" "global-main-layout"
copy_shot "timeline-main-list" "sync-status-main-bar"
copy_shot "timeline-main-list" "ai-status-chip"
copy_shot "timeline-main-list" "attachments-timeline-list"

capture_tui "sync-loading-view" "" 1400 700
capture_tui "sync-top-strip" $'Type "1"\nSleep 0.5s\nType "r"\nSleep 0.4s'
capture_tui "global-chat-open" $'Type "1"\nSleep 0.5s\nType "c"\nSleep 0.6s'
copy_shot "global-chat-open" "chat-panel-open"
capture_tui "global-logs-overlay" $'Type "l"\nSleep 0.6s'
capture_tui "global-narrow-terminal" "" 520 300

capture_tui "timeline-split-preview" $'Type "1"\nSleep 0.5s\nEnter\nSleep 1.2s'
capture_tui "mouse-navigation-links" $'Type "1"\nSleep 0.5s\nType "/"\nSleep 0.2s\nType "Link rendering stress"\nSleep 0.2s\nEnter\nSleep 0.8s\nEnter\nSleep 1s\nTab\nSleep 0.3s\nDown\nSleep 0.2s\nDown\nSleep 0.2s\nDown\nSleep 0.2s'
if should_capture "mouse-navigation-links"; then
	cp "$SCREENSHOT_DIR/mouse-navigation-links.png" assets/demo/mouse-navigation-links.png
fi
capture_tui "timeline-search-results" $'Type "1"\nSleep 0.5s\nType "/"\nSleep 0.2s\nType "newsletter"\nSleep 0.2s\nEnter\nSleep 0.8s'
capture_tui "timeline-quick-reply-picker" $'Type "1"\nSleep 0.5s\nEnter\nSleep 1s\nCtrl+Q\nSleep 1.2s'
capture_tui "timeline-full-screen-reader" $'Type "1"\nSleep 0.5s\nEnter\nSleep 1s\nType "z"\nSleep 0.6s'
if [[ "$INCLUDE_RASTER" == "1" ]]; then
	capture_tui "timeline-inline-image-preview" $'Type "1"\nSleep 0.5s\nType "/"\nSleep 0.2s\nType "Creative Commons image sampler"\nSleep 0.2s\nEnter\nSleep 0.8s\nEnter\nSleep 1s\nType "z"\nSleep 1.2s\nDown 14\nSleep 2s' 1400 700 "./bin/herald --demo -image-protocol=kitty"
fi

capture_tui "search-timeline-input" $'Type "1"\nSleep 0.5s\nType "/"\nSleep 0.2s\nType "invoice"\nSleep 0.6s'
capture_tui "search-timeline-results" $'Type "1"\nSleep 0.5s\nType "/"\nSleep 0.2s\nType "invoice"\nSleep 0.2s\nEnter\nSleep 0.8s'
capture_tui "search-body-query" $'Type "1"\nSleep 0.5s\nType "/"\nSleep 0.2s\nType "/b invoice"\nSleep 0.6s'
capture_tui "search-contacts-semantic" $'Type "4"\nSleep 0.6s\nType "?"\nSleep 0.2s\nType "investors"\nSleep 0.6s'

capture_tui "text-selection-visual-mode" $'Type "1"\nSleep 0.5s\nEnter\nSleep 1s\nType "v"\nSleep 0.2s\nDown\nSleep 0.5s'
capture_tui "text-selection-full-screen" $'Type "1"\nSleep 0.5s\nEnter\nSleep 1s\nType "z"\nSleep 0.4s\nType "v"\nSleep 0.2s\nDown 2\nSleep 0.5s'
capture_tui "text-selection-mouse-mode" $'Type "1"\nSleep 0.5s\nEnter\nSleep 1s\nType "m"\nSleep 0.5s'

capture_tui "attachments-save-prompt" $'Type "1"\nSleep 0.5s\nEnter\nSleep 1s\nType "s"\nSleep 0.6s'

capture_tui "cleanup-main-summary" $'Type "3"\nSleep 0.8s'
capture_tui "cleanup-domain-mode" $'Type "3"\nSleep 0.8s\nType "d"\nSleep 0.6s'
capture_tui "cleanup-preview" $'Type "3"\nSleep 0.8s\nEnter\nSleep 0.5s\nTab\nSleep 0.2s\nEnter\nSleep 1s'
capture_tui "cleanup-delete-confirmation" $'Type "3"\nSleep 0.8s\nSpace\nSleep 0.2s\nType "D"\nSleep 0.6s'
copy_shot "cleanup-delete-confirmation" "destructive-delete-confirm"
capture_tui "destructive-archive-confirm" $'Type "3"\nSleep 0.8s\nSpace\nSleep 0.2s\nType "e"\nSleep 0.6s'
capture_tui "destructive-unsubscribe-confirm" $'Type "1"\nSleep 0.5s\nDown 3\nSleep 0.2s\nEnter\nSleep 1s\nType "u"\nSleep 0.6s'
capture_tui "destructive-progress" $'Type "3"\nSleep 0.8s\nSpace\nSleep 0.2s\nType "D"\nSleep 0.2s\nType "y"\nSleep 0.5s'
capture_tui "cleanup-rule-editor" $'Type "3"\nSleep 0.8s\nType "W"\nSleep 0.8s'
copy_shot "cleanup-rule-editor" "automation-rule-editor"
capture_tui "cleanup-manager" $'Type "3"\nSleep 0.8s\nType "C"\nSleep 0.8s'
copy_shot "cleanup-manager" "automation-cleanup-manager-list"
capture_tui "automation-cleanup-manager-edit" $'Type "3"\nSleep 0.8s\nType "C"\nSleep 0.4s\nType "n"\nSleep 0.8s'

capture_tui "automation-prompt-editor" $'Type "P"\nSleep 0.8s'
copy_shot "automation-prompt-editor" "ai-prompt-editor"

capture_tui "ai-classification-progress" $'Type "1"\nSleep 0.5s\nType "a"\nSleep 0.5s'

capture_tui "chat-waiting-response" $'Type "1"\nSleep 0.5s\nType "c"\nSleep 0.4s\nType "summarize recent unread"\nEnter\nSleep 0.5s'
capture_tui "chat-filtered-timeline" $'Type "1"\nSleep 0.5s\nType "c"\nSleep 0.4s\nType "find budget risk emails"\nEnter\nSleep 1s'

capture_tui "settings-main-panel" $'Type "S"\nSleep 0.8s'
capture_tui "settings-ai-provider" $'Type "S"\nSleep 0.8s\nTab 8\nSleep 0.5s'

capture_tui "compose-main-fields" $'Type "2"\nSleep 0.8s'
capture_tui "compose-autocomplete" $'Type "2"\nSleep 0.8s\nType "ma"\nSleep 0.8s'
capture_tui "compose-attachment-input" $'Type "2"\nSleep 0.8s\nCtrl+A\nSleep 0.6s'
capture_tui "attachments-compose-added" $'Type "2"\nSleep 0.8s\nCtrl+A\nSleep 0.2s\nType "'"$ATTACHMENT_FIXTURE"$'"\nEnter\nSleep 0.8s'
capture_tui "compose-markdown-preview" $'Type "2"\nSleep 0.8s\nTab 4\nType "# Follow-up notes"\nEnter\nEnter\nType "- Review the invoice risk."\nEnter\nType "- Reply before Friday."\nSleep 0.2s\nCtrl+P\nSleep 0.8s'
capture_tui "compose-ai-assistant" $'Type "2"\nSleep 0.8s\nTab 4\nType "Please make this reply more concise and helpful."\nSleep 0.2s\nCtrl+G\nSleep 1s'
copy_shot "compose-ai-assistant" "ai-compose-assist"

capture_tui "contacts-main-list" $'Type "4"\nSleep 0.8s'
capture_tui "contacts-detail" $'Type "4"\nSleep 0.8s\nEnter\nSleep 0.8s'
capture_tui "contacts-keyword-search" $'Type "4"\nSleep 0.8s\nType "/"\nSleep 0.2s\nType "demo"\nSleep 0.6s'
capture_tui "contacts-inline-preview" $'Type "4"\nSleep 0.8s\nEnter\nSleep 0.5s\nTab\nSleep 0.2s\nEnter\nSleep 0.8s'

capture_shell "mcp-tools-list-terminal" $'Type "sh demos/mcp-demo.sh tools"\nEnter\nSleep 1.5s'
capture_shell "demo-gif-vhs-run" $'Type "make docs-media"\nSleep 0.8s'

echo "Generated docs media in $SCREENSHOT_DIR and $DOC_DEMO_DIR"
