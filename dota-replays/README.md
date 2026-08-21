# dota-replays

Shared replay storage for parser tests and manta-labs debug tools. Not tied to `dota-web/storage`.

**Current patch:** 7.41d — new catalog replays should be recorded from this patch when possible (see [REPLAYS.md](REPLAYS.md)).

## Fetch catalog replays

Requires Ruby, Go, and network:

```bash
ruby fetch.rb          # all match IDs from REPLAYS.md
ruby fetch.rb 8915936762   # optional: specific ID(s) only
```

Multiple match IDs can be passed explicitly. Valid `.dem` files (PBDEMS2 header) are skipped; corrupt downloads are replaced automatically.

Override directory: `DOTA_REPLAYS_DIR=/path/to/replays`

## Explore public replays

Pull recent matches from OpenDota `publicMatches`, optionally filtered by rank and/or hero:

```bash
ruby explore.rb --rank ancient --limit 1
ruby explore.rb --hero invoker --rank legend --limit 3
ruby explore.rb --hero 74 --turbo          # include turbo; default skips game_mode 23
ruby explore.rb --min-age 30               # skip too-fresh matches (Valve often 404s)
ruby explore.rb --all-heroes               # fill gaps until every hero is in explored/
ruby explore.rb --all-heroes --rank divine # same, but only from divine public matches
```

`--all-heroes` scans existing `explored/` coverage, then greedily downloads matches that cover the most still-missing heroes (turbo enabled by default; `--limit` default 200).

Writes under `explored/{rank}/{match_id}/`:

- `{match_id}.dem.bz2` — Valve download (kept)
- `{match_id}.dem` — decompressed
- `{match_id}.json` — OpenDota public match row + match details

Shared helpers live in `lib/` (`opendota`, `replay`, `ranks`, `heroes`).

Flow:

1. `GET /publicMatches` (optional `min_rank` / `max_rank`)
2. Client-side hero / turbo / duration filters; paginate via `less_than_match_id`
3. `POST /request/{match_id}` then `GET /matches/{match_id}` → `replay_url`
4. Download + decompress via `go run ../parser/cmd/replay-decompress`

## Catalog

See [REPLAYS.md](REPLAYS.md) for match descriptions and what each replay is used for.
