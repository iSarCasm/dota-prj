#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
LAB="$ROOT/lasthits-debug"
TABLE="${TABLE:-$LAB/examples/pathcorner-lane-table.json}"
OUT="${OUT:-$LAB/examples/pathcorner-lane-map.svg}"
cd "$ROOT/proofs/pathcorner-lane-spawn"
exec python3 visualize.py "$TABLE" -o "$OUT" "$@"
