#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REPLAYS="${REPLAYS:-$ROOT/../dota-replays/8915936762.dem,$ROOT/../dota-replays/8934466456.dem}"
cd "$ROOT/lasthits-debug"
exec env GOWORK=off go run . -replays "$REPLAYS" -mode build-pathcorner-lane-spawn -format table "$@"
