#!/usr/bin/env python3
"""One SVG per pathcorner: each spawn is a dot (stdlib only).

Uses the same global world scale as visualize.py (overview map) — no per-cluster zoom.

Reads points JSON from:
  go run . -mode build-pathcorner-lane-spawn -format points

Usage:
  ./visualize-dots.sh
  python3 visualize_dots.py path/to/points.json -o-dir examples/pathcorner-lane-dots
"""

from __future__ import annotations

import argparse
import json
import math
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

REAL_LANE_COLORS = {
    "bot": "#4C78A8",
    "mid": "#59A14F",
    "top": "#E15759",
    "unknown": "#BAB0AC",
}

SVG_NS = "http://www.w3.org/2000/svg"
# Match visualize.py defaults
DEFAULT_WIDTH = 900
DEFAULT_HEIGHT = 900
MARGIN = 60


def safe_filename(pathcorner: str) -> str:
    name = re.sub(r"[^\w.\-]+", "_", pathcorner)
    return name.strip("_") or "pathcorner"


def load_points(path: Path) -> list[dict]:
    payload = json.loads(path.read_text())
    rows = payload.get("table")
    if rows is None:
        rows = list(payload.get("lookup", {}).values())
    return [r for r in rows if r.get("points")]


def row_spread(row: dict) -> float:
    """Max distance from mean (same definition as Go spawnPositionStats)."""
    if "spread" in row and row["spread"] is not None:
        return float(row["spread"])
    mean_x = float(row.get("mean_x", 0))
    mean_y = float(row.get("mean_y", 0))
    spread = 0.0
    for p in row["points"]:
        d = math.hypot(float(p["x"]) - mean_x, float(p["y"]) - mean_y)
        if d > spread:
            spread = d
    return spread


def enrich_rows(rows: list[dict]) -> list[dict]:
    out = []
    for row in rows:
        r = dict(row)
        r["mean_x"] = float(r.get("mean_x", 0))
        r["mean_y"] = float(r.get("mean_y", 0))
        r["spread"] = row_spread(r)
        out.append(r)
    return out


def global_world_bounds(rows: list[dict]) -> tuple[float, float, float, float]:
    """Same framing as visualize.py world_bounds (means ± spread + pad)."""
    xs, ys, rs = [], [], []
    for row in rows:
        xs.extend([row["mean_x"] - row["spread"], row["mean_x"] + row["spread"]])
        ys.extend([row["mean_y"] - row["spread"], row["mean_y"] + row["spread"]])
        rs.append(row["spread"])
    pad = max(max(rs) * 0.15, 400) if rs else 500
    return min(xs) - pad, max(xs) + pad, min(ys) - pad, max(ys) + pad


def plot_pathcorner(
    row: dict,
    out: Path,
    bounds: tuple[float, float, float, float],
    width: int,
    height: int,
    dot_r: float,
) -> None:
    points = row["points"]
    mean_x = row["mean_x"]
    mean_y = row["mean_y"]
    real_lane = row.get("real_lane", "unknown")
    color = REAL_LANE_COLORS.get(real_lane, REAL_LANE_COLORS["unknown"])
    min_x, max_x, min_y, max_y = bounds
    w_world = max_x - min_x
    h_world = max_y - min_y
    margin = MARGIN

    def tx(x: float) -> float:
        return margin + (x - min_x) / w_world * (width - 2 * margin)

    def ty(y: float) -> float:
        return height - margin - (y - min_y) / h_world * (height - 2 * margin)

    root = ET.Element("svg", {
        "xmlns": SVG_NS,
        "width": str(width),
        "height": str(height),
        "viewBox": f"0 0 {width} {height}",
    })

    ET.SubElement(root, "rect", {
        "x": "0", "y": "0", "width": str(width), "height": str(height),
        "fill": "#fafafa",
    })

    title = row["pathcorner"]
    subtitle = (
        f"real_lane={real_lane}  team={row.get('team', '')}  "
        f"spawns={row.get('spawns', len(points))}  "
        f"mean=({mean_x:.0f},{mean_y:.0f})  "
        f"scale=global"
    )
    ET.SubElement(root, "text", {
        "x": str(margin), "y": "22",
        "font-family": "monospace", "font-size": "12", "font-weight": "bold",
    }).text = title
    ET.SubElement(root, "text", {
        "x": str(margin), "y": "40",
        "font-family": "sans-serif", "font-size": "11", "fill": "#444",
    }).text = subtitle

    if min_x <= 0 <= max_x:
        ET.SubElement(root, "line", {
            "x1": f"{tx(0):.2f}", "y1": str(margin),
            "x2": f"{tx(0):.2f}", "y2": str(height - margin),
            "stroke": "#ddd", "stroke-width": "1",
        })
    if min_y <= 0 <= max_y:
        ET.SubElement(root, "line", {
            "x1": str(margin), "y1": f"{ty(0):.2f}",
            "x2": str(width - margin), "y2": f"{ty(0):.2f}",
            "stroke": "#ddd", "stroke-width": "1",
        })

    for p in points:
        ET.SubElement(root, "circle", {
            "cx": f"{tx(float(p['x'])):.2f}",
            "cy": f"{ty(float(p['y'])):.2f}",
            "r": f"{dot_r:.2f}",
            "fill": color,
            "fill-opacity": "0.55",
            "stroke": "none",
        })

    mx, my = tx(mean_x), ty(mean_y)
    ET.SubElement(root, "circle", {
        "cx": f"{mx:.2f}", "cy": f"{my:.2f}", "r": "5",
        "fill": "none", "stroke": "#111", "stroke-width": "1.5",
    })
    ET.SubElement(root, "line", {
        "x1": f"{mx - 8:.2f}", "y1": f"{my:.2f}",
        "x2": f"{mx + 8:.2f}", "y2": f"{my:.2f}",
        "stroke": "#111", "stroke-width": "1.2",
    })
    ET.SubElement(root, "line", {
        "x1": f"{mx:.2f}", "y1": f"{my - 8:.2f}",
        "x2": f"{mx:.2f}", "y2": f"{my + 8:.2f}",
        "stroke": "#111", "stroke-width": "1.2",
    })

    ET.SubElement(root, "text", {
        "x": str(margin), "y": str(height - 18),
        "font-family": "sans-serif", "font-size": "10", "fill": "#666",
    }).text = "dot = spawn; crosshair = mean; same scale as pathcorner-lane-map.svg"

    out.parent.mkdir(parents=True, exist_ok=True)
    tree = ET.ElementTree(root)
    if hasattr(ET, "indent"):
        ET.indent(tree, space="  ")
    tree.write(out, encoding="unicode", xml_declaration=True)


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("input", nargs="?", default="examples/pathcorner-lane-points.json")
    p.add_argument(
        "-o-dir", "--output-dir",
        default="examples/pathcorner-lane-dots",
        help="directory for per-pathcorner SVG files",
    )
    p.add_argument("--width", type=int, default=DEFAULT_WIDTH)
    p.add_argument("--height", type=int, default=DEFAULT_HEIGHT)
    p.add_argument("--dot-r", type=float, default=2.2)
    args = p.parse_args()

    inp = Path(args.input)
    if not inp.is_file():
        print(f"not found: {inp}", file=sys.stderr)
        return 1

    rows = enrich_rows(load_points(inp))
    if not rows:
        print("empty points table", file=sys.stderr)
        return 1

    bounds = global_world_bounds(rows)
    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    written = 0
    for row in rows:
        name = safe_filename(row["pathcorner"])
        out = out_dir / f"{name}.svg"
        plot_pathcorner(row, out, bounds, args.width, args.height, args.dot_r)
        written += 1

    print(f"wrote {written} SVGs → {out_dir} (global scale {bounds})")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
