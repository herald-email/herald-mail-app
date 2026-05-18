#!/usr/bin/env bash
set -euo pipefail

formula="herald-email/herald/herald"
category="install-on-request"
api_base="${HOMEBREW_ANALYTICS_API_BASE:-https://formulae.brew.sh/api/analytics}"
windows=("3d" "7d" "30d" "90d")

usage() {
  cat <<'EOF'
Usage: scripts/homebrew-install-stats.sh [options]

Print Homebrew Formulae install analytics for Herald.

Options:
  --formula FORMULA     Formula name to query (default: herald-email/herald/herald)
  --category CATEGORY   Analytics category: install or install-on-request
                        (default: install-on-request)
  --help                Show this help text

Notes:
  formulae.brew.sh currently publishes 30d, 90d, and 365d formula windows.
  This script still probes 3d and 7d so unsupported windows are visible.
EOF
}

while (($#)); do
  case "$1" in
    --formula)
      if [[ $# -lt 2 || -z "$2" ]]; then
        echo "error: --formula requires a value" >&2
        exit 2
      fi
      formula="$2"
      shift 2
      ;;
    --category)
      if [[ $# -lt 2 || -z "$2" ]]; then
        echo "error: --category requires a value" >&2
        exit 2
      fi
      category="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$category" in
  install|install-on-request)
    ;;
  *)
    echo "error: --category must be install or install-on-request" >&2
    exit 2
    ;;
esac

for command in curl jq; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "error: required command not found: $command" >&2
    exit 127
  fi
done

printf 'Formula: %s\n' "$formula"
printf 'Category: %s\n' "$category"
printf 'API: %s\n\n' "$api_base"
printf 'window\tstart_date\tend_date\tcount\tpercent\tstatus\n'

for window in "${windows[@]}"; do
  url="${api_base}/${category}/${window}.json"
  curl_error="$(mktemp)"
  if json="$(curl -fsSL "$url" 2>"$curl_error")"; then
    start_date="$(jq -r '.start_date // "-"' <<<"$json")"
    end_date="$(jq -r '.end_date // "-"' <<<"$json")"
    row="$(jq -r --arg formula "$formula" '
      (.items // [])
      | map(select(.formula == $formula))
      | first
      | if . == null then ["0", "0"] else [.count, .percent] end
      | @tsv
    ' <<<"$json")"
    count="${row%%$'\t'*}"
    percent="${row#*$'\t'}"
    printf '%s\t%s\t%s\t%s\t%s\tok\n' "$window" "$start_date" "$end_date" "$count" "$percent"
  else
    curl_status=$?
    status="unavailable"
    if [[ "$curl_status" -eq 22 ]] || grep -q 'The requested URL returned error:' "$curl_error"; then
      status="unsupported"
    fi
    printf '%s\t-\t-\t-\t-\t%s\n' "$window" "$status"
  fi
  rm -f "$curl_error"
done
