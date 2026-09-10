#!/usr/bin/env bash
set -euo pipefail

file_version="$(tr -d '[:space:]' < VERSION)"
source_version="$(sed -n 's/^const Version = "\([^"]*\)"/\1/p' version.go)"
if [[ -z "$file_version" || "$file_version" != "$source_version" ]]; then
  printf 'version mismatch: VERSION=%q source=%q\n' "$file_version" "$source_version" >&2
  exit 1
fi
printf 'version OK: %s\n' "$file_version"
