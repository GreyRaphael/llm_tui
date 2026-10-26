#!/usr/bin/env bash
# Enforce per-package statement coverage minimums.
#
# Usage: scripts/check_coverage.sh [report]
#   report  file containing the stdout of `go test -cover ./...`.
#           Without an argument, piped or redirected stdin is used as the
#           report; on a terminal (or when stdin is empty, e.g. /dev/null) the
#           suite is run from the repository root instead.
#
# Packages without test files are skipped. A package with tests but no
# configured minimum fails, so a new package cannot silently ship untested.
set -uo pipefail

# Minimum statement coverage per package. Keep in sync with the test suite when
# adding a package or a large untested surface.
MINIMUMS="
llm_tui 75
llm_tui/internal/api 80
llm_tui/internal/clipboard 85
llm_tui/internal/db 65
llm_tui/internal/tui 50
llm_tui/internal/tui/views 70
"

report="$(mktemp)"
trap 'rm -f "$report"' EXIT

if [[ $# -ge 1 && -n "${1:-}" ]]; then
	cat -- "$1" >"$report"
elif [[ -p /dev/stdin || -f /dev/stdin ]]; then
	cat >"$report"
else
	repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
	if ! (cd -- "$repo_root" && go test -cover ./... >"$report"); then
		cat "$report" >&2
		exit 1
	fi
fi

status=0
found=0
while IFS= read -r line; do
	# Expected shape: "ok  <pkg>  <duration>  coverage: 81.0% of statements"
	[[ "$line" == *"coverage:"* ]] || continue
	pkg="$(awk '{print $2}' <<<"$line")"
	pct="$(sed -n 's/.*coverage: \([0-9.]*\)%.*/\1/p' <<<"$line")"
	[[ -n "$pkg" && -n "$pct" ]] || continue
	found=1

	min="$(awk -v p="$pkg" '$1 == p { print $2 }' <<<"$MINIMUMS")"
	if [[ -z "$min" ]]; then
		printf 'FAIL %-40s coverage %5s%% (no minimum configured)\n' "$pkg" "$pct"
		status=1
	elif awk -v pct="$pct" -v min="$min" 'BEGIN { exit !(pct + 0 < min + 0) }'; then
		printf 'FAIL %-40s coverage %5s%% <  %s%%\n' "$pkg" "$pct" "$min"
		status=1
	else
		printf 'OK   %-40s coverage %5s%% >= %s%%\n' "$pkg" "$pct" "$min"
	fi
done <"$report"

if [[ "$found" -eq 0 ]]; then
	echo "No coverage lines found in the report." >&2
	exit 1
fi

if [[ "$status" -ne 0 ]]; then
	echo "Coverage thresholds not met." >&2
fi
exit "$status"
