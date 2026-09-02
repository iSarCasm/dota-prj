# matchsummary KDA proof

## What is proved

Combat-log KDA for a single hero matches OpenDota end-game totals when:

1. **Kills/deaths** come from `DOTA_COMBATLOG_DEATH` on hero targets (skip illusion killer/victim).
2. **Assists** use `assist_players` entries equal to `m_iPlayerID / 2` from the hero entity.
3. **Assists** only count on hero kills (`is_attacker_hero`), excluding kills/deaths where the analyzed hero is attacker or victim.

## Command

```bash
cd parser
go test ./internal/matchsummary/ -run 'KDA$' -count=1
```

## Replay

- `dota-replays/8915936762.dem` — Warlock, expected `0/9/14`
- `dota-replays/8934466456.dem` — Phantom Assassin, expected `4/6/4`

Fetch: `ruby dota-replays/fetch.rb`

## Expected output

All `TestReplay*_KDA` tests pass.
