#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
REPLAY="${REPLAY:-../../dota-replays/8915936762.dem}"
if [[ ! -f "$REPLAY" ]]; then
  echo "Replay not found: $REPLAY" >&2
  exit 1
fi
rm -rf output
go run . -out output -replay "$REPLAY" \
  -max-per-type 15 -max-per-replay-type 15 \
  -max-per-class 3 -max-per-replay-class 3
cp output/combat_logs_summary.txt examples/combat_logs_summary.txt
cp output/entities_summary.txt examples/entities_summary.txt
head -5 output/combat_logs/damage.txt > examples/damage.head.txt
head -5 output/combat_logs/death.txt > examples/death.head.txt
head -40 output/entities/cdota_basenpc.txt > examples/entity.head.txt
echo "Done. Review output/ and examples/"
