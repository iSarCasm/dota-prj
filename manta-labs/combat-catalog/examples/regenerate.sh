#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
REPLAY="${REPLAY:-../../dota-replays/8915936762.dem}"
if [[ ! -f "$REPLAY" ]]; then
  echo "Replay not found: $REPLAY" >&2
  exit 1
fi
rm -rf output
go run . -out output -replay "$REPLAY"
mkdir -p examples
cp output/summary.txt examples/summary.txt
head -n 40 output/heroes.txt > examples/heroes.head.txt
head -n 40 output/items.txt > examples/items.head.txt
head -n 40 output/spells.txt > examples/spells.head.txt
echo "Done. Review output/ and examples/"
