# Combat-log name catalog (heroes / items / spells)

## What is being proved

Parsing explored (or catalog) replays with manta yields stable unique sets of:

- hero unit names (`npc_dota_hero_*`)
- item names (`item_*`, including purchases)
- spell / ability inflictors from combat log

from `CombatLogNames` indices on combat-log entries.

## Commands

```bash
cd manta-labs/combat-catalog
go mod tidy

# Catalog replay (fast, committed examples)
./examples/regenerate.sh

# Full explored pool
go run . -out output -explored ../../dota-replays/explored
```

Or:

```bash
./manta-labs/proofs/combat-catalog/run.sh
```

## Expected output

`examples/summary.txt` for the Warlock catalog dem (`8915936762`):

```text
# combat-catalog summary
replays	1
heroes	10
items	124
spells	194
```

Snippets: `examples/heroes.head.txt`, `items.head.txt`, `spells.head.txt`.

## Regenerate examples

```bash
cd manta-labs/combat-catalog && ./examples/regenerate.sh
```
