#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
source scripts/manifest-common.sh
[[ -s MANIFEST.sha256 ]] || { echo 'manifest missing or empty' >&2; exit 1; }
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
: > "$tmp/listed"
while read -r expected path || [[ -n "${expected:-}${path:-}" ]]; do
  if [[ ! "$expected" =~ ^[0-9a-f]{64}$ || "$path" != ./* ]]; then
    echo 'malformed manifest entry' >&2
    exit 1
  fi
  path="${path#./}"
  [[ -f "$path" ]] || { printf 'manifest entry missing from checkout: %s\n' "$path" >&2; exit 1; }
  actual="$(hash_file "$path")"
  [[ "$actual" == "$expected" ]] || { printf 'manifest hash mismatch: %s\n' "$path" >&2; exit 1; }
  printf '%s\n' "$path" >> "$tmp/listed"
done < MANIFEST.sha256
LC_ALL=C sort "$tmp/listed" > "$tmp/sorted"
manifest_files > "$tmp/expected"
if ! diff -u "$tmp/expected" "$tmp/sorted"; then
  echo 'manifest inventory mismatch (omitted, duplicate, or excluded files)' >&2
  exit 1
fi
echo 'manifest OK'
