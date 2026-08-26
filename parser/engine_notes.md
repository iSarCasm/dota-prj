1. Combat Log timestamps are usually +10 seconds from Entity log timestamps (probably start screen diff)

2. Missed last-hit / deny debugging: see `manta-labs/lasthits-debug/` (README, notes, examples). Replays live in **`dota-replays/`** (`ruby dota-replays/fetch.rb`). Lane creep entity names are pathcorners — correlate by post-damage health from combat log, not EntityNames. **Proof:** `manta-labs/proofs/pathcorner-map/run.sh`

3. All manta-labs findings require a reproducible script + README — see `manta-labs/proofs/README.md` and Cursor rule `manta-labs-findings`.

4. **Manta demo-packet message order (same tick)** — `github.com/dotabuff/manta` sorts inner messages in `onCDemoPacket` by priority (lower runs first). Within one tick:
   - **Priority 0** (default): combat-log entries — run **first**
   - **Priority 5**: `svc_PacketEntities` (entity updates) — run **second**
   - **Priority 10**: legacy game events — run last

   So combat log → entity updates on a tick is correct. Trace output can look backwards because entity lines often carry a **later game timestamp** than the combat line they pair with; that is wall-clock on the event, not callback order.

   **Cross-tick:** outer demo messages are non-decreasing in `p.Tick`. After the first combat-log callback at tick `T`, no entity callback with `tick < T` appears later — i.e. all entity updates for tick `N` finish before the first combat log for tick `N+1`. **Proof:** `manta-labs/proofs/tick-ordering/run.sh`.

   **Lasthits implication:** queue hero DAMAGE on combat log with `p.Tick`; bind on the **following entity updates** using replay tick (not game time — entity and combat timestamps can differ on the same tick). See `internal/lasthits/correlation.md`. Do **not** finalize pending from the combat-log callback — busy ticks have many combat lines before entities run. Safe closes: `closePendingHeroDamageBeforeTick` on the next tick's combat log, and `finalizePendingHeroDamageForTick` on same-tick creep death. Never finalize a batch with 0 candidates (reopen — entities for that tick have not run yet).

5. **Pathcorner lane naming is asymmetric by team** — lane creep `EntityNames` use pathcorner strings, not `npc_dota_creep_*`. Prefix usage on spawns:
   - **goodguys:** `lane_bot_pathcorner_*` + `lane_mid_pathcorner_*` (no top-named spawns on PA replay; rare `lane_top_pathcorner_goodguys_*` on others)
   - **badguys:** `lane_top_pathcorner_*` + `lane_mid_pathcorner_*` primarily; missing bot lane folded into mid bucket (e.g. `lane_mid_pathcorner_badguys_7` → geographic **top**, not mid)
   - **`lane_mid_*` ≠ mid lane** — classify by spawn position or lookup table
   - **Lane table:** `manta-labs/proofs/pathcorner-lane-spawn/run.sh` → `pathcorner-lane-table.txt` / `.json` (`lookup[pathcorner].real_lane`)
   - **Consistency (2 replays):** `real_lane` stable for all 9 overlapping pathcorners; spawn x/y varies (±50 for high-traffic, ±1000+ for rare). Proof: `compare-replays.sh` → `pathcorner-lane-consistency.txt`. Full findings: `manta-labs/proofs/pathcorner-lane-spawn/README.md`

6. **`goodguys` / `badguys` in pathcorner EntityNames is not team** — suffix is Valve routing metadata.
   - **Map side from spawn corner:** bottom-left / SW → `good` (Radiant), top-right / NE (`x>0,y>0`) → `bad` (Dire). Implemented in `creeps.GetCreepSide` via pathcorner lookup (not the name suffix). Examples: `lane_mid_pathcorner_badguys_7` → good (SW); `lane_mid_pathcorner_goodguys_1` → bad (NE). Proof table/SVG: `manta-labs/proofs/pathcorner-lane-spawn/`.
   - **Not combat team:** Health-vote mapping usually pairs suffix → `npc_dota_creep_*` team (`pathcorner-map`), but entity stream can disagree at bind time (PA @ 4:00: combat `badguys_ranged`, entity `lane_mid_pathcorner_goodguys_1`). Use combat-log NPC names + health correlation for enemy/friendly; use `GetCreepSide` only for map-side from pathcorner geography.
   - **Lane** cannot be trusted from the pathcorner name prefix (`lane_mid_*` overflow). Each map side has 3 spawn slots (top/mid/bot); use `creeps.GetCreepLaneFromSpawnLocation(x,y)` (nearest centroid). Proof: `manta-labs/proofs/spawn-lane-clusters/`.

7. Creeps on creep damage only shows up in a combat log IF a creep deals a killing blow, normal damage does not show up