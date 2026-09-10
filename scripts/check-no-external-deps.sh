#!/usr/bin/env bash
set -euo pipefail

module="$(go list -m)"
mapfile -t modules < <(go list -m all)
if [[ ${#modules[@]} -ne 1 || "${modules[0]}" != "$module" ]]; then
  printf 'external Go modules detected:\n' >&2
  printf '  %s\n' "${modules[@]}" >&2
  exit 1
fi
printf 'dependency policy OK: %s is stdlib-only\n' "$module"
