#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REPLAYS="${REPLAYS:-$("$ROOT/proofs/pathcorner-lane-spawn/list-replays.sh")}"
cd "$ROOT/lasthits-debug"
exec env GOWORK=off go run . -replays "$REPLAYS" -mode build-pathcorner-lane-spawn -format table "$@"
