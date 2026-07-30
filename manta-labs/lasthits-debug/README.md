# lasthits-debug

Debug tool for **missed last-hit / deny** detection in `parser/internal/lasthits`.

It prints interleaved **combat log** and **OnEntity** events so you can verify that combat-log damage/death lines correlate with the correct creep entity.

## Background

Missed CS detection needs two data sources:

1. **Combat log** — knows *what* happened (Warlock damaged `npc_dota_creep_badguys_flagbearer`, Huskar killed it) but not *which* entity instance when several creeps share a name.
2. **Entity updates** — know per-entity `m_iHealth` and death, but **do not** expose the combat-log NPC name on lane creeps.

### Critical finding (entity names)

On `CDOTA_BaseNPC_Creep_Lane`:

| Field | Example value | Usable for combat-log name? |
|-------|---------------|----------------------------|
| `m_iUnitNameIndex` → `EntityNames` | `lane_mid_pathcorner_badguys_4` | **No** — path corner |
| `m_pEntity.m_nameStringTableIndex` → `EntityNames` | `npc_dota_creep_lane` | **No** — generic class |

**Do not correlate by entity name.** The production handler (`parser/internal/lasthits`) matches by **post-damage health** from the combat log (`CMsgDOTACombatLogEntry.health`) to `m_iHealth` on the entity update that follows.

Assumption: **combat log callbacks run before entity updates** in the same tick (same pattern as PT switch detection in `parser/internal/pt`).

See `notes.md` for the Warlock replay walkthrough.

## Setup

```bash
cd manta-labs/lasthits-debug
go mod tidy
```

Requires only `github.com/dotabuff/manta`. Game time uses a local copy of `parser/internal/timeandpauses` math in `gameclock.go` (the lab cannot import `dota2/internal/*`).

## Usage

```text
go run . -replay <path.dem> [-mode <mode>] [-from SEC] [-to SEC] [filters]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-replay` | *(required)* | Path to `.dem` file |
| `-mode` | `trace` | See modes below |
| `-from` | `160` | Window start (game seconds) |
| `-to` | `170` | Window end (game seconds) |
| `-health` | `0` | Filter by `m_iHealth` (entity-names / health-match) |
| `-hero` | | Combat-log attacker/target substring |
| `-target` | | Combat-log target substring |
| `-out` | | Write to file instead of stdout |
| `-format` | `text` | For `build-pathcorner-map`: `text` or `json` |
| `-map-votes` | `unique` | For `build-pathcorner-map`: `unique` or `spawn` |

### Modes

| Mode | Purpose |
|------|---------|
| `trace` | Full interleaved COMBAT + ENTITY creep lines in the window |
| `warlock-badguys` | Compact DAMAGE/DEATH for Warlock or `badguys` creeps |
| `entity-names` | Dump EntityNames lookups — shows pathcorner vs generic names |
| `health-match` | Match one post-damage health from combat log to entity index |
| `dump-fields` | Dump identity-related entity fields (model, unit type, names) |
| `proof-pathcorner` | **Proof** that pathcorner entity names ≠ combat-log creep NPC names |
| `build-pathcorner-map` | Build presumed pathcorner → combat-log name mapping from health correlation |

### Build pathcorner → combat-log mapping

Scan a full replay once, correlate combat-log DAMAGE/DEATH post-health to entity pathcorner (unique match per tick, team-filtered), and output vote counts plus a presumed winner:

```bash
go run . -replay ../../dota-replays/8915936762.dem \
  -mode build-pathcorner-map

# JSON for scripts (includes "lookup" map):
go run . -replay ../../dota-replays/8915936762.dem \
  -mode build-pathcorner-map -format json -out examples/pathcorner-map.json
```

| Flag | Default | Description |
|------|---------|-------------|
| `-map-votes` | `unique` | `unique` = only count when exactly one lane creep matches post-health that tick; `spawn` = same but entity must be at full HP |
| `-format` | `text` | `text` or `json` |

**Caveats:** mapping is empirical per replay. `conflict: true` means multiple combat-log names received votes for the same pathcorner (health collisions or the pathcorner is reused by different creep types over time). Prefer entries with high vote totals and `conflict: false`. See `examples/pathcorner-map.txt`.

## Proofs

Formal reproducible proofs live under `manta-labs/proofs/`. Index: [../proofs/README.md](../proofs/README.md).

### Proof: cannot map pathcorner → combat-log creep name

```bash
go run . -replay ../../dota-replays/8915936762.dem \
  -mode proof-pathcorner -from 160 -to 170
```

Or: `../../proofs/no-pathcorner-to-combatlog/run.sh`

Expected: all three checks **PASS** and conclusion line. See `examples/proof-pathcorner.txt`.

Checks:

1. No `m_iUnitNameIndex` value resolves to `npc_dota_creep_*` on lane creeps.
2. Health-correlated entity has a pathcorner string while combat log has a typed NPC name (e.g. `…_flagbearer` vs `lane_mid_pathcorner_badguys_4`).
3. Same pathcorner at creep spawn (first full-health tick) with different `m_iMaxHealth` on different entities — pathcorner is a waypoint, not creep archetype.

## Examples

### 1. Known missed deny — replay `8915936762`, Warlock @ 2:44–2:46

Warlock damages enemy flagbearer; Huskar gets the deny ~1.5s later.

```bash
go run . -replay ../../dota-replays/8915936762.dem \
  -mode warlock-badguys -from 163 -to 170
```

Expected highlights:

```text
gt=164.200 DAMAGE attacker=npc_dota_hero_warlock target=npc_dota_creep_badguys_flagbearer health=137 ...
gt=165.667 DEATH  attacker=npc_dota_hero_huskar     target=npc_dota_creep_badguys_flagbearer health=0 ...
```

Parser output should include a missed event near `165.73` for `npc_dota_creep_badguys_flagbearer`.

### 2. Interleaved trace around the same window

```bash
go run . -replay ../../dota-replays/8915936762.dem \
  -mode trace -from 164 -to 166 -hero warlock -target badguys \
  -out /tmp/lh_trace.txt
```

Look for a COMBAT line with `health=137`, then an ENTITY line with the same health on one entity index, then later `health=0 died=true` on that index.

### 3. Prove entity names are wrong

```bash
go run . -replay ../../dota-replays/8915936762.dem \
  -mode entity-names -from 164.1 -to 164.3 -health 137
```

Example output (see `examples/entity-names-flagbearer.txt`):

```text
gt=164.267 idx=2382 health=137 m_iUnitNameIndex=142->"lane_mid_pathcorner_badguys_4"(ok=true) ...
```

Compare to combat log target: `npc_dota_creep_badguys_flagbearer`.

### 4. Health correlation proof

```bash
go run . -replay ../../dota-replays/8915936762.dem \
  -mode health-match -from 164 -to 166 -health 137
```

Links combat log `health=137` to entity `idx=2382`, which later dies when Huskar gets the kill.

## Sample output files

Pre-captured snippets live in `examples/`:

- `warlock-flagbearer-window.txt` — `warlock-badguys` mode
- `entity-names-flagbearer.txt` — why name-based correlation fails
- `health-match-137.txt` — combat log ↔ entity index
- `proof-pathcorner.txt` — pathcorner ≠ combat-log NPC name proof

Regenerate after parser or replay changes:

```bash
./examples/regenerate.sh
```

## Related code

| Path | Role |
|------|------|
| `parser/internal/lasthits/lasthits.go` | Production missed-LH handler |
| `manta-labs/creeps/` | Earlier creep / combat-log experiments |
| `parser/internal/pt/pt.go` | Same combat-log → OnEntity ordering pattern |
