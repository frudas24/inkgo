#!/usr/bin/env bash
set -euo pipefail

minimum="${1:-74.0}"
profile="${2:-coverage.out}"

go test ./... -coverprofile="$profile"
total="$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"

awk -v total="$total" -v minimum="$minimum" 'BEGIN {
  printf "total coverage: %.1f%% (minimum %.1f%%)\n", total, minimum
  if (total + 0 < minimum + 0) exit 1
}'
