#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
source scripts/manifest-common.sh
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
manifest_files > "$tmp/files"
: > "$tmp/manifest"
while IFS= read -r path; do
  digest="$(hash_file "$path")"
  printf '%s  ./%s\n' "$digest" "$path" >> "$tmp/manifest"
done < "$tmp/files"
cat "$tmp/manifest" > MANIFEST.sha256
