# dota-replays

Shared replay storage for parser tests and manta-labs debug tools. Not tied to `dota-web/storage`.

**Current patch:** 7.41d — new catalog replays should be recorded from this patch when possible (see [REPLAYS.md](REPLAYS.md)).

## Fetch replays

Requires Ruby, Go, and network:

```bash
ruby fetch.rb          # all match IDs from REPLAYS.md
ruby fetch.rb 8915936762   # optional: specific ID(s) only
```

Multiple match IDs can be passed explicitly. Valid `.dem` files (PBDEMS2 header) are skipped; corrupt downloads are replaced automatically.

Flow mirrors `dota-web/app/jobs/match_analysis_job.rb`:

1. `POST https://api.opendota.com/api/request/{match_id}` (request parse)
2. `GET https://api.opendota.com/api/matches/{match_id}` → `replay_url`
3. Download `.dem.bz2` (Valve may serve bz2 or zstd despite the extension)
4. Decompress via `go run ../parser/cmd/replay-decompress` to `{match_id}.dem`

Override directory: `DOTA_REPLAYS_DIR=/path/to/replays`

## Catalog

See [REPLAYS.md](REPLAYS.md) for match descriptions and what each replay is used for.
