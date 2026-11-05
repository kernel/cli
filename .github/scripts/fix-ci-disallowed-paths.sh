#!/usr/bin/env bash
# Reads NUL-delimited paths on stdin and prints, NUL-delimited, the ones the
# CI fixer must not publish.
set -euo pipefail

while IFS= read -r -d '' path; do
  case "$path" in
    .* | */.*) ;;
    go.mod | go.sum | cmd/* | pkg/*) continue ;;
  esac
  printf '%s\0' "$path"
done
