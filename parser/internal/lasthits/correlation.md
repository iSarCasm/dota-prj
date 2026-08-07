# Hero-damage correlation (combat log ↔ entity)

Last-hit miss detection must bind a combat-log `DAMAGE` line to the entity idx whose health actually dropped. Two strategies were considered.

## Forward correlation (used)

Manta processes messages on a tick in priority order: **combat log first**, then `PacketEntities` (entity updates). See `parser/engine_notes.md`.

Flow:

1. `onCombatLogEntry` queues a pending hero-damage line (creep name, post-damage health, damage value, game time).
2. `onCreepHealthUpdate` runs `correlateHeroDamage`: if an entity’s health drop matches the signature and `dropGameTime >= pd.gameTime`, append that entity idx to `pd.candidates`.
3. After the tick window (`tickDuration`), pending lines finalize into a unique bind or a conflict group.

Entity game timestamps can be slightly **later** than the combat-log timestamp on the same tick; that is wall-clock on the event, not callback order. Forward correlation handles this because the entity callback runs after the combat-log callback.

## Retroactive correlation (removed)

**Idea:** When a combat-log `DAMAGE` line arrives, scan existing `creepTracks` for health drops that already happened and backfill `pd.candidates` — covering the case where entity updates were processed **before** the combat-log line on the same tick.

Implementation sketch (removed from code):

- On each new pending line, walk all tracks.
- Match if post-damage health and `prevHealth == health + damage` align, and `|lastDropAt - pd.gameTime| <= retroactiveDropMaxLag` (~0.1s).
- Also handle same-tick kill: entity dropped to post-damage HP then died before the damage line was seen.

**Why removed:** Experiments showed nothing depended on it:

- Manta callback order is combat log → entities, so forward correlation always runs with pending already queued.
- Commenting out the production call to retroactive correlate did not break unit tests, integration tests (`8915936762`), or the quality report.
- The only unit test that used retroactive called it **directly** after manually simulating entity-before-combat-log — it did not exercise the production hook.

If manta ordering ever changes or a replay proves entity-first delivery, retroactive correlation could be reintroduced. Until then, forward correlation plus the zero-candidate reopen guard in `finalizePendingBatch` is sufficient.

## Deferred death until combat log (removed)

**Idea:** When an entity’s health hit 0 before a matching combat-log `DEATH` line was seen, set `awaitingDeathCombatLog` on the track and defer miss/last-hit resolution. When the `DEATH` line arrived later, `resolveAwaitingDeathCombatLog` would finish `handleCreepDeath`.

**Why removed:** Same mistaken assumption as retroactive damage correlation — that entity callbacks can run before combat log on the same tick. Manta order is combat log first, then entities. On a kill tick:

1. Combat-log `DEATH` appends `pendingOtherDeath` (or `pendingHeroKills`).
2. Entity health → 0 runs `handleCreepDeath`, which consumes that pending entry via `consumeMatchingOtherDeath` / `hasPendingHeroKill`.

The defer branch was only exercised when unit tests manually simulated entity death before queuing `pendingOtherDeath`. Real replays and integration tests never needed it.

Entity death timestamps can still be **later** than combat-log death timestamps on the same tick; that is game-time on the event, not callback order.

## Close all pending on any creep death (fixed)

**Bug:** `closeAllPendingHeroDamage()` ran on every creep death and finalized **all** open hero-damage pending lines, often with zero entity candidates.

**Scenario:**

1. Combat-log damage 1  
2. Combat-log damage 2  
3. Unrelated creep dies  
4. Entity health drop 1  
5. Entity health drop 2  

If step 3 closes every pending line before steps 4–5, both combat-log lines finalize with no candidates and miss detection breaks.

**Fix:** `closePendingHeroDamageForDeadCreep(deadIdx)` only closes pending lines that already include `deadIdx` in `candidates`. Unrelated deaths do not touch other lines still collecting candidates.

See `TestUnrelatedCreepDeath_DoesNotPrematurelyFinalizeOtherPending`.
