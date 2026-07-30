# Proof: pathcorner names ≠ combat-log creep NPC names

## Finding

On lane creeps (`CDOTA_BaseNPC_Creep_Lane`), entity field `m_iUnitNameIndex` → `EntityNames` returns **path waypoint** strings (e.g. `lane_mid_pathcorner_badguys_4`), **not** combat-log unit names (e.g. `npc_dota_creep_badguys_flagbearer`).

Therefore you **cannot** correlate combat-log creep events to entities by matching entity name to combat-log `targetName`.

Creep type at spawn is carried by entity class + model/baseline fields; combat log uses a separate `CombatLogNames` table.

## Reproduce

From repo root (replay path adjustable):

```bash
cd manta-labs/lasthits-debug
go run . -replay ../../dota-replays/8915936762.dem \
  -mode proof-pathcorner -from 160 -to 170
```

Or use the wrapper:

```bash
./manta-labs/proofs/no-pathcorner-to-combatlog/run.sh
```

## Expected result

The script prints three checks and exits non-zero if any fail:

1. No `m_iUnitNameIndex` value resolves to `npc_dota_creep_*` on lane creeps.
2. Health-correlated entity has a pathcorner string while combat log has a typed NPC name (e.g. `…_flagbearer` vs `lane_mid_pathcorner_badguys_4`).
3. Same pathcorner at creep spawn (first full-health tick) appears with different `m_iMaxHealth` on different entities — pathcorner cannot identify melee vs ranged vs flagbearer.

See committed example: `manta-labs/lasthits-debug/examples/proof-pathcorner.txt`

Regenerate:

```bash
cd manta-labs/lasthits-debug && ./examples/regenerate.sh
```

## Reference case (Warlock flagbearer @ ~2:44)

| Source | Value |
|--------|--------|
| Combat log target | `npc_dota_creep_badguys_flagbearer` |
| Entity idx (health=137 match) | `2382` |
| `m_iUnitNameIndex` | `lane_mid_pathcorner_badguys_4` |

Same entity, two incompatible naming schemes — correlation must use health (or model), not name.
