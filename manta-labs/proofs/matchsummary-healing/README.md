# matchsummary healing proof

## What is proved

Combat-log healing for the analyzed hero matches OpenDota `hero_healing` when:

1. Sum `DOTA_COMBATLOG_HEAL` `value` to allied heroes (`is_target_hero`, not illusion).
2. Credit `damage_source_name` (fallback: `attacker_name`) to the owning hero.
3. Exclude self-heals (healer == target).

Note: Oracle and some other heroes may include partial self-heal in OpenDota's GC scalar; ally-only reconstruction matches most heroes exactly.

## Command

```bash
cd parser
go test ./internal/matchsummary/ -run Healing -count=1
```

## Replay

- `dota-replays/8915936762.dem` — Warlock, expected total `4365`
- `dota-replays/8934466456.dem` — Dazzle, expected total `5197`
- `dota-replays/8941817475.dem` — Lycan, expected total `3329`

## Expected output

All `TestReplay*_Healing` tests pass.
