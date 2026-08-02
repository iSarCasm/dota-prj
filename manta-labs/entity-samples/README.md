# replay-samples

Extract **example combat-log and entity lines** from one or more replays, grouped by type or entity class.

## Output layout

```text
output/
  combat_logs/
    damage.txt
    death.txt
    ...
  entities/
    cdota_unit_hero_warlock.txt
    cdota_npc_dota_lane_creep.txt
    ...
  combat_logs_summary.txt
  entities_summary.txt
```

Each file under `combat_logs/` contains up to N example lines for that `DOTA_COMBATLOG_*` type (space-separated `key=value`).

Each file under `entities/` contains up to N full entity snapshots for that class — one block per unique entity index, with **all decoded fields** from `Entity.Map()`. String-table indices get an `# EntityNames:` comment when resolvable.

## Usage

```bash
cd manta-labs/replay-samples
go mod tidy

# Multiple replays via repeated -replay or positional args
go run . -out output \
  -replay ../../dota-replays/8915936762.dem \
  -replay /path/to/other.dem

go run . -out output ../../dota-replays/*.dem
```

| Flag | Default | Description |
|------|---------|-------------|
| `-out` | `output` | Output directory |
| `-replay` | | Replay path (repeatable) |
| `-max-per-type` | `30` | Max combat log examples per type (all replays) |
| `-max-per-replay-type` | `10` | Max combat log examples per type from each replay |
| `-max-per-class` | `5` | Max entity examples per class (unique idx, all replays) |
| `-max-per-replay-class` | `3` | Max entity examples per class from each replay |

Positional arguments after flags are also treated as replay paths.

## Example lines

Combat log (DAMAGE):

```text
replay=8915936762.dem tick=11315 timestamp=164.200 type=DOTA_COMBATLOG_DAMAGE attacker=npc_dota_hero_warlock target=npc_dota_creep_badguys_flagbearer health=137 value=59
```

For DAMAGE events, `health` is post-hit HP and `value` is damage (`prev_health = health + value`).

Entity (lane creep, truncated):

```text
=== replay=8915936762.dem tick=4200 idx=1234 op=Created+Entered class=CDOTA_BaseNPC ===
m_iHealth=550
m_iMaxHealth=550
m_iUnitNameIndex=42  # EntityNames: lane_mid_pathcorner_badguys_4
m_pEntity.m_nameStringTableIndex=99  # EntityNames: npc_dota_thinker
...
```

Lane creeps use pathcorner names in `EntityNames`, not `npc_dota_creep_*`.

## Regenerate sample output

```bash
./examples/regenerate.sh
```

See `examples/` for committed snippets of expected output.
