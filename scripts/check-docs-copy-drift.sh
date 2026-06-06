#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/check-docs-copy-drift.sh [path ...]

Scans authored Herald docs for stale product-copy patterns. With no paths, the
checker scans the public docs source and README from the repository root.
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

repo_root="${HERALD_DOCS_COPY_DRIFT_ROOT:-}"
if [[ -z "$repo_root" ]]; then
  repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi

cd "$repo_root"

if [[ "$#" -gt 0 ]]; then
  scan_roots=("$@")
else
  scan_roots=("README.md" "docs/src/content/docs")
fi

fixed_patterns=(
  "Gmail OAuth is experimental"
  "run herald -experimental"
  "Cleanup tab"
  "F2 Cleanup"
  "switch to Cleanup"
)

regex_patterns=(
  '(^|[^[:alnum:]])`?2`?[[:space:]]*([:|=]|->|=>|-|\.|\))?[[:space:]]*Cleanup([^[:alnum:]]|$)'
  '(^|[^[:alnum:]])`?2`?[[:space:]]+(opens|selects|switches[[:space:]]+to|goes[[:space:]]+to)[[:space:]]+(the[[:space:]]+)?Cleanup([^[:alnum:]]|$)'
)

regex_labels=(
  'old tab order where 2 is Cleanup'
  'old tab shortcut copy where 2 opens Cleanup'
)

# Release archive pages can keep narrow historical wording when the allowlist
# names both the file and stale pattern.
allowlisted_matches=(
  "docs/src/content/docs/whats-new-in-v0-5.md|Cleanup tab"
)

normalize_path() {
  local path="$1"
  path="${path#./}"
  printf '%s\n' "$path"
}

is_allowlisted_match() {
  local file
  local pattern
  file="$(normalize_path "$1")"
  pattern="$2"
  local entry
  for entry in "${allowlisted_matches[@]}"; do
    if [[ "$entry" == "$file|$pattern" ]]; then
      return 0
    fi
  done
  return 1
}

collect_files() {
  local root
  for root in "${scan_roots[@]}"; do
    if [[ -d "$root" ]]; then
      find "$root" \
        -type f \
        \( -name '*.md' -o -name '*.mdx' -o -name '*.mdoc' \) \
        ! -path '*/.astro/*' \
        ! -path '*/dist/*' \
        ! -path '*/node_modules/*' \
        ! -path '*/public/*'
    elif [[ -f "$root" ]]; then
      printf '%s\n' "$root"
    else
      printf 'warning: docs copy drift path not found: %s\n' "$root" >&2
    fi
  done | sort -u
}

tmp_failures="$(mktemp)"
trap 'rm -f "$tmp_failures"' EXIT

files_scanned=0
while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  files_scanned=$((files_scanned + 1))

  for pattern in "${fixed_patterns[@]}"; do
    while IFS= read -r match; do
      [[ -z "$match" ]] && continue
      if is_allowlisted_match "$file" "$pattern"; then
        continue
      fi
      printf '%s:%s\n' "$(normalize_path "$file")" "$match" >>"$tmp_failures"
    done < <(grep -nF "$pattern" "$file" || true)
  done

  for i in "${!regex_patterns[@]}"; do
    pattern="${regex_patterns[$i]}"
    label="${regex_labels[$i]}"
    while IFS= read -r match; do
      [[ -z "$match" ]] && continue
      if is_allowlisted_match "$file" "$label"; then
        continue
      fi
      printf '%s:%s: %s\n' "$(normalize_path "$file")" "$match" "$label" >>"$tmp_failures"
    done < <(grep -nE "$pattern" "$file" || true)
  done
done < <(collect_files)

if [[ -s "$tmp_failures" ]]; then
  printf 'Docs copy drift detected. Update the stale copy or add a narrow release-archive allowlist entry.\n\n' >&2
  cat "$tmp_failures" >&2
  exit 1
fi

printf 'Docs copy drift check passed (%d files scanned).\n' "$files_scanned"
