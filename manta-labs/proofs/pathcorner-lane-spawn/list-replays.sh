#!/usr/bin/env bash
# Comma-separated paths to all .dem files in dota-replays/ (sorted).
set -euo pipefail
DEFAULT_DIR="$(cd "$(dirname "$0")/../../.." && pwd)/dota-replays"
DIR="${1:-$DEFAULT_DIR}"
shopt -s nullglob
files=("$DIR"/*.dem)
if ((${#files[@]} == 0)); then
  echo "no .dem files in $DIR" >&2
  exit 1
fi
IFS=,
echo "${files[*]}"
