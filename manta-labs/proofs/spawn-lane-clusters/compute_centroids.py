#!/usr/bin/env python3
"""Compute top/mid/bot spawn-cluster centroids per map side.

Input: pathcorner-lane-points.json (from build-pathcorner-lane-spawn -format points).

Side rule (same as creeps.GetCreepSide geography):
  x>0 and y>0 → bad (NE / top-right), else → good (SW / bottom-left).

Lane clusters (3 discrete spawn slots per side):
  good: top = lowest X / highest Y; mid = mid; bot = highest X / lowest Y
  bad:  top = lowest X / highest Y; mid = lower Y+X than bot; bot = highest X / low Y

Writes centroids TSV + Go snippet constants for GetCreepLaneFromSpawnLocation.
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from collections import defaultdict
from pathlib import Path

# Initial seeds from unique spawn slots (cell-aligned).
SEEDS = {
    "good": {
        "top": (-6784.0, -4224.0),
        "mid": (-5248.0, -4736.0),
        "bot": (-3712.0, -6272.0),
    },
    "bad": {
        "top": (3072.0, 5632.0),
        "mid": (3968.0, 3456.0),
        "bot": (6016.0, 3456.0),
    },
}


def side_of(x: float, y: float) -> str:
    return "bad" if x > 0 and y > 0 else "good"


def nearest_lane(side: str, x: float, y: float, cents: dict[str, tuple[float, float]]) -> str:
    best, best_d = "", 1e30
    for lane, (cx, cy) in cents[side].items():
        d = (x - cx) ** 2 + (y - cy) ** 2
        if d < best_d:
            best, best_d = lane, d
    return best


def refine(points: list[tuple[float, float, str]], seeds: dict) -> dict[str, dict[str, tuple[float, float]]]:
    cents = {s: dict(v) for s, v in seeds.items()}
    for _ in range(5):
        buckets: dict[tuple[str, str], list[tuple[float, float]]] = defaultdict(list)
        for x, y, side in points:
            lane = nearest_lane(side, x, y, cents)
            buckets[(side, lane)].append((x, y))
        for (side, lane), pts in buckets.items():
            mx = sum(p[0] for p in pts) / len(pts)
            my = sum(p[1] for p in pts) / len(pts)
            cents[side][lane] = (mx, my)
    return cents


def load_points(path: Path) -> list[tuple[float, float, str]]:
    payload = json.loads(path.read_text())
    out = []
    for row in payload.get("table", []):
        for p in row["points"]:
            x, y = float(p["x"]), float(p["y"])
            out.append((x, y, side_of(x, y)))
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument(
        "input",
        nargs="?",
        default="../../lasthits-debug/examples/pathcorner-lane-points.json",
    )
    ap.add_argument("-o", "--out-dir", default=".")
    args = ap.parse_args()

    inp = Path(args.input)
    if not inp.is_file():
        print(f"not found: {inp}", file=sys.stderr)
        return 1

    points = load_points(inp)
    if not points:
        print("no points", file=sys.stderr)
        return 1

    cents = refine(points, SEEDS)
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    # stats
    buckets: dict[tuple[str, str], list[tuple[float, float]]] = defaultdict(list)
    for x, y, side in points:
        lane = nearest_lane(side, x, y, cents)
        buckets[(side, lane)].append((x, y))

    tsv = out_dir / "centroids.tsv"
    lines = ["side\tlane\tmean_x\tmean_y\tspawns\tstd_x\tstd_y\tmax_dist\n"]
    go_lines = [
        "// Spawn lane centroids from manta-labs/proofs/spawn-lane-clusters (7 replays).\n",
        "// Do not hand-edit — regenerate via compute_centroids.py.\n",
    ]
    for side in ("good", "bad"):
        for lane in ("top", "mid", "bot"):
            pts = buckets[(side, lane)]
            mx, my = cents[side][lane]
            n = len(pts)
            sx = math.sqrt(sum((p[0] - mx) ** 2 for p in pts) / n) if n else 0
            sy = math.sqrt(sum((p[1] - my) ** 2 for p in pts) / n) if n else 0
            maxd = max(math.hypot(p[0] - mx, p[1] - my) for p in pts) if pts else 0
            lines.append(f"{side}\t{lane}\t{mx:.1f}\t{my:.1f}\t{n}\t{sx:.1f}\t{sy:.1f}\t{maxd:.1f}\n")
            go_lines.append(f'\t\t"{lane}": {{{mx:.1f}, {my:.1f}}},\n')
            print(f"{side:4} {lane:3}: ({mx:8.1f}, {my:8.1f}) n={n} max_dist={maxd:.0f}")

    tsv.write_text("".join(lines))

    # separation check
    ok = True
    for side in ("good", "bad"):
        for a, b in (("top", "mid"), ("mid", "bot"), ("top", "bot")):
            ax, ay = cents[side][a]
            bx, by = cents[side][b]
            inter = math.hypot(ax - bx, ay - by)
            max_intra = max(
                max(math.hypot(p[0] - cents[side][a][0], p[1] - cents[side][a][1]) for p in buckets[(side, a)]),
                max(math.hypot(p[0] - cents[side][b][0], p[1] - cents[side][b][1]) for p in buckets[(side, b)]),
            )
            print(f"{side} {a}-{b}: inter={inter:.0f} max_intra={max_intra:.0f}")
            if inter <= 2 * max_intra:
                ok = False
                print(f"  WARN: clusters not well separated", file=sys.stderr)

    go_path = out_dir / "centroids.go.snippet"
    # prettier go map body
    body = ["var spawnLaneCentroids = map[string]map[string]xy{\n"]
    for side in ("good", "bad"):
        body.append(f'\t"{side}": {{\n')
        for lane in ("top", "mid", "bot"):
            mx, my = cents[side][lane]
            body.append(f'\t\t"{lane}": {{{mx:.1f}, {my:.1f}}},\n')
        body.append("\t},\n")
    body.append("}\n")
    go_path.write_text("".join(body))

    print(f"wrote {tsv}")
    print(f"wrote {go_path}")
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
