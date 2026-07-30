#!/usr/bin/env bash
# Prove pathcorner entity names cannot map to combat-log creep NPC names.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
REPLAY="${REPLAY:-$ROOT/dota-replays/8915936762.dem}"
cd "$ROOT/manta-labs/lasthits-debug"
exec env GOWORK=off go run . -replay "$REPLAY" -mode proof-pathcorner -from 160 -to 170 "$@"
