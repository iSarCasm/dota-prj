#!/usr/bin/env python3
"""SVG of all spawn dots + GetCreepLaneFromSpawnLocation centroids (creeps.go)."""

from __future__ import annotations

import argparse
import json
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

from centroids import LANE_COLORS, SPAWN_LANE_CENTROIDS, draw_centroids

SVG_NS = "http://www.w3.org/2000/svg"


def load_points(path: Path) -> list[tuple[float, float]]:
    payload = json.loads(path.read_text())
    out = []
    for row in payload.get("table", []):
        for p in row["points"]:
            out.append((float(p["x"]), float(p["y"])))
    return out


def world_bounds(points: list[tuple[float, float]]) -> tuple[float, float, float, float]:
    xs = [p[0] for p in points]
    ys = [p[1] for p in points]
    for lanes in SPAWN_LANE_CENTROIDS.values():
        for x, y in lanes.values():
            xs.append(x)
            ys.append(y)
    pad = 800
    return min(xs) - pad, max(xs) + pad, min(ys) - pad, max(ys) + pad


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument(
        "input",
        nargs="?",
        default="../../lasthits-debug/examples/pathcorner-lane-points.json",
    )
    ap.add_argument(
        "-o", "--output",
        default="../../lasthits-debug/examples/spawn-lane-centroids/centroids-map.svg",
    )
    ap.add_argument("--width", type=int, default=900)
    ap.add_argument("--height", type=int, default=900)
    args = ap.parse_args()

    inp = Path(args.input)
    if not inp.is_file():
        print(f"not found: {inp}", file=sys.stderr)
        return 1

    points = load_points(inp)
    if not points:
        print("no points", file=sys.stderr)
        return 1

    min_x, max_x, min_y, max_y = world_bounds(points)
    w_world = max_x - min_x
    h_world = max_y - min_y
    width, height, margin = args.width, args.height, 60

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
    ET.SubElement(root, "text", {
        "x": str(margin), "y": "22",
        "font-family": "sans-serif", "font-size": "14", "font-weight": "bold",
    }).text = "Spawn lane centroids (creeps.GetCreepLaneFromSpawnLocation)"
    ET.SubElement(root, "text", {
        "x": str(margin), "y": "40",
        "font-family": "sans-serif", "font-size": "11", "fill": "#444",
    }).text = "grey dots = spawns; diamonds = side/lane centroids from creeps.go"

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

    for x, y in points:
        # color by nearest centroid lane (visual only)
        best_lane, best_d = "mid", 1e30
        for lanes in SPAWN_LANE_CENTROIDS.values():
            for lane, (cx, cy) in lanes.items():
                d = (x - cx) ** 2 + (y - cy) ** 2
                if d < best_d:
                    best_d, best_lane = d, lane
        ET.SubElement(root, "circle", {
            "cx": f"{tx(x):.2f}", "cy": f"{ty(y):.2f}", "r": "1.8",
            "fill": LANE_COLORS.get(best_lane, "#888"),
            "fill-opacity": "0.35",
        })

    draw_centroids(root, tx, ty, size=5)

    out = Path(args.output)
    out.parent.mkdir(parents=True, exist_ok=True)
    tree = ET.ElementTree(root)
    if hasattr(ET, "indent"):
        ET.indent(tree, space="  ")
    tree.write(out, encoding="unicode", xml_declaration=True)
    print(f"wrote {out} ({len(points)} dots, 6 centroids)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
