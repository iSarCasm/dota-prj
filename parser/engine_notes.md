1. Combat Log timestamps are usually +10 seconds from Entity log timestamps (probably start screen diff)

2. Missed last-hit / deny debugging: see `manta-labs/lasthits-debug/` (README, notes, examples). Replays live in **`dota-replays/`** (`ruby dota-replays/fetch.rb`). Lane creep entity names are pathcorners — correlate by post-damage health from combat log, not EntityNames. **Proof:** `manta-labs/proofs/pathcorner-map/run.sh`

3. All manta-labs findings require a reproducible script + README — see `manta-labs/proofs/README.md` and Cursor rule `manta-labs-findings`.

4. **Manta demo-packet message order (same tick)** — `github.com/dotabuff/manta` sorts inner messages in `onCDemoPacket` by priority (lower runs first). Within one tick:
   - **Priority 0** (default): combat-log entries — run **first**
   - **Priority 5**: `svc_PacketEntities` (entity updates) — run **second**
   - **Priority 10**: legacy game events — run last

   So combat log → entity updates on a tick is correct. Trace output can look backwards because entity lines often carry a **later game timestamp** than the combat line they pair with; that is wall-clock on the event, not callback order.

   **Lasthits implication:** queue hero DAMAGE on combat log; bind on the **following entity updates** (forward correlation). See `internal/lasthits/correlation.md`. Do **not** finalize or consume pending hero-damage from the combat-log callback — busy ticks have many combat lines before any entities run. Calling `closeAllPendingHeroDamage()` on every combat-log line closes pending with 0 candidates and marks it consumed before entities get a chance. Safe closes: entity update (after tick duration) and creep death. Never finalize a batch with 0 candidates (reopen — entities for that tick have not run yet).
