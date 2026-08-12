"""Spawn lane centroids — keep in sync with parser/internal/creeps/creeps.go."""

# From creeps.spawnLaneCentroids / compute_centroids.py (7 replays).
SPAWN_LANE_CENTROIDS = {
    "good": {
        "top": (-6720.7, -4100.7),
        "mid": (-5121.4, -4609.1),
        "bot": (-3834.3, -6217.5),
    },
    "bad": {
        "top": (3070.9, 5634.1),
        "mid": (4001.4, 3495.1),
        "bot": (6143.8, 3567.4),
    },
}

LANE_COLORS = {
    "bot": "#4C78A8",
    "mid": "#59A14F",
    "top": "#E15759",
}

SIDE_STROKE = {
    "good": "#111111",
    "bad": "#555555",
}


def draw_centroids(root, tx, ty, size: float = 4.0, labels: bool = True) -> None:
    """Draw small diamond + optional label for each side/lane centroid."""
    import xml.etree.ElementTree as ET

    for side, lanes in SPAWN_LANE_CENTROIDS.items():
        for lane, (x, y) in lanes.items():
            cx, cy = tx(x), ty(y)
            s = size
            points = f"{cx},{cy - s} {cx + s},{cy} {cx},{cy + s} {cx - s},{cy}"
            ET.SubElement(root, "polygon", {
                "points": points,
                "fill": LANE_COLORS.get(lane, "#888"),
                "fill-opacity": "1",
                "stroke": SIDE_STROKE.get(side, "#333"),
                "stroke-width": "1",
            })
            if labels:
                ET.SubElement(root, "text", {
                    "x": f"{cx + s + 3:.2f}",
                    "y": f"{cy - 3:.2f}",
                    "font-family": "monospace",
                    "font-size": "9",
                    "fill": "#333",
                }).text = f"{side}/{lane}"
