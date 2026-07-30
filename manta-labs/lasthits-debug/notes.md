# Last-hit correlation notes

## Problem

Combat log says "Warlock damaged melee creep" then "melee creep died to tower", but many creeps share the same name. We must tie a **specific entity** to the combat-log events.

## Message ordering

Combat log user messages are processed **before** `OnEntity` updates in the same tick. Pending combat-log events can be matched on the following entity batch.

## Why entity NPC names fail

Investigated on replay `8915936762` (Warlock), game time ~164s.

Combat log:

```text
DAMAGE attacker=npc_dota_hero_warlock target=npc_dota_creep_badguys_flagbearer health=137
```

Entity `idx=2382` at `gt=164.267`:

- `m_iUnitNameIndex` → `lane_mid_pathcorner_badguys_4` (path corner, not the creep)
- `m_pEntity.m_nameStringTableIndex` → `npc_dota_creep_lane` (generic)

Using entity name for `creepTypeFromTargetName()` always failed → **0 missed last hits** in the first entity-correlation attempt.

**Proof script:** `manta-labs/proofs/no-pathcorner-to-combatlog/` (runs `lasthits-debug -mode proof-pathcorner`). See `manta-labs/proofs/README.md`.

**Presumed mapping:** `lasthits-debug -mode build-pathcorner-map` health-correlates combat log → pathcorner across a full replay and outputs vote counts + JSON `lookup`. See `manta-labs/proofs/pathcorner-map/`. Treat `conflict: true` rows as ambiguous.

## Working approach (production)

1. **DAMAGE (our hero)** → queue `{creepName, postDamageHealth, gameTime}` from combat log.
2. **OnEntity creep** → find entity where `m_iHealth == postDamageHealth` and `prevHealth == postDamageHealth + damage`; store `creepName` on that entity track.
3. **DEATH (not our hero)** → queue `{creepName, gameTime}`.
4. **OnEntity creep death** → if tracked entity has matching `creepName` and recent hero damage → `missed_last_hit`.

## Reference case: missed deny @ 2:45

Replay: `dota-replays/8915936762.dem`  
Hero: Warlock

| Game time | Event |
|-----------|--------|
| 164.20 | Warlock damages `npc_dota_creep_badguys_flagbearer` → health 137 |
| 164.27 | Entity `2382` updates with `m_iHealth=137` |
| 165.67 | Huskar kills flagbearer (combat log DEATH) |
| 165.73 | Entity `2382` dies → missed deny recorded |

User described this as a missed deny on a badguys creep around 2:45–2:46; the creep type is **flagbearer**, not melee.

## Limitations

- **Health collision**: two creeps with the same post-damage health in the same tick are ambiguous (rare at low health).
- **Window**: damage and death must be within `missedLastHitWindowSec` (2s).
- **Neutrals**: use `CDOTA_BaseNPC_Creep*` entity class; correlation logic is the same.
