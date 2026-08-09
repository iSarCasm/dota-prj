# Pathcorner → real lane lookup

## Purpose

Lane creep `m_iUnitNameIndex` → `EntityNames` returns pathcorner strings (e.g. `lane_mid_pathcorner_badguys_7`), **not** combat-log creep names. This proof builds:

**pathcorner → mean spawn position (x, y) → real lane (top / mid / bot)**

## Findings

### 1. Pathcorner lane names are asymmetric by team

On lane-creep spawns, EntityNames prefixes differ by team:

| Team | Named prefixes (typical) | Missing lane |
|------|--------------------------|--------------|
| goodguys | `lane_bot_*`, `lane_mid_*` | top (no `lane_top_pathcorner_goodguys_*` on PA replay) |
| badguys | `lane_top_*`, `lane_mid_*` | bot (primary); rare `lane_bot_*` can appear |

The missing lane for each team is **folded into `lane_mid_pathcorner_*`**. Example: `lane_mid_pathcorner_badguys_7` has `mid` in the name but spawns at Dire base area → geographic **top** lane.

### 2. Geographic classification rules (empirical)

- **`lane_bot_*` / `lane_top_*`:** `real_lane` = name prefix (bot / top).
- **`lane_mid_*` at x>0, y>0:** center diagonal → **mid** (both teams).
- **`lane_mid_*` at base area (negative x/y):** infer from team — goodguys → nearer bot ref; badguys → nearer top ref.

### 3. `real_lane` is stable across replays

Compared replays `8915936762` and `8934466456`: **9 overlapping pathcorners, 0 `real_lane` mismatches.**

Key stable row: `lane_mid_pathcorner_badguys_7` → **top** in both games (pos delta ~52).

Run consistency check:

```bash
./manta-labs/proofs/pathcorner-lane-spawn/compare-replays.sh
```

Expected: `real_lane mismatches: 0`. See `examples/pathcorner-lane-consistency.txt`.

### 4. Spawn positions are only loosely stable

Mean start position varies by replay. High-traffic pathcorners stay close; rare ones swing:

| pathcorner | pos_delta | spawns (r1 / r2) |
|------------|-----------|------------------|
| `lane_mid_pathcorner_badguys_7` | ~52 | 263 / 329 |
| `lane_mid_pathcorner_goodguys_3` | ~58 | 301 / 329 |
| `lane_bot_pathcorner_goodguys_2` | ~54 | 83 / 112 |
| `lane_mid_pathcorner_badguys_4` | ~182 | 45 / 46 |
| `lane_bot_pathcorner_badguys_4` | ~1023 | 8 / 19 |
| `lane_top_pathcorner_badguys_2b` | ~1240 | 5 / 1 |

**Use `real_lane` as lookup; do not rely on exact start_x/y for geometry.**

### 5. Table coverage is replay-dependent

Merged table = union of pathcorners seen across replays. Some corners appear in only one game (e.g. `lane_bot_pathcorner_goodguys_3`, `lane_top_pathcorner_goodguys_2b` on Warlock replay only).

### 6. goodguys mid bucket

On tested replays, `lane_mid_pathcorner_goodguys_1` and `_3` both spawn at center diagonal → **mid**, not top-via-mid. Radiant top-lane overflow into mid bucket was **not observed** in these games (Radiant top sometimes uses rare `lane_top_pathcorner_goodguys_2b`).

## Reproduce lane table

Tab-separated table (merged from two replays):

```bash
./manta-labs/proofs/pathcorner-lane-spawn/run.sh
```

Or manually:

```bash
cd manta-labs/lasthits-debug
go run . -replays ../../dota-replays/8915936762.dem,../../dota-replays/8934466456.dem \
  -mode build-pathcorner-lane-spawn -format table
```

JSON lookup (`lookup[pathcorner].real_lane`):

```bash
go run . -replays ../../dota-replays/8915936762.dem,../../dota-replays/8934466456.dem \
  -mode build-pathcorner-lane-spawn -format json
```

## Expected output

- Lane table: `manta-labs/lasthits-debug/examples/pathcorner-lane-table.txt`
- Cross-replay consistency: `examples/pathcorner-lane-consistency.txt`

Example table rows:

```text
pathcorner	team	name_lane	start_x	start_y	real_lane	spawns
lane_bot_pathcorner_goodguys_2	goodguys	bot	-5209	-4934	bot	195
lane_mid_pathcorner_badguys_7	badguys	mid	-5224	-4987	top	592
lane_mid_pathcorner_badguys_4	badguys	mid	4329	4180	mid	91
```

## Regenerate examples

```bash
cd manta-labs/lasthits-debug && ./examples/regenerate.sh
./manta-labs/proofs/pathcorner-lane-spawn/compare-replays.sh \
  > manta-labs/lasthits-debug/examples/pathcorner-lane-consistency.txt
```
