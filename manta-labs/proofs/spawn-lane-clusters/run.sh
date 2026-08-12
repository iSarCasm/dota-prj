#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HERE="$(cd "$(dirname "$0")" && pwd)"
POINTS="${POINTS:-$ROOT/lasthits-debug/examples/pathcorner-lane-points.json}"
OUT="${OUT:-$ROOT/lasthits-debug/examples/spawn-lane-centroids}"
mkdir -p "$OUT"
cd "$HERE"
python3 compute_centroids.py "$POINTS" -o "$OUT"
python3 visualize.py "$POINTS" -o "$OUT/centroids-map.svg"
echo "centroids SVG: $OUT/centroids-map.svg"
