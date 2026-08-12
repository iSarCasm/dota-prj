#!/usr/bin/env bash
# Build points JSON (if needed) and write one SVG per pathcorner (spawn dots).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
LAB="$ROOT/lasthits-debug"
POINTS="${POINTS:-$LAB/examples/pathcorner-lane-points.json}"
OUT_DIR="${OUT_DIR:-$LAB/examples/pathcorner-lane-dots}"

if [[ ! -f "$POINTS" ]]; then
  REPLAYS="${REPLAYS:-$("$ROOT/proofs/pathcorner-lane-spawn/list-replays.sh")}"
  echo "building points JSON from $(echo "$REPLAYS" | tr ',' '\n' | wc -l) replays..." >&2
  cd "$LAB"
  env GOWORK=off go run . -replays "$REPLAYS" -mode build-pathcorner-lane-spawn -format points \
    > "$POINTS"
fi

cd "$ROOT/proofs/pathcorner-lane-spawn"
exec python3 visualize_dots.py "$POINTS" -o-dir "$OUT_DIR" "$@"
