#!/usr/bin/env bash
set -euo pipefail

file_version="$(tr -d '[:space:]' < VERSION)"
source_version="$(sed -n 's/^const Version = "\([^"]*\)"/\1/p' version.go)"
if [[ -z "$file_version" || "$file_version" != "$source_version" ]]; then
  printf 'version mismatch: VERSION=%q source=%q\n' "$file_version" "$source_version" >&2
  exit 1
fi

# Every document that states the latest published version has to state the same one. A
# release commit that updates only some of them ships contradictory instructions - v0.1.33
# bumped PORT_STATUS.md:7 and left :62 naming v0.1.32 - and a consumer cannot tell which
# tag is the release they should pin. VERSION is deliberately not compared here: a
# development tree may legitimately sit ahead of the last published tag.
published=''
published_source=''

note_published() {
  local file="$1" version="$2"
  if [[ -z "$published" ]]; then
    published="$version"
    published_source="$file"
    return
  fi
  if [[ "$published" != "$version" ]]; then
    printf 'latest published version disagrees: %s says %s, %s says %s\n' \
      "$published_source" "$published" "$file" "$version" >&2
    exit 1
  fi
}

# require_published_statements file sed-expression
#
# Fails when the statement it pins is gone, so a reworded document cannot make this check
# pass by matching nothing.
require_published_statements() {
  local file="$1" expr="$2" versions
  versions="$(sed -nE "$expr" "$file")"
  if [[ -z "$versions" ]]; then
    printf 'no "latest published version" statement matched in %s\n' "$file" >&2
    exit 1
  fi
  while IFS= read -r version; do
    note_published "$file" "$version"
  done <<< "$versions"
}

require_published_statements README.md \
  's/.*The latest published release is `v([0-9]+\.[0-9]+\.[0-9]+)`.*/\1/p'
require_published_statements docs/PORT_STATUS.md \
  's/.*`v([0-9]+\.[0-9]+\.[0-9]+)` is the latest publicly tagged release.*/\1/p'
require_published_statements docs/PORT_STATUS.md \
  's/.*The latest published tag is `v([0-9]+\.[0-9]+\.[0-9]+)`.*/\1/p'
require_published_statements docs/RELEASE.md \
  's/.*The latest published release is \*\*v([0-9]+\.[0-9]+\.[0-9]+)\*\*.*/\1/p'
require_published_statements docs/STABILITY.md \
  's/.*The latest published tag is `v([0-9]+\.[0-9]+\.[0-9]+)`.*/\1/p'

printf 'version OK: %s (latest published: %s)\n' "$file_version" "$published"
