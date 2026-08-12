# Spawn location → lane clusters

## Purpose

Lane creep EntityNames are pathcorners; lane cannot be read from the string.
Each map **side** (good = SW / bottom-left, bad = NE / top-right) has **three
discrete spawn slots** (top / mid / bot). This proof computes their mean
positions from merged replay spawn points.

## Findings

From 7 catalog replays (`pathcorner-lane-points.json`, 8699 spawns):

| side | lane | mean_x | mean_y | spawns | max dist to centroid |
|------|------|--------|--------|--------|----------------------|
| good | top  | -6720.7 | -4100.7 | ~1421 | ~316 |
| good | mid  | -5121.4 | -4609.1 | ~1453 | ~360 |
| good | bot  | -3834.3 | -6217.5 | ~1481 | ~319 |
| bad  | top  |  3070.9 |  5634.1 | ~1445 | ~257 |
| bad  | mid  |  4001.4 |  3495.1 | ~1448 | ~232 |
| bad  | bot  |  6143.8 |  3567.4 | ~1451 | ~256 |

Clusters are well separated (min inter-centroid ~1678 ≫ max intra ~360).

**Lane assignment:** nearest of the three centroids for the spawn’s map side
(`creeps.GetCreepLaneFromSpawnLocation`).

**Side from position:** `x>0 && y>0` → bad, else good (same geography as
`GetCreepSide` pathcorner table).

## Reproduce

Requires points JSON (from pathcorner-lane-spawn):

```bash
# if missing:
cd manta-labs/lasthits-debug
MERGED_REPLAYS="$(bash ../proofs/pathcorner-lane-spawn/list-replays.sh ../../dota-replays)"
go run . -replays "$MERGED_REPLAYS" -mode build-pathcorner-lane-spawn -format points \
  > examples/pathcorner-lane-points.json

./manta-labs/proofs/spawn-lane-clusters/run.sh
```

Expected output:
- `manta-labs/lasthits-debug/examples/spawn-lane-centroids/centroids.tsv`
- `centroids.go.snippet` for pasting into `parser/internal/creeps`
- `centroids-map.svg` — all spawn dots + 6 diamonds (`good|bad` × `top|mid|bot`)

Centroids are also drawn on:
- `examples/pathcorner-lane-map.svg` (`visualize.sh`)
- `examples/pathcorner-lane-dots/*.svg` (`visualize-dots.sh`)
