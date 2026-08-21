# manta-labs proofs

Reproducible scripts that prove parser/replay findings. Each proof has its own README with exact commands and expected output.

| Proof | Finding | Script |
|-------|---------|--------|
| [pathcorner-map](pathcorner-map/README.md) | Build empirical pathcorner → combat-log name lookup from health correlation | `lasthits-debug -mode build-pathcorner-map` |
| [pathcorner-lane-spawn](pathcorner-lane-spawn/README.md) | Pathcorner → real lane; cross-replay consistency | `run.sh`, `compare-replays.sh` |
| [spawn-lane-clusters](spawn-lane-clusters/README.md) | Per-side top/mid/bot spawn centroids for `GetCreepLaneFromSpawnLocation` | `run.sh` / `compute_centroids.py` |
| [tick-ordering](tick-ordering/README.md) | Entity updates for tick N finish before combat log for tick N+1 | `run.sh` |
| [combat-catalog](combat-catalog/README.md) | Unique hero / item / spell names from combat logs across replays | `combat-catalog` / `run.sh` |

When adding a new finding anywhere in the repo, add a row here and a folder under `manta-labs/proofs/` (or a dedicated lab tool) with README + runnable command.
