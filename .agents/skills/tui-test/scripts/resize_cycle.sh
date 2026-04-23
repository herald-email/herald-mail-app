#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 3 ] || [ "$#" -gt 5 ]; then
  echo "Usage: $0 <tmux-session> <output-dir> <label> [sequence] [sleep-seconds]" >&2
  echo 'Example: resize_cycle.sh test reports/evidence timeline-preview "220x50,120x40,80x24,50x15,80x24,120x40,220x50" 0.8' >&2
  exit 1
fi

SESSION="$1"
OUTPUT_DIR="$2"
LABEL="$3"
SEQUENCE="${4:-220x50,120x40,80x24,50x15,80x24,120x40,220x50}"
WAIT_SECS="${5:-0.8}"

mkdir -p "$OUTPUT_DIR"

IFS=',' read -r -a STEPS <<< "$SEQUENCE"

index=1
for step in "${STEPS[@]}"; do
  cols="${step%x*}"
  rows="${step#*x}"

  if [ -z "$cols" ] || [ -z "$rows" ] || [ "$cols" = "$step" ]; then
    echo "Invalid size step: $step" >&2
    exit 1
  fi

  tmux resize-window -t "$SESSION" -x "$cols" -y "$rows"
  sleep "$WAIT_SECS"

  out_file=$(printf "%s/%s_%02d_%s.txt" "$OUTPUT_DIR" "$LABEL" "$index" "$step")
  tmux capture-pane -t "$SESSION" -p > "$out_file"
  printf '%s\n' "$out_file"

  index=$((index + 1))
done
