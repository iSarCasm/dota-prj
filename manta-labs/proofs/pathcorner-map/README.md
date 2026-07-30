# Build pathcorner → combat-log creep name mapping

## Purpose

Entity `m_iUnitNameIndex` returns pathcorner strings; combat log uses `npc_dota_creep_*` names. This script **builds an empirical mapping** from one replay by health-correlating combat-log DAMAGE/DEATH events to entity pathcorners.

Use the output `lookup` table in other scripts when you want a presumed combat-log name from a pathcorner string.

## Reproduce

```bash
cd manta-labs/lasthits-debug
go run . -replay ../../dota-replays/8915936762.dem \
  -mode build-pathcorner-map
```

Or:

```bash
./manta-labs/proofs/pathcorner-map/run.sh
```

JSON for scripts:

```bash
go run . -replay ../../dota-replays/8915936762.dem \
  -mode build-pathcorner-map -format json -out /tmp/pathcorner-map.json
```

## Method

1. Queue combat-log lane-creep DAMAGE/DEATH with post-health.
2. On entity update, collect lane creeps with pathcorner names per tick.
3. When **exactly one** entity matches post-health that tick (`-map-votes unique`), record a vote: `pathcorner → combat_target`.
4. Filter: combat-log name must match pathcorner team (`goodguys` / `badguys`).
5. Presumed name = highest vote count; `conflict: true` if runner-up has ≥25% of winner's votes.

## Expected output

See `manta-labs/lasthits-debug/examples/pathcorner-map.txt` and `pathcorner-map.json`.

Example row:

```text
lane_mid_pathcorner_badguys_4 | npc_dota_creep_badguys_melee | badguys_flagbearer:210 badguys_melee:215 ... | 491 | true
  spawn_max_health: 550:45
```

`conflict: true` means do not trust the presumed name blindly — multiple creep types used that pathcorner in this replay.

## Regenerate examples

```bash
cd manta-labs/lasthits-debug && ./examples/regenerate.sh
```
