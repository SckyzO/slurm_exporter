#!/usr/bin/env python3
"""
Add the __inputs / __elements / __requires sections to Grafana dashboards
so they pass the grafana.com upload validator ("Old dashboard JSON format"
error otherwise).

The script preserves the rest of the file byte-for-byte: it parses the JSON
only to detect which panel types are used, then inserts the new sections as
text right after the opening "{" of the file.

Idempotent: re-running on an already-fixed file is a no-op.

Usage:
    python3 scripts/dashboards_add_export_format.py
"""
import glob
import json
import re
import sys

GRAFANA_MIN_VERSION = "10.0.0"

PANEL_PRETTY_NAMES = {
    "stat": "Stat",
    "gauge": "Gauge",
    "bargauge": "Bar gauge",
    "barchart": "Bar chart",
    "timeseries": "Time series",
    "table": "Table",
    "state-timeline": "State timeline",
    "status-history": "Status history",
    "row": "Row",
    "text": "Text",
    "piechart": "Pie chart",
    "heatmap": "Heatmap",
    "logs": "Logs",
    "alertlist": "Alert list",
    "annolist": "Annotations list",
    "dashlist": "Dashboard list",
    "news": "News",
    "nodeGraph": "Node Graph",
    "trend": "Trend",
    "histogram": "Histogram",
    "geomap": "Geomap",
    "candlestick": "Candlestick",
    "canvas": "Canvas",
    "flamegraph": "Flame Graph",
    "traces": "Traces",
}

DATASOURCE_INPUT = {
    "name": "DS_PROMETHEUS",
    "label": "Prometheus",
    "description": "",
    "type": "datasource",
    "pluginId": "prometheus",
    "pluginName": "Prometheus",
}

GRAFANA_REQUIRE = {
    "type": "grafana",
    "id": "grafana",
    "name": "Grafana",
    "version": GRAFANA_MIN_VERSION,
}

PROMETHEUS_REQUIRE = {
    "type": "datasource",
    "id": "prometheus",
    "name": "Prometheus",
    "version": "1.0.0",
}


def collect_panel_types(dashboard: dict) -> set[str]:
    """Walk the dashboard and collect all real panel `type` values.

    Skips template-variable nodes (which also have a 'type' field but are not
    panels) by requiring 'targets' or nested 'panels' on the same node.
    """
    types: set[str] = set()

    def walk(node):
        if isinstance(node, dict):
            t = node.get("type")
            if isinstance(t, str) and (
                "targets" in node or "panels" in node or t == "row"
            ):
                types.add(t)
            for v in node.values():
                walk(v)
        elif isinstance(node, list):
            for v in node:
                walk(v)

    walk(dashboard.get("panels", []))
    return types


def build_block(panel_types: set[str]) -> str:
    """Build the "__inputs/__elements/__requires" JSON block (indented 2 spaces).

    Returns text starting with two-space indented "__inputs" key and ending
    with a comma + newline (ready to be inserted right after the opening "{").
    """
    requires = [GRAFANA_REQUIRE, PROMETHEUS_REQUIRE]
    for t in sorted(panel_types):
        requires.append(
            {
                "type": "panel",
                "id": t,
                "name": PANEL_PRETTY_NAMES.get(t, t.title()),
                "version": "",
            }
        )

    block = {
        "__inputs": [DATASOURCE_INPUT],
        "__elements": {},
        "__requires": requires,
    }

    # json.dumps(..., indent=2) already emits 2-space indent for top-level
    # keys, which matches the existing dashboard files. We just need to
    # strip the outer "{ ... }" wrapper and trailing newline.
    text = json.dumps(block, indent=2, ensure_ascii=False)
    inner = text[1:-1].strip("\n")
    return inner + ",\n"


def fix_file(path: str) -> bool:
    with open(path, encoding="utf-8") as f:
        text = f.read()

    if '"__inputs"' in text:
        return False  # idempotent

    dashboard = json.loads(text)
    panel_types = collect_panel_types(dashboard)
    block = build_block(panel_types)

    # Insert right after the opening "{" + newline.
    new_text, n = re.subn(r"^\{\n", "{\n" + block, text, count=1)
    if n != 1:
        raise RuntimeError(f"{path}: could not locate opening brace")

    # Sanity: still parses as JSON.
    json.loads(new_text)

    with open(path, "w", encoding="utf-8") as f:
        f.write(new_text)
    return True


def main() -> int:
    paths = sorted(glob.glob("dashboards_grafana/*.json"))
    if not paths:
        print("No dashboards found in dashboards_grafana/", file=sys.stderr)
        return 1
    changed = 0
    for p in paths:
        if fix_file(p):
            print(f"  + {p}")
            changed += 1
        else:
            print(f"  = {p} (already has __inputs)")
    print(f"\n{changed} file(s) updated, {len(paths) - changed} unchanged.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
