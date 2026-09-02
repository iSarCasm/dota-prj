# matchsummary tower damage proof

## What is proved

Combat-log tower damage for the analyzed hero matches OpenDota `tower_damage` when:

1. Sum `DOTA_COMBATLOG_DAMAGE` `value` to towers, barracks (`_rax`), and the ancient (`_fort`).
2. Credit `damage_source_name` (fallback: `attacker_name`) to the owning hero.
3. Exclude other structures (effigy, filler, shrine/healer, etc.) even if `is_target_building` is set.

## Command

```bash
cd parser
go test ./internal/matchsummary/ -run TowerDamage -count=1
```

## Replay

- `dota-replays/8941961575.dem` — Terrorblade, expected total `24794`
- `dota-replays/8941817475.dem` — Slark, expected total `1046`
- `dota-replays/8915936762.dem` — Warlock, expected total `0`

## Expected output

All `TestReplay*_TowerDamage` tests pass.
