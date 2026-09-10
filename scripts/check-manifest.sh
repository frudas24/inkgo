#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

[[ -f MANIFEST.sha256 ]] || { echo 'MANIFEST.sha256 missing' >&2; exit 1; }

if grep -Eq '(^|/)(coverage\.out|[^/]+\.prof|[^/]+\.test)([[:space:]]|$)' MANIFEST.sha256; then
  echo 'manifest contains generated/test artifacts' >&2
  exit 1
fi

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    printf 'no SHA-256 utility found (need sha256sum or shasum)\n' >&2
    exit 1
  fi
}

while read -r expected path; do
  [[ -n "${expected:-}" && -n "${path:-}" ]] || continue
  path="${path#./}"
  if [[ ! -f "$path" ]]; then
    printf 'manifest entry missing from checkout: %s\n' "$path" >&2
    exit 1
  fi
  actual="$(hash_file "$path")"
  if [[ "$actual" != "$expected" ]]; then
    printf 'manifest hash mismatch: %s\nexpected %s\nactual   %s\n' "$path" "$expected" "$actual" >&2
    exit 1
  fi
done < MANIFEST.sha256

echo 'manifest OK'
