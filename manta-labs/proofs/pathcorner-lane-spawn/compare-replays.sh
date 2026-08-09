#!/usr/bin/env bash
# Compare pathcorner lane tables across replays.
# Proves: real_lane is stable; start positions vary more on rare pathcorners.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
LAB="$ROOT/lasthits-debug"
R1="${R1:-$ROOT/../dota-replays/8915936762.dem}"
R2="${R2:-$ROOT/../dota-replays/8934466456.dem}"

table() {
  local replay="$1"
  env GOWORK=off go run . -replay "$replay" -mode build-pathcorner-lane-spawn -format tsv 2>/dev/null \
    | awk -F'\t' 'NF >= 16 && $1 !~ /^#/ && $1 != "entity_name" { print }'
}

cd "$LAB"
T1="$(mktemp)"
T2="$(mktemp)"
trap 'rm -f "$T1" "$T2"' EXIT

table "$R1" | sort -t$'\t' -k1,1 > "$T1"
table "$R2" | sort -t$'\t' -k1,1 > "$T2"

echo "# pathcorner lane table cross-replay consistency"
echo "# replay_a: $R1"
echo "# replay_b: $R2"
echo "# columns: pathcorner | real_lane_a | real_lane_b | lane_match | pos_delta | spawns_a | spawns_b"
echo "pathcorner	real_lane_a	real_lane_b	lane_match	pos_delta	spawns_a	spawns_b"

python3 - "$T1" "$T2" << 'PY'
import math, sys

def load(path):
    rows = {}
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            parts = line.split("\t")
            if len(parts) < 16:
                continue
            name = parts[0]
            rows[name] = {
                "real_lane": parts[15],
                "x": float(parts[4]),
                "y": float(parts[5]),
                "spawns": int(parts[3]),
            }
    return rows

a = load(sys.argv[1])
b = load(sys.argv[2])
common = sorted(set(a) & set(b))
if not common:
    print("# no overlapping pathcorners", file=sys.stderr)
    sys.exit(1)

mismatch = 0
for name in common:
    ra, rb = a[name], b[name]
    match = ra["real_lane"] == rb["real_lane"]
    if not match:
        mismatch += 1
    delta = math.hypot(ra["x"] - rb["x"], ra["y"] - rb["y"])
    print(
        f"{name}\t{ra['real_lane']}\t{rb['real_lane']}\t{match}\t{delta:.0f}\t{ra['spawns']}\t{rb['spawns']}"
    )

print(f"# overlapping pathcorners: {len(common)}", file=sys.stderr)
print(f"# real_lane mismatches: {mismatch}", file=sys.stderr)
sys.exit(1 if mismatch else 0)
PY
