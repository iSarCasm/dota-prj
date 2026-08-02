# manta-labs proofs

Reproducible scripts that prove parser/replay findings. Each proof has its own README with exact commands and expected output.

| Proof | Finding | Script |
|-------|---------|--------|
| [pathcorner-map](pathcorner-map/README.md) | Build empirical pathcorner → combat-log name lookup from health correlation | `lasthits-debug -mode build-pathcorner-map` |

When adding a new finding anywhere in the repo, add a row here and a folder under `manta-labs/proofs/` (or a dedicated lab tool) with README + runnable command.
