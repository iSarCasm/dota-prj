#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REPLAY="${REPLAY:-$ROOT/../dota-replays/8915936762.dem}"
cd "$ROOT/lasthits-debug"
exec env GOWORK=off go run . -replay "$REPLAY" -mode build-pathcorner-map "$@"
