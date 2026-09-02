# matchsummary heroes killed proof

## What is proved

The heroes-killed table matches OpenDota hero entries in `killed` when:

1. On `DOTA_COMBATLOG_DEATH`, the analyzed hero is the killer (`attacker_name`, not illusion).
2. Target is an enemy hero (`is_target_hero`).
3. Rows are `{hero: npc_dota_hero_*, kills: N}` sorted by kills descending, then hero name.

## Command

```bash
cd parser
go test ./internal/matchsummary/ -run HeroesKilled -count=1
```

## Replay

- `dota-replays/8934466456.dem` — Phantom Assassin: Lich x2, Ancient Apparition x1, Dark Seer x1
- `dota-replays/8915936762.dem` — Warlock: empty (0 kills)

## Expected output

Both `TestReplay*_HeroesKilled` tests pass.
