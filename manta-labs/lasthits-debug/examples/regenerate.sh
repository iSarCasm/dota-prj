#!/usr/bin/env bash
# Regenerate example snippets from replay 8915936762 (Warlock missed deny case).
set -euo pipefail
cd "$(dirname "$0")/.."
REPLAY="${REPLAY:-../../dota-replays/8915936762.dem}"
if [[ ! -f "$REPLAY" ]]; then
  echo "Replay not found: $REPLAY" >&2
  echo "Set REPLAY= path/to/file.dem" >&2
  exit 1
fi
go run . -replay "$REPLAY" -mode warlock-badguys -from 163 -to 170 \
  | grep -E 'flagbearer|badguys_melee' > examples/warlock-flagbearer-window.txt || true
go run . -replay "$REPLAY" -mode entity-names -from 164.1 -to 164.3 -health 137 \
  > examples/entity-names-flagbearer.txt
go run . -replay "$REPLAY" -mode health-match -from 164 -to 166 -health 137 \
  > examples/health-match-137.txt
go run . -replay "$REPLAY" -mode build-pathcorner-map \
  > examples/pathcorner-map.txt
go run . -replay "$REPLAY" -mode build-pathcorner-map -format json \
  > examples/pathcorner-map.json
PA_REPLAY="${PA_REPLAY:-../../dota-replays/8934466456.dem}"
WARLOCK_REPLAY="${REPLAY:-../../dota-replays/8915936762.dem}"
MERGED_REPLAYS="${MERGED_REPLAYS:-$WARLOCK_REPLAY,$PA_REPLAY}"
go run . -replays "$MERGED_REPLAYS" -mode build-pathcorner-lane-spawn -format table \
  > examples/pathcorner-lane-table.txt
go run . -replays "$MERGED_REPLAYS" -mode build-pathcorner-lane-spawn -format markdown \
  > examples/pathcorner-lane-table.md
go run . -replays "$MERGED_REPLAYS" -mode build-pathcorner-lane-spawn -format tsv \
  > examples/pathcorner-lane-table.tsv
go run . -replays "$MERGED_REPLAYS" -mode build-pathcorner-lane-spawn -format json \
  > examples/pathcorner-lane-table.json
bash ../proofs/pathcorner-lane-spawn/compare-replays.sh \
  > examples/pathcorner-lane-consistency.txt 2>&1 || true
bash ../proofs/pathcorner-lane-spawn/visualize.sh
echo "Done. Review examples/*.{txt,json,svg}"
