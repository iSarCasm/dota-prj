#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
REPLAY="${REPLAY:-$HERE/../../../dota-replays/8915936762.dem}"
OUT="${OUT:-$HERE/../../lasthits-debug/examples/tick-ordering.txt}"
cd "$HERE"
env GOWORK=off go run . "$REPLAY" | tee "$OUT"
