#!/usr/bin/env python3
"""Plot pathcorner spawn means as SVG circles (center=mean, radius=spread).

No dependencies — stdlib only. Reads pathcorner-lane-table.json or .tsv.

Usage:
  ./visualize.sh
  python3 visualize.py path/to/table.json -o map.svg
"""

from __future__ import annotations

import argparse
import csv
import json
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

TEAM_STROKE = {
    "goodguys": "#111111",
    "badguys": "#555555",
}

SVG_NS = "http://www.w3.org/2000/svg"


def short_name(pathcorner: str) -> str:
    name = re.sub(r"^lane_(bot|mid|top)_pathcorner_", "", pathcorner)
    return re.sub(r"_(goodguys|badguys)_", "_", name)


def load_json(path: Path) -> list[dict]:
    payload = json.loads(path.read_text())
    rows = payload.get("table")
    if rows is None:
        rows = list(payload.get("lookup", {}).values())
    return rows


def load_tsv(path: Path) -> list[dict]:
    lines: list[str] = []
    with path.open(newline="") as f:
        for line in f:
            if line.startswith("#") or not line.strip():
                continue
            lines.append(line)
    reader = csv.DictReader(lines, delimiter="\t")
    return [
        {
            "pathcorner": row["entity_name"],
            "team": row["team"],
            "mean_x": float(row["mean_x"]),
            "mean_y": float(row["mean_y"]),
            "spread": float(row["spread"]),
            "real_lane": row["real_lane"],
        }
        for row in reader
    ]


def load_table(path: Path) -> list[dict]:
    if path.suffix.lower() == ".json":
        return load_json(path)
    return load_tsv(path)


def world_bounds(rows: list[dict]) -> tuple[float, float, float, float]:
    xs, ys, rs = [], [], []
    for row in rows:
        xs.extend([row["mean_x"] - row["spread"], row["mean_x"] + row["spread"]])
        ys.extend([row["mean_y"] - row["spread"], row["mean_y"] + row["spread"]])
        rs.append(row["spread"])
    pad = max(max(rs) * 0.15, 400) if rs else 500
    return min(xs) - pad, max(xs) + pad, min(ys) - pad, max(ys) + pad


def plot_svg(rows: list[dict], out: Path, title: str, width: int, height: int) -> None:
    min_x, max_x, min_y, max_y = world_bounds(rows)
    w_world = max_x - min_x
    h_world = max_y - min_y
    margin = 60

    def tx(x: float) -> float:
        return margin + (x - min_x) / w_world * (width - 2 * margin)

    def ty(y: float) -> float:
        # flip Y so map reads naturally (north-up-ish)
        return height - margin - (y - min_y) / h_world * (height - 2 * margin)

    def tr(r: float) -> float:
        return r / w_world * (width - 2 * margin)

    root = ET.Element("svg", {
        "xmlns": SVG_NS,
        "width": str(width),
        "height": str(height),
        "viewBox": f"0 0 {width} {height}",
    })

    defs = ET.SubElement(root, "defs")
    for lane, color in REAL_LANE_COLORS.items():
        if lane == "unknown":
            continue
        ET.SubElement(defs, "style", {"id": f"lane-{lane}"}).text = ""

    # title
    ET.SubElement(root, "text", {
        "x": str(margin), "y": "24",
        "font-family": "sans-serif", "font-size": "14", "font-weight": "bold",
    }).text = title
    ET.SubElement(root, "text", {
        "x": str(margin), "y": "42",
        "font-family": "sans-serif", "font-size": "11", "fill": "#444",
    }).text = "circle center = mean spawn, radius = spread"

    # axes through origin
    if min_x <= 0 <= max_x:
        ET.SubElement(root, "line", {
            "x1": str(tx(0)), "y1": str(margin),
            "x2": str(tx(0)), "y2": str(height - margin),
            "stroke": "#ddd", "stroke-width": "1",
        })
    if min_y <= 0 <= max_y:
        ET.SubElement(root, "line", {
            "x1": str(margin), "y1": str(ty(0)),
            "x2": str(width - margin), "y2": str(ty(0)),
            "stroke": "#ddd", "stroke-width": "1",
        })

    # mid diagonal
    d0x, d0y = tx(0), ty(0)
    d1x, d1y = tx(7500), ty(7500)
    if min_x <= 7500 <= max_x and min_y <= 7500 <= max_y:
        ET.SubElement(root, "line", {
            "x1": str(d0x), "y1": str(d0y),
            "x2": str(d1x), "y2": str(d1y),
            "stroke": "#ccc", "stroke-width": "1", "stroke-dasharray": "6,4",
        })

    # circles (spread radius)
    for row in sorted(rows, key=lambda r: r["spread"]):
        x, y, r = row["mean_x"], row["mean_y"], max(row["spread"], 1)
        lane = row.get("real_lane", "unknown")
        team = row.get("team", "")
        fill = REAL_LANE_COLORS.get(lane, REAL_LANE_COLORS["unknown"])
        stroke = TEAM_STROKE.get(team, "#333")
        ET.SubElement(root, "circle", {
            "cx": f"{tx(x):.2f}", "cy": f"{ty(y):.2f}", "r": f"{tr(r):.2f}",
            "fill": fill, "fill-opacity": "0.35",
            "stroke": stroke, "stroke-width": "1.5",
        })

    # center dots + labels on top
    for row in rows:
        x, y = row["mean_x"], row["mean_y"]
        team = row.get("team", "")
        stroke = TEAM_STROKE.get(team, "#333")
        ET.SubElement(root, "circle", {
            "cx": f"{tx(x):.2f}", "cy": f"{ty(y):.2f}", "r": "3",
            "fill": stroke,
        })
        ET.SubElement(root, "text", {
            "x": f"{tx(x) + 6:.2f}", "y": f"{ty(y) - 4:.2f}",
            "font-family": "monospace", "font-size": "9", "fill": "#111",
        }).text = short_name(row["pathcorner"])

    # legend
    lx, ly = margin, height - margin + 8
    for i, (lane, color) in enumerate(
        [(k, v) for k, v in REAL_LANE_COLORS.items() if k != "unknown"]
    ):
        yy = ly + i * 16
        ET.SubElement(root, "circle", {
            "cx": str(lx), "cy": str(yy - 4), "r": "7",
            "fill": color, "fill-opacity": "0.5", "stroke": "#333",
        })
        ET.SubElement(root, "text", {
            "x": str(lx + 14), "y": str(yy),
            "font-family": "sans-serif", "font-size": "10",
        }).text = f"real_lane={lane}"

    out.parent.mkdir(parents=True, exist_ok=True)
    tree = ET.ElementTree(root)
    if hasattr(ET, "indent"):
        ET.indent(tree, space="  ")
    tree.write(out, encoding="unicode", xml_declaration=True)


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("input", nargs="?", default="examples/pathcorner-lane-table.json")
    p.add_argument("-o", "--output", default="examples/pathcorner-lane-map.svg")
    p.add_argument("--title", default="Pathcorner spawn positions")
    p.add_argument("--width", type=int, default=900)
    p.add_argument("--height", type=int, default=900)
    args = p.parse_args()

    inp = Path(args.input)
    if not inp.is_file():
        print(f"not found: {inp}", file=sys.stderr)
        return 1

    rows = load_table(inp)
    if not rows:
        print("empty table", file=sys.stderr)
        return 1

    plot_svg(rows, Path(args.output), args.title, args.width, args.height)
    print(f"wrote {args.output} ({len(rows)} pathcorners)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
