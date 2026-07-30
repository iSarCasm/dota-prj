# Replay catalog

**Current patch:** 7.41d

Local `.dem` files are gitignored. Fetch all catalog replays with `ruby fetch.rb` (see [README.md](README.md)).

| Match ID   | Patch  | Hero    | Used by | Notes |
|------------|--------|---------|---------|-------|
| 8915936762 | 7.41d  | Warlock | `parser/internal/lasthits` integration test, manta-labs lasthits proofs | Reference replay for missed-LH correlation. Warlock damages `npc_dota_creep_badguys_flagbearer` to 137 HP at ~164s; Huskar kills at ~165.7s (missed deny ~2:45). Health-collision regression window ~4:06 (247s). |
