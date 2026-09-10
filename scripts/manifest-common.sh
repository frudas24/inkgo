#!/usr/bin/env bash
# Shared inventory policy for checkout and source-archive manifests.
manifest_files() {
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git ls-files --cached --others --exclude-standard
  else
    find . -type f ! -path './.git/*' | sed 's#^./##'
  fi | LC_ALL=C sort -u | while IFS= read -r path; do
    case "$path" in
      ''|MANIFEST.sha256|coverage.out|*.prof|*.test|.agent/*) continue ;;
    esac
    printf '%s\n' "$path"
  done
}

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    printf 'no SHA-256 utility found (need sha256sum or shasum)\n' >&2
    return 1
  fi
}
