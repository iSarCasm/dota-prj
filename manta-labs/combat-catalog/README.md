# combat-catalog

Collect unique **hero**, **item**, and **spell** names from combat-log entries across explored (or listed) replays.

## What is collected

From each `CMsgDOTACombatLogEntry`, resolve `CombatLogNames` for attacker / target / inflictor / damage_source (and purchase `value`):

| Bucket | Rule |
|--------|------|
| `heroes.txt` | `npc_dota_hero_*` |
| `items.txt` | `item_*` (incl. `DOTA_COMBATLOG_PURCHASE` / `ITEM`) |
| `spells.txt` | ability inflictors (`ABILITY` / `ABILITY_TRIGGER`) plus non-item damage inflictors |

Each line: `name<TAB>event_count<TAB>replay_count`

Also writes `summary.txt` and `catalog.json`.

## Usage

```bash
cd manta-labs/combat-catalog
go mod tidy

# All dems under dota-replays/explored (default)
go run . -out output

# Subset / explicit
go run . -out output -limit 5
go run . -out output -replay ../../dota-replays/8915936762.dem
go run . -out output -explored ../../dota-replays/explored
```

## Regenerate examples

```bash
./examples/regenerate.sh
```

## Proof

See [../proofs/combat-catalog/README.md](../proofs/combat-catalog/README.md).
