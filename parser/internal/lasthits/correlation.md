# Hero-damage correlation (combat log ↔ entity)

Last-hit miss detection must bind a combat-log `DAMAGE` line to the entity idx whose health actually dropped.

## Correlation key: replay tick

Combat log and entity callbacks share **`p.Tick`** from manta on the same demo tick. Game timestamps (`CurrentGameTime()`) can differ on the same tick (combat log runs before gamerules clock update), so correlation uses **replay tick**, not game time or tick duration.

`gameTime` is kept only for `Event.Timestamp` output.

## Forward correlation (used)

Manta processes messages on a tick in priority order: **combat log first**, then `PacketEntities` (entity updates). See `parser/engine_notes.md`.

Flow:

1. `onCombatLogEntry` queues pending hero damage `{creepName, tick, postHealth, damage}`.
2. At the start of each combat-log callback, `closePendingHeroDamageBeforeTick(currentTick)` closes pending from **earlier** replay ticks (entity phase for those ticks is done).
3. `onCreepHealthUpdate` runs `correlateHeroDamage`: if an entity health drop matches the signature and `entityTick >= pd.tick` within the replay-tick window, append that entity idx to `pd.candidates`.
4. On creep death, `finalizePendingHeroDamageForTick(tick)` closes same-tick pending before miss/last-hit resolution.
5. Closed pending batches finalize into a unique bind or a conflict group.

### Same-tick kill skip

When the entity dies on the same replay tick as the damage line, it may jump straight to 0 without reporting post-damage HP (e.g. combat log `health=24` but entity `91→0`). `heroDamageCorrelates` accepts this when `health <= 0 && entityTick == pd.tick`.

### Kill matching window

Damage ↔ death pairing uses replay ticks: `toTick - fromTick <= missedLastHitWindowTicks` (60 ticks = 2s at 30 Hz). Not game-time based.

## Retroactive correlation (removed)

**Idea:** When a combat-log `DAMAGE` line arrives, scan existing `creepTracks` for health drops that already happened.

**Why removed:** Manta callback order is combat log → entities, so forward correlation always runs with pending already queued. See git history / prior version of this doc.

## Deferred death until combat log (removed)

**Why removed:** On a kill tick, combat-log `DEATH` is queued before entity death runs `handleCreepDeath`. Entity-first delivery does not happen in manta's ordering.

## Close pending on creep death (removed)

**Bug (original):** `closeAllPendingHeroDamage()` on every creep death finalized **all** open pending lines.

**Current behavior:** Only `closePendingHeroDamageBeforeTick` (next tick's combat log) and `finalizePendingHeroDamageForTick` (same-tick death) close hero-damage pending. Never finalize a batch with 0 candidates (reopen — entities for that tick have not run yet).

See `TestUnrelatedCreepDeath_DoesNotPrematurelyFinalizeOtherPending`.
