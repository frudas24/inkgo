#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

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

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git ls-files | LC_ALL=C sort > "$tmp"
else
  find . -type f \
    ! -path './.git/*' \
    ! -name 'MANIFEST.sha256' \
    ! -name 'coverage.out' \
    ! -name '*.prof' \
    ! -name '*.test' \
    | sed 's#^./##' | LC_ALL=C sort > "$tmp"
fi

: > MANIFEST.sha256
while IFS= read -r path; do
  case "$path" in
    ''|MANIFEST.sha256|coverage.out|*.prof|*.test) continue ;;
  esac
  [[ -f "$path" ]] || continue
  printf '%s  ./%s\n' "$(hash_file "$path")" "$path" >> MANIFEST.sha256
done < "$tmp"
