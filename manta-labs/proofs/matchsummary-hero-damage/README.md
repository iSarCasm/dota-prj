# matchsummary hero damage proof

## What is proved

Combat-log hero damage for the analyzed hero matches OpenDota `hero_damage` when:

1. Sum `DOTA_COMBATLOG_DAMAGE` `value` to enemy heroes (`is_target_hero`, not illusion).
2. Credit `damage_source_name` (fallback: `attacker_name`) to the owning hero.
3. Skip self-damage (source hero == target hero).
4. `damage_type` maps to `physical` / `magical` / `pure` / `unknown`.

## Command

```bash
cd parser
go test ./internal/matchsummary/ -run HeroDamage -count=1
```

## Replay

- `dota-replays/8915936762.dem` — Warlock, expected total `13488`
- `dota-replays/8934466456.dem` — Phantom Assassin, expected total `10901`

## Expected output

Both `TestReplay*_HeroDamage` tests pass.
